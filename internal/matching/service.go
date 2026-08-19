package matching

import (
	"context"
	"math"

	"github.com/alterwalker/test_dating_bot/internal/ai"
	"github.com/alterwalker/test_dating_bot/internal/config"
	"github.com/alterwalker/test_dating_bot/internal/domain"
	"github.com/alterwalker/test_dating_bot/internal/storage"
	"github.com/google/uuid"
)

type Service struct {
	store  *storage.Store
	ai     ai.Client
	cfg    config.Config
}

func NewService(store *storage.Store, aiClient ai.Client, cfg config.Config) *Service {
	return &Service{store: store, ai: aiClient, cfg: cfg}
}

func (s *Service) FindMatches(ctx context.Context, viewerID uuid.UUID, limit int) ([]domain.Match, int, error) {
	prof, err := s.store.GetProfile(ctx, viewerID)
	if err != nil {
		return nil, 0, err
	}
	if prof.Status != domain.ProfileConfirmed {
		return nil, 0, storage.ErrConflict
	}

	viewer, err := s.store.GetCandidate(ctx, viewerID)
	if err != nil {
		return nil, 0, err
	}

	candidates, err := s.store.ListConfirmedCandidates(ctx, viewerID, s.cfg.DemoMode, s.cfg.RetrievalMode, s.cfg.RetrievalTopK)
	if err != nil {
		return nil, 0, err
	}

	hidden, err := s.store.ListHiddenCandidateIDs(ctx, viewerID)
	if err != nil {
		return nil, 0, err
	}
	candidates = ExcludeHidden(candidates, hidden)

	filtered := FilterCandidates(viewer, candidates, s.cfg.DemoMode)
	retrieved := TopRetrieve(viewer, filtered, s.cfg.RetrievalTopK)
	ranked := RankMatches(viewer, retrieved, limit)

	total := len(filtered)
	matches := make([]domain.Match, 0, len(ranked))
	for i, sc := range ranked {
		explanation := ""
		if i < 3 {
			explanation, _ = s.ai.Explain(ctx, ai.ExplainRequest{
				ViewerName:         viewer.Raw.Name,
				ViewerAge:          viewer.Raw.Age,
				ViewerInterests:    viewer.Enriched.Interests,
				ViewerValues:       viewer.Enriched.Values,
				ViewerIntent:       viewer.Enriched.RelationshipIntent,
				ViewerSummary:      viewer.Enriched.Summary,
				CandidateName:      sc.Row.Raw.Name,
				CandidateAge:       sc.Row.Raw.Age,
				CandidateInterests: sc.Row.Enriched.Interests,
				CandidateValues:    sc.Row.Enriched.Values,
				CandidateIntent:    sc.Row.Enriched.RelationshipIntent,
				CandidateSummary:   sc.Row.Enriched.Summary,
				SharedInterests:    sc.Breakdown.SharedInterests,
				SharedValues:       sc.Breakdown.SharedValues,
				IntentMatch:        IntentCompatible(viewer.Enriched.RelationshipIntent, sc.Row.Enriched.RelationshipIntent),
				OutgoingDiff:       math.Abs(viewer.Enriched.LifestyleAxes.Outgoing - sc.Row.Enriched.LifestyleAxes.Outgoing),
				FamilyDiff:         math.Abs(viewer.Enriched.LifestyleAxes.FamilyOriented - sc.Row.Enriched.LifestyleAxes.FamilyOriented),
			})
		}

		matches = append(matches, domain.Match{
			CandidateID:   sc.Row.UserID,
			CandidateName: sc.Row.Raw.Name,
			CandidateAge:  sc.Row.Raw.Age,
			IsFictional:   sc.Row.UserKind == domain.UserKindFictional,
			ExternalID:    sc.Row.ExternalID,
			Score:         round2(sc.MatchScore),
			Breakdown:     sc.Breakdown,
			Summary:       sc.Row.Enriched.Summary,
			Explanation:   explanation,
		})
	}
	return matches, total, nil
}

func (s *Service) Icebreaker(ctx context.Context, viewerID, candidateID uuid.UUID) (domain.IcebreakerResult, error) {
	prof, err := s.store.GetProfile(ctx, viewerID)
	if err != nil {
		return domain.IcebreakerResult{}, err
	}
	if prof.Status != domain.ProfileConfirmed {
		return domain.IcebreakerResult{}, storage.ErrConflict
	}

	viewer, candidate, err := s.store.GetViewerCandidate(ctx, viewerID, candidateID)
	if err != nil {
		return domain.IcebreakerResult{}, err
	}

	sharedInterests := Intersection(viewer.Enriched.Interests, candidate.Enriched.Interests)
	sharedValues := Intersection(viewer.Enriched.Values, candidate.Enriched.Values)

	result, err := s.ai.Icebreaker(ctx, ai.IcebreakerRequest{
		ViewerName:         viewer.Raw.Name,
		ViewerAge:          viewer.Raw.Age,
		ViewerInterests:    viewer.Enriched.Interests,
		ViewerValues:       viewer.Enriched.Values,
		ViewerSummary:      viewer.Enriched.Summary,
		CandidateName:      candidate.Raw.Name,
		CandidateAge:       candidate.Raw.Age,
		CandidateInterests: candidate.Enriched.Interests,
		CandidateValues:    candidate.Enriched.Values,
		CandidateSummary:   candidate.Enriched.Summary,
		SharedInterests:    sharedInterests,
		SharedValues:       sharedValues,
	})
	if err != nil {
		return domain.IcebreakerResult{}, err
	}
	result.ViewerID = viewerID
	result.CandidateID = candidateID
	result.CandidateName = candidate.Raw.Name
	result.SharedInterests = sharedInterests
	result.SharedValues = sharedValues
	return result, nil
}

func (s *Service) HideMatch(ctx context.Context, viewerID, candidateID uuid.UUID) error {
	prof, err := s.store.GetProfile(ctx, viewerID)
	if err != nil {
		return err
	}
	if prof.Status != domain.ProfileConfirmed {
		return storage.ErrConflict
	}
	if _, err := s.store.GetProfile(ctx, candidateID); err != nil {
		return err
	}
	return s.store.HideMatch(ctx, viewerID, candidateID)
}

func (s *Service) GetCandidateProfile(ctx context.Context, viewerID, candidateID uuid.UUID) (domain.CandidateProfile, error) {
	prof, err := s.store.GetProfile(ctx, viewerID)
	if err != nil {
		return domain.CandidateProfile{}, err
	}
	if prof.Status != domain.ProfileConfirmed {
		return domain.CandidateProfile{}, storage.ErrConflict
	}

	viewer, candidate, err := s.store.GetViewerCandidate(ctx, viewerID, candidateID)
	if err != nil {
		return domain.CandidateProfile{}, err
	}

	candidateProf, err := s.store.GetProfile(ctx, candidateID)
	if err != nil {
		return domain.CandidateProfile{}, err
	}
	if candidateProf.Status != domain.ProfileConfirmed {
		return domain.CandidateProfile{}, storage.ErrNotFound
	}

	sharedInterests := Intersection(viewer.Enriched.Interests, candidate.Enriched.Interests)
	sharedValues := Intersection(viewer.Enriched.Values, candidate.Enriched.Values)

	return domain.CandidateProfile{
		CandidateID:        candidateID,
		Name:               candidate.Raw.Name,
		Age:                candidate.Raw.Age,
		City:               candidate.Raw.City,
		Gender:             candidate.Raw.Gender,
		IsFictional:        candidate.UserKind == domain.UserKindFictional,
		ExternalID:         candidate.ExternalID,
		Summary:            candidate.Enriched.Summary,
		Interests:          candidate.Enriched.Interests,
		Values:             candidate.Enriched.Values,
		RelationshipIntent: candidate.Enriched.RelationshipIntent,
		CommunicationStyle: candidate.Enriched.CommunicationStyle,
		IdealEvening:       candidate.Raw.PromptIdealEvening,
		RelationshipValues: candidate.Raw.PromptRelationshipValues,
		InterestsText:      candidate.Raw.PromptOccupation,
		SharedInterests:    sharedInterests,
		SharedValues:       sharedValues,
	}, nil
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
