package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/genspark/tg-browser-bot/internal/keyboard"
	"github.com/genspark/tg-browser-bot/internal/render"
	"github.com/genspark/tg-browser-bot/internal/search"
	"github.com/genspark/tg-browser-bot/internal/store"
)

// 50 MB — лимит на видео-аплоад через Bot API (через URL-загрузку тоже фактически 20MB,
// поэтому будем сами скачивать в память и слать как файл).
const maxVideoBytes = 50 * 1024 * 1024
const maxImageBytes = 10 * 1024 * 1024

// openResult реагирует на нажатие номера у результата.
func (h *Handler) openResult(ctx context.Context, chatID int64, kind store.SearchKind, idx int) {
	sess := h.Store.Get(chatID)
	if sess == nil {
		h.reply(chatID, "Сессия истекла. Пришли запрос заново.")
		return
	}

	switch kind {
	case store.KindWeb:
		items := pageOrEmpty(sess.WebResults, sess.Page)
		if idx < 0 || idx >= len(items) {
			h.reply(chatID, "Нет такого результата.")
			return
		}
		h.openURL(ctx, chatID, items[idx].URL, kind)

	case store.KindImages:
		items := pageOrEmptyImg(sess.ImageResults, sess.Page)
		if idx < 0 || idx >= len(items) {
			h.reply(chatID, "Нет такого результата.")
			return
		}
		h.sendImageDetailed(chatID, items[idx], kind)

	case store.KindVideos:
		items := pageOrEmptyVid(sess.VideoResults, sess.Page)
		if idx < 0 || idx >= len(items) {
			h.reply(chatID, "Нет такого результата.")
			return
		}
		h.playYouTube(ctx, chatID, items[idx], kind)

	case store.KindNews:
		items := pageOrEmptyNews(sess.NewsResults, sess.Page)
		if idx < 0 || idx >= len(items) {
			h.reply(chatID, "Нет такого результата.")
			return
		}
		h.openURL(ctx, chatID, items[idx].URL, kind)
	}
}

// openURL fetches the page and renders its content in-chat (text + media + buttons).
func (h *Handler) openURL(ctx context.Context, chatID int64, rawURL string, kind store.SearchKind) {
	// Если это YouTube — сразу пытаемся проиграть.
	if search.IsYouTube(rawURL) {
		if id := search.ExtractYouTubeID(rawURL); id != "" {
			h.playYouTubeByID(ctx, chatID, id, "", "", "", "", kind)
			return
		}
	}

	notice := tgbotapi.NewMessage(chatID, "🌐 Загружаю <code>"+escapeHTML(truncate(rawURL, 80))+"</code>...")
	notice.ParseMode = tgbotapi.ModeHTML
	loading, _ := h.API.Send(notice)

	art, err := h.Page.Fetch(ctx, rawURL)
	if loading.MessageID != 0 {
		_, _ = h.API.Request(tgbotapi.NewDeleteMessage(chatID, loading.MessageID))
	}
	if err != nil {
		h.sendError(chatID, err)
		return
	}

	// Сохраним в сессию: что открыто, какие ссылки/медиа.
	h.Store.Update(chatID, func(s *store.Session) {
		s.OpenedURL = art.URL
		s.PageTitle = art.Title
		s.PageImages = art.Images
		s.PageVideos = art.Videos
		// объединяем «обычные» ссылки + youtube-id (как ссылки)
		links := make([]store.PageLink, 0, len(art.Links)+len(art.YTIDs))
		for _, l := range art.Links {
			links = append(links, store.PageLink{Text: l.Text, URL: l.URL})
		}
		for _, id := range art.YTIDs {
			links = append(links, store.PageLink{
				Text: "🎬 YouTube " + id,
				URL:  "https://www.youtube.com/watch?v=" + id,
			})
		}
		s.PageLinks = links
		s.Page = 0 // переиспользуем поле для пагинации ссылок открытой страницы
	})

	// 1) Шапка — заголовок + текст статьи (без кликабельных <a>!), URL — code-блоком.
	h.sendArticleHeader(chatID, art)

	// 2) Картинки — альбомом до 10 (если есть).
	h.sendInlineImages(chatID, art.Images)

	// 3) Видео из <video>/<source> — пытаемся проиграть.
	for _, vurl := range art.Videos {
		h.playDirectVideo(ctx, chatID, vurl, art.Title)
	}

	// 4) YouTube эмбеды на странице — проигрываем первый автоматически,
	//    остальные оставим кнопками.
	if len(art.YTIDs) > 0 {
		h.playYouTubeByID(ctx, chatID, art.YTIDs[0], art.Title, "", "", "", "")
	}

	// 5) Финал — клавиатура с inline-ссылками и навигацией.
	h.sendOpenedPageKeyboard(chatID, string(kind))
}

func (h *Handler) sendArticleHeader(chatID int64, art *render.Article) {
	title := art.Title
	if title == "" {
		title = art.URL
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s</b>\n", escapeHTML(truncate(title, 200)))
	fmt.Fprintf(&b, "<code>%s</code>\n\n", escapeHTML(truncate(art.URL, 100)))
	if art.Description != "" {
		fmt.Fprintf(&b, "<i>%s</i>\n\n", escapeHTML(truncate(art.Description, 400)))
	}
	if art.Text != "" {
		fmt.Fprint(&b, escapeHTML(truncate(art.Text, 3000)))
	} else {
		b.WriteString("<i>Не удалось извлечь читаемый текст со страницы.</i>")
	}

	// топ-картинка отдельным сообщением (если есть)
	if art.TopImage != "" {
		photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(art.TopImage))
		photo.Caption = truncate(title, 1000)
		if _, err := h.API.Send(photo); err != nil {
			// если не получилось из URL — скачаем сами
			h.sendImageURL(chatID, art.TopImage, truncate(title, 200))
		}
	}

	msg := tgbotapi.NewMessage(chatID, b.String())
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true
	_, _ = h.API.Send(msg)
}

// sendInlineImages — слой картинок альбомами по 10.
func (h *Handler) sendInlineImages(chatID int64, images []string) {
	// убираем то, что уже было top-image (первое в списке) — лимит до 10 в группе.
	if len(images) == 0 {
		return
	}
	maxTotal := 30
	if len(images) > maxTotal {
		images = images[:maxTotal]
	}
	for start := 0; start < len(images); start += 10 {
		end := start + 10
		if end > len(images) {
			end = len(images)
		}
		var media []interface{}
		for _, u := range images[start:end] {
			media = append(media, tgbotapi.NewInputMediaPhoto(tgbotapi.FileURL(u)))
		}
		if len(media) == 1 {
			photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(images[start]))
			if _, err := h.API.Send(photo); err != nil {
				h.sendImageURL(chatID, images[start], "")
			}
			continue
		}
		mg := tgbotapi.NewMediaGroup(chatID, media)
		if _, err := h.API.SendMediaGroup(mg); err != nil {
			// фоллбек — поштучно
			for _, u := range images[start:end] {
				h.sendImageURL(chatID, u, "")
			}
		}
	}
}

// sendImageURL скачивает картинку сами и шлёт через upload (на случай если
// Telegram не может загрузить URL).
func (h *Handler) sendImageURL(chatID int64, imgURL, caption string) {
	// сначала пробуем «дешёвый» способ — Telegram сам тянет URL.
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(imgURL))
	if caption != "" {
		photo.Caption = caption
	}
	if _, err := h.API.Send(photo); err == nil {
		return
	}
	// фоллбек — скачаем и зальём как файл.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	data, _, _, err := h.HTTP.Download(ctx, imgURL, maxImageBytes)
	if err != nil || len(data) == 0 {
		h.reply(chatID, "🖼 <code>"+escapeHTML(truncate(imgURL, 100))+"</code> (не удалось загрузить)")
		return
	}
	file := tgbotapi.FileBytes{Name: "image.jpg", Bytes: data}
	up := tgbotapi.NewPhoto(chatID, file)
	if caption != "" {
		up.Caption = caption
	}
	_, _ = h.API.Send(up)
}

// sendImageDetailed — клик по картинке в результатах: показать большое фото
// + кнопку открыть страницу-источник.
func (h *Handler) sendImageDetailed(chatID int64, it store.ImageItem, kind store.SearchKind) {
	caption := truncate(it.Title, 800)
	if it.Source != "" {
		caption += "\n" + truncate(it.Source, 100)
	}
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(it.ImageURL))
	photo.Caption = caption

	// клавиатура — открыть страницу в боте.
	rows := [][]tgbotapi.InlineKeyboardButton{}
	if it.PageURL != "" {
		id := h.Store.RegisterURL(chatID, it.PageURL)
		rows = append(rows, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("🌐 Открыть страницу", "u|"+id),
		})
	}
	rows = append(rows, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("⬅️ К результатам", "b|"+string(kind)),
	})
	photo.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.API.Send(photo); err != nil {
		h.sendImageURL(chatID, it.ImageURL, caption)
	}
}

// --- Видео ---

// playYouTube — клик по номеру в результатах /vid.
func (h *Handler) playYouTube(ctx context.Context, chatID int64, it store.VideoItem, kind store.SearchKind) {
	id := it.VideoID
	if id == "" {
		id = search.ExtractYouTubeID(it.URL)
	}
	if id == "" {
		h.reply(chatID, "Не удалось определить ID видео.")
		return
	}
	h.playYouTubeByID(ctx, chatID, id, it.Title, it.Author, it.Duration, it.Thumb, kind)
}

// playYouTubeByID — общая функция: качает mp4 через Invidious и шлёт как video.
func (h *Handler) playYouTubeByID(ctx context.Context, chatID int64, id, title, author, duration, thumb string, kindOpt ...store.SearchKind) {
	kind := store.SearchKind("")
	if len(kindOpt) > 0 {
		kind = kindOpt[0]
	}

	notice := tgbotapi.NewMessage(chatID, "▶️ Готовлю видео...")
	loading, _ := h.API.Send(notice)
	defer func() {
		if loading.MessageID != 0 {
			_, _ = h.API.Request(tgbotapi.NewDeleteMessage(chatID, loading.MessageID))
		}
	}()

	info, err := h.YT.FetchInfo(ctx, id)
	if err != nil || info == nil {
		// не смогли — отдадим хотя бы превью и кнопку «открыть в YouTube как ссылку»
		h.sendVideoCardFallback(chatID, id, title, author, duration, thumb, kind)
		return
	}
	if title == "" {
		title = info.Title
	}
	if author == "" {
		author = info.Author
	}
	if thumb == "" {
		thumb = info.Thumbnail
	}

	best := info.BestPlayable(maxVideoBytes)
	if best == nil || best.URL == "" {
		h.sendVideoCardFallback(chatID, id, title, author, duration, thumb, kind)
		return
	}

	// Скачиваем mp4 в память (до 50 МБ).
	data, _, _, derr := h.HTTP.Download(ctx, best.URL, maxVideoBytes)
	if derr != nil || len(data) == 0 {
		h.sendVideoCardFallback(chatID, id, title, author, duration, thumb, kind)
		return
	}

	caption := "<b>" + escapeHTML(truncate(title, 200)) + "</b>"
	if author != "" {
		caption += "\n👤 " + escapeHTML(author)
	}
	if duration != "" {
		caption += " ⏱ " + escapeHTML(duration)
	}

	vid := tgbotapi.NewVideo(chatID, tgbotapi.FileBytes{Name: id + ".mp4", Bytes: data})
	vid.Caption = caption
	vid.ParseMode = tgbotapi.ModeHTML
	vid.SupportsStreaming = true
	if best.Width > 0 {
		vid.Width = best.Width
	}
	if best.Height > 0 {
		vid.Height = best.Height
	}
	if info.LengthSec > 0 {
		vid.Duration = info.LengthSec
	}
	if kind != "" {
		vid.ReplyMarkup = keyboard.SimpleBackKeyboard(string(kind))
	}
	if _, err := h.API.Send(vid); err != nil {
		// если Telegram не принял (например, слишком большой) — фоллбек.
		h.sendVideoCardFallback(chatID, id, title, author, duration, thumb, kind)
	}
}

// playDirectVideo — для <video> с прямой mp4-ссылкой найденной на странице.
func (h *Handler) playDirectVideo(ctx context.Context, chatID int64, vurl, title string) {
	// сначала проверим тип/размер
	final, ct, sz, err := h.HTTP.Probe(ctx, vurl)
	if err != nil {
		return
	}
	lct := strings.ToLower(ct)
	if !strings.Contains(lct, "video/") && !strings.HasSuffix(strings.ToLower(final), ".mp4") &&
		!strings.HasSuffix(strings.ToLower(final), ".webm") {
		return
	}
	if sz > 0 && sz > maxVideoBytes {
		// слишком большое — оставим как ссылку-кнопку (через sendOpenedPageKeyboard уже добавится)
		return
	}
	data, _, _, derr := h.HTTP.Download(ctx, final, maxVideoBytes)
	if derr != nil || len(data) == 0 {
		return
	}
	vid := tgbotapi.NewVideo(chatID, tgbotapi.FileBytes{Name: "video.mp4", Bytes: data})
	if title != "" {
		vid.Caption = truncate(title, 1000)
	}
	vid.SupportsStreaming = true
	_, _ = h.API.Send(vid)
}

// sendVideoCardFallback — если по какой-то причине не смогли скачать видео,
// шлём превью с кнопкой «Открыть в боте» (т.е. попробовать снова) и кнопкой
// «Открыть страницу YouTube в боте» (попадёт на парсер, но без видео).
func (h *Handler) sendVideoCardFallback(chatID int64, id, title, author, duration, thumb string, kind store.SearchKind) {
	if thumb == "" {
		thumb = "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg"
	}
	caption := "<b>" + escapeHTML(truncate(title, 200)) + "</b>\n"
	if author != "" {
		caption += "👤 " + escapeHTML(author) + "\n"
	}
	if duration != "" {
		caption += "⏱ " + escapeHTML(duration) + "\n"
	}
	caption += "\n⚠️ Не получилось скачать поток — попробуй ещё раз."

	// callback кнопка проиграть ещё раз
	playID := h.Store.RegisterURL(chatID, "https://www.youtube.com/watch?v="+id)
	rows := [][]tgbotapi.InlineKeyboardButton{
		{tgbotapi.NewInlineKeyboardButtonData("🔁 Попробовать снова", "v|"+playID)},
	}
	if kind != "" {
		rows = append(rows, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("⬅️ К результатам", "b|"+string(kind)),
		})
	}

	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(thumb))
	photo.Caption = caption
	photo.ParseMode = tgbotapi.ModeHTML
	photo.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.API.Send(photo); err != nil {
		msg := tgbotapi.NewMessage(chatID, caption)
		msg.ParseMode = tgbotapi.ModeHTML
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
		_, _ = h.API.Send(msg)
	}
}

// playVideoFromURL — обработчик callback'а v|<id>. Универсальный.
func (h *Handler) playVideoFromURL(ctx context.Context, chatID int64, rawURL string) {
	if search.IsYouTube(rawURL) {
		id := search.ExtractYouTubeID(rawURL)
		if id != "" {
			h.playYouTubeByID(ctx, chatID, id, "", "", "", "")
			return
		}
	}
	h.playDirectVideo(ctx, chatID, rawURL, "")
}

// sendAllPageImages — кнопка «Все фото (N)» под открытой страницей.
func (h *Handler) sendAllPageImages(ctx context.Context, chatID int64) {
	sess := h.Store.Get(chatID)
	if sess == nil || len(sess.PageImages) == 0 {
		h.reply(chatID, "Нет картинок на этой странице.")
		return
	}
	h.sendInlineImages(chatID, sess.PageImages)
	_ = ctx
}

// sendAllPageVideos — кнопка «Видео (N)» под открытой страницей.
func (h *Handler) sendAllPageVideos(ctx context.Context, chatID int64) {
	sess := h.Store.Get(chatID)
	if sess == nil {
		return
	}
	if len(sess.PageVideos) == 0 {
		h.reply(chatID, "Прямых видеофайлов на странице не нашлось.")
		return
	}
	for _, v := range sess.PageVideos {
		h.playDirectVideo(ctx, chatID, v, sess.PageTitle)
	}
}

// sendOpenedPageKeyboard — рендерит сообщение со ссылками внутри открытой
// страницы как inline-кнопками (НЕ переходим наружу, бот сам их открывает).
func (h *Handler) sendOpenedPageKeyboard(chatID int64, kind string) {
	sess := h.Store.Get(chatID)
	if sess == nil {
		return
	}
	if len(sess.PageLinks) == 0 && len(sess.PageImages) == 0 && len(sess.PageVideos) == 0 {
		return
	}

	// пагинация
	pg := sess.Page
	total := len(sess.PageLinks)
	start := pg * linkPagePer
	if start < 0 {
		start = 0
	}
	if start >= total && total > 0 {
		start = ((total - 1) / linkPagePer) * linkPagePer
		h.Store.Update(chatID, func(s *store.Session) { s.Page = start / linkPagePer })
	}
	end := start + linkPagePer
	if end > total {
		end = total
	}

	linkIDs := make([]string, 0, end-start)
	linkLabels := make([]string, 0, end-start)
	for _, l := range sess.PageLinks[start:end] {
		id := h.Store.RegisterURL(chatID, l.URL)
		linkIDs = append(linkIDs, id)
		label := l.Text
		if label == "" {
			label = l.URL
		}
		linkLabels = append(linkLabels, label)
	}

	// id'ы медиа (для «все фото» / «все видео» — нам собственно сами id не нужны,
	// keyboard смотрит только на длину)
	imageIDs := make([]string, len(sess.PageImages))
	videoIDs := make([]string, len(sess.PageVideos))

	kb := keyboard.OpenedPageKeyboard(kind, linkIDs, linkLabels, imageIDs, videoIDs, pg, total, linkPagePer)

	text := fmt.Sprintf("🔗 <b>Ссылки на странице</b>: %d", total)
	if total > linkPagePer {
		text += fmt.Sprintf(" (показаны %d–%d)", start+1, end)
	}
	if len(sess.PageImages) > 0 {
		text += fmt.Sprintf("\n🖼 Картинок: %d", len(sess.PageImages))
	}
	if len(sess.PageVideos) > 0 {
		text += fmt.Sprintf("\n🎥 Прямых видео: %d", len(sess.PageVideos))
	}
	text += "\n\n<i>Нажми любую кнопку — открою прямо в боте, без перехода наружу.</i>"

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true
	msg.ReplyMarkup = kb
	_, _ = h.API.Send(msg)
}

// changeOpenedPageLinks — пагинация ссылок открытой страницы.
func (h *Handler) changeOpenedPageLinks(chatID int64, delta int) {
	sess := h.Store.Get(chatID)
	if sess == nil || len(sess.PageLinks) == 0 {
		return
	}
	totalPages := (len(sess.PageLinks) + linkPagePer - 1) / linkPagePer
	target := sess.Page + delta
	if target < 0 {
		target = 0
	}
	if target >= totalPages {
		target = totalPages - 1
	}
	h.Store.Update(chatID, func(s *store.Session) { s.Page = target })
	h.sendOpenedPageKeyboard(chatID, string(store.KindWeb))
}
