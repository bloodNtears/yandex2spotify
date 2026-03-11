package spotify

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

type retryTransport struct {
	base http.RoundTripper
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	if resp.StatusCode != http.StatusTooManyRequests {
		return resp, nil
	}

	retryAfter := resp.Header.Get("Retry-After")
	seconds, _ := strconv.Atoi(retryAfter)
	if seconds <= 0 {
		seconds = 30
	}
	resp.Body.Close()

	wait := formatDuration(seconds)
	log.Printf("Spotify вернул 429 (rate limit). Ожидание %s. "+
		"Можно подождать или перезапустить приложение через %s — кэш сохранит прогресс.", wait, wait)

	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	defer timer.Stop()

	select {
	case <-timer.C:
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}

	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("recreate request body for retry: %w", err)
		}
		req.Body = body
	}

	return t.base.RoundTrip(req)
}
