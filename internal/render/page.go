package render

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// PageFetcher loads an arbitrary URL and extracts a readable plain-text
// representation of its main content, plus all media (images, videos)
// and outgoing links, so the bot can show the page _inside_ Telegram.
type PageFetcher struct {
	get func(ctx context.Context, url string) (string, error)
}

// NewPageFetcher creates a PageFetcher with the given GET function.
func NewPageFetcher(get func(ctx context.Context, url string) (string, error)) *PageFetcher {
	return &PageFetcher{get: get}
}

// Link — гиперссылка, найденная на странице.
type Link struct {
	Text string
	URL  string
}

// Article is the extracted, render-ready representation of a web page.
type Article struct {
	URL         string
	Title       string
	Description string
	Text        string
	TopImage    string

	Images []string // прямые URL <img src>
	Videos []string // прямые URL <video>/<source> mp4/webm
	YTIDs  []string // youtube video ids, найденные в iframe/ссылках
	Links  []Link   // внутренние/внешние ссылки страницы
}

// Fetch loads the given URL and returns a cleaned-up Article.
func (p *PageFetcher) Fetch(ctx context.Context, rawURL string) (*Article, error) {
	if rawURL == "" {
		return nil, errors.New("empty url")
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}
	body, err := p.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	base, _ := url.Parse(rawURL)
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

	// Top image (og:image)
	if v, ok := doc.Find(`meta[property="og:image"]`).Attr("content"); ok {
		art.TopImage = absURL(base, strings.TrimSpace(v))
	}

	// --- собираем медиа ДО зачистки тегов ---
	seenImg := map[string]bool{}
	addImg := func(u string) {
		u = absURL(base, strings.TrimSpace(u))
		if u == "" || seenImg[u] {
			return
		}
		if !isLikelyImageURL(u) {
			return
		}
		seenImg[u] = true
		art.Images = append(art.Images, u)
	}
	if art.TopImage != "" {
		addImg(art.TopImage)
	}
	// og:image (на случай нескольких)
	doc.Find(`meta[property="og:image"], meta[name="twitter:image"]`).Each(func(_ int, s *goquery.Selection) {
		if v, ok := s.Attr("content"); ok {
			addImg(v)
		}
	})
	// обычные <img>
	doc.Find("img").Each(func(_ int, s *goquery.Selection) {
		for _, attr := range []string{"src", "data-src", "data-original", "data-lazy-src", "data-srcset"} {
			if v, ok := s.Attr(attr); ok && v != "" {
				// data-srcset / srcset — берём первый
				if attr == "data-srcset" || attr == "srcset" {
					v = firstFromSrcset(v)
				}
				addImg(v)
				break
			}
		}
		// также пробуем srcset
		if v, ok := s.Attr("srcset"); ok && v != "" {
			addImg(firstFromSrcset(v))
		}
	})
	doc.Find("source[srcset]").Each(func(_ int, s *goquery.Selection) {
		if v, ok := s.Attr("srcset"); ok {
			addImg(firstFromSrcset(v))
		}
	})

	// <video> + <source>
	seenVid := map[string]bool{}
	addVid := func(u string) {
		u = absURL(base, strings.TrimSpace(u))
		if u == "" || seenVid[u] {
			return
		}
		if !isLikelyVideoURL(u) {
			return
		}
		seenVid[u] = true
		art.Videos = append(art.Videos, u)
	}
	doc.Find("video").Each(func(_ int, s *goquery.Selection) {
		if v, ok := s.Attr("src"); ok {
			addVid(v)
		}
		s.Find("source").Each(func(_ int, ss *goquery.Selection) {
			if v, ok := ss.Attr("src"); ok {
				addVid(v)
			}
		})
	})
	// og:video / twitter:player:stream
	doc.Find(`meta[property="og:video"], meta[property="og:video:url"], meta[property="og:video:secure_url"], meta[name="twitter:player:stream"]`).Each(func(_ int, s *goquery.Selection) {
		if v, ok := s.Attr("content"); ok {
			addVid(v)
		}
	})

	// youtube/youtu.be ссылки и iframe-эмбеды
	seenYT := map[string]bool{}
	addYT := func(id string) {
		if id == "" || seenYT[id] {
			return
		}
		seenYT[id] = true
		art.YTIDs = append(art.YTIDs, id)
	}
	doc.Find("iframe").Each(func(_ int, s *goquery.Selection) {
		src, _ := s.Attr("src")
		if src == "" {
			return
		}
		src = absURL(base, src)
		if id := extractYTID(src); id != "" {
			addYT(id)
		}
	})
	// также найдем «голые» youtube-ссылки внутри страницы
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		if id := extractYTID(absURL(base, href)); id != "" {
			addYT(id)
		}
	})

	// ссылки (для inline-кнопок «открыть в боте»)
	seenLink := map[string]bool{}
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		href = absURL(base, href)
		if !isHTTPLink(href) {
			return
		}
		if seenLink[href] {
			return
		}
		seenLink[href] = true
		text := strings.TrimSpace(s.Text())
		text = collapseWS(text)
		if text == "" {
			text = href
		}
		if len([]rune(text)) > 80 {
			text = string([]rune(text)[:80]) + "…"
		}
		art.Links = append(art.Links, Link{Text: text, URL: href})
	})

	// --- теперь чистим узлы и берём текст ---
	doc.Find("script, style, noscript, nav, footer, header, aside, form, iframe, svg").Remove()

	candidates := []string{"article", "main", "[role=main]", ".post", ".article", ".entry-content", "#content", "body"}
	var text string
	for _, sel := range candidates {
		ssel := doc.Find(sel).First()
		if ssel.Length() == 0 {
			continue
		}
		t := strings.TrimSpace(ssel.Text())
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

// --- helpers ---

func absURL(base *url.URL, href string) string {
	if href == "" {
		return ""
	}
	href = strings.TrimSpace(href)
	if strings.HasPrefix(href, "data:") || strings.HasPrefix(href, "javascript:") ||
		strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") {
		return ""
	}
	if strings.HasPrefix(href, "//") {
		if base != nil && base.Scheme != "" {
			return base.Scheme + ":" + href
		}
		return "https:" + href
	}
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	if base == nil {
		return href
	}
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return base.ResolveReference(u).String()
}

func firstFromSrcset(s string) string {
	// "url1 1x, url2 2x" → url1
	parts := strings.Split(s, ",")
	if len(parts) == 0 {
		return ""
	}
	first := strings.TrimSpace(parts[0])
	if sp := strings.Index(first, " "); sp > 0 {
		first = first[:sp]
	}
	return first
}

func isLikelyImageURL(u string) bool {
	lu := strings.ToLower(u)
	// прямой файл
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp", ".gif", ".avif", ".bmp"} {
		if strings.Contains(lu, ext) {
			return true
		}
	}
	// многие CDN отдают без расширения, но в пути есть /image/ или подобное
	if strings.Contains(lu, "/image") || strings.Contains(lu, "/photo") ||
		strings.Contains(lu, "img") || strings.Contains(lu, "thumb") ||
		strings.Contains(lu, "media") {
		return true
	}
	return false
}

func isLikelyVideoURL(u string) bool {
	lu := strings.ToLower(u)
	for _, ext := range []string{".mp4", ".webm", ".mov", ".m3u8", ".mpd", ".mkv"} {
		if strings.Contains(lu, ext) {
			return true
		}
	}
	return false
}

func isHTTPLink(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

func extractYTID(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Host)
	switch {
	case strings.Contains(host, "youtu.be"):
		id := strings.TrimPrefix(u.Path, "/")
		if len(id) >= 11 {
			return id[:11]
		}
	case strings.Contains(host, "youtube.com") || strings.Contains(host, "youtube-nocookie.com"):
		if v := u.Query().Get("v"); len(v) >= 11 {
			return v[:11]
		}
		// /embed/<id> /shorts/<id> /v/<id>
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		for i, p := range parts {
			if (p == "embed" || p == "shorts" || p == "v") && i+1 < len(parts) {
				id := parts[i+1]
				if len(id) >= 11 {
					return id[:11]
				}
			}
		}
	}
	return ""
}

// EnsureScheme adds https:// to bare domains, leaves http(s) urls intact.
func EnsureScheme(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	if strings.HasPrefix(u, "//") {
		return "https:" + u
	}
	return "https://" + u
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
	out := b.String()
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(out)
}
