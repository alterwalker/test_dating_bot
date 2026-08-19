package profile

import (
	"context"
	"fmt"

	"github.com/alterwalker/test_dating_bot/internal/domain"
	"github.com/alterwalker/test_dating_bot/internal/storage"
	"github.com/google/uuid"
)

type Service struct {
	store *storage.Store
}

func NewService(store *storage.Store) *Service {
	return &Service{store: store}
}

func (s *Service) RegisterTelegramUser(ctx context.Context, telegramID int64, username *string) (domain.User, error) {
	user, err := s.store.UpsertTelegramUser(ctx, telegramID, username)
	if err != nil {
		return domain.User{}, err
	}
	if err := s.store.EnsureProfile(ctx, user.ID); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (s *Service) GetProfile(ctx context.Context, userID uuid.UUID) (domain.Profile, error) {
	return s.store.GetProfile(ctx, userID)
}

func (s *Service) UpdateRaw(ctx context.Context, userID uuid.UUID, raw domain.RawProfile) (domain.Profile, error) {
	if err := s.store.EnsureProfile(ctx, userID); err != nil {
		return domain.Profile{}, err
	}
	return s.store.UpdateRawProfile(ctx, userID, raw)
}

func (s *Service) StartEnrich(ctx context.Context, userID uuid.UUID) (uuid.UUID, domain.ProfileStatus, error) {
	prof, err := s.store.GetProfile(ctx, userID)
	if err != nil {
		return uuid.Nil, "", err
	}
	if !prof.Raw.ValidForEnrich() {
		return uuid.Nil, prof.Status, fmt.Errorf("profile incomplete")
	}
	if prof.Status == domain.ProfileProcessing {
		return uuid.Nil, prof.Status, storage.ErrAlreadyProcessing
	}
	if err := s.store.SetProfileProcessing(ctx, userID); err != nil {
		return uuid.Nil, prof.Status, err
	}
	jobID, err := s.store.CreateJob(ctx, domain.JobEnrichProfile, map[string]any{"user_id": userID.String()})
	if err != nil {
		return uuid.Nil, prof.Status, err
	}
	return jobID, domain.ProfileProcessing, nil
}

func (s *Service) Confirm(ctx context.Context, userID uuid.UUID) (domain.Profile, error) {
	return s.store.ConfirmProfile(ctx, userID)
}

func (s *Service) DeleteProfile(ctx context.Context, userID uuid.UUID) error {
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	if user.Kind != domain.UserKindTelegram {
		return storage.ErrConflict
	}
	return s.store.ResetTelegramProfile(ctx, userID)
}
