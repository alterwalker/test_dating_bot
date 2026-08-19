package seedenrich

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alterwalker/test_dating_bot/internal/ai"
	"github.com/alterwalker/test_dating_bot/internal/domain"
	"github.com/alterwalker/test_dating_bot/internal/seedgen"
	"github.com/alterwalker/test_dating_bot/internal/storage"
)

type Options struct {
	InputPath    string
	OutputPath   string
	InPlace      bool
	Offset       int
	Limit        int
	Workers      int
	LoadDB       bool
	DatabaseURL  string
	Checkpoint   int
	MaxRetries   int
	SkipExisting bool // default true via cmd/enrich_seed
}

type Result struct {
	Processed int
	Skipped   int
	Failed    int
	Output    string
}

func HasEmbedding(e seedgen.Entry) bool {
	return len(e.Embedding) > 0
}

func LoadEntries(path string) ([]seedgen.Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []seedgen.Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func WriteEntries(path string, entries []seedgen.Entry) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(entries); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func EnrichEntry(ctx context.Context, client ai.Client, raw domain.RawProfile) (seedgen.Entry, error) {
	enriched, err := client.Extract(ctx, raw)
	if err != nil {
		return seedgen.Entry{}, err
	}
	if raw.RelationshipIntent != "" {
		enriched.RelationshipIntent = raw.RelationshipIntent
	}
	text := domain.BuildEmbeddingText(raw, enriched)
	embedding, err := client.Embed(ctx, text)
	if err != nil {
		return seedgen.Entry{}, err
	}
	return seedgen.Entry{
		Raw:            raw,
		Enriched:       enriched,
		Embedding:      embedding,
		EmbeddingModel: client.EmbedModel(),
		SkipLLM:        true,
	}, nil
}

func Run(ctx context.Context, client ai.Client, opts Options) (Result, error) {
	entries, err := LoadEntries(opts.InputPath)
	if err != nil {
		return Result{}, err
	}

	outPath := opts.OutputPath
	if outPath == "" {
		if opts.InPlace {
			outPath = opts.InputPath
		} else {
			return Result{}, fmt.Errorf("укажите -out или -in-place")
		}
	}

	start := opts.Offset
	if start < 0 {
		start = 0
	}
	if start > len(entries) {
		return Result{}, fmt.Errorf("offset %d больше числа записей %d", start, len(entries))
	}

	end := len(entries)
	if opts.Limit > 0 && start+opts.Limit < end {
		end = start + opts.Limit
	}

	workers := opts.Workers
	if workers <= 0 {
		workers = 3
	}
	retries := opts.MaxRetries
	if retries <= 0 {
		retries = 3
	}
	checkpointEvery := opts.Checkpoint
	if checkpointEvery <= 0 {
		checkpointEvery = 25
	}

	var store *storage.Store
	if opts.LoadDB {
		store, err = storage.New(ctx, opts.DatabaseURL)
		if err != nil {
			return Result{}, fmt.Errorf("db: %w", err)
		}
		defer store.Close()
	}

	type job struct {
		idx int
	}

	jobs := make(chan job)
	var wg sync.WaitGroup
	var failed int64
	var processed int64
	var skipped int64
	var mu sync.Mutex
	flush := func() error {
		mu.Lock()
		defer mu.Unlock()
		return WriteEntries(outPath, entries)
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				idx := j.idx
				entry := entries[idx]
				var enriched seedgen.Entry
				var lastErr error
				for attempt := 0; attempt < retries; attempt++ {
					if attempt > 0 {
						delay := time.Duration(attempt*attempt) * time.Second
						select {
						case <-ctx.Done():
							return
						case <-time.After(delay):
						}
					}
					enriched, lastErr = EnrichEntry(ctx, client, entry.Raw)
					if lastErr == nil {
						break
					}
				}
				if lastErr != nil {
					atomic.AddInt64(&failed, 1)
					fmt.Printf("FAIL %s: %v\n", entry.ExternalID, lastErr)
					continue
				}

				mu.Lock()
				enriched.ExternalID = entry.ExternalID
				entries[idx] = enriched
				n := atomic.AddInt64(&processed, 1)
				mu.Unlock()

				if store != nil {
					if err := store.InsertFictionalProfile(ctx, enriched.ExternalID, enriched.Raw, enriched.Enriched, enriched.Embedding, enriched.EmbeddingModel); err != nil {
						atomic.AddInt64(&failed, 1)
						fmt.Printf("DB FAIL %s: %v\n", enriched.ExternalID, err)
						continue
					}
				}

				fmt.Printf("OK %s (%s) [%d/%d]\n", enriched.ExternalID, enriched.Raw.Name, n, end-start)

				if int(n)%checkpointEvery == 0 {
					if err := flush(); err != nil {
						fmt.Printf("checkpoint write: %v\n", err)
					}
				}
			}
		}()
	}

	for idx := start; idx < end; idx++ {
		if opts.SkipExisting && HasEmbedding(entries[idx]) {
			atomic.AddInt64(&skipped, 1)
			continue
		}
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return Result{}, ctx.Err()
		case jobs <- job{idx: idx}:
		}
	}
	close(jobs)
	wg.Wait()

	if err := flush(); err != nil {
		return Result{}, err
	}

	return Result{
		Processed: int(processed),
		Skipped:   int(skipped),
		Failed:    int(failed),
		Output:    outPath,
	}, nil
}
