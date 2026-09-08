package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/my/app/internal/gmailconn"
	infra_mysql "github.com/my/app/internal/infra/mysql"
	"github.com/my/app/internal/mailpoll"
	channelsmysql "github.com/my/app/internal/repositories/channels/mysql"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/providercrypto"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("gmailpoll: load config: %v", err)
	}

	if strings.TrimSpace(cfg.OAuth.GoogleClientID) == "" || strings.TrimSpace(cfg.OAuth.GoogleClientSecret) == "" {
		log.Fatal("gmailpoll: GOOGLE_OAUTH_CLIENT_ID and GOOGLE_OAUTH_CLIENT_SECRET are required")
	}
	providerKey, err := providercrypto.ParseKey(cfg.LLM.ProviderConfigKey)
	if err != nil {
		log.Fatalf("gmailpoll: AI_PROVIDER_KEY invalid: %v", err)
	}

	secret := os.Getenv("LARAVEL_WEBHOOK_SECRET")
	if secret == "" {
		log.Fatal("gmailpoll: LARAVEL_WEBHOOK_SECRET is required")
	}

	central, err := infra_mysql.Open(cfg.MySQL)
	if err != nil {
		log.Fatalf("gmailpoll: open mysql: %v", err)
	}
	if central == nil {
		log.Fatal("gmailpoll: mysql is not enabled (set MYSQL_ENABLED=1 and MYSQL_DSN)")
	}
	defer central.Close()

	inboundURL := env("INBOUND_URL", "http://go-api:8080/v1/inbound/email")
	fwd := mailpoll.NewForwarder(inboundURL, secret, &http.Client{Timeout: 15 * time.Second})

	repo := channelsmysql.New(central)
	tokens := gmailconn.NewTokenSource(cfg.OAuth.GoogleClientID, cfg.OAuth.GoogleClientSecret, "", &http.Client{Timeout: 10 * time.Second})
	client := gmailconn.NewClient("", &http.Client{Timeout: 20 * time.Second})
	interval := envDuration("GMAIL_POLL_INTERVAL", 60*time.Second)
	systemMailbox := os.Getenv("IMAP_USER")

	reader := gmailconn.NewReader(repo, tokens, client, fwd, providerKey, interval, systemMailbox)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("gmailpoll started: interval=%s inbound=%s", interval, inboundURL)
	reader.Run(ctx)
	log.Println("gmailpoll: shutting down")
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
