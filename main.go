package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"my-go-exchange/internal/api"
	"my-go-exchange/internal/market"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	exchange, err := market.NewLocalMarket()
	if err != nil {
		log.Fatalf("create market: %v", err)
	}
	if err := exchange.Start(ctx); err != nil {
		log.Fatalf("start market: %v", err)
	}

	authenticator, err := apiAuthenticator()
	if err != nil {
		log.Fatalf("configure API authentication: %v", err)
	}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	server := &http.Server{
		Addr: addr,
		Handler: api.NewServer(exchange, api.Config{
			Authenticator:  authenticator,
			AllowedOrigins: splitCSV(os.Getenv("API_ALLOWED_ORIGINS")),
		}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("exchange API listening on %s", addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Printf("serve API: %v", err)
		}
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown API: %v", err)
	}
}

func apiAuthenticator() (api.Authenticator, error) {
	token := strings.TrimSpace(os.Getenv("API_AUTH_TOKEN"))
	if token == "" {
		return nil, errors.New("API_AUTH_TOKEN is required")
	}

	accountID := int64(1)
	if raw := strings.TrimSpace(os.Getenv("API_ACCOUNT_ID")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			return nil, errors.New("API_ACCOUNT_ID must be a positive integer")
		}
		accountID = parsed
	}
	return api.NewStaticTokenAuthenticator(token, accountID)
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	values := strings.Split(raw, ",")
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
