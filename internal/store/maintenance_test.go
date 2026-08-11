package store

import (
	"context"
	"testing"
	"time"

	"github.com/afterwords/afterwords/internal/clock"
	"github.com/afterwords/afterwords/internal/model"
)

func TestPruneExpiredUsesInjectedClock(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fake := clock.NewManual(time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC))
	database, err := Open(ctx, "file:maintenance-test?mode=memory&cache=shared", fake)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	owner, err := database.CreateOwner(ctx, "owner@example.com", "unused")
	if err != nil {
		t.Fatal(err)
	}
	value, _, err := database.CreateSwitch(ctx, owner.ID, CreateSwitchParams{Name: "Letters", IntervalDays: 7, GraceDays: 3, Reminders: model.DefaultReminderSchedule()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateSession(ctx, owner.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateMagicToken(ctx, owner.ID, value.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := database.FireDrillToken(ctx, owner.ID, value.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ReserveVaultItem(ctx, owner.ID, value.ID); err != nil {
		t.Fatal(err)
	}
	fake.Advance(2 * time.Hour)
	removed, err := database.PruneExpired(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 4 {
		t.Fatalf("removed=%d, want 4", removed)
	}
	for _, table := range []string{"sessions", "checkin_tokens", "fire_drills", "vault_reservations"} {
		var count int
		if err := database.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 && table != "checkin_tokens" {
			t.Fatalf("%s count=%d, want 0", table, count)
		}
		// The switch's long-lived API token intentionally remains.
		if table == "checkin_tokens" && count != 1 {
			t.Fatalf("checkin_tokens count=%d, want one API token", count)
		}
	}
}
