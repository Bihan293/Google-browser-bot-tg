package search

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/genspark/tg-browser-bot/internal/store"
)

// ImageSearcher uses DuckDuckGo's image endpoint to fetch images for a query.
// The endpoint is unofficial but stable and returns JSON we can parse.
type ImageSearcher struct {
	http *HTTPClient
}

// NewImageSearcher returns a new ImageSearcher.
func NewImageSearcher(c *HTTPClient) *ImageSearcher {
	return &ImageSearcher{http: c}
}

var vqdRe = regexp.MustCompile(`vqd=['"]?([\d-]+)['"]?`)

// Search returns up to `limit` images for the query.
func (i *ImageSearcher) Search(ctx context.Context, query string, limit int) ([]store.ImageItem, error) {
	if limit <= 0 {
		limit = 10
	}

	// Step 1: get vqd token (DDG requires it for image queries).
	tokenURL := "https://duckduckgo.com/?q=" + url.QueryEscape(query) + "&iax=images&ia=images"
	body, _, err := i.http.Get(ctx, tokenURL)
	if err != nil {
		return nil, err
	}
	m := vqdRe.FindStringSubmatch(body)
	if len(m) < 2 {
		return nil, errors.New("could not resolve image search token")
	}
	vqd := m[1]

	// Step 2: call the image JSON endpoint.
	q := url.Values{}
	q.Set("l", "wt-wt")
	q.Set("o", "json")
	q.Set("q", query)
	q.Set("vqd", vqd)
	q.Set("f", ",,,")
	q.Set("p", "1")
	endpoint := "https://duckduckgo.com/i.js?" + q.Encode()

	raw, _, err := i.http.Get(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Results []struct {
			Title     string `json:"title"`
			Image     string `json:"image"`
			URL       string `json:"url"`
			Source    string `json:"source"`
			Thumbnail string `json:"thumbnail"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}

	out := make([]store.ImageItem, 0, limit)
	for _, r := range payload.Results {
		if len(out) >= limit {
			break
		}
		img := r.Image
		if img == "" {
			img = r.Thumbnail
		}
		if img == "" {
			continue
		}
		out = append(out, store.ImageItem{
			Title:    strings.TrimSpace(r.Title),
			ImageURL: img,
			Source:   r.Source,
		})
	}
	return out, nil
}
