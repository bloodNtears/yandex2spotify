package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const apiBase = "https://api.spotify.com/v1"

type Client struct {
	http *http.Client
}

func NewClient(httpClient *http.Client) *Client {
	transport := httpClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	httpClient.Transport = &retryTransport{base: transport}
	return &Client{http: httpClient}
}

func (c *Client) HTTPClient() *http.Client {
	return c.http
}

func (c *Client) CurrentUserID(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/me", nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("GET /me failed: status=%s body=%s", resp.Status, string(raw))
	}

	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return out.ID, nil
}

func (c *Client) Search(ctx context.Context, query, searchType string) (*SearchResult, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("type", searchType)
	params.Set("limit", "1")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/search?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("search failed: status=%s body=%s", resp.Status, string(raw))
	}

	var result SearchResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}
