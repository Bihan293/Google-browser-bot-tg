package render

import (
	"context"
	"errors"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// PageFetcher loads an arbitrary URL and extracts a readable plain-text
// representation of its main content, so the bot can show the page
// _inside_ Telegram instead of redirecting the user out.
type PageFetcher struct {
	get func(ctx context.Context, url string) (string, error)
}

// NewPageFetcher creates a PageFetcher with the given GET function.
// The function is injected so we keep this package decoupled from the
// concrete HTTP client implementation.
func NewPageFetcher(get func(ctx context.Context, url string) (string, error)) *PageFetcher {
	return &PageFetcher{get: get}
}

// Article is the extracted, render-ready representation of a web page.
type Article struct {
	URL         string
	Title       string
	Description string
	Text        string
	TopImage    string
}

// Fetch loads the given URL and returns a cleaned-up Article.
func (p *PageFetcher) Fetch(ctx context.Context, rawURL string) (*Article, error) {
	if rawURL == "" {
		return nil, errors.New("empty url")
	}
	body, err := p.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	art := &Article{URL: rawURL}

	// Title preference: <meta og:title> -> <title>
	if v, ok := doc.Find(`meta[property="og:title"]`).Attr("content"); ok && strings.TrimSpace(v) != "" {
		art.Title = strings.TrimSpace(v)
	}
	if art.Title == "" {
		art.Title = strings.TrimSpace(doc.Find("title").First().Text())
	}

	// Description
	if v, ok := doc.Find(`meta[property="og:description"]`).Attr("content"); ok && strings.TrimSpace(v) != "" {
		art.Description = strings.TrimSpace(v)
	} else if v, ok := doc.Find(`meta[name="description"]`).Attr("content"); ok {
		art.Description = strings.TrimSpace(v)
	}

	// Top image
	if v, ok := doc.Find(`meta[property="og:image"]`).Attr("content"); ok {
		art.TopImage = strings.TrimSpace(v)
	}

	// Remove noisy nodes.
	doc.Find("script, style, noscript, nav, footer, header, aside, form, iframe, svg").Remove()

	// Pick the most "article-like" container.
	candidates := []string{"article", "main", "[role=main]", ".post", ".article", ".entry-content", "#content", "body"}
	var text string
	for _, sel := range candidates {
		sel := doc.Find(sel).First()
		if sel.Length() == 0 {
			continue
		}
		t := strings.TrimSpace(sel.Text())
		if len(t) > len(text) {
			text = t
		}
		if len(text) > 1500 {
			break
		}
	}
	art.Text = collapseWS(text)
	return art, nil
}

func collapseWS(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		switch r {
		case '\r':
			continue
		case '\n':
			b.WriteRune('\n')
			prevSpace = true
		case '\t', ' ', '\u00a0':
			if !prevSpace {
				b.WriteRune(' ')
				prevSpace = true
			}
		default:
			b.WriteRune(r)
			prevSpace = false
		}
	}
	// collapse runs of >2 newlines
	out := b.String()
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(out)
}
