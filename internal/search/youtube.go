package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// YouTubeFetcher resolves a YouTube video id to a directly-playable MP4
// URL using Invidious public mirrors (no API key, no JS).
type YouTubeFetcher struct {
	http *HTTPClient
}

// NewYouTubeFetcher creates a new fetcher.
func NewYouTubeFetcher(c *HTTPClient) *YouTubeFetcher { return &YouTubeFetcher{http: c} }

// VideoFormat — один из доступных стримов из Invidious /api/v1/videos.
type VideoFormat struct {
	URL       string `json:"url"`
	Type      string `json:"type"`
	Container string `json:"container"`
	Encoding  string `json:"encoding"`
	Quality   string `json:"quality"`
	Itag      string `json:"itag"`
	Bitrate   string `json:"bitrate"`
	Size      string `json:"size"` // bytes as string
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
}

// VideoInfo — лёгкий слепок ответа Invidious.
type VideoInfo struct {
	Title         string        `json:"title"`
	Author        string        `json:"author"`
	LengthSec     int           `json:"lengthSeconds"`
	Thumbnail     string        `json:"-"`
	FormatStreams []VideoFormat `json:"formatStreams"`
	// VideoThumbnails — поле с несколькими размерами.
	VideoThumbnails []struct {
		URL    string `json:"url"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	} `json:"videoThumbnails"`
}

// BestPlayable — выбирает самый подходящий формат (mp4 с аудио, не больше maxBytes).
func (v *VideoInfo) BestPlayable(maxBytes int64) *VideoFormat {
	if v == nil || len(v.FormatStreams) == 0 {
		return nil
	}
	// Сортировка по приоритету: mp4 360p (itag=18) — идеален для tg.
	var best *VideoFormat
	for i := range v.FormatStreams {
		f := &v.FormatStreams[i]
		if !strings.Contains(strings.ToLower(f.Container), "mp4") &&
			!strings.Contains(strings.ToLower(f.Type), "video/mp4") {
			continue
		}
		// itag=18 — mp4 360p with AAC audio, идеально для tg <=50MB.
		if f.Itag == "18" {
			return f
		}
		if best == nil {
			best = f
		}
	}
	return best
}

// Мирроры Invidious. Если первый не отвечает — пробуем следующий.
var invidiousMirrors = []string{
	"https://yewtu.be",
	"https://invidious.fdn.fr",
	"https://invidious.privacydev.net",
	"https://inv.nadeko.net",
	"https://invidious.projectsegfau.lt",
	"https://invidious.lunar.icu",
}

// FetchInfo — основной метод: возвращает VideoInfo для YouTube id.
func (f *YouTubeFetcher) FetchInfo(ctx context.Context, id string) (*VideoInfo, error) {
	if id == "" {
		return nil, errors.New("empty youtube id")
	}
	var lastErr error
	for _, base := range invidiousMirrors {
		api := fmt.Sprintf("%s/api/v1/videos/%s", base, id)
		body, _, err := f.http.GetWithHeaders(ctx, api, map[string]string{
			"Accept": "application/json",
		})
		if err != nil || body == "" {
			lastErr = err
			continue
		}
		var info VideoInfo
		if jerr := json.Unmarshal([]byte(body), &info); jerr != nil {
			lastErr = jerr
			continue
		}
		// Распакуем плейсхолдеры url'ов.
		for i := range info.FormatStreams {
			info.FormatStreams[i].URL = jsonURLClean(info.FormatStreams[i].URL)
		}
		// Лучший thumbnail.
		if len(info.VideoThumbnails) > 0 {
			pref := []string{"maxresdefault", "sddefault", "hqdefault", "mqdefault"}
			for _, p := range pref {
				for _, t := range info.VideoThumbnails {
					if strings.Contains(t.URL, p) {
						info.Thumbnail = t.URL
						break
					}
				}
				if info.Thumbnail != "" {
					break
				}
			}
			if info.Thumbnail == "" {
				info.Thumbnail = info.VideoThumbnails[0].URL
			}
		}
		if info.Thumbnail == "" {
			info.Thumbnail = "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg"
		}
		// Также: если в formatStreams ничего нет — fallback к /latest_version?itag=18.
		if len(info.FormatStreams) == 0 {
			info.FormatStreams = []VideoFormat{
				{URL: fmt.Sprintf("%s/latest_version?id=%s&itag=18", base, id),
					Container: "mp4", Itag: "18", Type: "video/mp4"},
			}
		}
		return &info, nil
	}
	if lastErr == nil {
		lastErr = errors.New("all invidious mirrors unavailable")
	}
	return nil, lastErr
}

func jsonURLClean(s string) string {
	s = strings.ReplaceAll(s, `\u0026`, "&")
	s = strings.ReplaceAll(s, `\/`, "/")
	s = strings.ReplaceAll(s, `&amp;`, "&")
	return s
}

// --- Утилиты идентификации YouTube URL ---

var ytIDRe = regexp.MustCompile(`(?i)(?:v=|/embed/|/shorts/|youtu\.be/|/v/)([A-Za-z0-9_-]{11})`)

// IsYouTube — true, если строка похожа на YouTube-ссылку.
func IsYouTube(u string) bool { return ExtractYouTubeID(u) != "" }

// ExtractYouTubeID — извлекает 11-символьный id из любой формы YouTube-URL.
func ExtractYouTubeID(u string) string {
	if m := ytIDRe.FindStringSubmatch(u); len(m) >= 2 {
		return m[1]
	}
	return ""
}
