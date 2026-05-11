package handlers

import (
	"net/url"
	"strings"

	"github.com/genspark/tg-browser-bot/internal/store"
)

// hostOfURL returns the bare hostname (without www.) for a URL, or "".
func hostOfURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.TrimPrefix(u.Host, "www.")
}

// ---- HTML escaping helpers ----

// escapeHTML escapes a string for safe inclusion in HTML body / text nodes
// for the Telegram HTML parse mode.
func escapeHTML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return r.Replace(s)
}

// escapeAttr escapes a string for use inside an HTML attribute (e.g. href).
// Currently unused but kept for future link rendering.
var _ = escapeAttr

func escapeAttr(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
	)
	return r.Replace(s)
}

// truncate cuts a string to n runes (with an ellipsis appended if cut).
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 1 {
		return string(runes[:n])
	}
	return string(runes[:n-1]) + "…"
}

// isLikelyURL is a cheap test for "looks like a URL".
func isLikelyURL(s string) bool {
	s = strings.TrimSpace(s)
	if strings.ContainsAny(s, " \n\t") {
		return false
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return true
	}
	// bare domain like example.com/path
	if strings.Contains(s, ".") && !strings.HasPrefix(s, ".") {
		// must contain a likely TLD char run
		dot := strings.LastIndex(s, ".")
		tail := s[dot+1:]
		if len(tail) >= 2 && len(tail) <= 24 {
			return true
		}
	}
	return false
}

// ---- pagination helpers ----

func pageOrEmpty(pages [][]store.WebItem, page int) []store.WebItem {
	if page < 0 || page >= len(pages) {
		return nil
	}
	return pages[page]
}

func pageOrEmptyImg(pages [][]store.ImageItem, page int) []store.ImageItem {
	if page < 0 || page >= len(pages) {
		return nil
	}
	return pages[page]
}

func pageOrEmptyVid(pages [][]store.VideoItem, page int) []store.VideoItem {
	if page < 0 || page >= len(pages) {
		return nil
	}
	return pages[page]
}

func pageOrEmptyNews(pages [][]store.NewsItem, page int) []store.NewsItem {
	if page < 0 || page >= len(pages) {
		return nil
	}
	return pages[page]
}

// ---- chunking helpers ----

func chunkWeb(in []store.WebItem, size int) [][]store.WebItem {
	out := [][]store.WebItem{}
	for i := 0; i < len(in); i += size {
		end := i + size
		if end > len(in) {
			end = len(in)
		}
		out = append(out, in[i:end])
	}
	return out
}
func chunkImg(in []store.ImageItem, size int) [][]store.ImageItem {
	out := [][]store.ImageItem{}
	for i := 0; i < len(in); i += size {
		end := i + size
		if end > len(in) {
			end = len(in)
		}
		out = append(out, in[i:end])
	}
	return out
}
func chunkVid(in []store.VideoItem, size int) [][]store.VideoItem {
	out := [][]store.VideoItem{}
	for i := 0; i < len(in); i += size {
		end := i + size
		if end > len(in) {
			end = len(in)
		}
		out = append(out, in[i:end])
	}
	return out
}
func chunkNews(in []store.NewsItem, size int) [][]store.NewsItem {
	out := [][]store.NewsItem{}
	for i := 0; i < len(in); i += size {
		end := i + size
		if end > len(in) {
			end = len(in)
		}
		out = append(out, in[i:end])
	}
	return out
}

// flatLen* — total items already in cache.
func flatLenWeb(p [][]store.WebItem) int {
	n := 0
	for _, pg := range p {
		n += len(pg)
	}
	return n
}
func flatLenImg(p [][]store.ImageItem) int {
	n := 0
	for _, pg := range p {
		n += len(pg)
	}
	return n
}
func flatLenVid(p [][]store.VideoItem) int {
	n := 0
	for _, pg := range p {
		n += len(pg)
	}
	return n
}
func flatLenNews(p [][]store.NewsItem) int {
	n := 0
	for _, pg := range p {
		n += len(pg)
	}
	return n
}
