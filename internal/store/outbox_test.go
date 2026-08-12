package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yewud/afterwords/internal/clock"
	"github.com/yewud/afterwords/internal/model"
)

func TestMarkMessageFailedDeadLettersExactlyOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fake := clock.NewManual(time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC))
	database, err := Open(ctx, "file:outbox-dead-letter?mode=memory&cache=shared", fake)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	owner, err := database.CreateOwner(ctx, "owner@example.com", "unused")
	if err != nil {
		t.Fatal(err)
	}
	value, _, err := database.CreateSwitch(ctx, owner.ID, CreateSwitchParams{
		Name: "Letters", IntervalDays: 7, GraceDays: 3, Reminders: model.DefaultReminderSchedule(),
	})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := database.EnqueueMessage(ctx, model.OutboxMessage{
		SwitchID: value.ID, Kind: "release_recipient", ToEmail: "recipient@example.com",
		Subject: "Release", BodyHTML: "<p>release</p>", BodyText: "release",
	})
	if err != nil {
		t.Fatal(err)
	}

	for attempt := 1; attempt < maxDeliveryAttempts; attempt++ {
		deadLettered, err := database.MarkMessageFailed(ctx, messageID, errors.New("temporary SMTP failure"))
		if err != nil {
			t.Fatal(err)
		}
		if deadLettered {
			t.Fatalf("attempt %d reached terminal state early", attempt)
		}
	}

	terminalError := errors.New("line one\rline two\n" + strings.Repeat("x", 600))
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	errorsSeen := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deadLettered, err := database.MarkMessageFailed(ctx, messageID, terminalError)
			results <- deadLettered
			errorsSeen <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errorsSeen)
	terminalTransitions := 0
	for result := range results {
		if result {
			terminalTransitions++
		}
	}
	for err := range errorsSeen {
		if err != nil {
			t.Fatal(err)
		}
	}
	if terminalTransitions != 1 {
		t.Fatalf("terminal transitions=%d, want exactly 1", terminalTransitions)
	}

	var attempts int
	var lastError string
	if err := database.DB().QueryRowContext(ctx, "SELECT attempts,last_error FROM outbox WHERE id=?", messageID).Scan(&attempts, &lastError); err != nil {
		t.Fatal(err)
	}
	if attempts != maxDeliveryAttempts {
		t.Fatalf("attempts=%d, want %d", attempts, maxDeliveryAttempts)
	}
	if len(lastError) != 500 || strings.ContainsAny(lastError, "\r\n") {
		t.Fatalf("terminal error was not safely bounded to one line: len=%d", len(lastError))
	}
	pending, err := database.PendingMessages(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("dead-lettered message remained pending: %#v", pending)
	}

	entries, err := database.ListAudit(ctx, owner.ID, value.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	deadLetters := 0
	for _, entry := range entries {
		if entry.Event != "email.dead_lettered" {
			continue
		}
		deadLetters++
		if entry.Meta["messageId"] != messageID || entry.Meta["kind"] != "release_recipient" || entry.Meta["attempts"] != float64(maxDeliveryAttempts) {
			t.Fatalf("unexpected dead-letter metadata: %#v", entry.Meta)
		}
		if _, leaked := entry.Meta["error"]; leaked {
			t.Fatal("delivery error must not be copied into the audit trail")
		}
	}
	if deadLetters != 1 {
		t.Fatalf("dead-letter audit entries=%d, want 1", deadLetters)
	}
	if _, err := database.DB().ExecContext(ctx, `CREATE TRIGGER reject_email_requeue BEFORE INSERT ON audit_log
      WHEN NEW.event='email.requeued' BEGIN SELECT RAISE(FAIL, 'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if err := database.RetryDeadLetter(ctx, owner.ID, value.ID, messageID); err == nil {
		t.Fatal("retry succeeded even though its audit insert failed")
	}
	if err := database.DB().QueryRowContext(ctx, "SELECT attempts FROM outbox WHERE id=?", messageID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != maxDeliveryAttempts {
		t.Fatalf("failed audited retry changed attempts to %d", attempts)
	}
	if _, err := database.DB().ExecContext(ctx, "DROP TRIGGER reject_email_requeue"); err != nil {
		t.Fatal(err)
	}
	if err := database.RetryDeadLetter(ctx, "another-owner", value.ID, messageID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner retry error=%v, want ErrNotFound", err)
	}
	if err := database.RetryDeadLetter(ctx, owner.ID, value.ID, messageID); err != nil {
		t.Fatal(err)
	}
	var retriedAttempts int
	var retriedError *string
	var retriedAt string
	if err := database.DB().QueryRowContext(ctx, "SELECT attempts,last_error,scheduled_at FROM outbox WHERE id=?", messageID).Scan(&retriedAttempts, &retriedError, &retriedAt); err != nil {
		t.Fatal(err)
	}
	if retriedAttempts != 0 || retriedError != nil || retriedAt != formatTime(fake.Now()) {
		t.Fatalf("retried message attempts=%d error=%v scheduled=%q", retriedAttempts, retriedError, retriedAt)
	}
	if err := database.RetryDeadLetter(ctx, owner.ID, value.ID, messageID); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("second retry error=%v, want ErrInvalidState", err)
	}
	entries, err = database.ListAudit(ctx, owner.ID, value.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	requeued := 0
	for _, entry := range entries {
		if entry.Event == "email.requeued" {
			requeued++
		}
	}
	if requeued != 1 {
		t.Fatalf("email.requeued audit entries=%d, want 1", requeued)
	}
}

func TestMarkMessageFailedIgnoresUnknownMessage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, err := Open(ctx, "file:outbox-completed?mode=memory&cache=shared", clock.NewManual(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	deadLettered, err := database.MarkMessageFailed(ctx, "missing", errors.New("failure"))
	if err != nil || deadLettered {
		t.Fatalf("missing message: deadLettered=%t err=%v", deadLettered, err)
	}
}
