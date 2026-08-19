package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/alterwalker/test_dating_bot/internal/ai"
	"github.com/alterwalker/test_dating_bot/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrAlreadyProcessing = errors.New("profile already processing")
)

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) UpsertTelegramUser(ctx context.Context, telegramID int64, username *string) (domain.User, error) {
	const q = `
INSERT INTO users (user_kind, telegram_id, username)
VALUES ('telegram', $1, $2)
ON CONFLICT (telegram_id) DO UPDATE SET username = COALESCE(EXCLUDED.username, users.username)
RETURNING id, user_kind, telegram_id, external_id, username, created_at`

	row := s.pool.QueryRow(ctx, q, telegramID, username)
	return scanUser(row)
}

func (s *Store) GetUser(ctx context.Context, id uuid.UUID) (domain.User, error) {
	const q = `SELECT id, user_kind, telegram_id, external_id, username, created_at FROM users WHERE id = $1`
	row := s.pool.QueryRow(ctx, q, id)
	u, err := scanUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) UpsertFictionalUser(ctx context.Context, externalID string) (domain.User, error) {
	const q = `
INSERT INTO users (user_kind, external_id)
VALUES ('fictional', $1)
ON CONFLICT (external_id) DO UPDATE SET external_id = EXCLUDED.external_id
RETURNING id, user_kind, telegram_id, external_id, username, created_at`

	row := s.pool.QueryRow(ctx, q, externalID)
	return scanUser(row)
}

func (s *Store) GetProfile(ctx context.Context, userID uuid.UUID) (domain.Profile, error) {
	const q = `
SELECT user_id, status, raw, enriched, confirmed_at, embedding_model
FROM profiles WHERE user_id = $1`

	var p domain.Profile
	var rawJSON, enrichedJSON []byte
	var status string
	row := s.pool.QueryRow(ctx, q, userID)
	if err := row.Scan(&p.UserID, &status, &rawJSON, &enrichedJSON, &p.ConfirmedAt, &p.EmbeddingModel); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Profile{}, ErrNotFound
		}
		return domain.Profile{}, err
	}
	p.Status = domain.ProfileStatus(status)
	if err := json.Unmarshal(rawJSON, &p.Raw); err != nil {
		return domain.Profile{}, err
	}
	if len(enrichedJSON) > 0 && string(enrichedJSON) != "null" {
		var enriched domain.EnrichedProfile
		if err := json.Unmarshal(enrichedJSON, &enriched); err != nil {
			return domain.Profile{}, err
		}
		p.Enriched = &enriched
	}
	return p, nil
}

func (s *Store) EnsureProfile(ctx context.Context, userID uuid.UUID) error {
	const q = `INSERT INTO profiles (user_id) VALUES ($1) ON CONFLICT (user_id) DO NOTHING`
	_, err := s.pool.Exec(ctx, q, userID)
	return err
}

func (s *Store) UpdateRawProfile(ctx context.Context, userID uuid.UUID, raw domain.RawProfile) (domain.Profile, error) {
	current, err := s.GetProfile(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		if err := s.EnsureProfile(ctx, userID); err != nil {
			return domain.Profile{}, err
		}
		current = domain.Profile{UserID: userID, Status: domain.ProfileDraft, Raw: domain.RawProfile{}}
	} else if err != nil {
		return domain.Profile{}, err
	}

	merged := current.Raw.Merge(raw)
	rawJSON, err := json.Marshal(merged)
	if err != nil {
		return domain.Profile{}, err
	}

	const q = `
UPDATE profiles SET raw = $2, updated_at = now()
WHERE user_id = $1
RETURNING user_id, status, raw, enriched, confirmed_at, embedding_model`

	row := s.pool.QueryRow(ctx, q, userID, rawJSON)
	return scanProfile(row)
}

func (s *Store) SetProfileProcessing(ctx context.Context, userID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
UPDATE profiles SET status = 'processing', updated_at = now()
WHERE user_id = $1 AND status IN ('draft', 'ready', 'confirmed')`, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		prof, err := s.GetProfile(ctx, userID)
		if err != nil {
			return err
		}
		if prof.Status == domain.ProfileProcessing {
			return ErrAlreadyProcessing
		}
		return ErrConflict
	}
	return nil
}

func (s *Store) SaveEnrichedProfile(ctx context.Context, userID uuid.UUID, enriched domain.EnrichedProfile, embedding []float32, model string) error {
	enrichedJSON, err := json.Marshal(enriched)
	if err != nil {
		return err
	}
	vec := pgvector.NewVector(embedding)
	now := time.Now()

	_, err = s.pool.Exec(ctx, `
UPDATE profiles
SET enriched = $2, embedding = $3, embedding_model = $4, embedded_at = $5,
    status = 'ready', updated_at = now()
WHERE user_id = $1 AND status = 'processing'`, userID, enrichedJSON, vec, model, now)
	return err
}

func (s *Store) ResetTelegramProfile(ctx context.Context, userID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	user, err := scanUser(tx.QueryRow(ctx, `SELECT id, user_kind, telegram_id, external_id, username, created_at FROM users WHERE id = $1`, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if user.Kind != domain.UserKindTelegram {
		return ErrConflict
	}

	_, err = tx.Exec(ctx, `
DELETE FROM jobs
WHERE type = 'enrich_profile'
  AND payload->>'user_id' = $1
  AND status IN ('pending', 'running')`, userID.String())
	if err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `
UPDATE profiles SET
  status = 'draft',
  raw = '{}',
  enriched = NULL,
  embedding = NULL,
  embedding_model = NULL,
  embedded_at = NULL,
  confirmed_at = NULL,
  updated_at = now()
WHERE user_id = $1`, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err := tx.Exec(ctx, `INSERT INTO profiles (user_id) VALUES ($1) ON CONFLICT (user_id) DO NOTHING`, userID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (s *Store) ConfirmProfile(ctx context.Context, userID uuid.UUID) (domain.Profile, error) {
	row := s.pool.QueryRow(ctx, `
UPDATE profiles SET status = 'confirmed', confirmed_at = now(), updated_at = now()
WHERE user_id = $1 AND status = 'ready'
RETURNING user_id, status, raw, enriched, confirmed_at, embedding_model`, userID)
	p, err := scanProfile(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Profile{}, ErrConflict
	}
	return p, err
}

func (s *Store) CreateJob(ctx context.Context, jobType domain.JobType, payload map[string]any) (uuid.UUID, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = s.pool.QueryRow(ctx, `
INSERT INTO jobs (type, payload) VALUES ($1, $2) RETURNING id`,
		string(jobType), payloadJSON).Scan(&id)
	return id, err
}

func (s *Store) ClaimJob(ctx context.Context) (domain.Job, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Job{}, err
	}
	defer tx.Rollback(ctx)

	const q = `
SELECT id, type, payload, status, attempts, max_attempts, last_error, run_after
FROM jobs
WHERE status = 'pending' AND run_after <= now()
ORDER BY created_at
FOR UPDATE SKIP LOCKED
LIMIT 1`

	row := tx.QueryRow(ctx, q)
	job, err := scanJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Job{}, ErrNotFound
	}
	if err != nil {
		return domain.Job{}, err
	}

	_, err = tx.Exec(ctx, `UPDATE jobs SET status = 'running', attempts = attempts + 1, updated_at = now() WHERE id = $1`, job.ID)
	if err != nil {
		return domain.Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Job{}, err
	}
	job.Status = domain.JobRunning
	return job, nil
}

func (s *Store) FinishJob(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE jobs SET status = 'done', updated_at = now() WHERE id = $1`, id)
	return err
}

func (s *Store) FailJob(ctx context.Context, id uuid.UUID, jobErr error, attempts, maxAttempts int) error {
	runAfter := time.Now().Add(time.Duration(attempts*attempts) * time.Second)
	status := domain.JobPending
	if attempts >= maxAttempts {
		status = domain.JobFailed
	}
	msg := jobErr.Error()
	_, err := s.pool.Exec(ctx, `
UPDATE jobs SET status = $2, last_error = $3, run_after = $4, updated_at = now()
WHERE id = $1`, id, string(status), msg, runAfter)
	return err
}

func (s *Store) GetCandidate(ctx context.Context, userID uuid.UUID) (domain.CandidateRow, error) {
	return s.getCandidateRow(ctx, userID)
}

func (s *Store) GetViewerCandidate(ctx context.Context, viewerID, candidateID uuid.UUID) (domain.CandidateRow, domain.CandidateRow, error) {
	viewer, err := s.getCandidateRow(ctx, viewerID)
	if err != nil {
		return domain.CandidateRow{}, domain.CandidateRow{}, err
	}
	candidate, err := s.getCandidateRow(ctx, candidateID)
	if err != nil {
		return domain.CandidateRow{}, domain.CandidateRow{}, err
	}
	return viewer, candidate, nil
}

func (s *Store) HideMatch(ctx context.Context, viewerID, candidateID uuid.UUID) error {
	if viewerID == candidateID {
		return ErrConflict
	}
	_, err := s.pool.Exec(ctx, `
INSERT INTO match_hides (viewer_id, candidate_id)
VALUES ($1, $2)
ON CONFLICT (viewer_id, candidate_id) DO NOTHING`, viewerID, candidateID)
	return err
}

func (s *Store) ListHiddenCandidateIDs(ctx context.Context, viewerID uuid.UUID) (map[uuid.UUID]struct{}, error) {
	const q = `SELECT candidate_id FROM match_hides WHERE viewer_id = $1`
	rows, err := s.pool.Query(ctx, q, viewerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[uuid.UUID]struct{}{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

func (s *Store) ListConfirmedCandidates(ctx context.Context, viewerID uuid.UUID, demoMode bool, retrievalMode string, topK int) ([]domain.CandidateRow, error) {
	viewer, err := s.getCandidateRow(ctx, viewerID)
	if err != nil {
		return nil, err
	}

	if retrievalMode == "ann" && len(viewer.Embedding) > 0 {
		return s.listCandidatesANN(ctx, viewer, demoMode, topK)
	}
	return s.listCandidatesAll(ctx, viewerID, demoMode)
}

func (s *Store) listCandidatesANN(ctx context.Context, viewer domain.CandidateRow, demoMode bool, topK int) ([]domain.CandidateRow, error) {
	vec := pgvector.NewVector(viewer.Embedding)
	kindFilter := ""
	if !demoMode {
		kindFilter = "AND u.user_kind = 'telegram'"
	}

	q := fmt.Sprintf(`
SELECT u.id, u.user_kind, u.external_id, p.raw, p.enriched, p.embedding
FROM profiles p
JOIN users u ON u.id = p.user_id
WHERE p.status = 'confirmed'
  AND p.user_id != $1
  AND p.embedding IS NOT NULL
  AND lower(p.raw->>'city') = lower($2)
  %s
ORDER BY p.embedding <=> $3
LIMIT $4`, kindFilter)

	rows, err := s.pool.Query(ctx, q, viewer.UserID, viewer.Raw.City, vec, topK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanCandidateRows(rows)
}

func (s *Store) listCandidatesAll(ctx context.Context, viewerID uuid.UUID, demoMode bool) ([]domain.CandidateRow, error) {
	kindFilter := ""
	if !demoMode {
		kindFilter = "AND u.user_kind = 'telegram'"
	}
	q := fmt.Sprintf(`
SELECT u.id, u.user_kind, u.external_id, p.raw, p.enriched, p.embedding
FROM profiles p
JOIN users u ON u.id = p.user_id
WHERE p.status = 'confirmed' AND p.user_id != $1 %s`, kindFilter)

	rows, err := s.pool.Query(ctx, q, viewerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCandidateRows(rows)
}

func (s *Store) getCandidateRow(ctx context.Context, userID uuid.UUID) (domain.CandidateRow, error) {
	const q = `
SELECT u.id, u.user_kind, u.external_id, p.raw, p.enriched, p.embedding
FROM profiles p
JOIN users u ON u.id = p.user_id
WHERE p.user_id = $1`

	var row domain.CandidateRow
	var rawJSON, enrichedJSON []byte
	var vec *pgvector.Vector
	var kind string
	err := s.pool.QueryRow(ctx, q, userID).Scan(&row.UserID, &kind, &row.ExternalID, &rawJSON, &enrichedJSON, &vec)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CandidateRow{}, ErrNotFound
	}
	if err != nil {
		return domain.CandidateRow{}, err
	}
	row.UserKind = domain.UserKind(kind)
	if err := json.Unmarshal(rawJSON, &row.Raw); err != nil {
		return domain.CandidateRow{}, err
	}
	if err := json.Unmarshal(enrichedJSON, &row.Enriched); err != nil {
		return domain.CandidateRow{}, err
	}
	if vec != nil {
		row.Embedding = vec.Slice()
	}
	return row, nil
}

func (s *Store) InsertFictionalProfile(ctx context.Context, externalID string, raw domain.RawProfile, enriched domain.EnrichedProfile, embedding []float32, model string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var userID uuid.UUID
	err = tx.QueryRow(ctx, `
INSERT INTO users (user_kind, external_id)
VALUES ('fictional', $1)
ON CONFLICT (external_id) DO UPDATE SET external_id = EXCLUDED.external_id
RETURNING id`, externalID).Scan(&userID)
	if err != nil {
		return err
	}

	rawJSON, _ := json.Marshal(raw)
	enrichedJSON, _ := json.Marshal(enriched)
	vec := pgvector.NewVector(embedding)
	now := time.Now()

	_, err = tx.Exec(ctx, `
INSERT INTO profiles (user_id, status, raw, enriched, embedding, embedding_model, embedded_at, confirmed_at)
VALUES ($1, 'confirmed', $2, $3, $4, $5, $6, $6)
ON CONFLICT (user_id) DO UPDATE SET
  raw = EXCLUDED.raw,
  enriched = EXCLUDED.enriched,
  embedding = EXCLUDED.embedding,
  embedding_model = EXCLUDED.embedding_model,
  embedded_at = EXCLUDED.embedded_at,
  status = 'confirmed',
  confirmed_at = EXCLUDED.confirmed_at,
  updated_at = now()`, userID, rawJSON, enrichedJSON, vec, model, now)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func scanUser(row pgx.Row) (domain.User, error) {
	var u domain.User
	var kind string
	err := row.Scan(&u.ID, &kind, &u.TelegramID, &u.ExternalID, &u.Username, &u.CreatedAt)
	u.Kind = domain.UserKind(kind)
	return u, err
}

func scanProfile(row pgx.Row) (domain.Profile, error) {
	var p domain.Profile
	var rawJSON, enrichedJSON []byte
	var status string
	err := row.Scan(&p.UserID, &status, &rawJSON, &enrichedJSON, &p.ConfirmedAt, &p.EmbeddingModel)
	if err != nil {
		return domain.Profile{}, err
	}
	p.Status = domain.ProfileStatus(status)
	if err := json.Unmarshal(rawJSON, &p.Raw); err != nil {
		return domain.Profile{}, err
	}
	if len(enrichedJSON) > 0 && string(enrichedJSON) != "null" {
		var enriched domain.EnrichedProfile
		if err := json.Unmarshal(enrichedJSON, &enriched); err != nil {
			return domain.Profile{}, err
		}
		p.Enriched = &enriched
	}
	return p, nil
}

func scanJob(row pgx.Row) (domain.Job, error) {
	var job domain.Job
	var jobType, status string
	var payloadJSON []byte
	err := row.Scan(&job.ID, &jobType, &payloadJSON, &status, &job.Attempts, &job.MaxAttempts, &job.LastError, &job.RunAfter)
	if err != nil {
		return domain.Job{}, err
	}
	job.Type = domain.JobType(jobType)
	job.Status = domain.JobStatus(status)
	_ = json.Unmarshal(payloadJSON, &job.Payload)
	return job, nil
}

func scanCandidateRows(rows pgx.Rows) ([]domain.CandidateRow, error) {
	var out []domain.CandidateRow
	for rows.Next() {
		var row domain.CandidateRow
		var rawJSON, enrichedJSON []byte
		var vec *pgvector.Vector
		var kind string
		if err := rows.Scan(&row.UserID, &kind, &row.ExternalID, &rawJSON, &enrichedJSON, &vec); err != nil {
			return nil, err
		}
		row.UserKind = domain.UserKind(kind)
		if err := json.Unmarshal(rawJSON, &row.Raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(enrichedJSON, &row.Enriched); err != nil {
			return nil, err
		}
		if vec != nil {
			row.Embedding = vec.Slice()
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) AdminCityStats(ctx context.Context, limit int) (domain.AdminStatsResponse, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `
SELECT
  MAX(p.raw->>'city') AS city,
  COUNT(*) FILTER (WHERE p.raw->>'gender' = 'male') AS male,
  COUNT(*) FILTER (WHERE p.raw->>'gender' = 'female') AS female,
  COUNT(*) AS total
FROM profiles p
WHERE p.status = 'confirmed'
  AND COALESCE(TRIM(p.raw->>'city'), '') <> ''
GROUP BY lower(TRIM(p.raw->>'city'))
ORDER BY total DESC
LIMIT $1`

	rows, err := s.pool.Query(ctx, q, limit)
	if err != nil {
		return domain.AdminStatsResponse{}, err
	}
	defer rows.Close()

	var resp domain.AdminStatsResponse
	for rows.Next() {
		var row domain.AdminCityStats
		if err := rows.Scan(&row.City, &row.Male, &row.Female, &row.Total); err != nil {
			return domain.AdminStatsResponse{}, err
		}
		resp.Cities = append(resp.Cities, row)
	}
	if err := rows.Err(); err != nil {
		return domain.AdminStatsResponse{}, err
	}

	const totalQ = `SELECT COUNT(*) FROM profiles WHERE status = 'confirmed'`
	if err := s.pool.QueryRow(ctx, totalQ).Scan(&resp.TotalConfirmed); err != nil {
		return domain.AdminStatsResponse{}, err
	}

	if err := s.loadTokenUsage(ctx, &resp); err != nil {
		return domain.AdminStatsResponse{}, err
	}

	return resp, nil
}

func (s *Store) RecordTokenUsage(ctx context.Context, rec ai.TokenUsageRecord) error {
	const q = `
INSERT INTO ai_token_usage (model, operation, source, prompt_tokens, completion_tokens, total_tokens, request_count, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, 1, now())
ON CONFLICT (model, operation, source) DO UPDATE SET
  prompt_tokens = ai_token_usage.prompt_tokens + EXCLUDED.prompt_tokens,
  completion_tokens = ai_token_usage.completion_tokens + EXCLUDED.completion_tokens,
  total_tokens = ai_token_usage.total_tokens + EXCLUDED.total_tokens,
  request_count = ai_token_usage.request_count + 1,
  updated_at = now()`
	_, err := s.pool.Exec(ctx, q,
		rec.Model, rec.Operation, rec.Source,
		rec.PromptTokens, rec.CompletionTokens, rec.TotalTokens,
	)
	return err
}

func (s *Store) loadTokenUsage(ctx context.Context, resp *domain.AdminStatsResponse) error {
	const q = `
SELECT model, operation, source, prompt_tokens, completion_tokens, total_tokens, request_count
FROM ai_token_usage
ORDER BY total_tokens DESC`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()

	byModel := map[string]*domain.AITokenUsageByModel{}
	for rows.Next() {
		var row domain.AITokenUsageRow
		if err := rows.Scan(
			&row.Model, &row.Operation, &row.Source,
			&row.PromptTokens, &row.CompletionTokens, &row.TotalTokens, &row.RequestCount,
		); err != nil {
			return err
		}
		resp.TokenUsage = append(resp.TokenUsage, row)

		m := byModel[row.Model]
		if m == nil {
			m = &domain.AITokenUsageByModel{Model: row.Model}
			byModel[row.Model] = m
		}
		m.PromptTokens += row.PromptTokens
		m.CompletionTokens += row.CompletionTokens
		m.TotalTokens += row.TotalTokens
		m.RequestCount += row.RequestCount
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, m := range byModel {
		resp.TokenByModel = append(resp.TokenByModel, *m)
	}
	sort.Slice(resp.TokenByModel, func(i, j int) bool {
		return resp.TokenByModel[i].TotalTokens > resp.TokenByModel[j].TotalTokens
	})
	return nil
}
