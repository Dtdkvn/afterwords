package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/afterwords/afterwords/internal/clock"
	"github.com/afterwords/afterwords/internal/store"
)

func TestCiphertextOnlyVaultAPIAndCheckin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fake := clock.NewManual(time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC))
	database, err := store.Open(ctx, "file:server-test?mode=memory&cache=shared", fake)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	testServer := httptest.NewServer(New(database, fake, "http://example.test", false, false, "", logger).Handler())
	t.Cleanup(testServer.Close)

	setup := requestJSON(t, testServer.Client(), http.MethodPost, testServer.URL+"/api/setup", `{"email":"owner@example.com","password":"correct horse battery staple"}`, "", nil)
	if setup.Code != http.StatusCreated {
		t.Fatalf("setup status=%d body=%s", setup.Code, setup.Body)
	}
	var setupBody struct {
		CSRFToken string `json:"csrfToken"`
	}
	decodeTestJSON(t, setup.Body, &setupBody)
	if setup.Cookie == nil || setupBody.CSRFToken == "" {
		t.Fatal("setup did not return session and CSRF credentials")
	}
	csp := setup.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self' 'wasm-unsafe-eval'") || strings.Contains(csp, "https:") {
		t.Fatalf("unexpected script policy: %s", csp)
	}
	if setup.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("API response Cache-Control=%q, want no-store", setup.Header.Get("Cache-Control"))
	}

	create := requestJSON(t, testServer.Client(), http.MethodPost, testServer.URL+"/api/switches",
		`{"name":"My letters","intervalDays":7,"graceDays":3,"reminders":{"beforeDays":[3,1],"overdueDays":[0,1,2]}}`, setupBody.CSRFToken, setup.Cookie)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body)
	}
	var created struct {
		Switch struct {
			ID string `json:"id"`
		} `json:"switch"`
		APIToken string `json:"apiToken"`
	}
	decodeTestJSON(t, create.Body, &created)

	withoutCSRF := requestJSON(t, testServer.Client(), http.MethodPost, testServer.URL+"/api/switches/"+created.Switch.ID+"/checkin", `{}`, "", setup.Cookie)
	if withoutCSRF.Code != http.StatusForbidden {
		t.Fatalf("mutation without CSRF status=%d, want 403", withoutCSRF.Code)
	}

	reserve := requestJSON(t, testServer.Client(), http.MethodPost, testServer.URL+"/api/switches/"+created.Switch.ID+"/vault/reserve", `{}`, setupBody.CSRFToken, setup.Cookie)
	if reserve.Code != http.StatusCreated {
		t.Fatalf("reserve status=%d body=%s", reserve.Code, reserve.Body)
	}
	var reservation struct {
		ItemID string `json:"itemId"`
		AAD    string `json:"aad"`
	}
	decodeTestJSON(t, reserve.Body, &reservation)
	iv := base64.RawURLEncoding.EncodeToString(make([]byte, 12))
	ciphertext := base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	malicious := `{"iv":"` + iv + `","ciphertext":"` + ciphertext + `","aad":"` + reservation.AAD + `","size":16,"plaintext":"server must reject this"}`
	rejected := requestJSON(t, testServer.Client(), http.MethodPut, testServer.URL+"/api/switches/"+created.Switch.ID+"/vault/"+reservation.ItemID, malicious, setupBody.CSRFToken, setup.Cookie)
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("plaintext payload status=%d body=%s", rejected.Code, rejected.Body)
	}

	valid := `{"iv":"` + iv + `","ciphertext":"` + ciphertext + `","aad":"` + reservation.AAD + `","size":16}`
	accepted := requestJSON(t, testServer.Client(), http.MethodPut, testServer.URL+"/api/switches/"+created.Switch.ID+"/vault/"+reservation.ItemID, valid, setupBody.CSRFToken, setup.Cookie)
	if accepted.Code != http.StatusOK {
		t.Fatalf("ciphertext payload status=%d body=%s", accepted.Code, accepted.Body)
	}

	wrapped := base64.RawURLEncoding.EncodeToString(make([]byte, 48))
	salt := base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	maliciousKeyslot := `{"wrappedDk":"` + wrapped + `","iv":"` + iv + `","salt":"` + salt + `","kdfParams":{"memorySize":8192,"iterations":1,"parallelism":1,"hashLength":32},"passphrase":"must never be accepted"}`
	rejectedKeyslot := requestJSON(t, testServer.Client(), http.MethodPut, testServer.URL+"/api/switches/"+created.Switch.ID+"/keyslot", maliciousKeyslot, setupBody.CSRFToken, setup.Cookie)
	if rejectedKeyslot.Code != http.StatusBadRequest {
		t.Fatalf("passphrase keyslot payload status=%d body=%s", rejectedKeyslot.Code, rejectedKeyslot.Body)
	}

	apiCheckin := requestJSON(t, testServer.Client(), http.MethodPost, testServer.URL+"/api/checkin/"+created.APIToken, `{}`, "", nil)
	if apiCheckin.Code != http.StatusOK {
		t.Fatalf("api check-in status=%d body=%s", apiCheckin.Code, apiCheckin.Body)
	}
}

func TestUntrustedForwardingHeadersCannotBypassAuthRateLimit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fake := clock.NewManual(time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC))
	database, err := store.Open(ctx, "file:rate-limit-test?mode=memory&cache=shared", fake)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	testServer := httptest.NewServer(New(database, fake, "http://example.test", false, false, "", slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	t.Cleanup(testServer.Close)
	client := testServer.Client()
	for attempt := 1; attempt <= 11; attempt++ {
		request, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/login", bytes.NewBufferString(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", attempt))
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if attempt <= 10 && response.StatusCode == http.StatusTooManyRequests {
			t.Fatalf("rate limited too early on attempt %d", attempt)
		}
		if attempt == 11 && response.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("spoofed forwarding headers bypassed rate limit: status=%d", response.StatusCode)
		}
	}
}

func TestMagicCheckinRequiresExplicitPOST(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	start := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	fake := clock.NewManual(start)
	database, err := store.Open(ctx, "file:magic-confirm-test?mode=memory&cache=shared", fake)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	owner, err := database.CreateOwner(ctx, "owner@example.com", "not-used-in-handler-test")
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := database.CreateSwitch(ctx, owner.ID, store.CreateSwitchParams{
		Name: "Family letters", IntervalDays: 7, GraceDays: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	token, err := database.CreateMagicToken(ctx, owner.ID, created.ID, 48*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	fake.Advance(2 * time.Hour)
	testServer := httptest.NewServer(New(database, fake, "http://example.test", false, false, "", slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	t.Cleanup(testServer.Close)

	getResponse, err := testServer.Client().Get(testServer.URL + "/c/" + token)
	if err != nil {
		t.Fatal(err)
	}
	getBody, err := io.ReadAll(getResponse.Body)
	_ = getResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if getResponse.StatusCode != http.StatusOK || !strings.Contains(string(getBody), "Ready to check in?") {
		t.Fatalf("GET confirmation status=%d body=%s", getResponse.StatusCode, getBody)
	}
	if getResponse.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("magic confirmation Cache-Control=%q, want no-store", getResponse.Header.Get("Cache-Control"))
	}
	afterGet, err := database.SwitchByID(ctx, owner.ID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !afterGet.LastCheckin.Equal(created.LastCheckin) || !afterGet.NextDeadline.Equal(created.NextDeadline) {
		t.Fatal("GET consumed the magic link or changed the switch deadline")
	}

	postResponse, err := testServer.Client().Post(testServer.URL+"/c/"+token, "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	postBody, err := io.ReadAll(postResponse.Body)
	_ = postResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if postResponse.StatusCode != http.StatusOK || !strings.Contains(string(postBody), "You&#39;re checked in") {
		t.Fatalf("POST check-in status=%d body=%s", postResponse.StatusCode, postBody)
	}
	afterPost, err := database.SwitchByID(ctx, owner.ID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !afterPost.LastCheckin.Equal(fake.Now()) || !afterPost.NextDeadline.Equal(fake.Now().AddDate(0, 0, created.IntervalDays)) {
		t.Fatalf("POST did not reset deadline: last=%s deadline=%s", afterPost.LastCheckin, afterPost.NextDeadline)
	}
	if _, err := database.CheckInWithMagicToken(ctx, token); !errors.Is(err, store.ErrTokenUsed) {
		t.Fatalf("magic token remained reusable: %v", err)
	}
}

func TestEmailAndDisplayNameValidation(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"owner@example.com", "first.last+tag@sub.example.com", "OWNER@EXAMPLE.COM"} {
		if !validEmail(value) {
			t.Errorf("validEmail(%q) = false", value)
		}
	}
	for _, value := range []string{
		"Name <owner@example.com>", "owner@example.com ", " owner@example.com", "a @example.com",
		"a@exam ple.com", "a@example", "a\r\n@example.com", "a@example.com, b@example.com", "",
	} {
		if validEmail(value) {
			t.Errorf("validEmail(%q) = true", value)
		}
	}
	for _, value := range []string{"Family letters", "Élodie's notes", "手紙"} {
		if !validDisplayName(value, 100) {
			t.Errorf("validDisplayName(%q) = false", value)
		}
	}
	for _, value := range []string{"", "line\nbreak", "nul\x00byte", "control\u0085name"} {
		if validDisplayName(value, 100) {
			t.Errorf("validDisplayName(%q) = true", value)
		}
	}
}

func TestOptionalSetupTokenProtectsFirstBoot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fake := clock.NewManual(time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC))
	database, err := store.Open(ctx, "file:setup-token-test?mode=memory&cache=shared", fake)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	const setupToken = "local-bootstrap-secret"
	testServer := httptest.NewServer(New(database, fake, "http://example.test", false, false, setupToken, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	t.Cleanup(testServer.Close)

	bootstrap := requestJSON(t, testServer.Client(), http.MethodGet, testServer.URL+"/api/bootstrap", "", "", nil)
	if bootstrap.Code != http.StatusOK || bytes.Contains(bootstrap.Body, []byte(setupToken)) {
		t.Fatalf("bootstrap leaked token or failed: status=%d body=%s", bootstrap.Code, bootstrap.Body)
	}
	var bootstrapBody struct {
		SetupTokenRequired bool `json:"setupTokenRequired"`
	}
	decodeTestJSON(t, bootstrap.Body, &bootstrapBody)
	if !bootstrapBody.SetupTokenRequired {
		t.Fatal("bootstrap did not announce setup token requirement")
	}
	wrong := requestJSON(t, testServer.Client(), http.MethodPost, testServer.URL+"/api/setup",
		`{"email":"owner@example.com","password":"correct horse battery staple","setupToken":"wrong"}`, "", nil)
	if wrong.Code != http.StatusForbidden {
		t.Fatalf("wrong setup token status=%d body=%s", wrong.Code, wrong.Body)
	}
	required, err := database.SetupRequired(ctx)
	if err != nil || !required {
		t.Fatalf("wrong token claimed owner account: required=%v err=%v", required, err)
	}
	accepted := requestJSON(t, testServer.Client(), http.MethodPost, testServer.URL+"/api/setup",
		`{"email":"owner@example.com","password":"correct horse battery staple","setupToken":"local-bootstrap-secret"}`, "", nil)
	if accepted.Code != http.StatusCreated {
		t.Fatalf("correct setup token status=%d body=%s", accepted.Code, accepted.Body)
	}
}

type testResponse struct {
	Code   int
	Body   []byte
	Cookie *http.Cookie
	Header http.Header
}

func requestJSON(t *testing.T, client *http.Client, method, target, body, csrf string, cookie *http.Cookie) testResponse {
	t.Helper()
	request, err := http.NewRequest(method, target, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	result := testResponse{Code: response.StatusCode, Body: responseBody, Header: response.Header.Clone()}
	for _, candidate := range response.Cookies() {
		if candidate.Name == sessionCookie {
			result.Cookie = candidate
		}
	}
	return result
}

func decodeTestJSON(t *testing.T, input []byte, destination any) {
	t.Helper()
	if err := json.Unmarshal(input, destination); err != nil {
		t.Fatalf("decode %s: %v", input, err)
	}
}
