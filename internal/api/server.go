package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/alterwalker/test_dating_bot/internal/domain"
	"github.com/alterwalker/test_dating_bot/internal/matching"
	"github.com/alterwalker/test_dating_bot/internal/profile"
	"github.com/alterwalker/test_dating_bot/internal/storage"
	"github.com/google/uuid"
)

type Server struct {
	profiles *profile.Service
	matches  *matching.Service
	store    *storage.Store
}

func NewServer(profiles *profile.Service, matches *matching.Service, store *storage.Store) *Server {
	return &Server{profiles: profiles, matches: matches, store: store}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /v1/users", s.handleCreateUser)
	mux.HandleFunc("GET /v1/users/{id}/profile", s.handleGetProfile)
	mux.HandleFunc("PUT /v1/users/{id}/profile/raw", s.handlePutRaw)
	mux.HandleFunc("POST /v1/users/{id}/profile/enrich", s.handleEnrich)
	mux.HandleFunc("GET /v1/users/{id}/profile/status", s.handleProfileStatus)
	mux.HandleFunc("POST /v1/users/{id}/profile/confirm", s.handleConfirm)
	mux.HandleFunc("DELETE /v1/users/{id}/profile", s.handleDeleteProfile)
	mux.HandleFunc("GET /v1/admin/stats/cities", s.handleAdminCityStats)
	mux.HandleFunc("GET /v1/users/{id}/matches", s.handleMatches)
	mux.HandleFunc("GET /v1/users/{id}/matches/{candidate_id}", s.handleMatchCandidate)
	mux.HandleFunc("POST /v1/users/{id}/matches/{candidate_id}/icebreaker", s.handleIcebreaker)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TelegramID int64   `json:"telegram_id"`
		Username   *string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	user, err := s.profiles.RegisterTelegramUser(r.Context(), req.TelegramID, req.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	prof, err := s.profiles.GetProfile(r.Context(), id)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, prof)
}

func (s *Server) handlePutRaw(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var raw domain.RawProfile
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	prof, err := s.profiles.UpdateRaw(r.Context(), id, raw)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, prof)
}

func (s *Server) handleEnrich(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	jobID, status, err := s.profiles.StartEnrich(r.Context(), id)
	if errors.Is(err, storage.ErrAlreadyProcessing) {
		writeError(w, http.StatusConflict, "already processing")
		return
	}
	if err != nil {
		if strings.Contains(err.Error(), "incomplete") {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": status, "job_id": jobID})
}

func (s *Server) handleProfileStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	prof, err := s.profiles.GetProfile(r.Context(), id)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": prof.Status, "error": nil})
}

func (s *Server) handleConfirm(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	prof, err := s.profiles.Confirm(r.Context(), id)
	if errors.Is(err, storage.ErrConflict) {
		writeError(w, http.StatusConflict, "profile not ready")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, prof)
}

func (s *Server) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := s.profiles.DeleteProfile(r.Context(), id); errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if errors.Is(err, storage.ErrConflict) {
		writeError(w, http.StatusConflict, "cannot delete fictional profile")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminCityStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.AdminCityStats(r.Context(), 10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleMatches(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	limit := 10
	matches, total, err := s.matches.FindMatches(r.Context(), id, limit)
	if errors.Is(err, storage.ErrConflict) {
		writeError(w, http.StatusConflict, "profile not confirmed")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":                      id,
		"matches":                      matches,
		"total_candidates_after_filters": total,
	})
}

func (s *Server) handleMatchCandidate(w http.ResponseWriter, r *http.Request) {
	viewerID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	candidateID, err := uuid.Parse(r.PathValue("candidate_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid candidate id")
		return
	}
	profile, err := s.matches.GetCandidateProfile(r.Context(), viewerID, candidateID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, storage.ErrConflict) {
		writeError(w, http.StatusConflict, "profile not confirmed")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (s *Server) handleIcebreaker(w http.ResponseWriter, r *http.Request) {
	viewerID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	candidateID, err := uuid.Parse(r.PathValue("candidate_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid candidate id")
		return
	}
	result, err := s.matches.Icebreaker(r.Context(), viewerID, candidateID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, storage.ErrConflict) {
		writeError(w, http.StatusConflict, "profile not confirmed")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
