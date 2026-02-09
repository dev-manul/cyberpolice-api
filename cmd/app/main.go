package main

import (
	"go.uber.org/fx"

	"cyberpolice-api/internal/config"
	"cyberpolice-api/internal/geoip"
	"cyberpolice-api/internal/httpserver"
	"cyberpolice-api/internal/mailer"
	"cyberpolice-api/internal/ratelimit"
	"cyberpolice-api/internal/telegrambot"
)

func main() {
	fx.New(
		fx.Provide(
			config.Load,
			geoip.NewResolver,
			mailer.NewTelegramSender,
			ratelimit.NewIPRateLimiter,
			httpserver.NewMux,
			httpserver.NewServer,
		),
		fx.Invoke(
			ratelimit.StartCleanup,
			httpserver.RegisterRoutes,
			httpserver.Start,
			telegrambot.RegisterWebhook,
		),
	).Run()
}
