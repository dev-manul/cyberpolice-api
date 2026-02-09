package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"go.uber.org/fx"

	"cyberpolice-api/internal/config"
	"cyberpolice-api/internal/geoip"
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

func RegisterRoutes(mux *http.ServeMux, cfg config.Config, mailer mailer.Mailer, limiter *ratelimit.IPRateLimiter, geo *geoip.Resolver) {
	handler := ratelimit.Middleware(limiter, SubmitHandler(mailer, geo))
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

func SubmitHandler(m mailer.Mailer, geo *geoip.Resolver) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			log.Printf("reject method=%s path=%s", r.Method, r.URL.Path)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		rawBody, _ := io.ReadAll(io.LimitReader(r.Body, 256*1024))
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewBuffer(rawBody))

		if isJSONRequest(r.Header.Get("Content-Type")) {
			if r.Form == nil {
				r.Form = make(url.Values)
			}
			if err := parseJSONBody(r.Form, rawBody); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
		} else {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}

			if r.Form.Get("urgency") == "" || r.Form.Get("summary") == "" {
				mergePlainBodyPairs(r.Form, rawBody)
			}
		}

		if err := validateForm(r.Form); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		body := buildEmailBody(r.Form)
		ip := firstForwardedIP(r)
		body = appendRequesterIP(body, ip)
		body = appendGeo(body, ip, geo)
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

func appendRequesterIP(body string, ip string) string {
	if ip == "" {
		return body
	}
	return body + "\nIP: " + ip + "\n"
}

func firstForwardedIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		for _, part := range parts {
			ip := strings.TrimSpace(part)
			if ip != "" {
				return ip
			}
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func appendGeo(body, ip string, geo *geoip.Resolver) string {
	if ip == "" || geo == nil {
		return body
	}
	loc, ok := geo.Lookup(ip)
	if !ok {
		return body
	}
	if loc.Country != "" {
		body += "Country: " + loc.Country + "\n"
	}
	if loc.City != "" {
		body += "City: " + loc.City + "\n"
	}
	return body
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

func isJSONRequest(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	return strings.HasPrefix(contentType, "application/json")
}

func parseJSONBody(dst url.Values, raw []byte) error {
	if len(raw) == 0 {
		return errors.New("empty body")
	}

	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}

	for key, value := range data {
		switch v := value.(type) {
		case string:
			if v != "" {
				dst.Add(key, v)
			}
		case []any:
			for _, item := range v {
				s := formatValue(item)
				if s != "" {
					dst.Add(key, s)
				}
			}
		default:
			s := formatValue(v)
			if s != "" {
				dst.Add(key, s)
			}
		}
	}

	return nil
}

func formatValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprintf("%v", t)
	case bool:
		if t {
			return "yes"
		}
		return "no"
	default:
		text := fmt.Sprintf("%v", t)
		if !utf8.ValidString(text) {
			return ""
		}
		return text
	}
}
