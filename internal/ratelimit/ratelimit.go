package ratelimit

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"go.uber.org/fx"
	"golang.org/x/time/rate"

	"cyberpolice-api/internal/config"
)

type IPRateLimiter struct {
	mu      sync.Mutex
	clients map[string]*clientEntry
	r       rate.Limit
	b       int
	ttl     time.Duration
}

type clientEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func NewIPRateLimiter(cfg config.Config) *IPRateLimiter {
	return &IPRateLimiter{
		clients: make(map[string]*clientEntry),
		r:       rate.Limit(cfg.RateLimitRPS),
		b:       cfg.RateLimitBurst,
		ttl:     10 * time.Minute,
	}
}

func StartCleanup(lc fx.Lifecycle, limiter *IPRateLimiter) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go limiter.cleanupLoop(ctx)
			return nil
		},
	})
}

func (l *IPRateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.clients[ip]
	if !ok {
		entry = &clientEntry{
			limiter:  rate.NewLimiter(l.r, l.b),
			lastSeen: time.Now(),
		}
		l.clients[ip] = entry
	}

	entry.lastSeen = time.Now()
	return entry.limiter.Allow()
}

func (l *IPRateLimiter) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.cleanup()
		}
	}
}

func (l *IPRateLimiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-l.ttl)
	for ip, entry := range l.clients {
		if entry.lastSeen.Before(cutoff) {
			delete(l.clients, ip)
		}
	}
}

func Middleware(limiter *IPRateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			ip = host
		}

		if !limiter.Allow(ip) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
