package ai

import "context"

const (
	OpExtract     = "extract"
	OpEmbed       = "embed"
	OpExplain     = "explain"
	OpIcebreaker  = "icebreaker"

	SourceAPI        = "api"
	SourceWorker     = "worker"
	SourceEnrichSeed = "enrich_seed"
	SourceSeed       = "seed"
)

type TokenUsageRecord struct {
	Model            string
	Operation        string
	Source           string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type TokenRecorder interface {
	RecordTokenUsage(ctx context.Context, rec TokenUsageRecord) error
}

type UsageConfig struct {
	Recorder TokenRecorder
	Source   string
}

func (c *OpenAIClient) recordUsage(ctx context.Context, model, operation string, prompt, completion, total int) {
	if c.usage == nil || c.usage.Recorder == nil || total <= 0 {
		return
	}
	_ = c.usage.Recorder.RecordTokenUsage(ctx, TokenUsageRecord{
		Model:            model,
		Operation:        operation,
		Source:           c.usage.Source,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      total,
	})
}
