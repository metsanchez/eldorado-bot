package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"eldorado-bot/internal/config"
	"eldorado-bot/internal/eldorado"
	"eldorado-bot/internal/logger"
	"eldorado-bot/internal/logic"
	"eldorado-bot/internal/storage"
	"eldorado-bot/internal/telegram"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		cancel()
	}()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config load error: %v", err)
	}

	logg := logger.New(os.Stdout)

	st, err := storage.NewJSONStorage("storage.json")
	if err != nil {
		logg.Errorf("storage init error: %v", err)
		os.Exit(1)
	}

	eldoradoClient := eldorado.NewClient(cfg.EldoradoBaseURL, cfg.EldoradoEmail, cfg.EldoradoPassword, cfg.EldoradoCookies, cfg.EldoradoXSRFToken, logg)

	telegramClient := telegram.NewClient(cfg.TelegramBotToken, cfg.TelegramChatID, logg)

	runner := logic.NewRunner(
		logg,
		cfg,
		eldoradoClient,
		telegramClient,
		st,
	)

	logg.Infof("starting Eldorado seller bot")

	if err := runner.Start(ctx); err != nil {
		logg.Errorf("runner exited with error: %v", err)
		time.Sleep(2 * time.Second)
		os.Exit(1)
	}

	logg.Infof("Eldorado seller bot stopped")
}
