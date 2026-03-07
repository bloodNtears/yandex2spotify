package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

type PlaylistEntry struct {
	SpotifyID   string          `json:"spotify_id"`
	AddedTracks map[string]bool `json:"added_tracks"`
}

type Cache struct {
	mu   sync.Mutex
	path string

	Tracks  map[string]string `json:"tracks"`
	Albums  map[string]string `json:"albums"`
	Artists map[string]string `json:"artists"`

	Playlists    map[string]*PlaylistEntry `json:"playlists"`
	SavedLikes   map[string]bool           `json:"saved_likes"`
	SavedAlbums  map[string]bool           `json:"saved_albums"`
	SavedArtists map[string]bool           `json:"saved_artists"`
}

func Load(path string) (*Cache, error) {
	c := &Cache{
		path:         path,
		Tracks:       make(map[string]string),
		Albums:       make(map[string]string),
		Artists:      make(map[string]string),
		Playlists:    make(map[string]*PlaylistEntry),
		SavedLikes:   make(map[string]bool),
		SavedAlbums:  make(map[string]bool),
		SavedArtists: make(map[string]bool),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return c, nil
		}
		return nil, fmt.Errorf("read cache file: %w", err)
	}

	if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("parse cache file: %w", err)
	}

	if c.Tracks == nil {
		c.Tracks = make(map[string]string)
	}
	if c.Albums == nil {
		c.Albums = make(map[string]string)
	}
	if c.Artists == nil {
		c.Artists = make(map[string]string)
	}
	if c.Playlists == nil {
		c.Playlists = make(map[string]*PlaylistEntry)
	}
	for _, entry := range c.Playlists {
		if entry.AddedTracks == nil {
			entry.AddedTracks = make(map[string]bool)
		}
	}
	if c.SavedLikes == nil {
		c.SavedLikes = make(map[string]bool)
	}
	if c.SavedAlbums == nil {
		c.SavedAlbums = make(map[string]bool)
	}
	if c.SavedArtists == nil {
		c.SavedArtists = make(map[string]bool)
	}

	return c, nil
}

func (c *Cache) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}

	if err := os.WriteFile(c.path, data, 0o644); err != nil {
		return fmt.Errorf("write cache file: %w", err)
	}
	return nil
}

func (c *Cache) GetTrack(yandexID string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id, ok := c.Tracks[yandexID]
	return id, ok
}

func (c *Cache) SetTrack(yandexID, spotifyID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Tracks[yandexID] = spotifyID
}

func (c *Cache) GetAlbum(yandexID string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id, ok := c.Albums[yandexID]
	return id, ok
}

func (c *Cache) SetAlbum(yandexID, spotifyID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Albums[yandexID] = spotifyID
}

func (c *Cache) GetArtist(yandexID string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id, ok := c.Artists[yandexID]
	return id, ok
}

func (c *Cache) SetArtist(yandexID, spotifyID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Artists[yandexID] = spotifyID
}

func (c *Cache) GetPlaylist(yandexKind string) (*PlaylistEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.Playlists[yandexKind]
	return entry, ok
}

func (c *Cache) SetPlaylist(yandexKind, spotifyID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Playlists[yandexKind] = &PlaylistEntry{
		SpotifyID:   spotifyID,
		AddedTracks: make(map[string]bool),
	}
}

func (c *Cache) AddPlaylistTrack(yandexKind, spotifyTrackID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.Playlists[yandexKind]; ok {
		entry.AddedTracks[spotifyTrackID] = true
	}
}

func (c *Cache) IsLikeSaved(spotifyID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.SavedLikes[spotifyID]
}

func (c *Cache) MarkLikeSaved(spotifyID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SavedLikes[spotifyID] = true
}

func (c *Cache) IsAlbumSaved(spotifyID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.SavedAlbums[spotifyID]
}

func (c *Cache) MarkAlbumSaved(spotifyID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SavedAlbums[spotifyID] = true
}

func (c *Cache) IsArtistSaved(spotifyID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.SavedArtists[spotifyID]
}

func (c *Cache) MarkArtistSaved(spotifyID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SavedArtists[spotifyID] = true
}
