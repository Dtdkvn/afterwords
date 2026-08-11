package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/yewud/afterwords/internal/auth"
	"github.com/yewud/afterwords/internal/store"
)

const sessionTTL = 30 * 24 * time.Hour

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "The database is unavailable.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": s.clock.Now()})
}

func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) {
	required, err := s.store.SetupRequired(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "Could not inspect setup state.")
		return
	}
	response := map[string]any{"setupRequired": required, "setupTokenRequired": required && s.setupToken != "", "authenticated": false}
	if cookie, err := r.Cookie(sessionCookie); err == nil && cookie.Value != "" {
		if session, err := s.store.Session(r.Context(), cookie.Value); err == nil {
			if user, err := s.store.UserByID(r.Context(), session.UserID); err == nil {
				response["authenticated"] = true
				response["user"] = user
				response["csrfToken"] = session.CSRFToken
			}
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email      string `json:"email"`
		Password   string `json:"password"`
		SetupToken string `json:"setupToken"`
	}
	if err := decodeStrict(w, r, &body, 16*1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Use only email, password, and optional setupToken fields.")
		return
	}
	if s.setupToken != "" && !equalSecret(body.SetupToken, s.setupToken) {
		writeError(w, http.StatusForbidden, "setup_token_required", "Enter the setup token configured by the instance administrator.")
		return
	}
	if !validEmail(body.Email) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_email", "Enter a valid email address.")
		return
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "weak_password", err.Error())
		return
	}
	user, err := s.store.CreateOwner(r.Context(), body.Email, hash)
	if errors.Is(err, store.ErrAlreadySetup) {
		writeError(w, http.StatusConflict, "already_setup", "Afterwords already has an owner account.")
		return
	}
	if err != nil {
		s.logger.Error("create owner", "error", err)
		writeError(w, http.StatusInternalServerError, "setup_failed", "Could not create the owner account.")
		return
	}
	session, err := s.store.CreateSession(r.Context(), user.ID, sessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "session_failed", "The account was created, but sign-in failed.")
		return
	}
	s.setSessionCookie(w, session.ID, session.ExpiresAt)
	writeJSON(w, http.StatusCreated, map[string]any{"user": user, "csrfToken": session.CSRFToken})
}

func equalSecret(provided, expected string) bool {
	providedHash := sha256.Sum256([]byte(provided))
	expectedHash := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(providedHash[:], expectedHash[:]) == 1
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeStrict(w, r, &body, 16*1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Use only email and password fields.")
		return
	}
	user, hash, err := s.store.UserCredentials(r.Context(), strings.TrimSpace(body.Email))
	if err != nil {
		// Perform comparable Argon2 work to reduce account-enumeration timing differences.
		_, _ = auth.VerifyPassword(body.Password, s.dummyAuthHash)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Email or password is incorrect.")
		return
	}
	matched, err := auth.VerifyPassword(body.Password, hash)
	if err != nil || !matched {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Email or password is incorrect.")
		return
	}
	session, err := s.store.CreateSession(r.Context(), user.ID, sessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "session_failed", "Could not create a session.")
		return
	}
	s.setSessionCookie(w, session.ID, session.ExpiresAt)
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "csrfToken": session.CSRFToken})
}

func (s *Server) currentSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"user": userFrom(r), "csrfToken": sessionFrom(r).CSRFToken})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteSession(r.Context(), sessionFrom(r).ID); err != nil {
		writeError(w, http.StatusInternalServerError, "logout_failed", "Could not end this session. Please try again.")
		return
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}
