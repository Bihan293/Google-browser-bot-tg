package keyboard

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Callback data prefixes. We keep them short to stay under Telegram's
// 64-byte callback_data limit.
const (
	CBOpen = "o" // open a specific result   o|<kind>|<idx>
	CBPage = "p" // change page              p|<kind>|<delta>
	CBKind = "k" // switch search kind       k|<kind>
	CBBack = "b" // back to results list     b|<kind>
	CBNoop = "x"
)

// MainMenu returns the persistent reply keyboard with the main actions.
func MainMenu() tgbotapi.ReplyKeyboardMarkup {
	kb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🔎 Поиск"),
			tgbotapi.NewKeyboardButton("🖼 Картинки"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🎥 Видео"),
			tgbotapi.NewKeyboardButton("📰 Новости"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("ℹ️ Помощь"),
		),
	)
	kb.ResizeKeyboard = true
	return kb
}

// ResultsKeyboard builds an inline keyboard for a results page:
//
//	row per result (Open #N)
//	bottom row: prev / page / next + switch kind buttons
func ResultsKeyboard(kind string, page, total, perPage int) tgbotapi.InlineKeyboardMarkup {
	rows := [][]tgbotapi.InlineKeyboardButton{}

	// Open buttons in rows of 5
	rowBuf := []tgbotapi.InlineKeyboardButton{}
	for i := 0; i < total; i++ {
		btn := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("%d", i+1),
			fmt.Sprintf("%s|%s|%d", CBOpen, kind, i),
		)
		rowBuf = append(rowBuf, btn)
		if len(rowBuf) == 5 {
			rows = append(rows, rowBuf)
			rowBuf = nil
		}
	}
	if len(rowBuf) > 0 {
		rows = append(rows, rowBuf)
	}

	// Pagination row
	rows = append(rows, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("⬅️", fmt.Sprintf("%s|%s|-1", CBPage, kind)),
		tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("стр. %d", page+1), fmt.Sprintf("%s||0", CBNoop)),
		tgbotapi.NewInlineKeyboardButtonData("➡️", fmt.Sprintf("%s|%s|1", CBPage, kind)),
	})

	// Kind switcher
	rows = append(rows, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("🌐 Web", fmt.Sprintf("%s|web", CBKind)),
		tgbotapi.NewInlineKeyboardButtonData("🖼 Img", fmt.Sprintf("%s|img", CBKind)),
		tgbotapi.NewInlineKeyboardButtonData("🎥 Vid", fmt.Sprintf("%s|vid", CBKind)),
		tgbotapi.NewInlineKeyboardButtonData("📰 News", fmt.Sprintf("%s|news", CBKind)),
	})
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// OpenResultKeyboard is the keyboard shown when a single result is opened.
func OpenResultKeyboard(kind string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ К результатам", fmt.Sprintf("%s|%s", CBBack, kind)),
		),
	)
}
