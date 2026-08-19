package storage_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/alterwalker/test_dating_bot/internal/ai"
	"github.com/alterwalker/test_dating_bot/internal/domain"
	"github.com/alterwalker/test_dating_bot/internal/jobs"
	"github.com/alterwalker/test_dating_bot/internal/matching"
	"github.com/alterwalker/test_dating_bot/internal/profile"
	"github.com/alterwalker/test_dating_bot/internal/storage"
	"github.com/alterwalker/test_dating_bot/internal/config"
)

func TestIntegrationMatchingFlow(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()
	store, err := storage.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer store.Close()

	aiClient := ai.NewMockClient(1536)
	processor := jobs.NewProcessor(store, aiClient)
	profiles := profile.NewService(store)
	matchSvc := matching.NewService(store, aiClient, config.Config{
		DemoMode:       true,
		RetrievalMode:  "ann",
		RetrievalTopK:  50,
	})

	raw := domain.RawProfile{
		Name:                     "Тест",
		Age:                      28,
		City:                     "Москва",
		Gender:                   "female",
		Seeking:                  []string{"male"},
		AgeMin:                   intPtr(25),
		AgeMax:                   intPtr(35),
		RelationshipIntent:       "serious",
		PromptIdealEvening:       "Пробежка в парке и ужин дома с книгой",
		PromptRelationshipValues: "Честность, поддержка и общие планы",
		PromptOccupation:         "Product designer, бегаю и есть собака",
	}
	enriched := domain.EnrichedProfile{
		Interests:          []string{"бег", "дизайн"},
		Values:             []string{"честность"},
		RelationshipIntent: "serious",
		LifestyleAxes: domain.LifestyleAxes{
			Outgoing: 0.4, FamilyOriented: 0.7, CareerFocused: 0.5, Adventurous: 0.5, Homebody: 0.6,
		},
		CommunicationStyle:   "спокойный",
		DealbreakersDetected: []string{},
		Summary:              "Тестовый пользователь для integration test",
	}
	text := domain.BuildEmbeddingText(raw, enriched)
	embedding, err := aiClient.Embed(ctx, text)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.InsertFictionalProfile(ctx, "вымышленный_90001", raw, enriched, embedding, aiClient.EmbedModel()); err != nil {
		t.Fatalf("seed fictional: %v", err)
	}

	user, err := store.UpsertTelegramUser(ctx, 999001, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := profiles.UpdateRaw(ctx, user.ID, raw); err != nil {
		t.Fatal(err)
	}
	if _, _, err := profiles.StartEnrich(ctx, user.ID); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		processed, err := processor.RunOnce(ctx)
		if err != nil {
			t.Fatalf("worker: %v", err)
		}
		if processed {
			prof, err := store.GetProfile(ctx, user.ID)
			if err == nil && prof.Status == domain.ProfileReady {
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	if _, err := profiles.Confirm(ctx, user.ID); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	matches, total, err := matchSvc.FindMatches(ctx, user.ID, 5)
	if err != nil {
		t.Fatalf("matches: %v", err)
	}
	if total == 0 {
		t.Fatal("expected candidates after filters")
	}
	t.Logf("matches=%d total_filtered=%d", len(matches), total)
}

func intPtr(v int) *int { return &v }
