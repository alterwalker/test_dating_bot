package ai

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"github.com/alterwalker/test_dating_bot/internal/domain"
)

const mockEmbedModel = "mock-deterministic-v1"

type MockClient struct {
	dims int
}

func NewMockClient(dims int) *MockClient {
	if dims <= 0 {
		dims = 1536
	}
	return &MockClient{dims: dims}
}

func (m *MockClient) Mode() string       { return "mock" }
func (m *MockClient) EmbedModel() string { return mockEmbedModel }
func (m *MockClient) Dimensions() int    { return m.dims }

func (m *MockClient) Extract(_ context.Context, raw domain.RawProfile) (domain.EnrichedProfile, error) {
	text := strings.ToLower(raw.PromptIdealEvening + " " + raw.PromptRelationshipValues + " " + raw.PromptOccupation)

	interests := uniqueStrings(extractKeywords(text, interestKeywords))
	if len(interests) == 0 {
		interests = []string{"общение"}
	}

	values := uniqueStrings(extractKeywords(text, valueKeywords))
	if len(values) == 0 {
		values = []string{"уважение"}
	}

	axes := domain.LifestyleAxes{
		Outgoing:       axisScore(text, []string{"тусов", "друз", "бар", "мероприят", "компан"}),
		FamilyOriented: axisScore(text, []string{"семь", "дети", "надолго", "стабильн"}),
		CareerFocused:  axisScore(text, []string{"карьер", "работ", "проект", "it", "разработ"}),
		Adventurous:    axisScore(text, []string{"путешеств", "нов", "спонтан", "приключ"}),
		Homebody:       axisScore(text, []string{"дом", "книг", "уют", "сериал", "диван"}),
	}

	intent := strings.TrimSpace(raw.RelationshipIntent)
	if intent == "" {
		intent = "unsure"
		switch {
		case containsAny(text, []string{"серьез", "серьёз", "надолго", "семь", "отношен"}):
			intent = "serious"
		case containsAny(text, []string{"легк", "без обязательств", "свидан"}):
			intent = "casual"
		case containsAny(text, []string{"дружб"}):
			intent = "friendship"
		}
	}

	dealbreakers := []string{}
	if containsAny(text, []string{"не переношу курен", "некурящ", "протiv курен"}) {
		dealbreakers = append(dealbreakers, "курение")
	}

	summary := fmt.Sprintf("%s, %d. Интересы: %s. Ценит %s.",
		raw.Name, raw.Age, strings.Join(interests, ", "), strings.Join(values, ", "))

	return domain.EnrichedProfile{
		Interests:            interests,
		Values:               values,
		LifestyleAxes:        axes,
		RelationshipIntent:   intent,
		CommunicationStyle:   "спокойный",
		DealbreakersDetected: dealbreakers,
		Summary:              summary,
		ExtractionNotes:      "mock: keyword extraction",
	}, nil
}

func (m *MockClient) Embed(_ context.Context, text string) ([]float32, error) {
	return hashToVector(text, m.dims), nil
}

func (m *MockClient) Explain(_ context.Context, req ExplainRequest) (string, error) {
	var parts []string
	if len(req.SharedInterests) > 0 {
		parts = append(parts, fmt.Sprintf("Вам близки: %s.", strings.Join(req.SharedInterests, ", ")))
	}
	if len(req.SharedValues) > 0 {
		parts = append(parts, fmt.Sprintf("Общие ценности: %s.", strings.Join(req.SharedValues, ", ")))
	}
	if req.IntentMatch {
		parts = append(parts, "Вы оба ищете похожий формат отношений.")
	}
	if len(parts) == 0 {
		return "Профили частично совместимы по параметрам анкеты.", nil
	}
	return strings.Join(parts, " "), nil
}

func (m *MockClient) Icebreaker(_ context.Context, req IcebreakerRequest) (domain.IcebreakerResult, error) {
	topics := mockTopics(req.SharedInterests, req.SharedValues)
	opener := mockOpener(req.SharedInterests, req.SharedValues, req.CandidateName)
	return domain.IcebreakerResult{
		SharedInterests:    req.SharedInterests,
		SharedValues:       req.SharedValues,
		ConversationTopics: topics,
		OpenerMessage:      opener,
		CandidateName:      req.CandidateName,
	}, nil
}

func mockTopics(interests, values []string) []string {
	interestQuestions := map[string]string{
		"бег":         "Как часто выходите на пробежку?",
		"готовка":     "Какое блюдо готовите чаще всего?",
		"it":          "Чем сейчас увлекаетесь в pet-проектах или на работе?",
		"йога":        "Как давно практикуете йогу?",
		"дизайн":      "Что вдохновляет в дизайне в последнее время?",
		"путешествия": "Куда мечтаете поехать следующей поездкой?",
		"собаки":      "Расскажите про вашего питомца — какая порода?",
		"кошки":       "Как зовут вашу кошку и какой у неё характер?",
		"музыка":      "Какую музыку слушаете чаще всего?",
		"фотография":  "Что любите снимать больше всего?",
		"чтение":      "Какую книгу недавно не могли оторваться дочитать?",
		"театр":       "Были недавно на спектакле или выставке?",
	}
	valueQuestions := map[string]string{
		"честность": "Как для вас проявляется честность в отношениях?",
		"юмор":      "Какой юмор вам ближе — лёгкий или с иронией?",
		"поддержка": "Как вам комфортнее поддерживать друг друга в сложные дни?",
		"семья":     "Насколько для вас важны семейные традиции?",
		"уважение":  "Что для вас значит уважение к личному пространству?",
	}

	var topics []string
	seen := map[string]struct{}{}
	add := func(q string) {
		if q == "" {
			return
		}
		if _, ok := seen[q]; ok {
			return
		}
		seen[q] = struct{}{}
		topics = append(topics, q)
	}

	for _, interest := range interests {
		if q, ok := interestQuestions[interest]; ok {
			add(q)
		}
		if len(topics) >= 5 {
			break
		}
	}
	for _, value := range values {
		if q, ok := valueQuestions[value]; ok {
			add(q)
		}
		if len(topics) >= 5 {
			break
		}
	}

	fallbacks := []string{
		"Чем увлекаетесь в свободное время?",
		"Как обычно проводите выходные?",
		"Что вас вдохновило на этой неделе?",
		"Есть ли место в городе, куда любите ходить?",
		"Какой фильм или сериал смотрели недавно?",
	}
	for _, fb := range fallbacks {
		if len(topics) >= 3 {
			break
		}
		add(fb)
	}
	if len(topics) > 5 {
		topics = topics[:5]
	}
	return topics
}

func mockOpener(interests, values []string, candidateName string) string {
	if len(interests) >= 2 {
		return fmt.Sprintf("Привет, %s! Заметил, что нам обоим интересны %s и %s — с чего начали?", candidateName, interests[0], interests[1])
	}
	if len(interests) == 1 {
		return fmt.Sprintf("Привет, %s! Увидел, что нам обоим близок %s — как давно этим занимаетесь?", candidateName, interests[0])
	}
	if len(values) > 0 {
		return fmt.Sprintf("Привет, %s! Кажется, нам обеим важна %s — как вы к этому пришли?", candidateName, values[0])
	}
	return fmt.Sprintf("Привет, %s! Рад знакомству — как проходит ваш день?", candidateName)
}

var interestKeywords = map[string][]string{
	"бег":         {"бег", "марафон", "пробеж"},
	"готовка":     {"готов", "кулинар", "кухн"},
	"it":          {"разработ", "backend", "frontend", "програм", "it"},
	"йога":        {"йог"},
	"дизайн":      {"дизайн", "ux", "ui"},
	"путешествия": {"путешеств"},
	"собаки":      {"собак", "labrador", "лабрадор"},
	"кошки":       {"кошк", "кот"},
}

var valueKeywords = map[string][]string{
	"честность":  {"честн"},
	"юмор":       {"юмор"},
	"поддержка":  {"поддерж"},
	"уважение":   {"уважен"},
	"семья":      {"семь"},
}

func extractKeywords(text string, table map[string][]string) []string {
	var out []string
	for tag, keys := range table {
		if containsAny(text, keys) {
			out = append(out, tag)
		}
	}
	return out
}

func axisScore(text string, keys []string) float64 {
	if containsAny(text, keys) {
		return 0.75
	}
	return 0.45
}

func containsAny(text string, keys []string) bool {
	for _, k := range keys {
		if strings.Contains(text, k) {
			return true
		}
	}
	return false
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func hashToVector(text string, dims int) []float32 {
	sum := sha256.Sum256([]byte(text))
	vec := make([]float32, dims)
	for i := 0; i < dims; i++ {
		idx := (i * 4) % len(sum)
		u := binary.LittleEndian.Uint32(sum[idx : idx+4])
		vec[i] = float32(u%1000)/500 - 1
	}
	return l2Normalize(vec)
}

func l2Normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x * x)
	}
	if sum == 0 {
		return v
	}
	norm := float32(math.Sqrt(sum))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x / norm
	}
	return out
}

func CosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i] * b[i])
		na += float64(a[i] * a[i])
		nb += float64(b[i] * b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
