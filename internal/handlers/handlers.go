package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/genspark/tg-browser-bot/internal/keyboard"
	"github.com/genspark/tg-browser-bot/internal/render"
	"github.com/genspark/tg-browser-bot/internal/search"
	"github.com/genspark/tg-browser-bot/internal/store"
)

const perPage = 8
const linkPagePer = 10 // ссылок-кнопок под открытой страницей на одну страницу

// Handler wires the bot API together with the searchers and session store.
type Handler struct {
	API    *tgbotapi.BotAPI
	Web    *search.WebSearcher
	Images *search.ImageSearcher
	Videos *search.VideoSearcher
	News   *search.NewsSearcher
	YT     *search.YouTubeFetcher
	HTTP   *search.HTTPClient
	Page   *render.PageFetcher
	Store  *store.Store
}

// New creates a new Handler.
func New(api *tgbotapi.BotAPI, w *search.WebSearcher, i *search.ImageSearcher,
	v *search.VideoSearcher, n *search.NewsSearcher, yt *search.YouTubeFetcher,
	httpClient *search.HTTPClient, p *render.PageFetcher, s *store.Store) *Handler {
	return &Handler{
		API: api, Web: w, Images: i, Videos: v, News: n,
		YT: yt, HTTP: httpClient, Page: p, Store: s,
	}
}

// HandleUpdate is the entry point invoked by the webhook server for every
// incoming update.
func (h *Handler) HandleUpdate(ctx context.Context, u tgbotapi.Update) {
	switch {
	case u.CallbackQuery != nil:
		h.handleCallback(ctx, u.CallbackQuery)
	case u.Message != nil:
		h.handleMessage(ctx, u.Message)
	}
}

// ---------------- messages ----------------

func (h *Handler) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	if msg.IsCommand() {
		h.handleCommand(ctx, msg)
		return
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	// Map reply-keyboard buttons to commands.
	switch text {
	case "🔎 Поиск":
		h.reply(msg.Chat.ID, "Напиши что искать в вебе. Пример: <code>лучшие книги по Go</code>")
		h.setMode(msg.Chat.ID, store.KindWeb)
		return
	case "🖼 Картинки":
		h.reply(msg.Chat.ID, "Напиши что найти в картинках. Пример: <code>закат над морем</code>")
		h.setMode(msg.Chat.ID, store.KindImages)
		return
	case "🎥 Видео":
		h.reply(msg.Chat.ID, "Напиши что найти в видео. Пример: <code>обзор iphone</code>")
		h.setMode(msg.Chat.ID, store.KindVideos)
		return
	case "📰 Новости":
		h.reply(msg.Chat.ID, "Напиши тему новостей. Пример: <code>искусственный интеллект</code>")
		h.setMode(msg.Chat.ID, store.KindNews)
		return
	case "🔞 18+":
		h.toggleNSFW(msg.Chat.ID)
		return
	case "ℹ️ Помощь":
		h.sendHelp(msg.Chat.ID)
		return
	}

	// If user sends a URL directly — open the page inside the bot.
	if isLikelyURL(text) {
		h.openURL(ctx, msg.Chat.ID, ensureScheme(text), store.KindWeb)
		return
	}

	// Otherwise treat as a search query for the current mode (defaults to web).
	sess := h.Store.Get(msg.Chat.ID)
	kind := store.KindWeb
	if sess != nil && sess.Kind != "" {
		kind = sess.Kind
	}
	h.runSearch(ctx, msg.Chat.ID, text, kind)
}

func (h *Handler) handleCommand(ctx context.Context, msg *tgbotapi.Message) {
	cmd := msg.Command()
	arg := strings.TrimSpace(msg.CommandArguments())
	switch cmd {
	case "start":
		h.sendStart(msg.Chat.ID, msg.From.FirstName)
	case "help":
		h.sendHelp(msg.Chat.ID)
	case "search", "s", "web":
		if arg == "" {
			h.reply(msg.Chat.ID, "Использование: <code>/search запрос</code>")
			return
		}
		h.runSearch(ctx, msg.Chat.ID, arg, store.KindWeb)
	case "img", "image", "images":
		if arg == "" {
			h.reply(msg.Chat.ID, "Использование: <code>/img запрос</code>")
			return
		}
		h.runSearch(ctx, msg.Chat.ID, arg, store.KindImages)
	case "vid", "video", "videos":
		if arg == "" {
			h.reply(msg.Chat.ID, "Использование: <code>/vid запрос</code>")
			return
		}
		h.runSearch(ctx, msg.Chat.ID, arg, store.KindVideos)
	case "news":
		if arg == "" {
			h.reply(msg.Chat.ID, "Использование: <code>/news тема</code>")
			return
		}
		h.runSearch(ctx, msg.Chat.ID, arg, store.KindNews)
	case "open":
		if arg == "" {
			h.reply(msg.Chat.ID, "Использование: <code>/open https://...</code>")
			return
		}
		h.openURL(ctx, msg.Chat.ID, ensureScheme(arg), store.KindWeb)
	case "nsfw", "adult", "18":
		h.toggleNSFW(msg.Chat.ID)
	default:
		h.reply(msg.Chat.ID, "Неизвестная команда. /help — список команд.")
	}
}

// ---------------- callbacks ----------------

func (h *Handler) handleCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	defer func() {
		_, _ = h.API.Request(tgbotapi.NewCallback(cb.ID, ""))
	}()

	parts := strings.Split(cb.Data, "|")
	if len(parts) == 0 {
		return
	}
	chatID := cb.Message.Chat.ID

	switch parts[0] {
	case keyboard.CBNoop:
		return

	case keyboard.CBKind:
		if len(parts) < 2 {
			return
		}
		newKind := store.SearchKind(parts[1])
		sess := h.Store.Get(chatID)
		if sess == nil || sess.Query == "" {
			h.reply(chatID, "Сначала пришли запрос для поиска.")
			return
		}
		h.runSearch(ctx, chatID, sess.Query, newKind)

	case keyboard.CBPage:
		if len(parts) < 3 {
			return
		}
		delta, _ := strconv.Atoi(parts[2])
		h.changePage(ctx, chatID, store.SearchKind(parts[1]), delta)

	case keyboard.CBOpen:
		if len(parts) < 3 {
			return
		}
		idx, _ := strconv.Atoi(parts[2])
		h.openResult(ctx, chatID, store.SearchKind(parts[1]), idx)

	case keyboard.CBBack:
		if len(parts) < 2 {
			return
		}
		h.showCurrentPage(ctx, chatID, store.SearchKind(parts[1]))

	case keyboard.CBURL:
		if len(parts) < 2 {
			return
		}
		raw := h.Store.ResolveURL(chatID, parts[1])
		if raw == "" {
			h.reply(chatID, "Ссылка устарела — повтори действие.")
			return
		}
		h.openURL(ctx, chatID, raw, store.KindWeb)

	case keyboard.CBPlay:
		if len(parts) < 2 {
			return
		}
		if parts[1] == "all" {
			h.sendAllPageVideos(ctx, chatID)
			return
		}
		raw := h.Store.ResolveURL(chatID, parts[1])
		if raw == "" {
			h.reply(chatID, "Видео устарело — попробуй ещё раз.")
			return
		}
		h.playVideoFromURL(ctx, chatID, raw)

	case keyboard.CBImg:
		if len(parts) < 2 {
			return
		}
		if parts[1] == "all" {
			h.sendAllPageImages(ctx, chatID)
			return
		}
		raw := h.Store.ResolveURL(chatID, parts[1])
		if raw == "" {
			h.reply(chatID, "Картинка устарела — попробуй ещё раз.")
			return
		}
		h.sendImageURL(chatID, raw, "")

	case keyboard.CBPgLinks:
		if len(parts) < 2 {
			return
		}
		delta, _ := strconv.Atoi(parts[1])
		h.changeOpenedPageLinks(chatID, delta)

	case keyboard.CBNSFW:
		h.toggleNSFW(chatID)
	}
}

// ---------------- helpers ----------------

func (h *Handler) setMode(chatID int64, kind store.SearchKind) {
	h.Store.Update(chatID, func(s *store.Session) { s.Kind = kind })
}

func (h *Handler) toggleNSFW(chatID int64) {
	sess := h.Store.Update(chatID, func(s *store.Session) { s.NSFW = !s.NSFW })
	state := "выключен"
	if sess.NSFW {
		state = "включён ✅"
	}
	h.reply(chatID, fmt.Sprintf("🔞 Режим <b>18+</b> %s. SafeSearch у DuckDuckGo и YouTube переключён.", state))
}

func (h *Handler) sendStart(chatID int64, name string) {
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(
		"<b>Привет, %s!</b>\n\nЯ — Telegram-браузер. Ищу <b>сайты, картинки, видео и новости</b> и показываю их прямо здесь, без перехода наружу.\n\n"+
			"• Пришли запрос текстом или используй кнопки ниже.\n"+
			"• Пришли ссылку — открою страницу прямо тут, со всеми картинками и видео.\n"+
			"• Видео из YouTube проигрываю в чате.\n"+
			"• Ссылки в результатах — это <b>кнопки</b>: жми, и я открою страницу <i>внутри</i> бота.\n"+
			"• Кнопка <b>🔞 18+</b> — переключить SafeSearch.\n\n"+
			"Команды: /help",
		escapeHTML(name),
	))
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard.MainMenu()
	_, _ = h.API.Send(msg)
}

func (h *Handler) sendHelp(chatID int64) {
	text := "<b>Команды:</b>\n" +
		"/search <i>запрос</i> — поиск по сайтам\n" +
		"/img <i>запрос</i> — поиск картинок\n" +
		"/vid <i>запрос</i> — поиск видео (проигрываю в чате)\n" +
		"/news <i>тема</i> — поиск новостей\n" +
		"/open <i>url</i> — открыть страницу внутри бота\n" +
		"/nsfw — переключить режим 18+\n\n" +
		"Можно просто прислать текст — бот найдёт по последнему режиму. " +
		"Пришли ссылку — бот покажет содержимое страницы здесь же: текст, картинки, видео, " +
		"а все внутренние ссылки превратятся в кнопки, которые бот открывает сам."
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard.MainMenu()
	_, _ = h.API.Send(msg)
}

func (h *Handler) reply(chatID int64, text string) {
	m := tgbotapi.NewMessage(chatID, text)
	m.ParseMode = tgbotapi.ModeHTML
	m.DisableWebPagePreview = true
	_, _ = h.API.Send(m)
}

func (h *Handler) sendError(chatID int64, err error) {
	h.reply(chatID, "⚠️ Ошибка: <code>"+escapeHTML(err.Error())+"</code>")
}

// ensureScheme — гарантирует, что URL начинается с http(s)://.
func ensureScheme(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return u
	}
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	return "https://" + u
}
