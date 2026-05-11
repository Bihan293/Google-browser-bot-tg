package handlers

import (
	"context"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/genspark/tg-browser-bot/internal/keyboard"
	"github.com/genspark/tg-browser-bot/internal/store"
)

// openResult reacts to a tap on a numeric button next to a result entry.
// Depending on the result kind it shows the article text, the picture,
// or a YouTube video preview — always inside the chat, no external redirect.
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
		h.sendImage(chatID, items[idx], kind)

	case store.KindVideos:
		items := pageOrEmptyVid(sess.VideoResults, sess.Page)
		if idx < 0 || idx >= len(items) {
			h.reply(chatID, "Нет такого результата.")
			return
		}
		h.sendVideoCard(chatID, items[idx], kind)

	case store.KindNews:
		items := pageOrEmptyNews(sess.NewsResults, sess.Page)
		if idx < 0 || idx >= len(items) {
			h.reply(chatID, "Нет такого результата.")
			return
		}
		h.openURL(ctx, chatID, items[idx].URL, kind)
	}
}

// openURL fetches the page and renders its content in-chat.
func (h *Handler) openURL(ctx context.Context, chatID int64, rawURL string, kind store.SearchKind) {
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

	title := art.Title
	if title == "" {
		title = rawURL
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s</b>\n", escapeHTML(truncate(title, 200)))
	fmt.Fprintf(&b, "<code>%s</code>\n\n", escapeHTML(truncate(art.URL, 100)))
	if art.Description != "" {
		fmt.Fprintf(&b, "<i>%s</i>\n\n", escapeHTML(truncate(art.Description, 400)))
	}
	if art.Text != "" {
		// Telegram message size is 4096 chars — keep some headroom.
		fmt.Fprint(&b, escapeHTML(truncate(art.Text, 3200)))
	} else {
		b.WriteString("<i>Не удалось извлечь читаемый текст со страницы.</i>")
	}

	// If the article has a top image, try sending it first.
	if art.TopImage != "" {
		photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(art.TopImage))
		photo.Caption = truncate(title, 1000)
		_, _ = h.API.Send(photo)
	}

	msg := tgbotapi.NewMessage(chatID, b.String())
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true
	msg.ReplyMarkup = keyboard.OpenResultKeyboard(string(kind))
	_, _ = h.API.Send(msg)
}

func (h *Handler) sendImage(chatID int64, it store.ImageItem, kind store.SearchKind) {
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(it.ImageURL))
	caption := truncate(it.Title, 800)
	if it.Source != "" {
		caption += "\n" + truncate(it.Source, 100)
	}
	photo.Caption = caption
	photo.ReplyMarkup = keyboard.OpenResultKeyboard(string(kind))
	if _, err := h.API.Send(photo); err != nil {
		// fallback to link
		h.reply(chatID, "<a href=\""+escapeAttr(it.ImageURL)+"\">"+escapeHTML(it.Title)+"</a>")
	}
}

func (h *Handler) sendVideoCard(chatID int64, it store.VideoItem, kind store.SearchKind) {
	caption := "<b>" + escapeHTML(it.Title) + "</b>\n"
	if it.Author != "" {
		caption += "👤 " + escapeHTML(it.Author) + "\n"
	}
	if it.Duration != "" {
		caption += "⏱ " + escapeHTML(it.Duration) + "\n"
	}
	caption += "\n<a href=\"" + escapeAttr(it.URL) + "\">▶️ Открыть в YouTube</a>"

	if it.Thumb != "" {
		photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(it.Thumb))
		photo.Caption = caption
		photo.ParseMode = tgbotapi.ModeHTML
		photo.ReplyMarkup = keyboard.OpenResultKeyboard(string(kind))
		if _, err := h.API.Send(photo); err == nil {
			return
		}
	}
	msg := tgbotapi.NewMessage(chatID, caption)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard.OpenResultKeyboard(string(kind))
	_, _ = h.API.Send(msg)
}
