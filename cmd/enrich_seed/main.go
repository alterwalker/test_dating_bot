package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/alterwalker/test_dating_bot/internal/ai"
	"github.com/alterwalker/test_dating_bot/internal/config"
	"github.com/alterwalker/test_dating_bot/internal/seedenrich"
	"github.com/alterwalker/test_dating_bot/internal/storage"
)

func main() {
	file := flag.String("file", "seed/fictional_profiles.json", "input seed JSON")
	out := flag.String("out", "", "output JSON (default: with -in-place same as -file)")
	inPlace := flag.Bool("in-place", false, "overwrite input file")
	offset := flag.Int("offset", 0, "start index (for resume)")
	limit := flag.Int("limit", 0, "max profiles to process (0 = all from offset)")
	workers := flag.Int("workers", 3, "parallel OpenAI requests")
	loadDB := flag.Bool("load-db", false, "upsert each profile into Postgres after enrich")
	allowMock := flag.Bool("allow-mock", false, "allow AI_MOCK=true (for tests only)")
	skipExisting := flag.Bool("skip-existing", true, "skip entries that already have embedding")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	if cfg.AIMock && !*allowMock {
		log.Fatal("enrich_seed требует реальный OpenAI: установите AI_MOCK=false и OPENAI_API_KEY. Для тестов: -allow-mock")
	}

	var usage *ai.UsageConfig
	if cfg.DatabaseURL != "" {
		store, err := storage.New(context.Background(), cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("db (token usage): %v", err)
		}
		defer store.Close()
		usage = &ai.UsageConfig{Recorder: store, Source: ai.SourceEnrichSeed}
	} else {
		log.Println("warning: DATABASE_URL not set — token usage will not be recorded")
	}

	client, err := ai.NewClient(cfg, usage)
	if err != nil {
		log.Fatal(err)
	}

	entries, err := seedenrich.LoadEntries(*file)
	if err != nil {
		log.Fatal(err)
	}
	total := len(entries)
	end := total
	if *offset < 0 {
		log.Fatal("offset must be >= 0")
	}
	if *limit > 0 {
		end = *offset + *limit
		if end > total {
			end = total
		}
	}
	rangeCount := end - *offset
	if rangeCount <= 0 {
		log.Fatal("nothing to process: check -offset/-limit")
	}

	toProcess := 0
	skippedInRange := 0
	for idx := *offset; idx < end; idx++ {
		if *skipExisting && seedenrich.HasEmbedding(entries[idx]) {
			skippedInRange++
		} else {
			toProcess++
		}
	}

	fmt.Printf("enrich_seed: mode=%s model=%s embed=%s\n", client.Mode(), cfg.OpenAIModel, client.EmbedModel())
	fmt.Printf("profiles: %d total, range=%d (offset=%d), to_process=%d, skip_existing=%v\n",
		total, rangeCount, *offset, toProcess, *skipExisting)
	if *skipExisting && skippedInRange > 0 {
		fmt.Printf("skipped in range (already have embedding): %d\n", skippedInRange)
	}
	fmt.Printf("workers=%d in_place=%v load_db=%v\n", *workers, *inPlace, *loadDB)
	if client.Mode() == "openai" && toProcess > 0 {
		fmt.Printf("≈ API calls: %d extract + %d embed = %d requests\n", toProcess, toProcess, toProcess*2)
	}
	if toProcess == 0 {
		fmt.Println("nothing to enrich in this range — all entries already have embeddings")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	result, err := seedenrich.Run(ctx, client, seedenrich.Options{
		InputPath:    *file,
		OutputPath:   *out,
		InPlace:      *inPlace,
		Offset:       *offset,
		Limit:        *limit,
		Workers:      *workers,
		LoadDB:       *loadDB,
		DatabaseURL:  cfg.DatabaseURL,
		SkipExisting: *skipExisting,
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\ndone: processed=%d skipped=%d failed=%d output=%s\n",
		result.Processed, result.Skipped, result.Failed, result.Output)
	if result.Failed > 0 {
		os.Exit(1)
	}
}
