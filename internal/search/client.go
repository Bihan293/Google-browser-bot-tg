package search

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
)

// HTTPClient is a small wrapper around http.Client with sane defaults
// (timeout, browser-like User-Agent, redirect handling).
type HTTPClient struct {
	c  *http.Client
	ua string
}

// NewHTTPClient returns a configured *HTTPClient.
func NewHTTPClient(timeout time.Duration) *HTTPClient {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &HTTPClient{
		c: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
		ua: defaultUA,
	}
}

// Get performs an HTTP GET with browser-like headers.
func (h *HTTPClient) Get(ctx context.Context, rawURL string) (string, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", nil, err
	}
	h.applyHeaders(req)

	resp, err := h.c.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	// limit body to 4 MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return "", resp, err
	}
	if resp.StatusCode >= 400 {
		return "", resp, fmt.Errorf("http %d for %s", resp.StatusCode, rawURL)
	}
	return string(body), resp, nil
}

// PostForm sends application/x-www-form-urlencoded data.
func (h *HTTPClient) PostForm(ctx context.Context, rawURL string, values url.Values) (string, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(values.Encode()))
	if err != nil {
		return "", nil, err
	}
	h.applyHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := h.c.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return "", resp, err
	}
	if resp.StatusCode >= 400 {
		return "", resp, fmt.Errorf("http %d for %s", resp.StatusCode, rawURL)
	}
	return string(body), resp, nil
}

func (h *HTTPClient) applyHeaders(req *http.Request) {
	req.Header.Set("User-Agent", h.ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,ru;q=0.8")
}
