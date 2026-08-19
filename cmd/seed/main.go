package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/alterwalker/test_dating_bot/internal/ai"
	"github.com/alterwalker/test_dating_bot/internal/config"
	"github.com/alterwalker/test_dating_bot/internal/domain"
	"github.com/alterwalker/test_dating_bot/internal/storage"
)

type seedEntry struct {
	ExternalID      string                 `json:"external_id"`
	Raw             domain.RawProfile      `json:"raw"`
	Enriched        domain.EnrichedProfile `json:"enriched"`
	Embedding       []float32              `json:"embedding"`
	EmbeddingModel  string                 `json:"embedding_model"`
	SkipLLM         bool                   `json:"skip_llm"`
	Note            string                 `json:"_note,omitempty"`
}

func main() {
	file := flag.String("file", "seed/fictional_profiles.json", "seed file path")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if *file != "" {
		cfg.DemoSeedFile = *file
	}

	ctx := context.Background()
	store, err := storage.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer store.Close()

	data, err := os.ReadFile(cfg.DemoSeedFile)
	if err != nil {
		log.Fatalf("read seed: %v", err)
	}

	var entries []seedEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		log.Fatalf("parse seed: %v", err)
	}

	aiClient, err := ai.NewClient(cfg, &ai.UsageConfig{Recorder: store, Source: ai.SourceSeed})
	if err != nil {
		log.Fatalf("ai: %v", err)
	}

	for _, e := range entries {
		embedding := e.Embedding
		model := e.EmbeddingModel
		if len(embedding) == 0 {
			text := domain.BuildEmbeddingText(e.Raw, e.Enriched)
			embedding, err = aiClient.Embed(ctx, text)
			if err != nil {
				log.Fatalf("embed %s: %v", e.ExternalID, err)
			}
		}
		if model == "" {
			model = aiClient.EmbedModel()
		}
		if err := store.InsertFictionalProfile(ctx, e.ExternalID, e.Raw, e.Enriched, embedding, model); err != nil {
			log.Fatalf("insert %s: %v", e.ExternalID, err)
		}
		log.Printf("seeded %s (%s)", e.ExternalID, e.Raw.Name)
	}
	log.Printf("done: %d profiles", len(entries))
}
