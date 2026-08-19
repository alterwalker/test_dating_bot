package domain

import (
	"time"

	"github.com/google/uuid"
)

type UserKind string

const (
	UserKindTelegram  UserKind = "telegram"
	UserKindFictional UserKind = "fictional"
)

type ProfileStatus string

const (
	ProfileDraft      ProfileStatus = "draft"
	ProfileProcessing ProfileStatus = "processing"
	ProfileReady      ProfileStatus = "ready"
	ProfileConfirmed  ProfileStatus = "confirmed"
)

type JobType string

const (
	JobEnrichProfile JobType = "enrich_profile"
)

type JobStatus string

const (
	JobPending JobStatus = "pending"
	JobRunning JobStatus = "running"
	JobDone    JobStatus = "done"
	JobFailed  JobStatus = "failed"
)

type User struct {
	ID         uuid.UUID `json:"id"`
	Kind       UserKind  `json:"user_kind"`
	TelegramID *int64    `json:"telegram_id"`
	ExternalID *string   `json:"external_id"`
	Username   *string   `json:"username,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type LifestyleAxes struct {
	Outgoing       float64 `json:"outgoing"`
	FamilyOriented float64 `json:"family_oriented"`
	CareerFocused  float64 `json:"career_focused"`
	Adventurous    float64 `json:"adventurous"`
	Homebody       float64 `json:"homebody"`
}

type RawProfile struct {
	Name                      string   `json:"name"`
	Age                       int      `json:"age"`
	City                      string   `json:"city"`
	Gender                    string   `json:"gender"`
	Seeking                   []string `json:"seeking"`
	AgeMin                    *int     `json:"age_min,omitempty"`
	AgeMax                    *int     `json:"age_max,omitempty"`
	RelationshipIntent        string   `json:"relationship_intent,omitempty"`
	PromptIdealEvening        string   `json:"prompt_ideal_evening"`
	PromptRelationshipValues  string   `json:"prompt_relationship_values"`
	PromptOccupation          string   `json:"prompt_occupation"`
}

type EnrichedProfile struct {
	Interests           []string       `json:"interests"`
	Values              []string       `json:"values"`
	LifestyleAxes       LifestyleAxes  `json:"lifestyle_axes"`
	RelationshipIntent  string         `json:"relationship_intent"`
	CommunicationStyle  string         `json:"communication_style"`
	DealbreakersDetected []string      `json:"dealbreakers_detected"`
	Summary             string         `json:"summary"`
	ExtractionNotes     string         `json:"extraction_notes,omitempty"`
}

type Profile struct {
	UserID         uuid.UUID        `json:"user_id"`
	Status         ProfileStatus    `json:"status"`
	Raw            RawProfile       `json:"raw"`
	Enriched       *EnrichedProfile `json:"enriched"`
	ConfirmedAt    *time.Time       `json:"confirmed_at"`
	EmbeddingModel *string          `json:"embedding_model,omitempty"`
}

type Job struct {
	ID          uuid.UUID
	Type        JobType
	Payload     map[string]any
	Status      JobStatus
	Attempts    int
	MaxAttempts int
	LastError   *string
	RunAfter    time.Time
}

type MatchBreakdown struct {
	PrefAToB            float64  `json:"pref_a_to_b"`
	PrefBToA            float64  `json:"pref_b_to_a"`
	Harmonic            float64  `json:"harmonic"`
	EmbeddingSimilarity float64  `json:"embedding_similarity"`
	SharedInterests     []string `json:"shared_interests"`
	SharedValues        []string `json:"shared_values"`
}

type Match struct {
	CandidateID   uuid.UUID      `json:"candidate_id"`
	CandidateName string         `json:"candidate_name"`
	CandidateAge  int            `json:"candidate_age"`
	IsFictional   bool           `json:"is_fictional"`
	ExternalID    *string        `json:"external_id,omitempty"`
	Score         float64        `json:"score"`
	Breakdown     MatchBreakdown `json:"breakdown"`
	Summary       string         `json:"summary"`
	Explanation   string         `json:"explanation"`
}

type IcebreakerResult struct {
	ViewerID           uuid.UUID `json:"viewer_id"`
	CandidateID        uuid.UUID `json:"candidate_id"`
	CandidateName      string    `json:"candidate_name"`
	SharedInterests    []string  `json:"shared_interests"`
	SharedValues       []string  `json:"shared_values"`
	ConversationTopics []string  `json:"conversation_topics"`
	OpenerMessage      string    `json:"opener_message"`
}

type AdminCityStats struct {
	City   string `json:"city"`
	Male   int    `json:"male"`
	Female int    `json:"female"`
	Total  int    `json:"total"`
}

type AdminStatsResponse struct {
	TotalConfirmed int                   `json:"total_confirmed"`
	Cities         []AdminCityStats      `json:"cities"`
	TokenUsage     []AITokenUsageRow     `json:"token_usage,omitempty"`
	TokenByModel   []AITokenUsageByModel `json:"token_by_model,omitempty"`
}

type AITokenUsageRow struct {
	Model            string `json:"model"`
	Operation        string `json:"operation"`
	Source           string `json:"source"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	RequestCount     int64  `json:"request_count"`
}

type AITokenUsageByModel struct {
	Model            string `json:"model"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	RequestCount     int64  `json:"request_count"`
}

type CandidateProfile struct {
	CandidateID        uuid.UUID `json:"candidate_id"`
	Name               string    `json:"name"`
	Age                int       `json:"age"`
	City               string    `json:"city"`
	Gender             string    `json:"gender"`
	IsFictional        bool      `json:"is_fictional"`
	ExternalID         *string   `json:"external_id,omitempty"`
	Summary            string    `json:"summary"`
	Interests          []string  `json:"interests"`
	Values             []string  `json:"values"`
	RelationshipIntent string    `json:"relationship_intent"`
	CommunicationStyle string    `json:"communication_style"`
	IdealEvening       string    `json:"ideal_evening"`
	RelationshipValues string    `json:"relationship_values"`
	InterestsText      string    `json:"interests_text"`
	SharedInterests    []string  `json:"shared_interests"`
	SharedValues       []string  `json:"shared_values"`
}

type CandidateRow struct {
	UserID      uuid.UUID
	UserKind    UserKind
	ExternalID  *string
	Raw         RawProfile
	Enriched    EnrichedProfile
	Embedding   []float32
}

func (r RawProfile) Merge(partial RawProfile) RawProfile {
	out := r
	if partial.Name != "" {
		out.Name = partial.Name
	}
	if partial.Age > 0 {
		out.Age = partial.Age
	}
	if partial.City != "" {
		out.City = partial.City
	}
	if partial.Gender != "" {
		out.Gender = partial.Gender
	}
	if len(partial.Seeking) > 0 {
		out.Seeking = partial.Seeking
	}
	if partial.AgeMin != nil {
		out.AgeMin = partial.AgeMin
	}
	if partial.AgeMax != nil {
		out.AgeMax = partial.AgeMax
	}
	if partial.RelationshipIntent != "" {
		out.RelationshipIntent = partial.RelationshipIntent
	}
	if partial.PromptIdealEvening != "" {
		out.PromptIdealEvening = partial.PromptIdealEvening
	}
	if partial.PromptRelationshipValues != "" {
		out.PromptRelationshipValues = partial.PromptRelationshipValues
	}
	if partial.PromptOccupation != "" {
		out.PromptOccupation = partial.PromptOccupation
	}
	return out
}

func (r RawProfile) ValidForEnrich() bool {
	return r.Name != "" && r.Age > 0 && r.City != "" && r.Gender != "" &&
		len(r.Seeking) > 0 &&
		r.RelationshipIntent != "" &&
		len(r.PromptIdealEvening) >= 10 &&
		len(r.PromptRelationshipValues) >= 10 &&
		len(r.PromptOccupation) >= 10
}

func BuildEmbeddingText(raw RawProfile, enriched EnrichedProfile) string {
	return "Идеальный вечер: " + raw.PromptIdealEvening + "\n" +
		"Ценности: " + raw.PromptRelationshipValues + "\n" +
		"Интересы: " + raw.PromptOccupation + "\n" +
		"Summary: " + enriched.Summary
}
