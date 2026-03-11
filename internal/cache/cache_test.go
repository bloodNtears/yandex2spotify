package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func tempCachePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "cache.json")
}

func TestLoad_NonexistentFile(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "does_not_exist.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.Tracks == nil || c.Albums == nil {
		t.Fatal("expected initialized maps for tracks/albums")
	}
	if c.Playlists == nil || c.SavedLikes == nil || c.SavedAlbums == nil {
		t.Fatal("expected initialized maps for new cache fields")
	}
	if len(c.Tracks) != 0 || len(c.Albums) != 0 {
		t.Fatal("expected empty maps")
	}
}

func TestLoad_OldFormat_BackwardCompatibility(t *testing.T) {
	path := tempCachePath(t)
	oldJSON := `{
		"tracks": {"100:200": "spotifyTrack1"},
		"albums": {"300": "spotifyAlbum1"}
	}`
	if err := os.WriteFile(path, []byte(oldJSON), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	c, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if v, ok := c.Tracks["100:200"]; !ok || v != "spotifyTrack1" {
		t.Errorf("tracks not loaded: got %q, ok=%v", v, ok)
	}
	if v, ok := c.Albums["300"]; !ok || v != "spotifyAlbum1" {
		t.Errorf("albums not loaded: got %q, ok=%v", v, ok)
	}

	if c.Playlists == nil {
		t.Fatal("Playlists should be initialized, got nil")
	}
	if c.SavedLikes == nil || c.SavedAlbums == nil {
		t.Fatal("SavedLikes/SavedAlbums should be initialized")
	}
	if len(c.Playlists) != 0 || len(c.SavedLikes) != 0 || len(c.SavedAlbums) != 0 {
		t.Fatal("new fields should be empty for old-format cache")
	}
}

func TestLoad_FullFormat(t *testing.T) {
	path := tempCachePath(t)
	fullJSON := `{
		"tracks": {"1:2": "st1"},
		"albums": {"3": "sa1"},
		"playlists": {
			"100": {
				"spotify_id": "sp_pl_1",
				"added_tracks": {"st1": true, "st2": true}
			}
		},
		"saved_likes": {"st1": true},
		"saved_albums": {"sa1": true}
	}`
	if err := os.WriteFile(path, []byte(fullJSON), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	c, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entry, ok := c.Playlists["100"]
	if !ok {
		t.Fatal("playlist 100 not loaded")
	}
	if entry.SpotifyID != "sp_pl_1" {
		t.Errorf("playlist spotify_id: got %q, want %q", entry.SpotifyID, "sp_pl_1")
	}
	if len(entry.AddedTracks) != 2 || !entry.AddedTracks["st1"] || !entry.AddedTracks["st2"] {
		t.Errorf("playlist added_tracks: got %v", entry.AddedTracks)
	}
	if !c.SavedLikes["st1"] {
		t.Error("saved_likes should contain st1")
	}
	if !c.SavedAlbums["sa1"] {
		t.Error("saved_albums should contain sa1")
	}
}

func TestLoad_CorruptedJSON(t *testing.T) {
	path := tempCachePath(t)
	if err := os.WriteFile(path, []byte("{invalid json"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for corrupted JSON, got nil")
	}
}

func TestLoad_NilAddedTracksInPlaylist(t *testing.T) {
	path := tempCachePath(t)
	data := `{
		"tracks": {},
		"albums": {},
		"playlists": {
			"42": {
				"spotify_id": "sp42",
				"added_tracks": null
			}
		}
	}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	c, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entry, ok := c.Playlists["42"]
	if !ok {
		t.Fatal("playlist 42 not loaded")
	}
	if entry.AddedTracks == nil {
		t.Fatal("AddedTracks should be initialized to empty map, got nil")
	}
	if len(entry.AddedTracks) != 0 {
		t.Errorf("AddedTracks should be empty, got %v", entry.AddedTracks)
	}
}

func TestSaveAndReload(t *testing.T) {
	path := tempCachePath(t)
	c, _ := Load(path)

	c.SetTrack("y1", "s1")
	c.SetAlbum("y2", "s2")
	c.SetPlaylist("10", "sp10")
	c.AddPlaylistTrack("10", "trackA")
	c.AddPlaylistTrack("10", "trackB")
	c.MarkLikeSaved("s1")
	c.MarkAlbumSaved("s2")

	if err := c.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	c2, err := Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	if v, ok := c2.GetTrack("y1"); !ok || v != "s1" {
		t.Errorf("track: got %q, ok=%v", v, ok)
	}
	if v, ok := c2.GetAlbum("y2"); !ok || v != "s2" {
		t.Errorf("album: got %q, ok=%v", v, ok)
	}

	entry, ok := c2.GetPlaylist("10")
	if !ok || entry.SpotifyID != "sp10" {
		t.Errorf("playlist: got %+v, ok=%v", entry, ok)
	}
	if !entry.AddedTracks["trackA"] || !entry.AddedTracks["trackB"] {
		t.Errorf("playlist added tracks: got %v", entry.AddedTracks)
	}

	if !c2.IsLikeSaved("s1") {
		t.Error("like s1 should be saved")
	}
	if !c2.IsAlbumSaved("s2") {
		t.Error("album s2 should be saved")
	}
}

func TestSave_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	path := filepath.Join(dir, "cache.json")

	c, _ := Load(path)
	c.SetTrack("a", "b")

	if err := c.Save(); err != nil {
		t.Fatalf("save with nested dir: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cache file not created: %v", err)
	}
}

func TestSave_ValidJSON(t *testing.T) {
	path := tempCachePath(t)
	c, _ := Load(path)
	c.SetTrack("x", "y")
	if err := c.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("saved file is not valid JSON: %v", err)
	}

	requiredKeys := []string{"tracks", "albums", "playlists", "saved_likes", "saved_albums"}
	for _, key := range requiredKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("saved JSON missing key %q", key)
		}
	}
}

func TestTrackGetSet(t *testing.T) {
	c, _ := Load(tempCachePath(t))

	if _, ok := c.GetTrack("missing"); ok {
		t.Error("GetTrack should return false for missing key")
	}

	c.SetTrack("y100", "s100")
	if v, ok := c.GetTrack("y100"); !ok || v != "s100" {
		t.Errorf("GetTrack after Set: got %q, ok=%v", v, ok)
	}

	c.SetTrack("y100", "s200")
	if v, _ := c.GetTrack("y100"); v != "s200" {
		t.Errorf("SetTrack overwrite: got %q, want s200", v)
	}
}

func TestAlbumGetSet(t *testing.T) {
	c, _ := Load(tempCachePath(t))

	if _, ok := c.GetAlbum("missing"); ok {
		t.Error("GetAlbum should return false for missing key")
	}

	c.SetAlbum("y1", "s1")
	if v, ok := c.GetAlbum("y1"); !ok || v != "s1" {
		t.Errorf("GetAlbum after Set: got %q, ok=%v", v, ok)
	}
}

func TestPlaylistGetSetAdd(t *testing.T) {
	c, _ := Load(tempCachePath(t))

	if _, ok := c.GetPlaylist("missing"); ok {
		t.Error("GetPlaylist should return false for missing key")
	}

	c.SetPlaylist("5", "sp5")
	entry, ok := c.GetPlaylist("5")
	if !ok {
		t.Fatal("GetPlaylist should return true after Set")
	}
	if entry.SpotifyID != "sp5" {
		t.Errorf("SpotifyID: got %q, want sp5", entry.SpotifyID)
	}
	if entry.AddedTracks == nil || len(entry.AddedTracks) != 0 {
		t.Errorf("AddedTracks should be empty map, got %v", entry.AddedTracks)
	}

	c.AddPlaylistTrack("5", "t1")
	c.AddPlaylistTrack("5", "t2")
	c.AddPlaylistTrack("5", "t1")
	entry, _ = c.GetPlaylist("5")
	if len(entry.AddedTracks) != 2 {
		t.Errorf("AddedTracks should have 2 entries, got %d", len(entry.AddedTracks))
	}
	if !entry.AddedTracks["t1"] || !entry.AddedTracks["t2"] {
		t.Errorf("AddedTracks: got %v", entry.AddedTracks)
	}
}

func TestAddPlaylistTrack_NonexistentPlaylist(t *testing.T) {
	c, _ := Load(tempCachePath(t))

	c.AddPlaylistTrack("nonexistent", "track1")

	if _, ok := c.GetPlaylist("nonexistent"); ok {
		t.Error("AddPlaylistTrack should not create a playlist entry")
	}
}

func TestLikeIsMark(t *testing.T) {
	c, _ := Load(tempCachePath(t))

	if c.IsLikeSaved("x") {
		t.Error("IsLikeSaved should return false initially")
	}

	c.MarkLikeSaved("x")
	if !c.IsLikeSaved("x") {
		t.Error("IsLikeSaved should return true after MarkLikeSaved")
	}

	c.MarkLikeSaved("x")
	if !c.IsLikeSaved("x") {
		t.Error("MarkLikeSaved should be idempotent")
	}
}

func TestAlbumSavedIsMark(t *testing.T) {
	c, _ := Load(tempCachePath(t))

	if c.IsAlbumSaved("x") {
		t.Error("IsAlbumSaved should return false initially")
	}

	c.MarkAlbumSaved("x")
	if !c.IsAlbumSaved("x") {
		t.Error("IsAlbumSaved should return true after MarkAlbumSaved")
	}
}
