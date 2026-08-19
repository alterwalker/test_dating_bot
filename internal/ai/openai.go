package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alterwalker/test_dating_bot/internal/config"
	"github.com/alterwalker/test_dating_bot/internal/domain"
	openai "github.com/sashabaranov/go-openai"
)

type OpenAIClient struct {
	client         *openai.Client
	chatModel      string
	embeddingModel string
	dims           int
	usage          *UsageConfig
}

func NewOpenAIClient(cfg config.Config, usage *UsageConfig) (*OpenAIClient, error) {
	return &OpenAIClient{
		client:         openai.NewClient(cfg.OpenAIAPIKey),
		chatModel:      cfg.OpenAIModel,
		embeddingModel: cfg.EmbeddingModel,
		dims:           cfg.EmbeddingDimensions,
		usage:          usage,
	}, nil
}

func (c *OpenAIClient) Mode() string       { return "openai" }
func (c *OpenAIClient) EmbedModel() string { return c.embeddingModel }
func (c *OpenAIClient) Dimensions() int    { return c.dims }

func (c *OpenAIClient) Extract(ctx context.Context, raw domain.RawProfile) (domain.EnrichedProfile, error) {
	intentLine := ""
	if raw.RelationshipIntent != "" {
		intentLine = fmt.Sprintf("\nЦель знакомства (от пользователя): %s\n", raw.RelationshipIntent)
	}
	user := fmt.Sprintf(`Профиль пользователя:

Имя: %s
Возраст: %d
Город: %s
Пол: %s
Ищет: %s
%s
---

Ответ на вопрос «Идеальный вечер»:
%s

---

Ответ на вопрос «Что для тебя важно в отношениях»:
%s

---

Ответ на вопрос «Ваши интересы помимо работы»:
%s

---

Извлеки структурированный профиль в JSON.`,
		raw.Name, raw.Age, raw.City, raw.Gender, strings.Join(raw.Seeking, ", "),
		intentLine,
		raw.PromptIdealEvening, raw.PromptRelationshipValues, raw.PromptOccupation)

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.chatModel,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: extractorSystemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: user},
		},
		Temperature: 0.2,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		return domain.EnrichedProfile{}, err
	}
	if len(resp.Choices) == 0 {
		return domain.EnrichedProfile{}, fmt.Errorf("openai: empty response")
	}

	var enriched domain.EnrichedProfile
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &enriched); err != nil {
		return domain.EnrichedProfile{}, fmt.Errorf("parse enriched: %w", err)
	}
	if raw.RelationshipIntent != "" {
		enriched.RelationshipIntent = raw.RelationshipIntent
	}
	c.recordUsage(ctx, c.chatModel, OpExtract, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
	return enriched, nil
}

func (c *OpenAIClient) Embed(ctx context.Context, text string) ([]float32, error) {
	resp, err := c.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(c.embeddingModel),
		Input: text,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("openai: empty embedding")
	}
	out := make([]float32, len(resp.Data[0].Embedding))
	for i, v := range resp.Data[0].Embedding {
		out[i] = float32(v)
	}
	c.recordUsage(ctx, c.embeddingModel, OpEmbed, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
	return out, nil
}

func (c *OpenAIClient) Explain(ctx context.Context, req ExplainRequest) (string, error) {
	user := buildExplainUser(req)
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.chatModel,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: explainerSystemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: user},
		},
		Temperature: 0.5,
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai: empty response")
	}
	c.recordUsage(ctx, c.chatModel, OpExplain, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

func (c *OpenAIClient) Icebreaker(ctx context.Context, req IcebreakerRequest) (domain.IcebreakerResult, error) {
	user := buildIcebreakerUser(req)
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.chatModel,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: icebreakerSystemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: user},
		},
		Temperature: 0.6,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		return domain.IcebreakerResult{}, err
	}
	if len(resp.Choices) == 0 {
		return domain.IcebreakerResult{}, fmt.Errorf("openai: empty response")
	}

	var payload struct {
		ConversationTopics []string `json:"conversation_topics"`
		OpenerMessage      string   `json:"opener_message"`
	}
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &payload); err != nil {
		return domain.IcebreakerResult{}, err
	}

	c.recordUsage(ctx, c.chatModel, OpIcebreaker, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)

	return domain.IcebreakerResult{
		CandidateName:      req.CandidateName,
		SharedInterests:    req.SharedInterests,
		SharedValues:       req.SharedValues,
		ConversationTopics: payload.ConversationTopics,
		OpenerMessage:      payload.OpenerMessage,
	}, nil
}

const extractorSystemPrompt = `Ты — ассистент сервиса знакомств. Извлеки JSON-профиль из ответов пользователя.
Не выдумывай факты. Теги на русском lowercase. Оси lifestyle от 0 до 1.
Поля: interests, values, lifestyle_axes (outgoing, family_oriented, career_focused, adventurous, homebody),
relationship_intent (serious|casual|friendship|unsure), communication_style, dealbreakers_detected, summary.`

const explainerSystemPrompt = `Помоги пользователю понять, почему предложили кандидата.
2-3 предложения на русском, только факты из данных, без процентов и клише.`

const icebreakerSystemPrompt = `Предложи темы для беседы и первое сообщение на русском.
JSON: {"conversation_topics": [...3-5...], "opener_message": "..."}. Только факты из данных.`

func buildExplainUser(req ExplainRequest) string {
	return fmt.Sprintf(`Пользователь: %s, %d
Интересы: %s
Ценности: %s
Цель: %s
Кратко: %s

Кандидат: %s, %d
Интересы: %s
Ценности: %s
Цель: %s
Кратко: %s

Общие интересы: %s
Общие ценности: %s`,
		req.ViewerName, req.ViewerAge, strings.Join(req.ViewerInterests, ", "), strings.Join(req.ViewerValues, ", "),
		req.ViewerIntent, req.ViewerSummary,
		req.CandidateName, req.CandidateAge, strings.Join(req.CandidateInterests, ", "), strings.Join(req.CandidateValues, ", "),
		req.CandidateIntent, req.CandidateSummary,
		strings.Join(req.SharedInterests, ", "), strings.Join(req.SharedValues, ", "))
}

func buildIcebreakerUser(req IcebreakerRequest) string {
	return fmt.Sprintf(`Пользователь: %s, %d
Интересы: %s
Кратко: %s

Собеседник: %s, %d
Интересы: %s
Кратко: %s

Общие интересы: %s
Общие ценности: %s`,
		req.ViewerName, req.ViewerAge, strings.Join(req.ViewerInterests, ", "), req.ViewerSummary,
		req.CandidateName, req.CandidateAge, strings.Join(req.CandidateInterests, ", "), req.CandidateSummary,
		strings.Join(req.SharedInterests, ", "), strings.Join(req.SharedValues, ", "))
}
