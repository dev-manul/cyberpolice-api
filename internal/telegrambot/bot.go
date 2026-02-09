package telegrambot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"go.uber.org/fx"

	"cyberpolice-api/internal/config"
)

type update struct {
	UpdateID int      `json:"update_id"`
	Message  *message `json:"message"`
}

type message struct {
	Text string `json:"text"`
	Chat chat   `json:"chat"`
}

type chat struct {
	ID int64 `json:"id"`
}

type webhookResponse struct {
	OK bool `json:"ok"`
}

type setWebhookRequest struct {
	URL         string `json:"url"`
	SecretToken string `json:"secret_token,omitempty"`
}

func RegisterWebhook(lc fx.Lifecycle, cfg config.Config) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return setWebhook(ctx, cfg)
		},
	})
}

func NewWebhookHandler(cfg config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if cfg.TelegramWebhookSecret != "" {
			secret := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
			if secret != cfg.TelegramWebhookSecret {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 256*1024))
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		var upd update
		if err := json.Unmarshal(body, &upd); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if upd.Message != nil {
			text := strings.TrimSpace(upd.Message.Text)
			if strings.HasPrefix(text, "/myid") {
				log.Printf("telegram chat id: %d", upd.Message.Chat.ID)
			}
		}

		w.WriteHeader(http.StatusOK)
	})
}

func setWebhook(ctx context.Context, cfg config.Config) error {
	payload := setWebhookRequest{
		URL:         cfg.TelegramWebhookURL,
		SecretToken: cfg.TelegramWebhookSecret,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook", cfg.TelegramBotToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("setWebhook error: status=%d body=%s", resp.StatusCode, string(body))
	}

	var result webhookResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("setWebhook response not ok")
	}

	return nil
}
