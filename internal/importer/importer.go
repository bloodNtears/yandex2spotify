package importer

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/bloodNtears/yandex2spotify/internal/yandex"
	"github.com/zmb3/spotify/v2"
)

type Importer struct {
	yandex     *yandex.Client
	spotify    *spotify.Client
	spotifyUID string
	yandexUID  uint32

	notImported map[string][]string
}

func New(yc *yandex.Client, sc *spotify.Client) (*Importer, error) {
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

	var spotifyIDs []spotify.ID
	for i := len(albums) - 1; i >= 0; i-- {
		a := albums[i]
		name := formatAlbumName(a)
		query := buildAlbumQuery(a)

		log.Printf("Importing album: %s...", name)
		results, err := imp.spotify.Search(ctx, query, spotify.SearchTypeAlbum)
		if err != nil {
			log.Printf("  ERROR: search: %v", err)
			imp.notImported[section] = append(imp.notImported[section], name)
			continue
		}

		if results.Albums == nil || len(results.Albums.Albums) == 0 {
			if len(a.Artists) > 1 {
				query = fmt.Sprintf("%s %s", a.Artists[0].Name, a.Title)
				results, err = imp.spotify.Search(ctx, query, spotify.SearchTypeAlbum)
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

		spotifyIDs = append(spotifyIDs, results.Albums.Albums[0].ID)
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

	var spotifyIDs []spotify.ID
	for i := len(artists) - 1; i >= 0; i-- {
		a := artists[i]
		log.Printf("Importing artist: %s...", a.Name)

		results, err := imp.spotify.Search(ctx, a.Name, spotify.SearchTypeArtist)
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

		spotifyIDs = append(spotifyIDs, results.Artists.Artists[0].ID)
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

	for i := len(tracks) - 1; i >= 0; i-- {
		t := tracks[i]
		name := formatTrackName(t)
		query := buildTrackQuery(t)

		if len(query) > 100 {
			query = query[:100]
			log.Printf("  Query trimmed to 100 chars for: %s", name)
		}

		log.Printf("Importing track: %s...", name)
		results, err := imp.spotify.Search(ctx, query, spotify.SearchTypeTrack)
		if err != nil {
			log.Printf("  ERROR: search: %v", err)
			imp.notImported[section] = append(imp.notImported[section], name)
			continue
		}

		if results.Tracks == nil || len(results.Tracks.Tracks) == 0 {
			if len(t.Artists) > 1 {
				query = fmt.Sprintf("%s %s", t.Artists[0].Name, t.Title)
				results, err = imp.spotify.Search(ctx, query, spotify.SearchTypeTrack)
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

		spotifyIDs = append(spotifyIDs, results.Tracks.Tracks[0].ID)
		log.Printf("  OK")
	}

	return spotifyIDs
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
