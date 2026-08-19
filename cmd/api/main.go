package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alterwalker/test_dating_bot/internal/ai"
	"github.com/alterwalker/test_dating_bot/internal/api"
	"github.com/alterwalker/test_dating_bot/internal/config"
	"github.com/alterwalker/test_dating_bot/internal/matching"
	"github.com/alterwalker/test_dating_bot/internal/profile"
	"github.com/alterwalker/test_dating_bot/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	store, err := storage.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer store.Close()

	aiClient, err := ai.NewClient(cfg, &ai.UsageConfig{Recorder: store, Source: ai.SourceAPI})
	if err != nil {
		log.Fatalf("ai: %v", err)
	}

	profiles := profile.NewService(store)
	matches := matching.NewService(store, aiClient, cfg)
	server := api.NewServer(profiles, matches, store)

	httpServer := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: server.Handler(),
	}

	go func() {
		log.Printf("api listening on %s (ai=%s)", cfg.HTTPAddr, aiClient.Mode())
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}
