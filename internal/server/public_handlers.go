package server

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/yewud/afterwords/internal/store"
)

func (s *Server) magicCheckinConfirmation(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	action := html.EscapeString("/c/" + url.PathEscape(token))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Confirm check-in · Afterwords</title><body style="margin:0;background:#f5f1ea;color:#24312d;font-family:system-ui"><main style="max-width:42rem;margin:12vh auto;padding:2rem"><p style="color:#2f766d;font-weight:800;letter-spacing:.12em;text-transform:uppercase">Afterwords</p><h1 style="font:700 3rem Georgia,serif">Ready to check in?</h1><p style="font-size:1.15rem;line-height:1.7">Confirm below to reset your deadline. Merely opening this email link does not change your switch.</p><form method="post" action="%s"><button type="submit" style="border:0;margin-top:1rem;padding:.9rem 1.3rem;border-radius:999px;background:#2f766d;color:white;font:700 1rem system-ui;cursor:pointer">Confirm I’m here</button></form></main></body></html>`, action)
}

func (s *Server) magicCheckin(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	value, err := s.store.CheckInWithMagicToken(r.Context(), token)
	if errors.Is(err, store.ErrInvalidState) {
		s.magicResult(w, "This switch can no longer be reset", "The switch is paused or has already released. Sign in to review its status.", false)
		return
	}
	if err != nil {
		s.magicResult(w, "We could not complete that check-in", "Please sign in and use the dashboard instead.", false)
		return
	}
	s.magicResult(w, "You're checked in", fmt.Sprintf("%s is safe until %s.", html.EscapeString(value.Name), value.NextDeadline.Format("January 2, 2006")), true)
}

func (s *Server) magicResult(w http.ResponseWriter, title, message string, success bool) {
	color := "#a9653a"
	if success {
		color = "#2f766d"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s · Afterwords</title><body style="margin:0;background:#f5f1ea;color:#24312d;font-family:system-ui"><main style="max-width:42rem;margin:12vh auto;padding:2rem"><p style="color:%s;font-weight:800;letter-spacing:.12em;text-transform:uppercase">Afterwords</p><h1 style="font:700 3rem Georgia,serif">%s</h1><p style="font-size:1.15rem;line-height:1.7">%s</p><a href="/" style="display:inline-block;margin-top:1rem;padding:.85rem 1.2rem;border-radius:999px;background:%s;color:white;text-decoration:none;font-weight:700">Open dashboard</a></main></body></html>`, html.EscapeString(title), color, html.EscapeString(title), message, color)
}

func (s *Server) apiCheckin(w http.ResponseWriter, r *http.Request) {
	switchID, err := s.store.ConsumeCheckinToken(r.Context(), chi.URLParam(r, "token"), "api")
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_token", "The check-in token is invalid.")
		return
	}
	value, err := s.store.CheckIn(r.Context(), switchID, "api_token")
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"checkedIn": true, "state": value.State, "nextDeadline": value.NextDeadline})
}

func (s *Server) releaseBundle(w http.ResponseWriter, r *http.Request) {
	value, testMode, err := s.store.ResolveReleaseToken(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		writeError(w, http.StatusNotFound, "release_not_found", "This release is unavailable or has expired.")
		return
	}
	items, err := s.store.ListVaultItems(r.Context(), value.UserID, value.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bundle_failed", "Could not load the encrypted bundle.")
		return
	}
	trustees, err := s.store.ListTrustees(r.Context(), value.UserID, value.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bundle_failed", "Could not load recovery metadata.")
		return
	}
	publicTrustees := make([]map[string]any, 0, len(trustees))
	for _, trustee := range trustees {
		publicTrustees = append(publicTrustees, map[string]any{"name": trustee.Name, "shareIndex": trustee.ShareIndex, "shareFingerprint": trustee.ShareFingerprint})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"version": 1, "switchId": value.ID, "switchName": value.Name, "threshold": value.RecoveryThreshold,
		"shareCount": value.RecoveryShareCount, "trustees": publicTrustees, "items": items, "testMode": testMode,
	})
}
