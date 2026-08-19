package matching

import (
	"math"
	"sort"
	"strings"

	"github.com/alterwalker/test_dating_bot/internal/ai"
	"github.com/alterwalker/test_dating_bot/internal/domain"
)

func Harmonic(a, b float64) float64 {
	if a <= 0 || b <= 0 {
		return 0
	}
	return 2 * a * b / (a + b)
}

func Jaccard(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setA := toSet(a)
	setB := toSet(b)
	var inter int
	for k := range setA {
		if _, ok := setB[k]; ok {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func Intersection(a, b []string) []string {
	setB := toSet(b)
	var out []string
	for _, v := range a {
		if _, ok := setB[v]; ok {
			out = append(out, v)
		}
	}
	return unique(out)
}

func toSet(items []string) map[string]struct{} {
	m := make(map[string]struct{}, len(items))
	for _, v := range items {
		m[strings.ToLower(strings.TrimSpace(v))] = struct{}{}
	}
	return m
}

func unique(items []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, v := range items {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func AxisSimilarity(a, b domain.LifestyleAxes) float64 {
	diffs := []float64{
		math.Abs(a.Outgoing - b.Outgoing),
		math.Abs(a.FamilyOriented - b.FamilyOriented),
		math.Abs(a.CareerFocused - b.CareerFocused),
		math.Abs(a.Adventurous - b.Adventurous),
		math.Abs(a.Homebody - b.Homebody),
	}
	var sum float64
	for _, d := range diffs {
		sum += 1 - d
	}
	return sum / float64(len(diffs))
}

func IntentCompatible(a, b string) bool {
	if a == b {
		return true
	}
	if a == "unsure" || b == "unsure" {
		return true
	}
	if (a == "serious" && b == "casual") || (a == "casual" && b == "serious") {
		return false
	}
	return true
}

func DealbreakerConflict(a, b domain.EnrichedProfile) bool {
	setA := toSet(a.DealbreakersDetected)
	for _, v := range b.Interests {
		if _, ok := setA[strings.ToLower(v)]; ok {
			return true
		}
	}
	return false
}

func GenderSeekingCompatible(viewer, candidate domain.RawProfile) bool {
	return seekingMatch(viewer.Gender, candidate.Seeking) && seekingMatch(candidate.Gender, viewer.Seeking)
}

func seekingMatch(gender string, seeking []string) bool {
	for _, s := range seeking {
		if s == gender {
			return true
		}
	}
	return false
}

func AgeInRange(viewerAge int, min, max *int) bool {
	if min != nil && viewerAge < *min {
		return false
	}
	if max != nil && viewerAge > *max {
		return false
	}
	return true
}

func Preference(viewer domain.RawProfile, viewerEnriched domain.EnrichedProfile, candidate domain.RawProfile, candidateEnriched domain.EnrichedProfile) float64 {
	if !GenderSeekingCompatible(viewer, candidate) {
		return 0
	}
	if !AgeInRange(candidate.Age, viewer.AgeMin, viewer.AgeMax) {
		return 0
	}
	if !IntentCompatible(viewerEnriched.RelationshipIntent, candidateEnriched.RelationshipIntent) {
		return 0
	}
	if DealbreakerConflict(viewerEnriched, candidateEnriched) {
		return 0
	}

	score := 0.0
	if IntentCompatible(viewerEnriched.RelationshipIntent, candidateEnriched.RelationshipIntent) {
		score += 0.25
	}
	score += 0.35 * Jaccard(viewerEnriched.Values, candidateEnriched.Values)
	score += 0.25 * Jaccard(viewerEnriched.Interests, candidateEnriched.Interests)
	score += 0.15 * AxisSimilarity(viewerEnriched.LifestyleAxes, candidateEnriched.LifestyleAxes)
	return math.Min(score, 1)
}

type ScoredCandidate struct {
	Row           domain.CandidateRow
	RetrieveScore float64
	MatchScore    float64
	Breakdown     domain.MatchBreakdown
}

func ScorePair(viewer domain.CandidateRow, candidate domain.CandidateRow) ScoredCandidate {
	embedSim := ai.CosineSimilarity(viewer.Embedding, candidate.Embedding)
	retrieve := 0.5*embedSim +
		0.3*Jaccard(viewer.Enriched.Interests, candidate.Enriched.Interests) +
		0.2*Jaccard(viewer.Enriched.Values, candidate.Enriched.Values)

	prefAToB := Preference(viewer.Raw, viewer.Enriched, candidate.Raw, candidate.Enriched)
	prefBToA := Preference(candidate.Raw, candidate.Enriched, viewer.Raw, viewer.Enriched)
	harm := Harmonic(prefAToB, prefBToA)
	matchScore := 0.8*harm + 0.2*embedSim
	if matchScore > 1 {
		matchScore = 1
	}

	return ScoredCandidate{
		Row:           candidate,
		RetrieveScore: retrieve,
		MatchScore:    matchScore,
		Breakdown: domain.MatchBreakdown{
			PrefAToB:            prefAToB,
			PrefBToA:            prefBToA,
			Harmonic:            harm,
			EmbeddingSimilarity: embedSim,
			SharedInterests:     Intersection(viewer.Enriched.Interests, candidate.Enriched.Interests),
			SharedValues:        Intersection(viewer.Enriched.Values, candidate.Enriched.Values),
		},
	}
}

func FilterCandidates(viewer domain.CandidateRow, candidates []domain.CandidateRow, demoMode bool) []domain.CandidateRow {
	var out []domain.CandidateRow
	for _, c := range candidates {
		if c.UserID == viewer.UserID {
			continue
		}
		if !demoMode && c.UserKind == domain.UserKindFictional {
			continue
		}
		if strings.ToLower(c.Raw.City) != strings.ToLower(viewer.Raw.City) {
			continue
		}
		if !GenderSeekingCompatible(viewer.Raw, c.Raw) {
			continue
		}
		if !AgeInRange(c.Raw.Age, viewer.Raw.AgeMin, viewer.Raw.AgeMax) {
			continue
		}
		if !AgeInRange(viewer.Raw.Age, c.Raw.AgeMin, c.Raw.AgeMax) {
			continue
		}
		out = append(out, c)
	}
	return out
}

func RankMatches(viewer domain.CandidateRow, candidates []domain.CandidateRow, topK int) []ScoredCandidate {
	scored := make([]ScoredCandidate, 0, len(candidates))
	for _, c := range candidates {
		scored = append(scored, ScorePair(viewer, c))
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].MatchScore > scored[j].MatchScore
	})
	if topK > 0 && len(scored) > topK {
		scored = scored[:topK]
	}
	return scored
}

func TopRetrieve(viewer domain.CandidateRow, candidates []domain.CandidateRow, topK int) []domain.CandidateRow {
	scored := make([]ScoredCandidate, 0, len(candidates))
	for _, c := range candidates {
		scored = append(scored, ScorePair(viewer, c))
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].RetrieveScore > scored[j].RetrieveScore
	})
	if topK > 0 && len(scored) > topK {
		scored = scored[:topK]
	}
	out := make([]domain.CandidateRow, len(scored))
	for i, s := range scored {
		out[i] = s.Row
	}
	return out
}
