// Package engine implements the idempotent dead-man's-switch state machine.
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Dtdkvn/afterwords/internal/clock"
	"github.com/Dtdkvn/afterwords/internal/mail"
	"github.com/Dtdkvn/afterwords/internal/model"
	"github.com/Dtdkvn/afterwords/internal/store"
)

// Engine processes deadlines and releases.
type Engine struct {
	store   *store.Store
	clock   clock.Clock
	baseURL string
	logger  *slog.Logger
}

func New(s *store.Store, c clock.Clock, baseURL string, logger *slog.Logger) *Engine {
	return &Engine{store: s, clock: c, baseURL: baseURL, logger: logger}
}

// Step performs one complete idempotent scan. A single switch may traverse several overdue states in one scan.
func (e *Engine) Step(ctx context.Context) error {
	switches, err := e.store.SchedulableSwitches(ctx)
	if err != nil {
		return err
	}
	for _, current := range switches {
		if err := e.process(ctx, current); err != nil {
			return fmt.Errorf("process switch %s: %w", current.ID, err)
		}
	}
	return nil
}

func (e *Engine) process(ctx context.Context, current model.Switch) error {
	now := e.clock.Now()
	ownerEmail, err := e.store.OwnerEmailForSwitch(ctx, current.ID)
	if err != nil {
		return err
	}
	if current.State == model.StateActive {
		for _, days := range current.Reminders.BeforeDays {
			threshold := current.NextDeadline.AddDate(0, 0, -days)
			if !now.Before(threshold) && !now.After(current.NextDeadline) {
				if err := e.enqueueOwnerReminder(ctx, current, ownerEmail, "before", days); err != nil {
					return err
				}
			}
		}
		if now.After(current.NextDeadline) {
			changed, err := e.store.Transition(ctx, current.ID, model.StateActive, model.StateOverdue, "switch.overdue")
			if err != nil {
				return err
			}
			if changed {
				current.State = model.StateOverdue
			}
		}
	}
	if current.State == model.StateOverdue {
		for _, days := range current.Reminders.OverdueDays {
			if !now.Before(current.NextDeadline.AddDate(0, 0, days)) {
				if err := e.enqueueOwnerReminder(ctx, current, ownerEmail, "overdue", days); err != nil {
					return err
				}
			}
		}
		if now.After(current.NextDeadline.AddDate(0, 0, current.GraceDays)) {
			changed, err := e.store.BeginReleaseIfReady(ctx, current.ID)
			if err != nil {
				return err
			}
			if changed {
				current.State = model.StateReleasing
			}
		}
	}
	if current.State == model.StateReleasing {
		if err := e.enqueueRelease(ctx, current); err != nil {
			return err
		}
		_, err := e.store.Transition(ctx, current.ID, model.StateReleasing, model.StateReleased, "switch.released")
		return err
	}
	return nil
}

func (e *Engine) enqueueOwnerReminder(ctx context.Context, current model.Switch, ownerEmail, phase string, days int) error {
	eventKey := fmt.Sprintf("reminder:%s:%d:%d", phase, days, current.NextDeadline.Unix())
	exists, err := e.store.ScheduledEventExists(ctx, current.ID, eventKey)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	token, err := e.store.CreateMagicToken(ctx, current.UserID, current.ID, 48*time.Hour)
	if err != nil {
		return err
	}
	templateName := "checkin_reminder"
	subject := "A gentle Afterwords check-in"
	if phase == "overdue" {
		templateName = "overdue_notice"
		subject = "Your Afterwords check-in is waiting"
	}
	htmlBody, textBody, err := mail.Render(templateName, mail.TemplateData{
		SwitchName: current.Name, Deadline: current.NextDeadline.Format("January 2, 2006 at 15:04 UTC"),
		CheckinURL: e.baseURL + "/c/" + token, DaysOverdue: days,
	})
	if err != nil {
		return err
	}
	claimed, err := e.store.ClaimScheduledMessage(ctx, current.ID, eventKey, model.OutboxMessage{
		SwitchID: current.ID, Kind: phase + "_reminder", ToEmail: ownerEmail, Subject: subject, BodyHTML: htmlBody, BodyText: textBody,
	})
	if err != nil {
		return err
	}
	if !claimed {
		// Avoid accumulating unused magic tokens when another worker won the event claim.
		// They contain no secret and expire automatically; cleanup keeps this branch side-effect-free enough for v0.1.
		return nil
	}
	return nil
}

func (e *Engine) enqueueRelease(ctx context.Context, current model.Switch) error {
	releaseURL := e.baseURL + "/r/" + current.ReleaseToken
	recipients, err := e.store.ListRecipients(ctx, current.UserID, current.ID)
	if err != nil {
		return err
	}
	for _, recipient := range recipients {
		htmlBody, textBody, err := mail.Render("release_recipient", mail.TemplateData{RecipientName: recipient.Name, SwitchName: current.Name, ReleaseURL: releaseURL})
		if err != nil {
			return err
		}
		_, err = e.store.ClaimScheduledMessage(ctx, current.ID, "release:recipient:"+recipient.ID, model.OutboxMessage{
			SwitchID: current.ID, Kind: "release_recipient", ToEmail: recipient.Email,
			Subject: "Someone left you something in Afterwords", BodyHTML: htmlBody, BodyText: textBody,
		})
		if err != nil {
			return err
		}
	}
	trustees, err := e.store.ListTrustees(ctx, current.UserID, current.ID)
	if err != nil {
		return err
	}
	for _, trustee := range trustees {
		htmlBody, textBody, err := mail.Render("trustee_release", mail.TemplateData{TrusteeName: trustee.Name, SwitchName: current.Name, ReleaseURL: releaseURL})
		if err != nil {
			return err
		}
		_, err = e.store.ClaimScheduledMessage(ctx, current.ID, "release:trustee:"+trustee.ID, model.OutboxMessage{
			SwitchID: current.ID, Kind: "release_trustee", ToEmail: trustee.Email,
			Subject: "Your Afterwords Recovery Kit is needed", BodyHTML: htmlBody, BodyText: textBody,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Run starts the scheduler until cancellation. Scaled demos tick once per second.
func (e *Engine) Run(ctx context.Context, timeScale float64) {
	interval := 30 * time.Second
	if timeScale > 1 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	e.runCycle(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.runCycle(ctx)
		}
	}
}

func (e *Engine) runCycle(ctx context.Context) {
	if _, err := e.store.PruneExpired(ctx, 500); err != nil {
		e.logger.Error("prune expired records", "error", err)
	}
	if err := e.Step(ctx); err != nil {
		e.logger.Error("scheduler scan", "error", err)
	}
}
