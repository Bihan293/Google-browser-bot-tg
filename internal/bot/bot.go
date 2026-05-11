package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/genspark/tg-browser-bot/internal/config"
	"github.com/genspark/tg-browser-bot/internal/handlers"
	"github.com/genspark/tg-browser-bot/internal/render"
	"github.com/genspark/tg-browser-bot/internal/search"
	"github.com/genspark/tg-browser-bot/internal/store"
)

// Bot wires everything together: the Telegram API client, the webhook
// HTTP server, the handler and all search providers.
type Bot struct {
	cfg     *config.Config
	api     *tgbotapi.BotAPI
	handler *handlers.Handler
	srv     *http.Server
}

// New constructs a fully wired Bot.
func New(cfg *config.Config) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, fmt.Errorf("init telegram bot: %w", err)
	}
	api.Debug = cfg.Debug
	log.Printf("authorized on telegram account: @%s", api.Self.UserName)

	httpClient := search.NewHTTPClient(20 * time.Second)
	pageFetcher := render.NewPageFetcher(func(ctx context.Context, u string) (string, error) {
		body, _, err := httpClient.Get(ctx, u)
		return body, err
	})

	sessions := store.New(30 * time.Minute)
	h := handlers.New(
		api,
		search.NewWebSearcher(httpClient),
		search.NewImageSearcher(httpClient),
		search.NewVideoSearcher(httpClient),
		search.NewNewsSearcher(httpClient),
		search.NewYouTubeFetcher(httpClient),
		httpClient,
		pageFetcher,
		sessions,
	)

	b := &Bot{cfg: cfg, api: api, handler: h}
	return b, nil
}

// Run starts the webhook server. If WEBHOOK_URL is configured it also
// registers the webhook with Telegram. The function blocks until ctx is
// cancelled or the server errors out.
func (b *Bot) Run(ctx context.Context) error {
	if err := b.registerWebhook(); err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc(b.cfg.WebhookPath, b.webhookHandler)
	healthHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"status":"ok","bot":"`+b.api.Self.UserName+`","ts":"`+time.Now().UTC().Format(time.RFC3339)+`"}`)
	}
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "pong")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "tg-browser-bot is running. Use Telegram to talk to @"+b.api.Self.UserName)
	})

	// Make the handler aware of our own base URL so /ping can verify it.
	b.handler.SelfURL = b.cfg.WebhookURL

	b.srv = &http.Server{
		Addr:         ":" + b.cfg.Port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  180 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("listening on :%s (webhook path %s)", b.cfg.Port, b.cfg.WebhookPath)
		if err := b.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Println("shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return b.srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func (b *Bot) registerWebhook() error {
	full := b.cfg.FullWebhookURL()
	if full == "" {
		log.Println("WEBHOOK_URL not set — server will accept POST updates but Telegram is not configured")
		return nil
	}
	wh, err := tgbotapi.NewWebhook(full)
	if err != nil {
		return fmt.Errorf("build webhook: %w", err)
	}
	if _, err := b.api.Request(wh); err != nil {
		return fmt.Errorf("set webhook: %w", err)
	}
	info, err := b.api.GetWebhookInfo()
	if err != nil {
		return fmt.Errorf("get webhook info: %w", err)
	}
	if info.LastErrorDate != 0 {
		log.Printf("telegram webhook last error: %s", info.LastErrorMessage)
	}
	log.Printf("webhook registered: %s", full)
	return nil
}

func (b *Bot) webhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if b.cfg.WebhookSecret != "" {
		if r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != b.cfg.WebhookSecret {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 2*1024*1024))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	var update tgbotapi.Update
	if err := json.Unmarshal(body, &update); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	go func(u tgbotapi.Update) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic in handler: %v", rec)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()
		b.handler.HandleUpdate(ctx, u)
	}(update)

	w.WriteHeader(http.StatusOK)
}
