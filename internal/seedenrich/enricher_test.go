package seedenrich_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alterwalker/test_dating_bot/internal/ai"
	"github.com/alterwalker/test_dating_bot/internal/domain"
	"github.com/alterwalker/test_dating_bot/internal/seedenrich"
	"github.com/alterwalker/test_dating_bot/internal/seedgen"
)

func TestEnrichEntryMock(t *testing.T) {
	client := ai.NewMockClient(1536)
	raw := domain.RawProfile{
		Name:                     "Тест",
		Age:                      30,
		City:                     "Москва",
		Gender:                   "male",
		Seeking:                  []string{"female"},
		RelationshipIntent:       "serious",
		PromptIdealEvening:       "Пробежка и ужин дома с книгой",
		PromptRelationshipValues: "Честность и поддержка в отношениях",
		PromptOccupation:         "Увлекаюсь бегом и фотографией",
	}
	entry, err := seedenrich.EnrichEntry(context.Background(), client, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Embedding) != 1536 {
		t.Fatalf("expected 1536 dims, got %d", len(entry.Embedding))
	}
	if entry.Enriched.RelationshipIntent != "serious" {
		t.Fatalf("expected serious intent, got %s", entry.Enriched.RelationshipIntent)
	}
}

func TestRunMockPartial(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seed.json")
	entries := []seedgen.Entry{
		{
			ExternalID: "вымышленный_00001",
			Raw: domain.RawProfile{
				Name: "A", Age: 25, City: "Москва", Gender: "female", Seeking: []string{"male"},
				RelationshipIntent: "serious",
				PromptIdealEvening: "Вечер дома с чаем и музыкой",
				PromptRelationshipValues: "Честность и уважение друг к другу",
				PromptOccupation: "Люблю читать и рисовать",
			},
			SkipLLM: true,
		},
	}
	if err := seedgen.WriteFile(path, entries); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "out.json")
	client := ai.NewMockClient(1536)
	result, err := seedenrich.Run(context.Background(), client, seedenrich.Options{
		InputPath: path,
		OutputPath: out,
		Workers:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 {
		t.Fatalf("expected 1 processed, got %d", result.Processed)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 100 {
		t.Fatal("output too small")
	}
}
