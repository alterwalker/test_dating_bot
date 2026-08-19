package ai

import (
	"context"

	"github.com/alterwalker/test_dating_bot/internal/config"
	"github.com/alterwalker/test_dating_bot/internal/domain"
)

type ExplainRequest struct {
	ViewerName       string
	ViewerAge        int
	ViewerInterests  []string
	ViewerValues     []string
	ViewerIntent     string
	ViewerSummary    string
	CandidateName    string
	CandidateAge     int
	CandidateInterests []string
	CandidateValues  []string
	CandidateIntent  string
	CandidateSummary string
	SharedInterests  []string
	SharedValues     []string
	IntentMatch      bool
	OutgoingDiff     float64
	FamilyDiff       float64
}

type IcebreakerRequest struct {
	ViewerName         string
	ViewerAge          int
	ViewerInterests    []string
	ViewerValues       []string
	ViewerSummary      string
	CandidateName      string
	CandidateAge       int
	CandidateInterests []string
	CandidateValues    []string
	CandidateSummary   string
	SharedInterests    []string
	SharedValues       []string
}

type Client interface {
	Extract(ctx context.Context, raw domain.RawProfile) (domain.EnrichedProfile, error)
	Embed(ctx context.Context, text string) ([]float32, error)
	Explain(ctx context.Context, req ExplainRequest) (string, error)
	Icebreaker(ctx context.Context, req IcebreakerRequest) (domain.IcebreakerResult, error)
	Mode() string
	EmbedModel() string
	Dimensions() int
}

func NewClient(cfg config.Config, usage *UsageConfig) (Client, error) {
	if cfg.AIMock {
		return NewMockClient(cfg.EmbeddingDimensions), nil
	}
	return NewOpenAIClient(cfg, usage)
}
