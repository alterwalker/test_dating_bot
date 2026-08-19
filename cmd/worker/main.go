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
	"github.com/alterwalker/test_dating_bot/internal/config"
	"github.com/alterwalker/test_dating_bot/internal/jobs"
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

	aiClient, err := ai.NewClient(cfg, &ai.UsageConfig{Recorder: store, Source: ai.SourceWorker})
	if err != nil {
		log.Fatalf("ai: %v", err)
	}
	processor := jobs.NewProcessor(store, aiClient)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	go func() {
		log.Printf("worker health on %s (ai=%s)", cfg.WorkerHTTPAddr, aiClient.Mode())
		if err := http.ListenAndServe(cfg.WorkerHTTPAddr, mux); err != nil {
			log.Fatalf("health: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(cfg.WorkerPollInterval)
	defer ticker.Stop()

	log.Printf("worker polling every %s", cfg.WorkerPollInterval)

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			for {
				processed, err := processor.RunOnce(ctx)
				if err != nil {
					log.Printf("job error: %v", err)
				}
				if !processed {
					break
				}
			}
		}
	}
}
