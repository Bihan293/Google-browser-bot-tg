package search

import (
	"context"
	"encoding/xml"
	"net/url"
	"strings"

	"github.com/genspark/tg-browser-bot/internal/store"
)

// NewsSearcher uses Google News RSS (no API key needed) to fetch fresh
// news articles for a given query. We only consume the RSS feed — users
// never get redirected to Google; results are rendered inside the bot.
type NewsSearcher struct {
	http *HTTPClient
}

// NewNewsSearcher returns a new NewsSearcher.
func NewNewsSearcher(c *HTTPClient) *NewsSearcher {
	return &NewsSearcher{http: c}
}

type rss struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			PubDate     string `xml:"pubDate"`
			Description string `xml:"description"`
			Source      struct {
				Value string `xml:",chardata"`
				URL   string `xml:"url,attr"`
			} `xml:"source"`
		} `xml:"item"`
	} `xml:"channel"`
}

// Search returns up to `limit` news items.
func (n *NewsSearcher) Search(ctx context.Context, query string, limit int) ([]store.NewsItem, error) {
	if limit <= 0 {
		limit = 10
	}
	endpoint := "https://news.google.com/rss/search?q=" + url.QueryEscape(query) + "&hl=en-US&gl=US&ceid=US:en"
	body, _, err := n.http.Get(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	var feed rss
	if err := xml.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}

	out := make([]store.NewsItem, 0, limit)
	for _, it := range feed.Channel.Items {
		if len(out) >= limit {
			break
		}
		title := strings.TrimSpace(it.Title)
		link := strings.TrimSpace(it.Link)
		if title == "" || link == "" {
			continue
		}
		out = append(out, store.NewsItem{
			Title:   title,
			URL:     link,
			Source:  strings.TrimSpace(it.Source.Value),
			Snippet: stripHTML(it.Description),
			Date:    it.PubDate,
		})
	}
	return out, nil
}

// stripHTML removes HTML tags from a string (best-effort, no allocations beyond a builder).
func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return cleanWS(b.String())
}
