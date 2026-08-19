package matching

import (
	"testing"

	"github.com/alterwalker/test_dating_bot/internal/domain"
	"github.com/google/uuid"
)

func TestExcludeHidden(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	id3 := uuid.New()
	candidates := []domain.CandidateRow{
		{UserID: id1},
		{UserID: id2},
		{UserID: id3},
	}
	hidden := map[uuid.UUID]struct{}{id2: {}}
	out := ExcludeHidden(candidates, hidden)
	if len(out) != 2 {
		t.Fatalf("expected 2, got %d", len(out))
	}
	if out[0].UserID != id1 || out[1].UserID != id3 {
		t.Fatal("wrong candidates after exclude")
	}
}
