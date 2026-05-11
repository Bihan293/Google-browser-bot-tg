package handlers

import (
	"context"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/genspark/tg-browser-bot/internal/keyboard"
	"github.com/genspark/tg-browser-bot/internal/store"
)

// runSearch performs a search of the given kind, caches the first page,
// and sends results to the chat.
func (h *Handler) runSearch(ctx context.Context, chatID int64, query string, kind store.SearchKind) {
	notice := tgbotapi.NewMessage(chatID, "🔍 Ищу: <code>"+escapeHTML(query)+"</code>...")
	notice.ParseMode = tgbotapi.ModeHTML
	noticeMsg, _ := h.API.Send(notice)

	sess := h.Store.Update(chatID, func(s *store.Session) {
		s.Query = query
		s.Kind = kind
		s.Page = 0
		s.WebResults = nil
		s.ImageResults = nil
		s.VideoResults = nil
		s.NewsResults = nil
	})

	if err := h.loadPage(ctx, sess, kind, 0); err != nil {
		if noticeMsg.MessageID != 0 {
			_, _ = h.API.Request(tgbotapi.NewDeleteMessage(chatID, noticeMsg.MessageID))
		}
		h.sendError(chatID, err)
		return
	}

	if noticeMsg.MessageID != 0 {
		_, _ = h.API.Request(tgbotapi.NewDeleteMessage(chatID, noticeMsg.MessageID))
	}
	h.showCurrentPage(ctx, chatID, kind)
}

// changePage moves +/- one page and re-renders.
func (h *Handler) changePage(ctx context.Context, chatID int64, kind store.SearchKind, delta int) {
	sess := h.Store.Get(chatID)
	if sess == nil || sess.Query == "" {
		h.reply(chatID, "Сначала отправь поисковый запрос.")
		return
	}
	target := sess.Page + delta
	if target < 0 {
		target = 0
	}
	if err := h.loadPage(ctx, sess, kind, target); err != nil {
		h.sendError(chatID, err)
		return
	}
	h.Store.Update(chatID, func(s *store.Session) {
		s.Page = target
		s.Kind = kind
	})
	h.showCurrentPage(ctx, chatID, kind)
}

// loadPage makes sure session has results for the requested page index.
// If not, performs the search with a larger limit to fill the cache.
func (h *Handler) loadPage(ctx context.Context, sess *store.Session, kind store.SearchKind, page int) error {
	needed := (page + 1) * perPage

	switch kind {
	case store.KindWeb:
		have := flatLenWeb(sess.WebResults)
		if have >= needed {
			return nil
		}
		res, err := h.Web.Search(ctx, sess.Query, needed+perPage)
		if err != nil {
			return err
		}
		sess.WebResults = chunkWeb(res, perPage)
	case store.KindImages:
		have := flatLenImg(sess.ImageResults)
		if have >= needed {
			return nil
		}
		res, err := h.Images.Search(ctx, sess.Query, needed+perPage)
		if err != nil {
			return err
		}
		sess.ImageResults = chunkImg(res, perPage)
	case store.KindVideos:
		have := flatLenVid(sess.VideoResults)
		if have >= needed {
			return nil
		}
		res, err := h.Videos.Search(ctx, sess.Query, needed+perPage)
		if err != nil {
			return err
		}
		sess.VideoResults = chunkVid(res, perPage)
	case store.KindNews:
		have := flatLenNews(sess.NewsResults)
		if have >= needed {
			return nil
		}
		res, err := h.News.Search(ctx, sess.Query, needed+perPage)
		if err != nil {
			return err
		}
		sess.NewsResults = chunkNews(res, perPage)
	}
	return nil
}

// showCurrentPage renders the current page of the session for the given kind.
func (h *Handler) showCurrentPage(ctx context.Context, chatID int64, kind store.SearchKind) {
	sess := h.Store.Get(chatID)
	if sess == nil {
		h.reply(chatID, "Нет активного поиска. Пришли запрос текстом.")
		return
	}

	switch kind {
	case store.KindWeb:
		h.renderWebPage(chatID, sess)
	case store.KindImages:
		h.renderImagesPage(ctx, chatID, sess)
	case store.KindVideos:
		h.renderVideosPage(chatID, sess)
	case store.KindNews:
		h.renderNewsPage(chatID, sess)
	}
}

func (h *Handler) renderWebPage(chatID int64, sess *store.Session) {
	items := pageOrEmpty(sess.WebResults, sess.Page)
	if len(items) == 0 {
		h.reply(chatID, "Ничего не найдено по запросу <code>"+escapeHTML(sess.Query)+"</code>.")
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>🌐 Web:</b> <code>%s</code>\n\n", escapeHTML(sess.Query))
	for i, it := range items {
		fmt.Fprintf(&b, "<b>%d.</b> <a href=\"%s\">%s</a>\n", i+1, escapeAttr(it.URL), escapeHTML(truncate(it.Title, 120)))
		if it.Snippet != "" {
			fmt.Fprintf(&b, "<i>%s</i>\n", escapeHTML(truncate(it.Snippet, 220)))
		}
		fmt.Fprintf(&b, "<code>%s</code>\n\n", escapeHTML(truncate(it.URL, 80)))
	}
	msg := tgbotapi.NewMessage(chatID, b.String())
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true
	msg.ReplyMarkup = keyboard.ResultsKeyboard(string(store.KindWeb), sess.Page, len(items), perPage)
	_, _ = h.API.Send(msg)
}

func (h *Handler) renderImagesPage(ctx context.Context, chatID int64, sess *store.Session) {
	items := pageOrEmptyImg(sess.ImageResults, sess.Page)
	if len(items) == 0 {
		h.reply(chatID, "Картинок не найдено по запросу <code>"+escapeHTML(sess.Query)+"</code>.")
		return
	}

	// Send as media group (album) – max 10 per group.
	var media []interface{}
	max := len(items)
	if max > 10 {
		max = 10
	}
	for i := 0; i < max; i++ {
		it := items[i]
		photo := tgbotapi.NewInputMediaPhoto(tgbotapi.FileURL(it.ImageURL))
		caption := fmt.Sprintf("%d. %s", i+1, truncate(it.Title, 80))
		if it.Source != "" {
			caption += " — " + truncate(it.Source, 40)
		}
		photo.Caption = caption
		media = append(media, photo)
	}
	mg := tgbotapi.NewMediaGroup(chatID, media)
	if _, err := h.API.SendMediaGroup(mg); err != nil {
		// Fallback: send a text list with links if media group fails.
		var b strings.Builder
		fmt.Fprintf(&b, "<b>🖼 Картинки:</b> <code>%s</code>\n\n", escapeHTML(sess.Query))
		for i, it := range items {
			fmt.Fprintf(&b, "<b>%d.</b> <a href=\"%s\">%s</a>\n", i+1, escapeAttr(it.ImageURL), escapeHTML(truncate(it.Title, 100)))
		}
		h.reply(chatID, b.String())
	}

	// follow-up message with controls
	ctrl := tgbotapi.NewMessage(chatID, fmt.Sprintf("<b>🖼 Картинки:</b> <code>%s</code> — стр. %d", escapeHTML(sess.Query), sess.Page+1))
	ctrl.ParseMode = tgbotapi.ModeHTML
	ctrl.ReplyMarkup = keyboard.ResultsKeyboard(string(store.KindImages), sess.Page, len(items), perPage)
	_, _ = h.API.Send(ctrl)
	_ = ctx
}

func (h *Handler) renderVideosPage(chatID int64, sess *store.Session) {
	items := pageOrEmptyVid(sess.VideoResults, sess.Page)
	if len(items) == 0 {
		h.reply(chatID, "Видео не найдено по запросу <code>"+escapeHTML(sess.Query)+"</code>.")
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>🎥 Видео:</b> <code>%s</code>\n\n", escapeHTML(sess.Query))
	for i, it := range items {
		fmt.Fprintf(&b, "<b>%d.</b> <a href=\"%s\">%s</a>\n", i+1, escapeAttr(it.URL), escapeHTML(truncate(it.Title, 120)))
		meta := []string{}
		if it.Author != "" {
			meta = append(meta, escapeHTML(it.Author))
		}
		if it.Duration != "" {
			meta = append(meta, escapeHTML(it.Duration))
		}
		if len(meta) > 0 {
			fmt.Fprintf(&b, "<i>%s</i>\n", strings.Join(meta, " • "))
		}
		b.WriteString("\n")
	}
	msg := tgbotapi.NewMessage(chatID, b.String())
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true
	msg.ReplyMarkup = keyboard.ResultsKeyboard(string(store.KindVideos), sess.Page, len(items), perPage)
	_, _ = h.API.Send(msg)
}

func (h *Handler) renderNewsPage(chatID int64, sess *store.Session) {
	items := pageOrEmptyNews(sess.NewsResults, sess.Page)
	if len(items) == 0 {
		h.reply(chatID, "Новостей не найдено по запросу <code>"+escapeHTML(sess.Query)+"</code>.")
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<b>📰 Новости:</b> <code>%s</code>\n\n", escapeHTML(sess.Query))
	for i, it := range items {
		fmt.Fprintf(&b, "<b>%d.</b> <a href=\"%s\">%s</a>\n", i+1, escapeAttr(it.URL), escapeHTML(truncate(it.Title, 140)))
		if it.Source != "" || it.Date != "" {
			line := strings.TrimSpace(it.Source + " • " + it.Date)
			fmt.Fprintf(&b, "<i>%s</i>\n", escapeHTML(line))
		}
		if it.Snippet != "" {
			fmt.Fprintf(&b, "%s\n", escapeHTML(truncate(it.Snippet, 220)))
		}
		b.WriteString("\n")
	}
	msg := tgbotapi.NewMessage(chatID, b.String())
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true
	msg.ReplyMarkup = keyboard.ResultsKeyboard(string(store.KindNews), sess.Page, len(items), perPage)
	_, _ = h.API.Send(msg)
}
