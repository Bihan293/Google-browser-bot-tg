package search

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const (
	defaultUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
)

// HTTPClient is a small wrapper around http.Client with sane defaults
// (timeout, browser-like User-Agent, redirect handling, cookies).
type HTTPClient struct {
	c  *http.Client
	ua string
}

// NewHTTPClient returns a configured *HTTPClient.
func NewHTTPClient(timeout time.Duration) *HTTPClient {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	jar, _ := cookiejar.New(nil)
	return &HTTPClient{
		c: &http.Client{
			Timeout: timeout,
			Jar:     jar,
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, 6*1024*1024))
	if err != nil {
		return "", resp, err
	}
	if resp.StatusCode >= 400 {
		return "", resp, fmt.Errorf("http %d for %s", resp.StatusCode, rawURL)
	}
	return string(body), resp, nil
}

// GetWithHeaders is like Get but allows passing extra request headers.
func (h *HTTPClient) GetWithHeaders(ctx context.Context, rawURL string, hdrs map[string]string) (string, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", nil, err
	}
	h.applyHeaders(req)
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	resp, err := h.c.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 6*1024*1024))
	if err != nil {
		return "", resp, err
	}
	if resp.StatusCode >= 400 {
		return "", resp, fmt.Errorf("http %d for %s", resp.StatusCode, rawURL)
	}
	return string(body), resp, nil
}

// Probe — лёгкий запрос, чтобы узнать тип, размер и финальный URL.
func (h *HTTPClient) Probe(ctx context.Context, rawURL string) (finalURL, contentType string, contentLength int64, err error) {
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if e != nil {
		return "", "", 0, e
	}
	h.applyHeaders(req)
	req.Header.Set("Range", "bytes=0-0")
	resp, e := h.c.Do(req)
	if e != nil {
		return "", "", 0, e
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	final := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		final = resp.Request.URL.String()
	}
	cl := resp.ContentLength
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		if i := strings.LastIndex(cr, "/"); i >= 0 {
			var n int64
			_, _ = fmt.Sscanf(cr[i+1:], "%d", &n)
			if n > 0 {
				cl = n
			}
		}
	}
	return final, resp.Header.Get("Content-Type"), cl, nil
}

// Download streams a remote file (limited to maxBytes) and returns the bytes.
func (h *HTTPClient) Download(ctx context.Context, rawURL string, maxBytes int64) ([]byte, string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", "", err
	}
	h.applyHeaders(req)
	resp, err := h.c.Do(req)
	if err != nil {
		return nil, "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", "", fmt.Errorf("http %d for %s", resp.StatusCode, rawURL)
	}
	if maxBytes <= 0 {
		maxBytes = 50 * 1024 * 1024
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return nil, "", "", err
	}
	final := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		final = resp.Request.URL.String()
	}
	return data, resp.Header.Get("Content-Type"), final, nil
}

// newPostFormRequest builds a POST form request with extra headers but
// does not execute it. Useful when callers need full control (e.g. cookie).
func (h *HTTPClient) newPostFormRequest(ctx context.Context, rawURL string, values url.Values, hdrs map[string]string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	h.applyHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	return req, nil
}

// doRead executes a request and returns the response body as a string.
func (h *HTTPClient) doRead(req *http.Request) (string, *http.Response, error) {
	resp, err := h.c.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 6*1024*1024))
	if err != nil {
		return "", resp, err
	}
	if resp.StatusCode >= 400 {
		return "", resp, fmt.Errorf("http %d for %s", resp.StatusCode, req.URL.String())
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, 6*1024*1024))
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
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,ru;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
}
