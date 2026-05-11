package search

import (
	"context"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/genspark/tg-browser-bot/internal/store"
)

// WebSearcher performs textual web search using DuckDuckGo's HTML endpoint.
// DuckDuckGo does not require an API key and returns clean HTML we can parse.
type WebSearcher struct {
	http *HTTPClient
}

// NewWebSearcher creates a new WebSearcher.
func NewWebSearcher(c *HTTPClient) *WebSearcher {
	return &WebSearcher{http: c}
}

// Search returns up to `limit` web results for the given query.
func (w *WebSearcher) Search(ctx context.Context, query string, limit int) ([]store.WebItem, error) {
	if limit <= 0 {
		limit = 10
	}
	endpoint := "https://html.duckduckgo.com/html/"
	form := url.Values{}
	form.Set("q", query)
	form.Set("kl", "wt-wt")

	body, _, err := w.http.PostForm(ctx, endpoint, form)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	var results []store.WebItem
	doc.Find("div.result, div.web-result").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if len(results) >= limit {
			return false
		}
		title := strings.TrimSpace(s.Find("a.result__a").Text())
		link, _ := s.Find("a.result__a").Attr("href")
		snippet := strings.TrimSpace(s.Find("a.result__snippet, .result__snippet").Text())

		link = decodeDuckURL(link)
		if title == "" || link == "" {
			return true
		}
		results = append(results, store.WebItem{
			Title:   cleanWS(title),
			URL:     link,
			Snippet: cleanWS(snippet),
		})
		return true
	})

	return results, nil
}

// decodeDuckURL converts DDG redirect links (/l/?uddg=...) to the real URL.
func decodeDuckURL(raw string) string {
	if raw == "" {
		return raw
	}
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.Path == "/l/" || strings.HasSuffix(u.Path, "/l/") {
		if v := u.Query().Get("uddg"); v != "" {
			if dec, err := url.QueryUnescape(v); err == nil {
				return dec
			}
		}
	}
	return raw
}

func cleanWS(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}
