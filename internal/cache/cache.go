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

type Cache struct {
	mu   sync.Mutex
	path string

	Tracks  map[string]string `json:"tracks"`
	Albums  map[string]string `json:"albums"`
	Artists map[string]string `json:"artists"`
}

func Load(path string) (*Cache, error) {
	c := &Cache{
		path:    path,
		Tracks:  make(map[string]string),
		Albums:  make(map[string]string),
		Artists: make(map[string]string),
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
