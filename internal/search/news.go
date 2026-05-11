package search

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/genspark/tg-browser-bot/internal/store"
)

// NewsSearcher uses Google News RSS (no API key needed) to fetch fresh
// news articles for a given query. The RSS feed returns Google-internal
// redirect links (news.google.com/rss/articles/...). We resolve them to
// the actual publisher URLs so the bot can render the real article
// instead of Google News's stub page.
type NewsSearcher struct {
	http *HTTPClient

	// cache resolved redirects so we don't pay the HEAD round-trip twice.
	mu    sync.Mutex
	cache map[string]string
}

// NewNewsSearcher returns a new NewsSearcher.
func NewNewsSearcher(c *HTTPClient) *NewsSearcher {
	return &NewsSearcher{http: c, cache: map[string]string{}}
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

// hrefRe — все <a href="..."> в описании RSS-итема (Google кладёт туда
// финальную ссылку на статью).
var hrefRe = regexp.MustCompile(`(?i)<a\s+[^>]*href=["']([^"']+)["']`)

// Search returns up to `limit` news items with REAL article URLs.
func (n *NewsSearcher) Search(ctx context.Context, query string, limit int) ([]store.NewsItem, error) {
	if limit <= 0 {
		limit = 10
	}
	// Локаль учитываем из запроса: кириллица → русская выдача.
	hl, gl, ceid := "en-US", "US", "US:en"
	if hasCyrillic(query) {
		hl, gl, ceid = "ru", "RU", "RU:ru"
	}
	endpoint := "https://news.google.com/rss/search?q=" + url.QueryEscape(query) +
		"&hl=" + hl + "&gl=" + gl + "&ceid=" + ceid
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

		// 1) Сначала пробуем достать «настоящую» ссылку из <a href> в description.
		resolved := extractFirstHref(it.Description)
		if resolved == "" {
			resolved = link
		}
		// 2) Если это всё ещё Google News — следуем редиректу.
		if isGoogleNewsURL(resolved) {
			if real := n.resolveRedirect(ctx, resolved); real != "" {
				resolved = real
			}
		}

		// Если источник в RSS пуст — попробуем достать домен.
		source := strings.TrimSpace(it.Source.Value)
		if source == "" {
			source = hostOf(resolved)
		}

		// Заголовок Google часто содержит "Title - Source" в конце —
		// уберём дубль.
		if source != "" {
			title = strings.TrimSuffix(title, " - "+source)
		}

		out = append(out, store.NewsItem{
			Title:   title,
			URL:     resolved,
			Source:  source,
			Snippet: stripHTML(it.Description),
			Date:    formatPubDate(it.PubDate),
		})
	}
	return out, nil
}

// resolveRedirect раскрывает Google News redirect-ссылку до конечного URL
// издателя. Делает короткий GET с прерванным телом и читает Location/итог.
func (n *NewsSearcher) resolveRedirect(ctx context.Context, gnURL string) string {
	n.mu.Lock()
	if v, ok := n.cache[gnURL]; ok {
		n.mu.Unlock()
		return v
	}
	n.mu.Unlock()

	rctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	final, _, _, err := n.http.Probe(rctx, gnURL)
	if err == nil && final != "" && !isGoogleNewsURL(final) {
		n.mu.Lock()
		n.cache[gnURL] = final
		n.mu.Unlock()
		return final
	}

	// Fallback: parse full page and search for <c-wiz>/<a> with href to real article.
	body, _, gerr := n.http.GetWithHeaders(rctx, gnURL, map[string]string{
		"Accept":     "text/html,application/xhtml+xml",
		"Referer":    "https://news.google.com/",
		"User-Agent": defaultUA,
	})
	if gerr != nil {
		return ""
	}
	// Ищем первый http(s)-href, не указывающий на google.
	for _, m := range hrefRe.FindAllStringSubmatch(body, -1) {
		u := strings.TrimSpace(m[1])
		if (strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")) &&
			!isGoogleHost(u) {
			n.mu.Lock()
			n.cache[gnURL] = u
			n.mu.Unlock()
			return u
		}
	}
	return ""
}

// extractFirstHref — первый внешний href в HTML-описании RSS-итема.
func extractFirstHref(desc string) string {
	for _, m := range hrefRe.FindAllStringSubmatch(desc, -1) {
		u := strings.TrimSpace(m[1])
		if (strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")) &&
			!isGoogleHost(u) {
			return u
		}
	}
	return ""
}

func isGoogleNewsURL(u string) bool {
	lu := strings.ToLower(u)
	return strings.Contains(lu, "news.google.com")
}

func isGoogleHost(u string) bool {
	pu, err := url.Parse(u)
	if err != nil {
		return false
	}
	h := strings.ToLower(pu.Host)
	return strings.HasSuffix(h, "google.com") || strings.HasSuffix(h, "google.ru") ||
		strings.HasSuffix(h, "googleusercontent.com") || strings.HasSuffix(h, "gstatic.com")
}

func hostOf(u string) string {
	pu, err := url.Parse(u)
	if err != nil || pu.Host == "" {
		return ""
	}
	return strings.TrimPrefix(pu.Host, "www.")
}

func hasCyrillic(s string) bool {
	for _, r := range s {
		if r >= 0x0400 && r <= 0x04FF {
			return true
		}
	}
	return false
}

// formatPubDate приводит RFC1123Z дату из RSS к удобному виду «11 May 2026, 14:05».
func formatPubDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	layouts := []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC().Format("02 Jan 2006, 15:04 UTC")
		}
	}
	return s
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

// compile-time check we link net/http so we don't have unused import
// (kept here in case future code needs request building).
var _ = http.MethodGet
