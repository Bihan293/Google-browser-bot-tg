package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/genspark/tg-browser-bot/internal/bot"
	"github.com/genspark/tg-browser-bot/internal/config"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	b, err := bot.New(cfg)
	if err != nil {
		log.Fatalf("bot init error: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := b.Run(ctx); err != nil {
		log.Fatalf("bot stopped with error: %v", err)
	}
	log.Println("bye")
}
