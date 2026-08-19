package seedgen

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/alterwalker/test_dating_bot/internal/ai"
	"github.com/alterwalker/test_dating_bot/internal/domain"
)

type Entry struct {
	ExternalID     string                 `json:"external_id"`
	Raw            domain.RawProfile      `json:"raw"`
	Enriched       domain.EnrichedProfile `json:"enriched"`
	Embedding      []float32              `json:"embedding"`
	EmbeddingModel string                 `json:"embedding_model"`
	SkipLLM        bool                   `json:"skip_llm"`
}

var maleNames = []string{
	"Александр", "Дмитрий", "Максим", "Сергей", "Андрей", "Алексей", "Иван", "Михаил", "Кирилл", "Егор",
	"Никита", "Павел", "Роман", "Владимир", "Артём", "Илья", "Денис", "Антон", "Олег", "Тимофей",
}

var femaleNames = []string{
	"Анна", "Мария", "Елена", "Дарья", "Алина", "Полина", "Екатерина", "София", "Виктория", "Ксения",
	"Ольга", "Наталья", "Юлия", "Вера", "Татьяна", "Ирина", "Светлана", "Кристина", "Анастасия", "Марина",
}

var cities = []weightedCity{
	{ name: "Москва", weight: 70 },
	{ name: "Санкт-Петербург", weight: 20 },
	{ name: "Казань", weight: 3 },
	{ name: "Новосибирск", weight: 3 },
	{ name: "Екатеринбург", weight: 2 },
	{ name: "Краснодар", weight: 2 },
}

type weightedCity struct {
	name   string
	weight int
}

var eveningTemplates = []string{
	"После работы %s, потом %s дома.",
	"Люблю %s, а вечером %s.",
	"Идеально — %s и немного %s без суеты.",
	"Часто %s, иногда %s с близкими.",
	"%s, затем спокойный %s и чай.",
}

var eveningActivities = []string{
	"пробежка в парке", "йога", "прогулка", "готовка ужина", "настольные игры",
	"кино дома", "чтение", "встреча с друзьями", "музыка", "рисование",
}

var valuesTemplates = []string{
	"Для меня важны %s и %s в отношениях.",
	"Ценю %s, %s и честный разговор.",
	"Хочу %s, %s и уважение личного пространства.",
	"Ищу %s, %s и общие планы.",
}

var valueWords = []string{
	"честность", "поддержка", "юмор", "доверие", "верность", "семья", "свобода", "стабильность",
}

var interestsTemplates = []string{
	"Увлекаюсь %s и %s, часто %s.",
	"В свободное время %s, %s и иногда %s.",
	"Люблю %s, %s, а ещё %s.",
	"Мои хобби — %s, %s и %s.",
	"Помимо работы увлекаюсь %s, %s и %s.",
}

var interestTopics = []string{
	"бегом", "йогой", "путешествиями", "фотографией", "готовкой", "велосипедом",
	"настолками", "музыкой", "рисованием", "садоводством", "лыжами", "плаванием",
	"чтением", "кино", "театром", "скалолазанием", "волейболом", "шахматами",
}

var interestActivities = []string{
	"хожу в горы", "играю на гитаре", "изучаю новые рецепты", "бегаю по паркам",
	"смотрю документалки", "выращиваю растения", "хожу на выставки", "катаюсь на сапборде",
}

var relationshipIntents = []weightedValue{
	{value: "serious", weight: 60},
	{value: "casual", weight: 20},
	{value: "unsure", weight: 20},
}

type weightedValue struct {
	value  string
	weight int
}

func Generate(count, wideCount int, prefix string, rng *rand.Rand) []Entry {
	extractor := ai.NewMockClient(1536)
	ctx := context.Background()
	entries := make([]Entry, 0, count+wideCount)

	for i := 1; i <= count; i++ {
		entries = append(entries, buildEntry(extractor, ctx, prefix, i, randomRaw(rng, false)))
	}
	for i := 1; i <= wideCount; i++ {
		entries = append(entries, buildEntry(extractor, ctx, prefix, count+i, randomRaw(rng, true)))
	}
	return entries
}

func buildEntry(extractor *ai.MockClient, ctx context.Context, prefix string, n int, raw domain.RawProfile) Entry {
	enriched, err := extractor.Extract(ctx, raw)
	if err != nil {
		panic(err)
	}
	if raw.RelationshipIntent != "" {
		enriched.RelationshipIntent = raw.RelationshipIntent
	}
	return Entry{
		ExternalID:     rawExternalID(prefix, n),
		Raw:            raw,
		Enriched:       enriched,
		Embedding:      nil,
		EmbeddingModel: "mock-deterministic-v1",
		SkipLLM:        true,
	}
}

func rawExternalID(prefix string, n int) string {
	return fmt.Sprintf("%s%05d", prefix, n)
}

func randomRaw(rng *rand.Rand, wideAgeRange bool) domain.RawProfile {
	isMale := rng.Intn(2) == 0
	var name, gender string
	var seeking []string
	if isMale {
		name = pick(rng, maleNames)
		gender = "male"
		seeking = []string{"female"}
	} else {
		name = pick(rng, femaleNames)
		gender = "female"
		seeking = []string{"male"}
	}

	age := 21 + rng.Intn(25)
	var ageMin, ageMax int
	if wideAgeRange {
		ageMin = 18
		ageMax = 55
		// чуть чаще возраст в «популярных» диапазонах для demo-матчинга
		age = 24 + rng.Intn(18)
	} else {
		ageMin = age - 3 - rng.Intn(3)
		ageMax = age + 3 + rng.Intn(5)
		if ageMin < 18 {
			ageMin = 18
		}
		if ageMax > 55 {
			ageMax = 55
		}
	}

	evening := fmt.Sprintf(pick(rng, eveningTemplates),
		pick(rng, eveningActivities), pick(rng, eveningActivities))
	values := fmt.Sprintf(pick(rng, valuesTemplates),
		pick(rng, valueWords), pick(rng, valueWords))
	interests := fmt.Sprintf(pick(rng, interestsTemplates),
		pick(rng, interestTopics), pick(rng, interestTopics), pick(rng, interestActivities))

	return domain.RawProfile{
		Name:                     name,
		Age:                      age,
		City:                     pickWeightedCity(rng),
		Gender:                   gender,
		Seeking:                  seeking,
		AgeMin:                   &ageMin,
		AgeMax:                   &ageMax,
		RelationshipIntent:       pickWeightedValue(rng, relationshipIntents),
		PromptIdealEvening:       evening,
		PromptRelationshipValues: values,
		PromptOccupation:         interests,
	}
}

func pick(rng *rand.Rand, items []string) string {
	return items[rng.Intn(len(items))]
}

func pickWeightedCity(rng *rand.Rand) string {
	return pickWeightedValue(rng, toWeightedValues(cities))
}

func pickWeightedValue(rng *rand.Rand, items []weightedValue) string {
	total := 0
	for _, c := range items {
		total += c.weight
	}
	n := rng.Intn(total)
	for _, c := range items {
		n -= c.weight
		if n < 0 {
			return c.value
		}
	}
	return items[0].value
}

func toWeightedValues(cities []weightedCity) []weightedValue {
	out := make([]weightedValue, len(cities))
	for i, c := range cities {
		out[i] = weightedValue{value: c.name, weight: c.weight}
	}
	return out
}

func WriteFile(path string, entries []Entry) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(entries); err != nil {
		return err
	}
	return nil
}

func NormalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "вымышленный_"
	}
	if !strings.HasSuffix(prefix, "_") {
		prefix += "_"
	}
	return prefix
}

func NewRNG(seed int64) *rand.Rand {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return rand.New(rand.NewSource(seed))
}
