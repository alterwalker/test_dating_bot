package matching

import (
	"testing"

	"github.com/alterwalker/test_dating_bot/internal/domain"
	"github.com/google/uuid"
)

func TestHarmonic(t *testing.T) {
	got := Harmonic(0.8, 0.6)
	if got < 0.68 || got > 0.69 {
		t.Fatalf("unexpected harmonic: %v", got)
	}
	if Harmonic(0.9, 0) != 0 {
		t.Fatal("expected zero for one-sided")
	}
}

func TestJaccard(t *testing.T) {
	got := Jaccard([]string{"бег", "it"}, []string{"бег", "йога"})
	if got < 0.32 || got > 0.34 {
		t.Fatalf("unexpected jaccard: %v", got)
	}
}

func TestScorePairMutual(t *testing.T) {
	viewer := domain.CandidateRow{
		UserID: uuid.New(),
		Raw: domain.RawProfile{
			Name: "Анна", Age: 28, City: "Москва", Gender: "female", Seeking: []string{"male"},
			AgeMin: intPtr(25), AgeMax: intPtr(35),
		},
		Enriched: domain.EnrichedProfile{
			Interests:          []string{"бег", "дизайн"},
			Values:             []string{"честность"},
			RelationshipIntent: "serious",
			LifestyleAxes:      domain.LifestyleAxes{Outgoing: 0.4, FamilyOriented: 0.7, CareerFocused: 0.5, Adventurous: 0.5, Homebody: 0.6},
		},
		Embedding: []float32{1, 0, 0},
	}
	candidate := domain.CandidateRow{
		UserID: uuid.New(),
		Raw: domain.RawProfile{
			Name: "Кирилл", Age: 29, City: "Москва", Gender: "male", Seeking: []string{"female"},
			AgeMin: intPtr(24), AgeMax: intPtr(34),
		},
		Enriched: domain.EnrichedProfile{
			Interests:          []string{"бег", "готовка"},
			Values:             []string{"честность"},
			RelationshipIntent: "serious",
			LifestyleAxes:      domain.LifestyleAxes{Outgoing: 0.4, FamilyOriented: 0.7, CareerFocused: 0.6, Adventurous: 0.5, Homebody: 0.65},
		},
		Embedding: []float32{0.9, 0.1, 0},
	}

	score := ScorePair(viewer, candidate)
	if score.MatchScore <= 0 {
		t.Fatalf("expected positive score, got %v", score.MatchScore)
	}
	if score.MatchScore > 1 {
		t.Fatalf("score must be <= 1, got %v", score.MatchScore)
	}
	if len(score.Breakdown.SharedInterests) != 1 || score.Breakdown.SharedInterests[0] != "бег" {
		t.Fatalf("unexpected shared interests: %v", score.Breakdown.SharedInterests)
	}
}

func intPtr(v int) *int { return &v }
