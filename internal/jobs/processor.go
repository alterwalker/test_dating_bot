package jobs

import (
	"context"
	"fmt"

	"github.com/alterwalker/test_dating_bot/internal/ai"
	"github.com/alterwalker/test_dating_bot/internal/domain"
	"github.com/alterwalker/test_dating_bot/internal/storage"
	"github.com/google/uuid"
)

type Processor struct {
	store *storage.Store
	ai    ai.Client
}

func NewProcessor(store *storage.Store, aiClient ai.Client) *Processor {
	return &Processor{store: store, ai: aiClient}
}

func (p *Processor) RunOnce(ctx context.Context) (bool, error) {
	job, err := p.store.ClaimJob(ctx)
	if err != nil {
		if err == storage.ErrNotFound {
			return false, nil
		}
		return false, err
	}

	var runErr error
	switch job.Type {
	case domain.JobEnrichProfile:
		runErr = p.handleEnrich(ctx, job)
	default:
		runErr = fmt.Errorf("unknown job type: %s", job.Type)
	}

	if runErr != nil {
		attempts := job.Attempts + 1
		if failErr := p.store.FailJob(ctx, job.ID, runErr, attempts, job.MaxAttempts); failErr != nil {
			return true, failErr
		}
		return true, runErr
	}
	return true, p.store.FinishJob(ctx, job.ID)
}

func (p *Processor) handleEnrich(ctx context.Context, job domain.Job) error {
	rawID, ok := job.Payload["user_id"].(string)
	if !ok {
		return fmt.Errorf("invalid job payload")
	}
	userID, err := uuid.Parse(rawID)
	if err != nil {
		return err
	}

	prof, err := p.store.GetProfile(ctx, userID)
	if err != nil {
		return err
	}

	enriched, err := p.ai.Extract(ctx, prof.Raw)
	if err != nil {
		return err
	}
	if prof.Raw.RelationshipIntent != "" {
		enriched.RelationshipIntent = prof.Raw.RelationshipIntent
	}
	text := domain.BuildEmbeddingText(prof.Raw, enriched)
	embedding, err := p.ai.Embed(ctx, text)
	if err != nil {
		return err
	}
	return p.store.SaveEnrichedProfile(ctx, userID, enriched, embedding, p.ai.EmbedModel())
}
