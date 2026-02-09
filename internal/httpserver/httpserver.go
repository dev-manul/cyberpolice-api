package httpserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/fx"

	"cyberpolice-api/internal/config"
	"cyberpolice-api/internal/mailer"
	"cyberpolice-api/internal/ratelimit"
	"cyberpolice-api/internal/telegrambot"
)

func NewMux() *http.ServeMux {
	return http.NewServeMux()
}

func NewServer(cfg config.Config, mux *http.ServeMux) *http.Server {
	return &http.Server{
		Addr:         cfg.ServerAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}
}

func RegisterRoutes(mux *http.ServeMux, cfg config.Config, mailer mailer.Mailer, limiter *ratelimit.IPRateLimiter) {
	handler := ratelimit.Middleware(limiter, SubmitHandler(mailer))
	mux.Handle("/submit", handler)
	mux.Handle("/submib", handler)
	mux.Handle("/telegram/webhook", telegrambot.NewWebhookHandler(cfg))
}

func Start(lc fx.Lifecycle, srv *http.Server) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				_ = srv.ListenAndServe()
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return srv.Shutdown(ctx)
		},
	})
}

func SubmitHandler(m mailer.Mailer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		rawBody, _ := io.ReadAll(io.LimitReader(r.Body, 64*1024))
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewBuffer(rawBody))

		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if r.Form.Get("urgency") == "" || r.Form.Get("summary") == "" {
			mergePlainBodyPairs(r.Form, rawBody)
		}

		if err := validateForm(r.Form); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		body := buildEmailBody(r.Form)
		if err := m.Send("new case", body); err != nil {
			log.Printf("send message error: %v", err)
			http.Error(w, "failed to send", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func validateForm(v url.Values) error {
	if v.Get("urgency") == "" {
		return errors.New("urgency is required")
	}
	if v.Get("summary") == "" {
		return errors.New("summary is required")
	}
	if len(v.Get("summary")) > 500 {
		return errors.New("summary too long")
	}
	return nil
}

func buildEmailBody(values url.Values) string {
	var b strings.Builder

	writeList := func(label string, v []string) {
		if len(v) == 0 {
			return
		}
		b.WriteString(label)
		b.WriteString(": ")
		b.WriteString(strings.Join(v, ", "))
		b.WriteString("\n")
	}

	writeText := func(label, v string) {
		if v == "" {
			return
		}
		b.WriteString(label)
		b.WriteString(": ")
		b.WriteString(v)
		b.WriteString("\n")
	}

	writeList("Type", values["type"])
	writeText("Type other", values.Get("type_other_specify"))
	writeText("Urgency", values.Get("urgency"))
	writeText("Summary", values.Get("summary"))
	writeList("Platforms", values["platforms"])
	writeList("Evidence", values["evidence"])
	writeList("Actions", values["actions"])
	writeText("Country residence", values.Get("country_residence"))
	writeText("Country incident", values.Get("country_incident"))
	writeText("Contact name", values.Get("contact_name"))
	writeText("Contact method", values.Get("contact_method"))
	writeText("Urgent contact", values.Get("urgent_contact"))
	writeList("Privacy", values["privacy"])

	if b.Len() == 0 {
		return fmt.Sprintf("empty form submitted at %s\n", time.Now().Format(time.RFC3339))
	}

	return b.String()
}

func mergePlainBodyPairs(dst url.Values, raw []byte) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if strings.TrimSpace(text) == "" {
		return
	}

	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); {
		key := strings.TrimSpace(lines[i])
		i++
		if key == "" {
			continue
		}
		val := ""
		if i < len(lines) {
			val = strings.TrimSpace(lines[i])
			i++
		}
		dst.Add(key, val)
	}
}
