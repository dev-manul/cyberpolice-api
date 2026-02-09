package telegrambot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/fx"

	"cyberpolice-api/internal/config"
)

type updatesResponse struct {
	OK     bool     `json:"ok"`
	Result []update `json:"result"`
}

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

func StartPolling(lc fx.Lifecycle, cfg config.Config) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go pollUpdates(ctx, cfg.TelegramBotToken)
			return nil
		},
	})
}

func pollUpdates(ctx context.Context, token string) {
	client := &http.Client{Timeout: 40 * time.Second}
	offset := 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		updates, err := getUpdates(ctx, client, token, offset)
		if err != nil {
			log.Printf("telegram getUpdates error: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		for _, upd := range updates {
			if upd.UpdateID >= offset {
				offset = upd.UpdateID + 1
			}
			if upd.Message == nil {
				continue
			}

			text := strings.TrimSpace(upd.Message.Text)
			if text == "" {
				continue
			}

			if strings.HasPrefix(text, "/myid") {
				log.Printf("telegram chat id: %d", upd.Message.Chat.ID)
			}
		}
	}
}

func getUpdates(ctx context.Context, client *http.Client, token string, offset int) ([]update, error) {
	values := url.Values{}
	values.Set("timeout", "30")
	if offset > 0 {
		values.Set("offset", fmt.Sprintf("%d", offset))
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?%s", token, values.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("status=%d", resp.StatusCode)
	}

	var payload updatesResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if !payload.OK {
		return nil, fmt.Errorf("telegram response not ok")
	}
	return payload.Result, nil
}
