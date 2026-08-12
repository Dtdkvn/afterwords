package server

import (
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yewud/afterwords/internal/mail"
	"github.com/yewud/afterwords/internal/model"
	"github.com/yewud/afterwords/internal/store"
)

type switchInput struct {
	Name         string                 `json:"name"`
	IntervalDays int                    `json:"intervalDays"`
	GraceDays    int                    `json:"graceDays"`
	Reminders    model.ReminderSchedule `json:"reminders"`
}

func validateSwitchInput(body *switchInput) error {
	body.Name = strings.TrimSpace(body.Name)
	if !validDisplayName(body.Name, 80) {
		return errors.New("name must be between 1 and 80 characters")
	}
	if body.IntervalDays < 1 || body.IntervalDays > 365 {
		return errors.New("check-in interval must be between 1 and 365 days")
	}
	if body.GraceDays < 0 || body.GraceDays > 90 {
		return errors.New("grace period must be between 0 and 90 days")
	}
	if len(body.Reminders.BeforeDays) == 0 && len(body.Reminders.OverdueDays) == 0 {
		body.Reminders = model.DefaultReminderSchedule()
	}
	for _, collection := range [][]int{body.Reminders.BeforeDays, body.Reminders.OverdueDays} {
		seen := make(map[int]bool)
		for _, value := range collection {
			if value < 0 || value > 365 || seen[value] {
				return errors.New("reminder offsets must be unique values between 0 and 365")
			}
			seen[value] = true
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(body.Reminders.BeforeDays)))
	sort.Ints(body.Reminders.OverdueDays)
	return nil
}

func (s *Server) listSwitches(w http.ResponseWriter, r *http.Request) {
	values, err := s.store.ListSwitches(r.Context(), userFrom(r).ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "Could not load switches.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"switches": values})
}

func (s *Server) createSwitch(w http.ResponseWriter, r *http.Request) {
	var body switchInput
	if err := decodeStrict(w, r, &body, 32*1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "The switch settings are invalid.")
		return
	}
	if err := validateSwitchInput(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_switch", err.Error())
		return
	}
	value, token, err := s.store.CreateSwitch(r.Context(), userFrom(r).ID, store.CreateSwitchParams{
		Name: body.Name, IntervalDays: body.IntervalDays, GraceDays: body.GraceDays, Reminders: body.Reminders,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create_failed", "Could not create the switch.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"switch": value, "apiToken": token})
}

func (s *Server) getSwitch(w http.ResponseWriter, r *http.Request) {
	value, err := s.ownedSwitch(r)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"switch": value})
}

func (s *Server) updateSwitch(w http.ResponseWriter, r *http.Request) {
	if _, err := s.ownedSwitch(r); err != nil {
		writeStoreError(w, err)
		return
	}
	var body switchInput
	if err := decodeStrict(w, r, &body, 32*1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "The switch settings are invalid.")
		return
	}
	if err := validateSwitchInput(&body); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_switch", err.Error())
		return
	}
	value, err := s.store.UpdateSwitchSettings(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"), body.Name, body.IntervalDays, body.GraceDays, body.Reminders)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"switch": value})
}

func (s *Server) deleteSwitch(w http.ResponseWriter, r *http.Request) {
	err := s.store.DeleteSwitch(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) sessionCheckin(w http.ResponseWriter, r *http.Request) {
	value, err := s.ownedSwitch(r)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	value, err = s.store.CheckIn(r.Context(), value.ID, "dashboard")
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"switch": value})
}

func (s *Server) pauseSwitch(w http.ResponseWriter, r *http.Request) {
	value, err := s.store.Pause(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"switch": value})
}

func (s *Server) resumeSwitch(w http.ResponseWriter, r *http.Request) {
	value, err := s.store.Resume(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"switch": value})
}

func (s *Server) rotateAPIToken(w http.ResponseWriter, r *http.Request) {
	token, err := s.store.RotateAPIToken(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"apiToken": token})
}

func (s *Server) sendMagicLink(w http.ResponseWriter, r *http.Request) {
	value, err := s.ownedSwitch(r)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	token, err := s.store.CreateMagicToken(r.Context(), userFrom(r).ID, value.ID, 48*time.Hour)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	htmlBody, textBody, err := mail.Render("checkin_reminder", mail.TemplateData{SwitchName: value.Name,
		Deadline: value.NextDeadline.Format("January 2, 2006 at 15:04 UTC"), CheckinURL: s.baseURL + "/c/" + token})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "template_failed", "Could not prepare the check-in email.")
		return
	}
	_, err = s.store.EnqueueMessage(r.Context(), model.OutboxMessage{SwitchID: value.ID, Kind: "magic_checkin", ToEmail: userFrom(r).Email,
		Subject: "Your secure Afterwords check-in link", BodyHTML: htmlBody, BodyText: textBody})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "outbox_failed", "Could not queue the check-in email.")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": true})
}

func (s *Server) fireDrill(w http.ResponseWriter, r *http.Request) {
	value, err := s.ownedSwitch(r)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	token, err := s.store.FireDrillToken(r.Context(), userFrom(r).ID, value.ID, 24*time.Hour)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	releaseURL := s.baseURL + "/r/" + token + "?test=1"
	htmlBody, textBody, err := mail.Render("fire_drill_owner", mail.TemplateData{SwitchName: value.Name, ReleaseURL: releaseURL, Test: true})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "template_failed", "Could not prepare the fire drill.")
		return
	}
	_, err = s.store.EnqueueMessage(r.Context(), model.OutboxMessage{SwitchID: value.ID, Kind: "fire_drill_owner", ToEmail: userFrom(r).Email,
		Subject: "[TEST] Afterwords fire drill — " + value.Name, BodyHTML: htmlBody, BodyText: textBody})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "outbox_failed", "Could not queue the fire drill.")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": true, "previewURL": releaseURL})
}

func (s *Server) ownedSwitch(r *http.Request) (model.Switch, error) {
	return s.store.SwitchByID(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"))
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "The requested item was not found.")
	case errors.Is(err, store.ErrInvalidState):
		writeError(w, http.StatusConflict, "invalid_state", "That action is not available in the current switch state.")
	case errors.Is(err, store.ErrQuotaExceeded):
		writeError(w, http.StatusRequestEntityTooLarge, "vault_quota", "This switch has reached its 25 MB encrypted vault limit.")
	case errors.Is(err, store.ErrReservation):
		writeError(w, http.StatusConflict, "invalid_reservation", "Reserve a fresh vault item and try again.")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "That entry conflicts with an existing recipient or trustee.")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "The request could not be completed.")
	}
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.ListAudit(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"), 100)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (s *Server) retryDeadLetter(w http.ResponseWriter, r *http.Request) {
	err := s.store.RetryDeadLetter(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"), chi.URLParam(r, "messageID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": true})
}
