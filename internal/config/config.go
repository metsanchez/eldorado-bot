package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	EldoradoBaseURL   string
	EldoradoEmail     string
	EldoradoPassword  string
	EldoradoCookies   string
	EldoradoXSRFToken string

	ValorantGameID string

	PollIntervalOpenOrders  time.Duration
	PollIntervalOrderStatus time.Duration

	TelegramBotToken string
	TelegramChatID   int64

	MinOfferPrice float64
	MaxOfferPrice float64
	OfferMessage  string
	DeliveryTime  string

	BuyerAutoMessage string
	BuyerAutoImage   string
}

func Load() (*Config, error) {
	cfg := &Config{}

	cfg.EldoradoBaseURL = getEnvOrDefault("ELDORADO_BASE_URL", "https://www.eldorado.gg")
	cfg.EldoradoEmail = getEnvOrDefault("ELDORADO_EMAIL", "")
	cfg.EldoradoPassword = getEnvOrDefault("ELDORADO_PASSWORD", "")
	cfg.EldoradoCookies = getEnvOrDefault("ELDORADO_COOKIES", "")
	cfg.EldoradoXSRFToken = getEnvOrDefault("ELDORADO_XSRF_TOKEN", "")

	if cfg.EldoradoCookies == "" && (cfg.EldoradoEmail == "" || cfg.EldoradoPassword == "") {
		return nil, fmt.Errorf("either ELDORADO_COOKIES or both ELDORADO_EMAIL+ELDORADO_PASSWORD must be set")
	}

	cfg.ValorantGameID = getEnvOrDefault("VALORANT_GAME_ID", "")

	cfg.PollIntervalOpenOrders = getDurationOrDefault("POLL_INTERVAL_OPEN_ORDERS", 30*time.Second)
	cfg.PollIntervalOrderStatus = getDurationOrDefault("POLL_INTERVAL_ORDER_STATUS", 30*time.Second)

	cfg.TelegramBotToken = getEnvOrDefault("TELEGRAM_BOT_TOKEN", "")
	chatIDStr := getEnvOrDefault("TELEGRAM_CHAT_ID", "0")
	if cfg.TelegramBotToken != "" {
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil || chatID == 0 {
			return nil, fmt.Errorf("TELEGRAM_CHAT_ID must be a valid int64 when TELEGRAM_BOT_TOKEN is set")
		}
		cfg.TelegramChatID = chatID
	}

	cfg.MinOfferPrice = getFloatOrDefault("MIN_OFFER_PRICE", 0)
	cfg.MaxOfferPrice = getFloatOrDefault("MAX_OFFER_PRICE", 0)
	cfg.OfferMessage = getEnvOrDefault("OFFER_MESSAGE", "Merhaba, hızlı ve güvenli boost yapabilirim.")
	cfg.DeliveryTime = getEnvOrDefault("DELIVERY_TIME", "Hour1")

	cfg.BuyerAutoMessage = strings.ReplaceAll(getEnvOrDefault("BUYER_AUTO_MESSAGE", ""), `\n`, "\n")
	cfg.BuyerAutoImage = getEnvOrDefault("BUYER_AUTO_IMAGE", "")

	return cfg, nil
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getDurationOrDefault(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return def
}

func getFloatOrDefault(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return f
		}
	}
	return def
}
