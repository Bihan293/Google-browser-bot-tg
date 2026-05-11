package search

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"

	"github.com/genspark/tg-browser-bot/internal/store"
)

// VideoSearcher fetches videos via YouTube's public search page.
// It parses the embedded ytInitialData JSON payload (no API key required).
type VideoSearcher struct {
	http *HTTPClient
}

// NewVideoSearcher returns a new VideoSearcher.
func NewVideoSearcher(c *HTTPClient) *VideoSearcher {
	return &VideoSearcher{http: c}
}

var ytInitialRe = regexp.MustCompile(`(?s)var ytInitialData = (\{.*?\});</script>`)

// Search returns up to `limit` video results. When nsfw=true we turn off
// YouTube's restricted mode via the PREF cookie + ?safeSearch=None.
func (v *VideoSearcher) Search(ctx context.Context, query string, limit int, nsfw bool) ([]store.VideoItem, error) {
	if limit <= 0 {
		limit = 10
	}
	endpoint := "https://www.youtube.com/results?search_query=" + url.QueryEscape(query) + "&hl=en"
	hdrs := map[string]string{}
	if nsfw {
		// f2=8000000 disables restricted mode in YouTube PREF cookie.
		hdrs["Cookie"] = "PREF=f2=8000000&hl=en; CONSENT=YES+1"
	} else {
		hdrs["Cookie"] = "CONSENT=YES+1"
	}
	body, _, err := v.http.GetWithHeaders(ctx, endpoint, hdrs)
	if err != nil {
		return nil, err
	}

	m := ytInitialRe.FindStringSubmatch(body)
	if len(m) < 2 {
		return v.fallbackParse(body, limit), nil
	}

	var raw any
	if err := json.Unmarshal([]byte(m[1]), &raw); err != nil {
		return v.fallbackParse(body, limit), nil
	}

	items := walkVideoRenderers(raw, limit)
	if len(items) == 0 {
		items = v.fallbackParse(body, limit)
	}
	return items, nil
}

// walkVideoRenderers recursively extracts videoRenderer objects from
// the ytInitialData JSON tree.
func walkVideoRenderers(node any, limit int) []store.VideoItem {
	out := make([]store.VideoItem, 0, limit)
	var walk func(any)
	walk = func(n any) {
		if len(out) >= limit {
			return
		}
		switch v := n.(type) {
		case map[string]any:
			if vr, ok := v["videoRenderer"].(map[string]any); ok {
				item := parseVideoRenderer(vr)
				if item.URL != "" {
					out = append(out, item)
					if len(out) >= limit {
						return
					}
				}
			}
			for _, child := range v {
				walk(child)
				if len(out) >= limit {
					return
				}
			}
		case []any:
			for _, child := range v {
				walk(child)
				if len(out) >= limit {
					return
				}
			}
		}
	}
	walk(node)
	return out
}

func parseVideoRenderer(vr map[string]any) store.VideoItem {
	var item store.VideoItem

	if id, _ := vr["videoId"].(string); id != "" {
		item.VideoID = id
		item.URL = "https://www.youtube.com/watch?v=" + id
	}
	item.Title = extractRunsText(vr["title"])
	item.Author = extractRunsText(vr["ownerText"])
	if item.Author == "" {
		item.Author = extractRunsText(vr["longBylineText"])
	}
	if dt, ok := vr["lengthText"].(map[string]any); ok {
		if s, ok := dt["simpleText"].(string); ok {
			item.Duration = s
		}
	}
	if tb, ok := vr["thumbnail"].(map[string]any); ok {
		if arr, ok := tb["thumbnails"].([]any); ok && len(arr) > 0 {
			if last, ok := arr[len(arr)-1].(map[string]any); ok {
				if u, ok := last["url"].(string); ok {
					item.Thumb = u
				}
			}
		}
	}
	if item.Thumb == "" && item.VideoID != "" {
		item.Thumb = "https://i.ytimg.com/vi/" + item.VideoID + "/hqdefault.jpg"
	}
	return item
}

func extractRunsText(n any) string {
	m, ok := n.(map[string]any)
	if !ok {
		return ""
	}
	if s, ok := m["simpleText"].(string); ok {
		return s
	}
	runs, ok := m["runs"].([]any)
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, r := range runs {
		if rm, ok := r.(map[string]any); ok {
			if t, ok := rm["text"].(string); ok {
				b.WriteString(t)
			}
		}
	}
	return b.String()
}

// fallbackParse: simple regex over the HTML for /watch?v=ID links.
var watchRe = regexp.MustCompile(`/watch\?v=([A-Za-z0-9_-]{11})`)

func (v *VideoSearcher) fallbackParse(body string, limit int) []store.VideoItem {
	seen := map[string]bool{}
	out := []store.VideoItem{}
	for _, m := range watchRe.FindAllStringSubmatch(body, -1) {
		id := m[1]
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, store.VideoItem{
			Title:   "YouTube video",
			URL:     "https://www.youtube.com/watch?v=" + id,
			Thumb:   "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg",
			VideoID: id,
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}
