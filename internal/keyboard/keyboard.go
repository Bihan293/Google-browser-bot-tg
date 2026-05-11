package keyboard

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Callback data prefixes. We keep them short to stay under Telegram's
// 64-byte callback_data limit.
const (
	CBOpen   = "o"  // open a specific result   o|<kind>|<idx>
	CBPage   = "p"  // change page              p|<kind>|<delta>
	CBKind   = "k"  // switch search kind       k|<kind>
	CBBack   = "b"  // back to results list     b|<kind>
	CBURL    = "u"  // open URL by short id     u|<id>
	CBPlay   = "v"  // play a video by short id v|<id>
	CBImg    = "i"  // send a single image      i|<id>
	CBNSFW   = "n"  // toggle 18+ mode          n|on / n|off
	CBPgLinks = "pl" // page through extracted links of opened page  pl|<delta>
	CBNoop   = "x"
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
			tgbotapi.NewKeyboardButton("🔞 18+"),
			tgbotapi.NewKeyboardButton("📡 Пинг"),
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

// OpenedPageKeyboard — клавиатура, которую бот добавляет под открытой
// страницей. Она содержит кнопки на найденные ссылки внутри страницы
// (чтобы их открывал сам бот, а не внешний браузер), кнопки на медиа,
// и навигацию назад.
func OpenedPageKeyboard(kind string, linkIDs []string, linkLabels []string,
	imageIDs []string, videoIDs []string, linkPage int, totalLinks int, perPage int) tgbotapi.InlineKeyboardMarkup {

	rows := [][]tgbotapi.InlineKeyboardButton{}

	// link buttons — show as "🔗 1", "🔗 2"...
	rowBuf := []tgbotapi.InlineKeyboardButton{}
	for i, id := range linkIDs {
		label := fmt.Sprintf("🔗 %d", i+1+linkPage*perPage)
		if i < len(linkLabels) && linkLabels[i] != "" {
			lbl := linkLabels[i]
			if len(lbl) > 28 {
				lbl = lbl[:28] + "…"
			}
			label = fmt.Sprintf("🔗 %s", lbl)
		}
		rowBuf = append(rowBuf, tgbotapi.NewInlineKeyboardButtonData(
			label,
			fmt.Sprintf("%s|%s", CBURL, id),
		))
		if len(rowBuf) == 2 {
			rows = append(rows, rowBuf)
			rowBuf = nil
		}
	}
	if len(rowBuf) > 0 {
		rows = append(rows, rowBuf)
	}

	// links pagination (if needed)
	if totalLinks > perPage {
		rows = append(rows, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("⬅️ ссылки", fmt.Sprintf("%s|-1", CBPgLinks)),
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("стр. %d", linkPage+1), fmt.Sprintf("%s||0", CBNoop)),
			tgbotapi.NewInlineKeyboardButtonData("ссылки ➡️", fmt.Sprintf("%s|1", CBPgLinks)),
		})
	}

	// media buttons
	mediaRow := []tgbotapi.InlineKeyboardButton{}
	if len(imageIDs) > 0 {
		mediaRow = append(mediaRow,
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("🖼 Все фото (%d)", len(imageIDs)),
				fmt.Sprintf("%s|all", CBImg),
			))
	}
	if len(videoIDs) > 0 {
		mediaRow = append(mediaRow,
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("🎥 Видео (%d)", len(videoIDs)),
				fmt.Sprintf("%s|all", CBPlay),
			))
	}
	if len(mediaRow) > 0 {
		rows = append(rows, mediaRow)
	}

	// back row
	if kind != "" {
		rows = append(rows, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("⬅️ К результатам", fmt.Sprintf("%s|%s", CBBack, kind)),
		})
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// SimpleBackKeyboard — клавиатура возврата к результатам.
func SimpleBackKeyboard(kind string) tgbotapi.InlineKeyboardMarkup {
	if kind == "" {
		return tgbotapi.NewInlineKeyboardMarkup()
	}
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ К результатам", fmt.Sprintf("%s|%s", CBBack, kind)),
		),
	)
}

// OpenResultKeyboard left for backward compatibility — same as SimpleBackKeyboard.
func OpenResultKeyboard(kind string) tgbotapi.InlineKeyboardMarkup {
	return SimpleBackKeyboard(kind)
}
