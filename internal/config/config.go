package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	TelegramBotToken      string
	TelegramChatIDs       []string
	TelegramWebhookURL    string
	TelegramWebhookSecret string
	GeoIPDBPath           string
	ServerAddr            string
	RateLimitRPS          float64
	RateLimitBurst        int
}

func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		TelegramBotToken:      strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		TelegramWebhookURL:    strings.TrimSpace(os.Getenv("TELEGRAM_WEBHOOK_URL")),
		TelegramWebhookSecret: strings.TrimSpace(os.Getenv("TELEGRAM_WEBHOOK_SECRET")),
		GeoIPDBPath:           strings.TrimSpace(os.Getenv("GEOIP_DB_PATH")),
		ServerAddr:            strings.TrimSpace(os.Getenv("SERVER_ADDR")),
		RateLimitRPS:          1,
		RateLimitBurst:        5,
	}

	chatIDs := strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_IDS"))
	if chatIDs != "" {
		parts := strings.Split(chatIDs, ",")
		for _, part := range parts {
			id := strings.TrimSpace(part)
			if id != "" {
				cfg.TelegramChatIDs = append(cfg.TelegramChatIDs, id)
			}
		}
	}

	if cfg.ServerAddr == "" {
		cfg.ServerAddr = ":8080"
	}
	if cfg.GeoIPDBPath == "" {
		cfg.GeoIPDBPath = "GeoLite2-City.mmdb"
	}

	if v := strings.TrimSpace(os.Getenv("RATE_LIMIT_RPS")); v != "" {
		rps, err := strconv.ParseFloat(v, 64)
		if err != nil || rps <= 0 {
			return Config{}, fmt.Errorf("invalid RATE_LIMIT_RPS")
		}
		cfg.RateLimitRPS = rps
	}

	if v := strings.TrimSpace(os.Getenv("RATE_LIMIT_BURST")); v != "" {
		burst, err := strconv.Atoi(v)
		if err != nil || burst <= 0 {
			return Config{}, fmt.Errorf("invalid RATE_LIMIT_BURST")
		}
		cfg.RateLimitBurst = burst
	}

	if cfg.TelegramBotToken == "" {
		return Config{}, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	if len(cfg.TelegramChatIDs) == 0 {
		return Config{}, fmt.Errorf("TELEGRAM_CHAT_IDS is required")
	}
	if cfg.TelegramWebhookURL == "" {
		return Config{}, fmt.Errorf("TELEGRAM_WEBHOOK_URL is required")
	}

	return cfg, nil
}
