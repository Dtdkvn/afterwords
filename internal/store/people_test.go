package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Dtdkvn/afterwords/internal/clock"
	"github.com/Dtdkvn/afterwords/internal/model"
)

func TestPeopleCreationIsIdempotentAndConflictsFailClosed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, ownerID, switchID := newPeopleTestStore(t, ctx, "people-idempotency")

	firstRecipient, err := database.AddRecipient(ctx, ownerID, switchID, "Riley", "RILEY@example.com")
	if err != nil {
		t.Fatal(err)
	}
	retriedRecipient, err := database.AddRecipient(ctx, ownerID, switchID, "Riley", "riley@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if retriedRecipient.ID != firstRecipient.ID {
		t.Fatalf("recipient retry created a new row: %s != %s", retriedRecipient.ID, firstRecipient.ID)
	}
	if _, err := database.AddRecipient(ctx, ownerID, switchID, "Different person", "riley@example.com"); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed recipient retry error=%v, want ErrConflict", err)
	}
	recipients, err := database.ListRecipients(ctx, ownerID, switchID)
	if err != nil || len(recipients) != 1 {
		t.Fatalf("recipients=%d err=%v, want one", len(recipients), err)
	}

	fingerprint := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	firstTrustee, err := database.AddTrustee(ctx, ownerID, switchID, "Taylor", "TAYLOR@example.com", 1, fingerprint, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	retriedTrustee, err := database.AddTrustee(ctx, ownerID, switchID, "Taylor", "taylor@example.com", 1, fingerprint, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if retriedTrustee.ID != firstTrustee.ID {
		t.Fatalf("trustee retry created a new row: %s != %s", retriedTrustee.ID, firstTrustee.ID)
	}
	otherFingerprint := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err := database.AddTrustee(ctx, ownerID, switchID, "Taylor", "taylor@example.com", 1, otherFingerprint, 1, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed trustee retry error=%v, want ErrConflict", err)
	}
	trustees, err := database.ListTrustees(ctx, ownerID, switchID)
	if err != nil || len(trustees) != 1 {
		t.Fatalf("trustees=%d err=%v, want one", len(trustees), err)
	}
	current, err := database.SwitchByID(ctx, ownerID, switchID)
	if err != nil {
		t.Fatal(err)
	}
	if current.RecoveryThreshold != 2 || current.RecoveryShareCount != 2 {
		t.Fatalf("conflicting retry changed policy to %d-of-%d", current.RecoveryThreshold, current.RecoveryShareCount)
	}
	entries, err := database.ListAudit(ctx, ownerID, switchID, 100)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, entry := range entries {
		counts[entry.Event]++
	}
	if counts["recipient.added"] != 1 || counts["trustee.added"] != 1 {
		t.Fatalf("idempotent retries duplicated audit entries: %#v", counts)
	}
}

func TestSecurityMutationsRollbackWhenAuditInsertFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, ownerID, switchID := newPeopleTestStore(t, ctx, "mutation-audit-rollback")
	forceAuditFailure := func(event string) {
		t.Helper()
		if _, err := database.DB().ExecContext(ctx, `CREATE TRIGGER fail_test_audit BEFORE INSERT ON audit_log
          WHEN NEW.event='`+event+`' BEGIN SELECT RAISE(ABORT, 'forced audit failure'); END`); err != nil {
			t.Fatal(err)
		}
	}
	dropAuditFailure := func() {
		t.Helper()
		if _, err := database.DB().ExecContext(ctx, "DROP TRIGGER fail_test_audit"); err != nil {
			t.Fatal(err)
		}
	}

	forceAuditFailure("recipient.added")
	if _, err := database.AddRecipient(ctx, ownerID, switchID, "Riley", "riley@example.com"); err == nil {
		t.Fatal("recipient creation succeeded despite forced audit failure")
	}
	dropAuditFailure()
	recipients, err := database.ListRecipients(ctx, ownerID, switchID)
	if err != nil || len(recipients) != 0 {
		t.Fatalf("recipient mutation escaped rollback: count=%d err=%v", len(recipients), err)
	}

	forceAuditFailure("keyslot.updated")
	if _, err := database.UpsertKeyslot(ctx, ownerID, switchID, "wrapped", "iv", "salt", map[string]int{"memorySize": 8192}); err == nil {
		t.Fatal("keyslot update succeeded despite forced audit failure")
	}
	dropAuditFailure()
	if _, err := database.Keyslot(ctx, ownerID, switchID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("keyslot mutation escaped rollback: %v", err)
	}

	itemID, err := database.ReserveVaultItem(ctx, ownerID, switchID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.PutVaultItem(ctx, ownerID, switchID, itemID, "iv", "ciphertext", switchID+":"+itemID, 10); err != nil {
		t.Fatal(err)
	}
	forceAuditFailure("vault.item_deleted")
	if err := database.DeleteVaultItem(ctx, ownerID, switchID, itemID); err == nil {
		t.Fatal("vault deletion succeeded despite forced audit failure")
	}
	dropAuditFailure()
	items, err := database.ListVaultItems(ctx, ownerID, switchID)
	if err != nil || len(items) != 1 {
		t.Fatalf("vault deletion escaped rollback: count=%d err=%v", len(items), err)
	}
}

func TestMagicTokenAndCheckinRollbackTogether(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, ownerID, switchID := newPeopleTestStore(t, ctx, "magic-checkin-atomic")
	before, err := database.SwitchByID(ctx, ownerID, switchID)
	if err != nil {
		t.Fatal(err)
	}
	token, err := database.CreateMagicToken(ctx, ownerID, switchID, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB().ExecContext(ctx, `CREATE TRIGGER fail_magic_audit BEFORE INSERT ON audit_log
      WHEN NEW.event='switch.checked_in' BEGIN SELECT RAISE(ABORT, 'forced audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CheckInWithMagicToken(ctx, token); err == nil {
		t.Fatal("magic check-in succeeded despite forced audit failure")
	}
	afterFailure, err := database.SwitchByID(ctx, ownerID, switchID)
	if err != nil {
		t.Fatal(err)
	}
	if !afterFailure.LastCheckin.Equal(before.LastCheckin) || !afterFailure.NextDeadline.Equal(before.NextDeadline) {
		t.Fatal("failed magic check-in changed switch timing")
	}
	if _, err := database.DB().ExecContext(ctx, "DROP TRIGGER fail_magic_audit"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CheckInWithMagicToken(ctx, token); err != nil {
		t.Fatalf("rolled-back magic token was not reusable: %v", err)
	}
}

func newPeopleTestStore(t *testing.T, ctx context.Context, name string) (*Store, string, string) {
	t.Helper()
	fake := clock.NewManual(time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC))
	database, err := Open(ctx, "file:"+name+"?mode=memory&cache=shared", fake)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	owner, err := database.CreateOwner(ctx, name+"@example.com", "unused")
	if err != nil {
		t.Fatal(err)
	}
	value, _, err := database.CreateSwitch(ctx, owner.ID, CreateSwitchParams{
		Name: "Family letters", IntervalDays: 7, GraceDays: 3, Reminders: model.DefaultReminderSchedule(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return database, owner.ID, value.ID
}
