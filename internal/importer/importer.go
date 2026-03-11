package importer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bloodNtears/yandex2spotify/internal/cache"
	"github.com/bloodNtears/yandex2spotify/internal/spotify"
	"github.com/bloodNtears/yandex2spotify/internal/yandex"
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

	spotifyUID, err := sc.CurrentUserID(ctx)
	if err != nil {
		return nil, fmt.Errorf("get spotify user: %w", err)
	}
	log.Printf("Spotify user: %s", spotifyUID)

	return &Importer{
		yandex:      yc,
		spotify:     sc,
		spotifyUID:  spotifyUID,
		yandexUID:   status.Account.UID,
		cache:       c,
		limiter:     rate.NewLimiter(rate.Every(500*time.Millisecond), 1),
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

	var newIDs []string
	for _, id := range spotifyIDs {
		if !imp.cache.IsLikeSaved(id) {
			newIDs = append(newIDs, id)
		}
	}

	if len(newIDs) == 0 {
		log.Printf("All %d liked tracks already saved, skipping", len(spotifyIDs))
		return
	}
	log.Printf("%d of %d liked tracks already saved, saving %d new",
		len(spotifyIDs)-len(newIDs), len(spotifyIDs), len(newIDs))

	for _, chunk := range chunkIDs(newIDs, 40) {
		log.Printf("Saving %d liked tracks...", len(chunk))
		if err := saveTracksToLibrary(ctx, imp.spotify.HTTPClient(), chunk); err != nil {
			log.Printf("ERROR: save liked tracks: %v", err)
			continue
		}
		for _, id := range chunk {
			imp.cache.MarkLikeSaved(id)
		}
		if err := imp.cache.Save(); err != nil {
			log.Printf("WARNING: save cache: %v", err)
		}
	}
}

func saveTracksToLibrary(ctx context.Context, httpClient *http.Client, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	uris := make([]string, 0, len(ids))
	for _, id := range ids {
		uris = append(uris, "spotify:track:"+id)
	}

	q := url.Values{}
	q.Set("uris", strings.Join(uris, ","))

	endpoint := "https://api.spotify.com/v1/me/library?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("spotify save library failed: status=%s body=%s", resp.Status, string(raw))
	}

	return nil
}

func saveAlbumsToLibrary(ctx context.Context, httpClient *http.Client, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > 40 {
		return fmt.Errorf("too many ids: got %d, max 40", len(ids))
	}

	uris := make([]string, 0, len(ids))
	for _, id := range ids {
		uris = append(uris, "spotify:album:"+id)
	}

	q := url.Values{}
	q.Set("uris", strings.Join(uris, ","))

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		"https://api.spotify.com/v1/me/library?"+q.Encode(),
		nil,
	)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("spotify save albums failed: status=%s body=%s", resp.Status, string(raw))
	}

	return nil
}

func addItemsToPlaylist(ctx context.Context, httpClient *http.Client, playlistID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > 100 {
		return fmt.Errorf("too many ids: got %d, max 100", len(ids))
	}

	uris := make([]string, 0, len(ids))
	for _, id := range ids {
		uris = append(uris, "spotify:track:"+id)
	}

	payload := struct {
		URIs []string `json:"uris"`
	}{
		URIs: uris,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	endpoint := fmt.Sprintf("https://api.spotify.com/v1/playlists/%s/items", playlistID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("spotify add playlist items failed: status=%s body=%s", resp.Status, string(raw))
	}

	return nil
}

func createPlaylist(ctx context.Context, httpClient *http.Client, title string, public bool, description string) (string, error) {
	payload := map[string]any{
		"name":        title,
		"public":      public,
		"description": description,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"https://api.spotify.com/v1/me/playlists",
		bytes.NewReader(b),
	)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("create playlist failed: status=%s body=%s", resp.Status, string(raw))
	}

	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return out.ID, nil
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
	cacheKey := strconv.Itoa(pl.Kind)

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

	var spotifyPlaylistID string
	if entry, ok := imp.cache.GetPlaylist(cacheKey); ok {
		spotifyPlaylistID = entry.SpotifyID
		log.Printf("Using cached Spotify playlist for %q (ID: %s)", pl.Title, spotifyPlaylistID)
	} else {
		spotifyPlaylistID, err = createPlaylist(ctx, imp.spotify.HTTPClient(), pl.Title, false, "")
		if err != nil {
			log.Printf("ERROR: create Spotify playlist %q: %v", pl.Title, err)
			return
		}
		imp.cache.SetPlaylist(cacheKey, spotifyPlaylistID)
		if err := imp.cache.Save(); err != nil {
			log.Printf("WARNING: save cache: %v", err)
		}
		log.Printf("Created Spotify playlist: %s (ID: %s)", pl.Title, spotifyPlaylistID)
	}

	spotifyIDs := imp.searchTracks(ctx, tracks, section)
	if len(spotifyIDs) == 0 {
		log.Printf("No tracks matched on Spotify for playlist %q", pl.Title)
		return
	}

	entry, _ := imp.cache.GetPlaylist(cacheKey)
	var newIDs []string
	for _, id := range spotifyIDs {
		if !entry.AddedTracks[id] {
			newIDs = append(newIDs, id)
		}
	}

	if len(newIDs) == 0 {
		log.Printf("All %d tracks already added to playlist %q, skipping", len(spotifyIDs), pl.Title)
		return
	}
	log.Printf("%d of %d tracks already added, adding %d new tracks to playlist %q",
		len(spotifyIDs)-len(newIDs), len(spotifyIDs), len(newIDs), pl.Title)

	for _, chunk := range chunkIDs(newIDs, 100) {
		log.Printf("Adding %d tracks to playlist %q...", len(chunk), pl.Title)

		if err := addItemsToPlaylist(ctx, imp.spotify.HTTPClient(), spotifyPlaylistID, chunk); err != nil {
			log.Printf("ERROR: add tracks to playlist: %v", err)
			continue
		}

		for _, id := range chunk {
			imp.cache.AddPlaylistTrack(cacheKey, id)
		}
		if err := imp.cache.Save(); err != nil {
			log.Printf("WARNING: save cache: %v", err)
		}
	}
}

func (imp *Importer) importAlbums(ctx context.Context) {
	log.Println("Importing albums...")

	albumIDs, err := imp.yandex.LikedAlbumIDs(ctx, imp.yandexUID)
	if err != nil {
		log.Printf("ERROR: fetch liked album IDs: %v", err)
		return
	}
	log.Printf("Found %d liked albums, fetching details...", len(albumIDs))

	albums, err := imp.yandex.Albums(ctx, albumIDs)
	if err != nil {
		log.Printf("ERROR: fetch album details: %v", err)
		return
	}
	log.Printf("Loaded details for %d albums", len(albums))

	section := "Albums"
	imp.notImported[section] = nil

	total := len(albums)
	var spotifyIDs []string
	for i := len(albums) - 1; i >= 0; i-- {
		a := albums[i]
		name := formatAlbumName(a)
		yandexKey := strconv.Itoa(a.ID)
		progress := total - i

		if sid, ok := imp.cache.GetAlbum(yandexKey); ok {
			log.Printf("[%d/%d] Importing album: %s... CACHED", progress, total, name)
			spotifyIDs = append(spotifyIDs, sid)
			continue
		}

		query := strings.TrimSpace(buildAlbumQuery(a))
		log.Printf("[%d/%d] Importing album: %s... query=%q", progress, total, name, query)

		if query == "" {
			log.Printf("  ERROR: empty album search query")
			imp.notImported[section] = append(imp.notImported[section], name)
			continue
		}

		results, err := imp.throttledSearch(ctx, query, "album")
		if err != nil {
			log.Printf("  ERROR: search: %v", err)
			imp.notImported[section] = append(imp.notImported[section], name)
			continue
		}

		if results.Albums == nil || len(results.Albums.Items) == 0 {
			if len(a.Artists) > 1 {
				query = fmt.Sprintf("%s %s", a.Artists[0].Name, a.Title)
				results, err = imp.throttledSearch(ctx, query, "album")
				if err != nil || results.Albums == nil || len(results.Albums.Items) == 0 {
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

		sid := results.Albums.Items[0].ID
		spotifyIDs = append(spotifyIDs, sid)
		imp.cache.SetAlbum(yandexKey, sid)
		if err := imp.cache.Save(); err != nil {
			log.Printf("  WARNING: save cache: %v", err)
		}
		log.Printf("  OK")
	}

	if len(spotifyIDs) == 0 {
		log.Println("No albums matched on Spotify")
		return
	}

	var newIDs []string
	for _, id := range spotifyIDs {
		if !imp.cache.IsAlbumSaved(id) {
			newIDs = append(newIDs, id)
		}
	}

	if len(newIDs) == 0 {
		log.Printf("All %d albums already saved, skipping", len(spotifyIDs))
		return
	}
	log.Printf("%d of %d albums already saved, saving %d new",
		len(spotifyIDs)-len(newIDs), len(spotifyIDs), len(newIDs))

	for _, chunk := range chunkIDs(newIDs, 40) {
		log.Printf("Saving %d albums...", len(chunk))
		if err := saveAlbumsToLibrary(ctx, imp.spotify.HTTPClient(), chunk); err != nil {
			log.Printf("ERROR: save albums: %v", err)
			continue
		}
		for _, id := range chunk {
			imp.cache.MarkAlbumSaved(id)
		}
		if err := imp.cache.Save(); err != nil {
			log.Printf("WARNING: save cache: %v", err)
		}
	}
}

func (imp *Importer) searchTracks(ctx context.Context, tracks []yandex.Track, section string) []string {
	var spotifyIDs []string
	total := len(tracks)

	for i := len(tracks) - 1; i >= 0; i-- {
		t := tracks[i]
		name := formatTrackName(t)
		yandexKey := trackCacheKey(t)
		progress := total - i

		if sid, ok := imp.cache.GetTrack(yandexKey); ok {
			log.Printf("[%d/%d] Importing track: %s... CACHED", progress, total, name)
			spotifyIDs = append(spotifyIDs, sid)
			continue
		}

		query := buildTrackQuery(t)
		if len(query) > 100 {
			query = query[:100]
			log.Printf("  Query trimmed to 100 chars for: %s", name)
		}

		log.Printf("[%d/%d] Importing track: %s...", progress, total, name)
		results, err := imp.throttledSearch(ctx, query, "track")
		if err != nil {
			log.Printf("  ERROR: search: %v", err)
			imp.notImported[section] = append(imp.notImported[section], name)
			continue
		}

		if results.Tracks == nil || len(results.Tracks.Items) == 0 {
			if len(t.Artists) > 1 {
				query = fmt.Sprintf("%s %s", t.Artists[0].Name, t.Title)
				results, err = imp.throttledSearch(ctx, query, "track")
				if err != nil || results.Tracks == nil || len(results.Tracks.Items) == 0 {
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

		sid := results.Tracks.Items[0].ID
		spotifyIDs = append(spotifyIDs, sid)
		imp.cache.SetTrack(yandexKey, sid)
		if err := imp.cache.Save(); err != nil {
			log.Printf("  WARNING: save cache: %v", err)
		}
		log.Printf("  OK")
	}

	return spotifyIDs
}

func (imp *Importer) throttledSearch(ctx context.Context, query, searchType string) (*spotify.SearchResult, error) {
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
	title := strings.TrimSpace(a.Title)

	var artist string
	if len(a.Artists) > 0 {
		artist = strings.TrimSpace(a.Artists[0].Name)
	}

	switch {
	case title != "" && artist != "":
		return fmt.Sprintf(`album:"%s" artist:"%s"`, title, artist)
	case title != "":
		return fmt.Sprintf(`album:"%s"`, title)
	case artist != "":
		return fmt.Sprintf(`artist:"%s"`, artist)
	default:
		return ""
	}
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
