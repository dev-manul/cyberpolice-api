package mailer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"cyberpolice-api/internal/config"
)

type Mailer interface {
	Send(subject, body string) error
}

type TelegramSender struct {
	token   string
	chatIDs []string
}

func NewTelegramSender(cfg config.Config) Mailer {
	return &TelegramSender{
		token:   cfg.TelegramBotToken,
		chatIDs: cfg.TelegramChatIDs,
	}
}

func (t *TelegramSender) Send(subject, body string) error {
	text := subject + "\n\n" + body
	client := &http.Client{Timeout: 10 * time.Second}

	for _, chatID := range t.chatIDs {
		payload := map[string]string{
			"chat_id": chatID,
			"text":    text,
		}

		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(data))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("telegram error: status=%d body=%s", resp.StatusCode, string(body))
		}
	}

	return nil
}
