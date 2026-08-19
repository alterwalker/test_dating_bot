package ai

import (
	"context"
	"testing"

	"github.com/alterwalker/test_dating_bot/internal/domain"
)

func TestMockEmbedDeterministic(t *testing.T) {
	c := NewMockClient(1536)
	a, err := c.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 1536 {
		t.Fatalf("expected 1536 dims, got %d", len(a))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatal("embedding not deterministic")
		}
	}
}

func TestMockIcebreakerUniqueTopics(t *testing.T) {
	c := NewMockClient(1536)
	result, err := c.Icebreaker(context.Background(), IcebreakerRequest{
		CandidateName: "Анна",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ConversationTopics) < 3 {
		t.Fatalf("expected at least 3 topics, got %d", len(result.ConversationTopics))
	}
	seen := map[string]struct{}{}
	for _, topic := range result.ConversationTopics {
		if _, ok := seen[topic]; ok {
			t.Fatalf("duplicate topic: %q", topic)
		}
		seen[topic] = struct{}{}
	}
}

func TestMockExtract(t *testing.T) {
	c := NewMockClient(1536)
	raw := domain.RawProfile{
		Name: "Анна", Age: 28,
		PromptIdealEvening:       "Пробежка в парке и ужин дома",
		PromptRelationshipValues: "Честность и поддержка",
		PromptOccupation:         "Product designer, бегаю марафоны",
	}
	enriched, err := c.Extract(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(enriched.Interests) == 0 {
		t.Fatal("expected interests")
	}
}
