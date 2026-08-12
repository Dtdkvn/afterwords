package server

import (
	"encoding/base64"
	"net/http"
	"regexp"
	"strings"

	"github.com/Dtdkvn/afterwords/internal/mail"
	"github.com/Dtdkvn/afterwords/internal/model"
	"github.com/go-chi/chi/v5"
)

var fingerprintPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (s *Server) listVault(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListVaultItems(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) reserveVault(w http.ResponseWriter, r *http.Request) {
	id, err := s.store.ReserveVaultItem(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"itemId": id, "aad": chi.URLParam(r, "switchID") + ":" + id})
}

func (s *Server) putVault(w http.ResponseWriter, r *http.Request) {
	// Deliberately exact: unknown keys such as plaintext, title, passphrase, or share are rejected.
	var body struct {
		IV         string `json:"iv"`
		Ciphertext string `json:"ciphertext"`
		AAD        string `json:"aad"`
		Size       int64  `json:"size"`
	}
	if err := decodeStrict(w, r, &body, 36*1024*1024); err != nil {
		writeError(w, http.StatusBadRequest, "ciphertext_only", "Vault items accept only iv, ciphertext, aad, and size.")
		return
	}
	iv, err := base64.RawURLEncoding.DecodeString(body.IV)
	if err != nil || len(iv) != 12 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_iv", "AES-GCM IV must be 12 base64url-encoded bytes.")
		return
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(body.Ciphertext)
	if err != nil || len(ciphertext) < 16 || int64(len(ciphertext)) != body.Size {
		writeError(w, http.StatusUnprocessableEntity, "invalid_ciphertext", "Ciphertext must be valid base64url and match size.")
		return
	}
	item, err := s.store.PutVaultItem(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"), chi.URLParam(r, "itemID"), body.IV, body.Ciphertext, body.AAD, body.Size)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (s *Server) deleteVault(w http.ResponseWriter, r *http.Request) {
	err := s.store.DeleteVaultItem(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"), chi.URLParam(r, "itemID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getKeyslot(w http.ResponseWriter, r *http.Request) {
	slot, err := s.store.Keyslot(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keyslot": slot})
}

func (s *Server) putKeyslot(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WrappedDK string `json:"wrappedDk"`
		IV        string `json:"iv"`
		Salt      string `json:"salt"`
		KDFParams struct {
			MemorySize  int `json:"memorySize"`
			Iterations  int `json:"iterations"`
			Parallelism int `json:"parallelism"`
			HashLength  int `json:"hashLength"`
		} `json:"kdfParams"`
	}
	if err := decodeStrict(w, r, &body, 64*1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_keyslot", "The wrapped key payload is invalid.")
		return
	}
	wrapped, wrapErr := base64.RawURLEncoding.DecodeString(body.WrappedDK)
	iv, ivErr := base64.RawURLEncoding.DecodeString(body.IV)
	salt, saltErr := base64.RawURLEncoding.DecodeString(body.Salt)
	params := body.KDFParams
	if wrapErr != nil || len(wrapped) != 48 || ivErr != nil || len(iv) != 12 || saltErr != nil || len(salt) < 16 || len(salt) > 64 ||
		params.MemorySize < 8192 || params.MemorySize > 1048576 || params.Iterations < 1 || params.Iterations > 20 ||
		params.Parallelism < 1 || params.Parallelism > 16 || params.HashLength != 32 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_keyslot", "Wrapped key or Argon2id parameters are outside safe bounds.")
		return
	}
	slot, err := s.store.UpsertKeyslot(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"), body.WrappedDK, body.IV, body.Salt,
		map[string]int{"memorySize": params.MemorySize, "iterations": params.Iterations, "parallelism": params.Parallelism, "hashLength": params.HashLength})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keyslot": slot})
}

func (s *Server) listRecipients(w http.ResponseWriter, r *http.Request) {
	people, err := s.store.ListRecipients(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recipients": people})
}

func (s *Server) addRecipient(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	decodeErr := decodeStrict(w, r, &body, 16*1024)
	body.Name = strings.TrimSpace(body.Name)
	if decodeErr != nil || !validDisplayName(body.Name, 100) || !validEmail(body.Email) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_recipient", "Enter a recipient name and valid email.")
		return
	}
	person, err := s.store.AddRecipient(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"), body.Name, body.Email)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"recipient": person})
}

func (s *Server) deleteRecipient(w http.ResponseWriter, r *http.Request) {
	err := s.store.DeleteRecipient(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"), chi.URLParam(r, "personID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listTrustees(w http.ResponseWriter, r *http.Request) {
	people, err := s.store.ListTrustees(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"trustees": people})
}

func (s *Server) addTrustee(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name             string `json:"name"`
		Email            string `json:"email"`
		ShareIndex       int    `json:"shareIndex"`
		ShareFingerprint string `json:"shareFingerprint"`
		Threshold        int    `json:"threshold"`
		ShareCount       int    `json:"shareCount"`
	}
	decodeErr := decodeStrict(w, r, &body, 16*1024)
	body.Name = strings.TrimSpace(body.Name)
	if decodeErr != nil || !validDisplayName(body.Name, 100) || !validEmail(body.Email) ||
		body.ShareIndex < 1 || body.ShareIndex > 255 || !fingerprintPattern.MatchString(strings.ToLower(body.ShareFingerprint)) ||
		body.ShareCount < 1 || body.ShareCount > 255 || body.Threshold < 1 || body.Threshold > body.ShareCount ||
		(body.ShareCount > 1 && body.Threshold < 2) || body.ShareIndex > body.ShareCount {
		writeError(w, http.StatusUnprocessableEntity, "invalid_trustee", "Trustee recovery metadata is invalid.")
		return
	}
	switchID := chi.URLParam(r, "switchID")
	person, err := s.store.AddTrustee(r.Context(), userFrom(r).ID, switchID, body.Name, body.Email, body.ShareIndex,
		strings.ToLower(body.ShareFingerprint), body.Threshold, body.ShareCount)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	current, currentErr := s.store.SwitchByID(r.Context(), userFrom(r).ID, switchID)
	if currentErr != nil {
		writeStoreError(w, currentErr)
		return
	}
	htmlBody, textBody, renderErr := mail.Render("recovery_kit_trustee", mail.TemplateData{TrusteeName: person.Name, SwitchName: current.Name})
	if renderErr != nil {
		writeError(w, http.StatusInternalServerError, "template_failed", "Could not prepare the trustee instructions.")
		return
	}
	if _, err := s.store.ClaimScheduledMessage(r.Context(), switchID, "recovery-kit-instructions:"+person.ID, model.OutboxMessage{
		SwitchID: switchID, Kind: "recovery_kit_trustee", ToEmail: person.Email,
		Subject: "You have an Afterwords Recovery Kit", BodyHTML: htmlBody, BodyText: textBody,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "outbox_failed", "Could not queue the trustee instructions.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"trustee": person})
}

func (s *Server) markKitDelivered(w http.ResponseWriter, r *http.Request) {
	err := s.store.MarkKitDelivered(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"), chi.URLParam(r, "personID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteTrustee(w http.ResponseWriter, r *http.Request) {
	err := s.store.DeleteTrustee(r.Context(), userFrom(r).ID, chi.URLParam(r, "switchID"), chi.URLParam(r, "personID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
