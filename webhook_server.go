package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"gitea.stuzer.link/stuzer05/go-monobank"
	"stuzer.link/monobank-firefly3-bot/app"
)

func runWebhookServer() error {
	secret := os.Getenv("MONOBANK_WEBHOOK_SECRET")
	domain := os.Getenv("MONOBANK_WEBHOOK_DOMAIN")
	if secret == "" || url.PathEscape(secret) != secret {
		return errors.New("MONOBANK_WEBHOOK_SECRET must be non-empty and URL-path safe")
	}
	if domain == "" || strings.ContainsAny(domain, "/?#@ \t\r\n") {
		return errors.New("MONOBANK_WEBHOOK_DOMAIN must be a hostname, optionally with a port")
	}
	if os.Getenv("MONOBANK_TOKEN") == "" || os.Getenv("LISTEN") == "" {
		return errors.New("MONOBANK_TOKEN and LISTEN are required for the webhook server")
	}
	path := "/webhook/" + secret
	webhookURL := (&url.URL{Scheme: "https", Host: domain, Path: path}).String()
	var registered atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc(path, handleWebhook)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Allow Service routing before registration: Monobank validates the
		// public URL with a GET while the registration request is in flight.
		_ = json.NewEncoder(w).Encode(struct {
			Status            string `json:"status"`
			WebhookRegistered bool   `json:"webhook_registered"`
		}{Status: "pass", WebhookRegistered: registered.Load()})
	})
	listener, err := net.Listen("tcp", os.Getenv("LISTEN"))
	if err != nil {
		return errors.New("could not listen for Monobank webhooks")
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	defer server.Close()
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	log.Print("webhook server listening")

	// The Service/Tunnel may need time to route the first GET to this pod.
	// Do not print SDK errors: their content may include the secret URL.
	for attempt := 0; attempt < 6; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, registerErr := app.App().MonobankClient.Api.PersonalWebhookPost(ctx,
			monobank.SetWebHook{WebHookUrl: webhookURL}, os.Getenv("MONOBANK_TOKEN"))
		cancel()
		if registerErr == nil {
			registered.Store(true)
			log.Print("Monobank webhook registered")
			<-serveErr
			return errors.New("webhook HTTP server stopped")
		}
		if attempt == 5 {
			break
		}
		log.Print("Monobank webhook registration failed; retrying in 61 seconds")
		timer := time.NewTimer(61 * time.Second)
		select {
		case <-serveErr:
			timer.Stop()
			return errors.New("webhook HTTP server stopped")
		case <-timer.C:
		}
	}
	return errors.New("Monobank webhook registration failed after six attempts")
}
