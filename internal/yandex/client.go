package yandex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.music.yandex.net"

type Client struct {
	token      string
	httpClient *http.Client
}

func NewClient(token string, timeout time.Duration) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) do(ctx context.Context, method, path string, params url.Values) ([]byte, error) {
	uri := baseURL + "/" + path
	if len(params) > 0 {
		uri += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, uri, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "OAuth "+c.token)
	req.Header.Set("X-Yandex-Music-Client", "YandexMusicAndroid/24023231")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized: check your Yandex token")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (c *Client) AccountStatus(ctx context.Context) (*AccountStatus, error) {
	body, err := c.do(ctx, http.MethodGet, "account/status", nil)
	if err != nil {
		return nil, err
	}

	var resp accountStatusResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode account status: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("account status: %s", resp.Error)
	}
	return &resp.Result, nil
}

func (c *Client) Playlists(ctx context.Context, uid uint32) ([]Playlist, error) {
	path := fmt.Sprintf("users/%d/playlists/list", uid)
	body, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var resp playlistListResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode playlists: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("playlists: %s", resp.Error)
	}
	return resp.Result, nil
}

func (c *Client) PlaylistTracks(ctx context.Context, uid uint32, kind int) (*Playlist, error) {
	path := fmt.Sprintf("users/%d/playlists/%d", uid, kind)
	body, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var resp playlistResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode playlist tracks: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("playlist tracks: %s", resp.Error)
	}
	return resp.Result, nil
}

func (c *Client) LikedTracks(ctx context.Context, uid uint32) ([]LikedTrack, error) {
	path := fmt.Sprintf("users/%d/likes/tracks", uid)
	body, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var resp likedTracksResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode liked tracks: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("liked tracks: %s", resp.Error)
	}
	return resp.Result.Library.Tracks, nil
}

// Tracks fetches full track info by IDs (format: "trackId:albumId").
func (c *Client) Tracks(ctx context.Context, ids []string) ([]Track, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	uri := baseURL + "/tracks"
	form := url.Values{"track-ids": {strings.Join(ids, ",")}}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "OAuth "+c.token)
	req.Header.Set("X-Yandex-Music-Client", "YandexMusicAndroid/24023231")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tracks: unexpected status %d", resp.StatusCode)
	}

	var result tracksResp
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("decode tracks: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("tracks: %s", result.Error)
	}
	return result.Result, nil
}

func (c *Client) LikedAlbumIDs(ctx context.Context, uid uint32) ([]int, error) {
	path := fmt.Sprintf("users/%d/likes/albums", uid)
	body, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var resp likedAlbumsResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode liked albums: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("liked albums: %s", resp.Error)
	}

	ids := make([]int, len(resp.Result))
	for i, la := range resp.Result {
		ids[i] = la.ID
	}
	return ids, nil
}

func (c *Client) Albums(ctx context.Context, ids []int) ([]Album, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = strconv.Itoa(id)
	}

	uri := baseURL + "/albums"
	form := url.Values{"album-ids": {strings.Join(strs, ",")}}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "OAuth "+c.token)
	req.Header.Set("X-Yandex-Music-Client", "YandexMusicAndroid/24023231")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("albums: unexpected status %d", resp.StatusCode)
	}

	var result albumsResp
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("decode albums: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("albums: %s", result.Error)
	}
	return result.Result, nil
}
