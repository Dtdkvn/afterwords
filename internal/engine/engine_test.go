package engine

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/afterwords/afterwords/internal/clock"
	"github.com/afterwords/afterwords/internal/model"
	"github.com/afterwords/afterwords/internal/store"
)

func TestDeadlineToReleaseWithFakeClock(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	fake := clock.NewManual(start)
	db, err := store.Open(ctx, "file:engine-test?mode=memory&cache=shared", fake)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	owner, err := db.CreateOwner(ctx, "owner@example.com", "hash-for-engine-test")
	if err != nil {
		t.Fatal(err)
	}
	switchValue, _, err := db.CreateSwitch(ctx, owner.ID, store.CreateSwitchParams{
		Name: "Important things", IntervalDays: 7, GraceDays: 3, Reminders: model.DefaultReminderSchedule(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddRecipient(ctx, owner.ID, switchValue.ID, "Sam", "sam@example.com"); err != nil {
		t.Fatal(err)
	}
	trustee, err := db.AddTrustee(ctx, owner.ID, switchValue.ID, "Terry", "terry@example.com", 1, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.MarkKitDelivered(ctx, owner.ID, switchValue.ID, trustee.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertKeyslot(ctx, owner.ID, switchValue.ID, "wrapped-key", "iv", "salt", map[string]int{"memorySize": 8192}); err != nil {
		t.Fatal(err)
	}
	itemID, err := db.ReserveVaultItem(ctx, owner.ID, switchValue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.PutVaultItem(ctx, owner.ID, switchValue.ID, itemID, "iv", "ciphertext", switchValue.ID+":"+itemID, 16); err != nil {
		t.Fatal(err)
	}
	runner := New(db, fake, "http://afterwords.test", slog.New(slog.NewTextHandler(io.Discard, nil)))

	fake.Advance(8 * 24 * time.Hour)
	if err := runner.Step(ctx); err != nil {
		t.Fatal(err)
	}
	current, err := db.SwitchByID(ctx, owner.ID, switchValue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != model.StateOverdue {
		t.Fatalf("state after 8 days = %s, want OVERDUE", current.State)
	}
	count, err := db.OutboxCount(ctx, switchValue.ID, "overdue_reminder")
	if err != nil || count == 0 {
		t.Fatalf("overdue messages = %d, err=%v", count, err)
	}

	fake.Advance(3*24*time.Hour + time.Second)
	if err := runner.Step(ctx); err != nil {
		t.Fatal(err)
	}
	current, err = db.SwitchByID(ctx, owner.ID, switchValue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != model.StateReleased {
		t.Fatalf("state after grace = %s, want RELEASED", current.State)
	}
	for _, kind := range []string{"release_recipient", "release_trustee"} {
		count, err := db.OutboxCount(ctx, switchValue.ID, kind)
		if err != nil || count != 1 {
			t.Fatalf("%s messages = %d, err=%v", kind, count, err)
		}
	}
	// A repeated scan must not enqueue release mail twice.
	if err := runner.Step(ctx); err != nil {
		t.Fatal(err)
	}
	count, _ = db.OutboxCount(ctx, switchValue.ID, "release_recipient")
	if count != 1 {
		t.Fatalf("idempotent release count = %d, want 1", count)
	}
	if _, err := db.ReserveVaultItem(ctx, owner.ID, switchValue.ID); !errors.Is(err, store.ErrInvalidState) {
		t.Fatalf("released vault accepted mutation: %v", err)
	}
	if _, err := db.AddRecipient(ctx, owner.ID, switchValue.ID, "Late recipient", "late@example.com"); !errors.Is(err, store.ErrInvalidState) {
		t.Fatalf("released people list accepted mutation: %v", err)
	}
	if items, err := db.ListVaultItems(ctx, owner.ID, switchValue.ID); err != nil || len(items) != 1 {
		t.Fatalf("released ciphertext became unreadable: items=%d err=%v", len(items), err)
	}
}

func TestIncompleteSwitchFailsClosedAtReleaseBoundary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	start := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	fake := clock.NewManual(start)
	db, err := store.Open(ctx, "file:engine-readiness-test?mode=memory&cache=shared", fake)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	owner, err := db.CreateOwner(ctx, "owner@example.com", "hash-for-engine-test")
	if err != nil {
		t.Fatal(err)
	}
	switchValue, _, err := db.CreateSwitch(ctx, owner.ID, store.CreateSwitchParams{
		Name: "Unfinished setup", IntervalDays: 1, GraceDays: 0, Reminders: model.DefaultReminderSchedule(),
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := New(db, fake, "http://afterwords.test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	fake.Advance(2 * 24 * time.Hour)
	if err := runner.Step(ctx); err != nil {
		t.Fatal(err)
	}
	current, err := db.SwitchByID(ctx, owner.ID, switchValue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != model.StateOverdue {
		t.Fatalf("incomplete switch state=%s, want OVERDUE fail-closed", current.State)
	}
	if count, err := db.OutboxCount(ctx, switchValue.ID, "release_recipient"); err != nil || count != 0 {
		t.Fatalf("incomplete switch release messages=%d err=%v", count, err)
	}
}
