package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration for the bot.
type Config struct {
	BotToken      string
	WebhookURL    string // e.g. https://example.com
	WebhookPath   string // e.g. /tg/webhook
	WebhookSecret string
	Port          string
	Debug         bool
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		BotToken:      strings.TrimSpace(os.Getenv("BOT_TOKEN")),
		WebhookURL:    strings.TrimRight(strings.TrimSpace(os.Getenv("WEBHOOK_URL")), "/"),
		WebhookPath:   strings.TrimSpace(os.Getenv("WEBHOOK_PATH")),
		WebhookSecret: strings.TrimSpace(os.Getenv("WEBHOOK_SECRET")),
		Port:          strings.TrimSpace(os.Getenv("PORT")),
		Debug:         parseBool(os.Getenv("DEBUG"), false),
	}

	if cfg.BotToken == "" {
		return nil, errors.New("BOT_TOKEN is required")
	}
	if cfg.WebhookPath == "" {
		cfg.WebhookPath = "/tg/webhook"
	}
	if !strings.HasPrefix(cfg.WebhookPath, "/") {
		cfg.WebhookPath = "/" + cfg.WebhookPath
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if _, err := strconv.Atoi(cfg.Port); err != nil {
		return nil, errors.New("PORT must be a number")
	}
	return cfg, nil
}

// FullWebhookURL returns WebhookURL + WebhookPath.
// May be empty if WebhookURL is not set (useful for local debugging).
func (c *Config) FullWebhookURL() string {
	if c.WebhookURL == "" {
		return ""
	}
	return c.WebhookURL + c.WebhookPath
}

func parseBool(v string, def bool) bool {
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
