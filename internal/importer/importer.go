package importer

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/bloodNtears/yandex2spotify/internal/cache"
	"github.com/bloodNtears/yandex2spotify/internal/yandex"
	"github.com/zmb3/spotify/v2"
	"golang.org/x/time/rate"
)

type Importer struct {
	yandex     *yandex.Client
	spotify    *spotify.Client
	spotifyUID string
	yandexUID  uint32

	cache       *cache.Cache
	limiter     *rate.Limiter
	notImported map[string][]string
}

func New(yc *yandex.Client, sc *spotify.Client, c *cache.Cache) (*Importer, error) {
	ctx := context.Background()

	status, err := yc.AccountStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("get yandex account: %w", err)
	}
	log.Printf("Yandex user: %s (uid %d)", status.Account.DisplayName, status.Account.UID)

	spotifyUser, err := sc.CurrentUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("get spotify user: %w", err)
	}
	log.Printf("Spotify user: %s", spotifyUser.ID)

	return &Importer{
		yandex:      yc,
		spotify:     sc,
		spotifyUID:  spotifyUser.ID,
		yandexUID:   status.Account.UID,
		cache:       c,
		limiter:     rate.NewLimiter(rate.Every(300*time.Millisecond), 1), // ~3.33 rps, burst 1
		notImported: make(map[string][]string),
	}, nil
}

func (imp *Importer) ImportAll(ctx context.Context, ignore map[string]bool) {
	if !ignore["likes"] {
		imp.importLikes(ctx)
	}
	if !ignore["playlists"] {
		imp.importPlaylists(ctx)
	}
	if !ignore["albums"] {
		imp.importAlbums(ctx)
	}
	if !ignore["artists"] {
		imp.importArtists(ctx)
	}

	imp.printNotImported()
}

func (imp *Importer) importLikes(ctx context.Context) {
	log.Println("Importing liked tracks...")

	likedTracks, err := imp.yandex.LikedTracks(ctx, imp.yandexUID)
	if err != nil {
		log.Printf("ERROR: fetch liked tracks: %v", err)
		return
	}
	log.Printf("Found %d liked tracks", len(likedTracks))

	ids := make([]string, 0, len(likedTracks))
	for _, lt := range likedTracks {
		if lt.AlbumID != "" {
			ids = append(ids, lt.ID+":"+lt.AlbumID)
		}
	}

	tracks, err := imp.yandex.Tracks(ctx, ids)
	if err != nil {
		log.Printf("ERROR: fetch track details: %v", err)
		return
	}

	section := "Likes"
	imp.notImported[section] = nil

	spotifyIDs := imp.searchTracks(ctx, tracks, section)
	if len(spotifyIDs) == 0 {
		log.Println("No liked tracks matched on Spotify")
		return
	}

	for _, chunk := range chunkIDs(spotifyIDs, 50) {
		log.Printf("Saving %d liked tracks...", len(chunk))
		if err := imp.spotify.AddTracksToLibrary(ctx, chunk...); err != nil {
			log.Printf("ERROR: save liked tracks: %v", err)
		}
	}
}

func (imp *Importer) importPlaylists(ctx context.Context) {
	log.Println("Importing playlists...")

	playlists, err := imp.yandex.Playlists(ctx, imp.yandexUID)
	if err != nil {
		log.Printf("ERROR: fetch playlists: %v", err)
		return
	}
	log.Printf("Found %d playlists", len(playlists))

	for _, pl := range playlists {
		imp.importPlaylist(ctx, pl)
	}
}

func (imp *Importer) importPlaylist(ctx context.Context, pl yandex.Playlist) {
	log.Printf("Importing playlist: %s", pl.Title)

	section := pl.Title
	imp.notImported[section] = nil

	fullPlaylist, err := imp.yandex.PlaylistTracks(ctx, imp.yandexUID, pl.Kind)
	if err != nil {
		log.Printf("ERROR: fetch playlist tracks for %q: %v", pl.Title, err)
		return
	}

	var tracks []yandex.Track
	for _, ts := range fullPlaylist.Tracks {
		if ts.Track != nil {
			tracks = append(tracks, *ts.Track)
		}
	}

	if len(tracks) == 0 {
		log.Printf("Playlist %q has no tracks, skipping", pl.Title)
		return
	}

	spotifyPlaylist, err := imp.spotify.CreatePlaylistForUser(ctx, imp.spotifyUID, pl.Title, "", true, false)
	if err != nil {
		log.Printf("ERROR: create Spotify playlist %q: %v", pl.Title, err)
		return
	}
	log.Printf("Created Spotify playlist: %s (ID: %s)", spotifyPlaylist.Name, spotifyPlaylist.ID)

	spotifyIDs := imp.searchTracks(ctx, tracks, section)
	if len(spotifyIDs) == 0 {
		log.Printf("No tracks matched on Spotify for playlist %q", pl.Title)
		return
	}

	for _, chunk := range chunkIDs(spotifyIDs, 100) {
		log.Printf("Adding %d tracks to playlist %q...", len(chunk), pl.Title)
		_, err := imp.spotify.AddTracksToPlaylist(ctx, spotifyPlaylist.ID, chunk...)
		if err != nil {
			log.Printf("ERROR: add tracks to playlist: %v", err)
		}
	}
}

func (imp *Importer) importAlbums(ctx context.Context) {
	log.Println("Importing albums...")

	albums, err := imp.yandex.LikedAlbums(ctx, imp.yandexUID)
	if err != nil {
		log.Printf("ERROR: fetch liked albums: %v", err)
		return
	}
	log.Printf("Found %d liked albums", len(albums))

	section := "Albums"
	imp.notImported[section] = nil

	total := len(albums)
	var spotifyIDs []spotify.ID
	for i := len(albums) - 1; i >= 0; i-- {
		a := albums[i]
		name := formatAlbumName(a)
		yandexKey := strconv.Itoa(a.ID)
		progress := total - i

		if sid, ok := imp.cache.GetAlbum(yandexKey); ok {
			log.Printf("[%d/%d] Importing album: %s... CACHED", progress, total, name)
			spotifyIDs = append(spotifyIDs, spotify.ID(sid))
			continue
		}

		query := buildAlbumQuery(a)
		log.Printf("[%d/%d] Importing album: %s...", progress, total, name)
		results, err := imp.throttledSearch(ctx, query, spotify.SearchTypeAlbum)
		if err != nil {
			log.Printf("  ERROR: search: %v", err)
			imp.notImported[section] = append(imp.notImported[section], name)
			continue
		}

		if results.Albums == nil || len(results.Albums.Albums) == 0 {
			if len(a.Artists) > 1 {
				query = fmt.Sprintf("%s %s", a.Artists[0].Name, a.Title)
				results, err = imp.throttledSearch(ctx, query, spotify.SearchTypeAlbum)
				if err != nil || results.Albums == nil || len(results.Albums.Albums) == 0 {
					log.Printf("  NOT FOUND")
					imp.notImported[section] = append(imp.notImported[section], name)
					continue
				}
			} else {
				log.Printf("  NOT FOUND")
				imp.notImported[section] = append(imp.notImported[section], name)
				continue
			}
		}

		sid := results.Albums.Albums[0].ID
		spotifyIDs = append(spotifyIDs, sid)
		imp.cache.SetAlbum(yandexKey, string(sid))
		if err := imp.cache.Save(); err != nil {
			log.Printf("  WARNING: save cache: %v", err)
		}
		log.Printf("  OK")
	}

	if len(spotifyIDs) == 0 {
		log.Println("No albums matched on Spotify")
		return
	}

	for _, chunk := range chunkIDs(spotifyIDs, 50) {
		log.Printf("Saving %d albums...", len(chunk))
		if err := imp.spotify.AddAlbumsToLibrary(ctx, chunk...); err != nil {
			log.Printf("ERROR: save albums: %v", err)
		}
	}
}

func (imp *Importer) importArtists(ctx context.Context) {
	log.Println("Importing artists...")

	artists, err := imp.yandex.LikedArtists(ctx, imp.yandexUID)
	if err != nil {
		log.Printf("ERROR: fetch liked artists: %v", err)
		return
	}
	log.Printf("Found %d liked artists", len(artists))

	section := "Artists"
	imp.notImported[section] = nil

	total := len(artists)
	var spotifyIDs []spotify.ID
	for i := len(artists) - 1; i >= 0; i-- {
		a := artists[i]
		yandexKey := strconv.Itoa(a.ID)
		progress := total - i

		if sid, ok := imp.cache.GetArtist(yandexKey); ok {
			log.Printf("[%d/%d] Importing artist: %s... CACHED", progress, total, a.Name)
			spotifyIDs = append(spotifyIDs, spotify.ID(sid))
			continue
		}

		log.Printf("[%d/%d] Importing artist: %s...", progress, total, a.Name)
		results, err := imp.throttledSearch(ctx, a.Name, spotify.SearchTypeArtist)
		if err != nil {
			log.Printf("  ERROR: search: %v", err)
			imp.notImported[section] = append(imp.notImported[section], a.Name)
			continue
		}

		if results.Artists == nil || len(results.Artists.Artists) == 0 {
			log.Printf("  NOT FOUND")
			imp.notImported[section] = append(imp.notImported[section], a.Name)
			continue
		}

		sid := results.Artists.Artists[0].ID
		spotifyIDs = append(spotifyIDs, sid)
		imp.cache.SetArtist(yandexKey, string(sid))
		if err := imp.cache.Save(); err != nil {
			log.Printf("  WARNING: save cache: %v", err)
		}
		log.Printf("  OK")
	}

	if len(spotifyIDs) == 0 {
		log.Println("No artists matched on Spotify")
		return
	}

	for _, chunk := range chunkIDs(spotifyIDs, 50) {
		log.Printf("Following %d artists...", len(chunk))
		if err := imp.spotify.FollowArtist(ctx, chunk...); err != nil {
			log.Printf("ERROR: follow artists: %v", err)
		}
	}
}

func (imp *Importer) searchTracks(ctx context.Context, tracks []yandex.Track, section string) []spotify.ID {
	var spotifyIDs []spotify.ID
	total := len(tracks)

	for i := len(tracks) - 1; i >= 0; i-- {
		t := tracks[i]
		name := formatTrackName(t)
		yandexKey := trackCacheKey(t)
		progress := total - i

		if sid, ok := imp.cache.GetTrack(yandexKey); ok {
			log.Printf("[%d/%d] Importing track: %s... CACHED", progress, total, name)
			spotifyIDs = append(spotifyIDs, spotify.ID(sid))
			continue
		}

		query := buildTrackQuery(t)
		if len(query) > 100 {
			query = query[:100]
			log.Printf("  Query trimmed to 100 chars for: %s", name)
		}

		log.Printf("[%d/%d] Importing track: %s...", progress, total, name)
		results, err := imp.throttledSearch(ctx, query, spotify.SearchTypeTrack)
		if err != nil {
			log.Printf("  ERROR: search: %v", err)
			imp.notImported[section] = append(imp.notImported[section], name)
			continue
		}

		if results.Tracks == nil || len(results.Tracks.Tracks) == 0 {
			if len(t.Artists) > 1 {
				query = fmt.Sprintf("%s %s", t.Artists[0].Name, t.Title)
				results, err = imp.throttledSearch(ctx, query, spotify.SearchTypeTrack)
				if err != nil || results.Tracks == nil || len(results.Tracks.Tracks) == 0 {
					log.Printf("  NOT FOUND")
					imp.notImported[section] = append(imp.notImported[section], name)
					continue
				}
			} else {
				log.Printf("  NOT FOUND")
				imp.notImported[section] = append(imp.notImported[section], name)
				continue
			}
		}

		sid := results.Tracks.Tracks[0].ID
		spotifyIDs = append(spotifyIDs, sid)
		imp.cache.SetTrack(yandexKey, string(sid))
		if err := imp.cache.Save(); err != nil {
			log.Printf("  WARNING: save cache: %v", err)
		}
		log.Printf("  OK")
	}

	return spotifyIDs
}

func (imp *Importer) throttledSearch(ctx context.Context, query string, searchType spotify.SearchType) (*spotify.SearchResult, error) {
	if err := imp.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	return imp.spotify.Search(ctx, query, searchType)
}

func (imp *Importer) printNotImported() {
	hasItems := false
	for _, items := range imp.notImported {
		if len(items) > 0 {
			hasItems = true
			break
		}
	}
	if !hasItems {
		log.Println("All items imported successfully!")
		return
	}

	log.Println("--- Not imported items ---")
	for section, items := range imp.notImported {
		if len(items) == 0 {
			continue
		}
		log.Printf("%s:", section)
		for _, item := range items {
			log.Printf("  - %s", item)
		}
	}
}

func trackCacheKey(t yandex.Track) string {
	if len(t.Albums) > 0 {
		return fmt.Sprintf("%s:%d", t.ID, t.Albums[0].ID)
	}
	return t.ID
}

func formatTrackName(t yandex.Track) string {
	artists := make([]string, len(t.Artists))
	for i, a := range t.Artists {
		artists[i] = a.Name
	}
	return fmt.Sprintf("%s - %s", strings.Join(artists, ", "), t.Title)
}

func buildTrackQuery(t yandex.Track) string {
	name := formatTrackName(t)
	return strings.ReplaceAll(name, "- ", "")
}

func formatAlbumName(a yandex.Album) string {
	artists := make([]string, len(a.Artists))
	for i, art := range a.Artists {
		artists[i] = art.Name
	}
	if len(artists) > 0 {
		return fmt.Sprintf("%s - %s", strings.Join(artists, ", "), a.Title)
	}
	return a.Title
}

func buildAlbumQuery(a yandex.Album) string {
	name := formatAlbumName(a)
	return strings.ReplaceAll(name, "- ", "")
}

func chunkIDs[T any](ids []T, size int) [][]T {
	var chunks [][]T
	for i := 0; i < len(ids); i += size {
		end := i + size
		if end > len(ids) {
			end = len(ids)
		}
		chunks = append(chunks, ids[i:end])
	}
	return chunks
}
