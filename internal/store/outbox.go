package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/afterwords/afterwords/internal/model"
)

// EnqueueMessage appends mail to the durable outbox.
func (s *Store) EnqueueMessage(ctx context.Context, message model.OutboxMessage) (string, error) {
	id, err := newID()
	if err != nil {
		return "", err
	}
	now := s.clock.Now()
	if message.ScheduledAt.IsZero() {
		message.ScheduledAt = now
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `INSERT INTO outbox(id,switch_id,kind,to_email,subject,body_html,body_text,scheduled_at,created_at)
      VALUES(?,?,?,?,?,?,?,?,?)`, id, nullIfEmpty(message.SwitchID), message.Kind, message.ToEmail, message.Subject,
		message.BodyHTML, message.BodyText, formatTime(message.ScheduledAt), formatTime(now))
	if err != nil {
		return "", fmt.Errorf("enqueue mail: %w", err)
	}
	if message.SwitchID != "" {
		if err := insertAudit(ctx, tx, message.SwitchID, "email.queued", map[string]any{"kind": message.Kind, "messageId": id}, now); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

// ClaimScheduledMessage atomically reserves an idempotency key and its outbox message.
func (s *Store) ClaimScheduledMessage(ctx context.Context, switchID, eventKey string, message model.OutboxMessage) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	eventID, err := newID()
	if err != nil {
		return false, err
	}
	mailID, err := newID()
	if err != nil {
		return false, err
	}
	now := s.clock.Now()
	result, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO scheduled_events(id,switch_id,event_key,created_at) VALUES(?,?,?,?)", eventID, switchID, eventKey, formatTime(now))
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if count == 0 {
		return false, nil
	}
	if message.ScheduledAt.IsZero() {
		message.ScheduledAt = now
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO outbox(id,switch_id,kind,to_email,subject,body_html,body_text,scheduled_at,created_at)
      VALUES(?,?,?,?,?,?,?,?,?)`, mailID, switchID, message.Kind, message.ToEmail, message.Subject,
		message.BodyHTML, message.BodyText, formatTime(message.ScheduledAt), formatTime(now))
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE scheduled_events SET outbox_id=? WHERE id=?", mailID, eventID); err != nil {
		return false, err
	}
	if err := insertAudit(ctx, tx, switchID, "email.queued", map[string]any{"kind": message.Kind, "messageId": mailID}, now); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// ScheduledEventExists avoids generating disposable content for an event already claimed.
func (s *Store) ScheduledEventExists(ctx context.Context, switchID, eventKey string) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM scheduled_events WHERE switch_id=? AND event_key=?", switchID, eventKey).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// PendingMessages loads mail ready for the sender worker.
func (s *Store) PendingMessages(ctx context.Context, limit int) ([]model.OutboxMessage, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,COALESCE(switch_id,''),kind,to_email,subject,body_html,body_text,
      scheduled_at,sent_at,attempts,COALESCE(last_error,'') FROM outbox
      WHERE sent_at IS NULL AND attempts < 10 AND scheduled_at <= ? ORDER BY scheduled_at LIMIT ?`, formatTime(s.clock.Now()), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.OutboxMessage, 0)
	for rows.Next() {
		var value model.OutboxMessage
		var scheduled string
		var sent sql.NullString
		if err := rows.Scan(&value.ID, &value.SwitchID, &value.Kind, &value.ToEmail, &value.Subject, &value.BodyHTML,
			&value.BodyText, &scheduled, &sent, &value.Attempts, &value.LastError); err != nil {
			return nil, err
		}
		value.ScheduledAt, err = parseTime(scheduled)
		if err != nil {
			return nil, err
		}
		value.SentAt, err = parseOptionalTime(sent)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) MarkMessageSent(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var switchID, kind string
	err = tx.QueryRowContext(ctx, "SELECT COALESCE(switch_id,''),kind FROM outbox WHERE id=? AND sent_at IS NULL", id).Scan(&switchID, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	now := s.clock.Now()
	if _, err := tx.ExecContext(ctx, "UPDATE outbox SET sent_at=?,last_error=NULL WHERE id=? AND sent_at IS NULL", formatTime(now), id); err != nil {
		return err
	}
	if switchID != "" {
		if err := insertAudit(ctx, tx, switchID, "email.sent", map[string]any{"kind": kind, "messageId": id}, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) MarkMessageFailed(ctx context.Context, id string, attempts int, deliveryErr error) error {
	delay := time.Minute * time.Duration(1<<min(attempts, 6))
	safeError := "delivery failed"
	if deliveryErr != nil {
		safeError = deliveryErr.Error()
		if len(safeError) > 500 {
			safeError = safeError[:500]
		}
		safeError = strings.ReplaceAll(safeError, "\n", " ")
	}
	_, err := s.db.ExecContext(ctx, `UPDATE outbox SET attempts=attempts+1,last_error=?,scheduled_at=?
      WHERE id=? AND sent_at IS NULL`, safeError, formatTime(s.clock.Now().Add(delay)), id)
	return err
}

// OutboxCount is used by state-engine acceptance tests and diagnostics.
func (s *Store) OutboxCount(ctx context.Context, switchID, kind string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM outbox WHERE switch_id=? AND kind=?", switchID, kind).Scan(&count)
	return count, err
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// FireDrillToken creates a short-lived token without mutating real switch state.
func (s *Store) FireDrillToken(ctx context.Context, userID, switchID string, ttl time.Duration) (string, error) {
	if _, err := s.SwitchByID(ctx, userID, switchID); err != nil {
		return "", err
	}
	token, err := NewToken(32)
	if err != nil {
		return "", err
	}
	now := s.clock.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "INSERT INTO fire_drills(token,switch_id,expires_at,created_at) VALUES(?,?,?,?)", token, switchID, formatTime(now.Add(ttl)), formatTime(now)); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE switches SET fire_drill_at=?,updated_at=? WHERE id=?", formatTime(now), formatTime(now), switchID); err != nil {
		return "", err
	}
	if err := insertAudit(ctx, tx, switchID, "fire_drill.started", map[string]any{"realStateUnchanged": true}, now); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return token, nil
}

// ResolveReleaseToken resolves either a real released switch or an unexpired fire drill.
func (s *Store) ResolveReleaseToken(ctx context.Context, token string) (model.Switch, bool, error) {
	value, err := scanSwitch(s.db.QueryRowContext(ctx, switchSelect+" WHERE release_token=? AND state='RELEASED'", token))
	if err == nil {
		return value, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return model.Switch{}, false, err
	}
	var switchID, expires string
	err = s.db.QueryRowContext(ctx, "SELECT switch_id,expires_at FROM fire_drills WHERE token=?", token).Scan(&switchID, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Switch{}, false, ErrNotFound
	}
	if err != nil {
		return model.Switch{}, false, err
	}
	expiry, err := parseTime(expires)
	if err != nil {
		return model.Switch{}, false, err
	}
	if !expiry.After(s.clock.Now()) {
		return model.Switch{}, false, ErrTokenExpired
	}
	value, err = s.SwitchByIDInternal(ctx, switchID)
	return value, true, err
}
