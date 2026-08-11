package store

import (
	"context"
	"testing"
	"time"

	"github.com/afterwords/afterwords/internal/clock"
	"github.com/afterwords/afterwords/internal/model"
)

func TestUpdateSettingsAuditsOverdueReactivationTransactionally(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fake := clock.NewManual(time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC))
	database, err := Open(ctx, "file:settings-audit-test?mode=memory&cache=shared", fake)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	owner, err := database.CreateOwner(ctx, "owner@example.com", "not-used-in-store-test")
	if err != nil {
		t.Fatal(err)
	}
	value, _, err := database.CreateSwitch(ctx, owner.ID, CreateSwitchParams{Name: "Letters", IntervalDays: 7, GraceDays: 3, Reminders: model.DefaultReminderSchedule()})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := database.Transition(ctx, value.ID, model.StateActive, model.StateOverdue, "switch.overdue")
	if err != nil || !changed {
		t.Fatalf("mark overdue: changed=%v err=%v", changed, err)
	}
	updated, err := database.UpdateSwitchSettings(ctx, owner.ID, value.ID, "Letters", 14, 5, model.ReminderSchedule{BeforeDays: []int{4, 1}, OverdueDays: []int{0, 2}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.State != model.StateActive {
		t.Fatalf("state=%s, want ACTIVE", updated.State)
	}
	entries, err := database.ListAudit(ctx, owner.ID, value.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	foundTransition, foundSettings := false, false
	for _, entry := range entries {
		foundTransition = foundTransition || entry.Event == "switch.reactivated_by_settings"
		foundSettings = foundSettings || entry.Event == "switch.settings_updated"
	}
	if !foundTransition || !foundSettings {
		t.Fatalf("missing transactional audits: transition=%v settings=%v", foundTransition, foundSettings)
	}
}
