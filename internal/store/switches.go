package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yewud/afterwords/internal/model"
)

// CreateSwitchParams contains validated switch settings.
type CreateSwitchParams struct {
	Name         string
	IntervalDays int
	GraceDays    int
	Reminders    model.ReminderSchedule
}

// CreateSwitch creates an active switch and returns its one-time API check-in token.
func (s *Store) CreateSwitch(ctx context.Context, userID string, params CreateSwitchParams) (model.Switch, string, error) {
	id, err := newID()
	if err != nil {
		return model.Switch{}, "", err
	}
	releaseToken, err := NewToken(32)
	if err != nil {
		return model.Switch{}, "", err
	}
	apiToken, err := NewToken(32)
	if err != nil {
		return model.Switch{}, "", err
	}
	reminders, err := json.Marshal(params.Reminders)
	if err != nil {
		return model.Switch{}, "", err
	}
	now := s.clock.Now()
	switchValue := model.Switch{
		ID: id, UserID: userID, Name: strings.TrimSpace(params.Name), IntervalDays: params.IntervalDays,
		GraceDays: params.GraceDays, State: model.StateActive, NextDeadline: now.AddDate(0, 0, params.IntervalDays),
		LastCheckin: now, Reminders: params.Reminders, ReleaseToken: releaseToken, RecoveryThreshold: 1,
		RecoveryShareCount: 1, CreatedAt: now, UpdatedAt: now,
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Switch{}, "", err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `INSERT INTO switches(
        id,user_id,name,interval_days,grace_days,state,next_deadline_utc,last_checkin_utc,
        reminder_offsets_json,release_token,recovery_threshold,recovery_share_count,created_at,updated_at
      ) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, switchValue.ID, userID, switchValue.Name, switchValue.IntervalDays,
		switchValue.GraceDays, switchValue.State, formatTime(switchValue.NextDeadline), formatTime(now), string(reminders),
		releaseToken, 1, 1, formatTime(now), formatTime(now))
	if err != nil {
		return model.Switch{}, "", fmt.Errorf("insert switch: %w", err)
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO checkin_tokens(token_hash,switch_id,kind,created_at) VALUES(?,?,?,?)", HashToken(apiToken), id, "api", formatTime(now))
	if err != nil {
		return model.Switch{}, "", fmt.Errorf("insert api token: %w", err)
	}
	if err := insertAudit(ctx, tx, id, "switch.created", map[string]any{"state": model.StateActive}, now); err != nil {
		return model.Switch{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return model.Switch{}, "", err
	}
	return switchValue, apiToken, nil
}

// ListSwitches returns every switch owned by a user.
func (s *Store) ListSwitches(ctx context.Context, userID string) ([]model.Switch, error) {
	rows, err := s.db.QueryContext(ctx, switchSelect+" WHERE user_id=? ORDER BY created_at DESC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.Switch, 0)
	for rows.Next() {
		value, err := scanSwitch(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

// SchedulableSwitches returns switches the state engine may process.
func (s *Store) SchedulableSwitches(ctx context.Context) ([]model.Switch, error) {
	rows, err := s.db.QueryContext(ctx, switchSelect+" WHERE state IN ('ACTIVE','OVERDUE','RELEASING') ORDER BY next_deadline_utc")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.Switch
	for rows.Next() {
		value, err := scanSwitch(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

// SwitchByID loads an owned switch.
func (s *Store) SwitchByID(ctx context.Context, userID, id string) (model.Switch, error) {
	row := s.db.QueryRowContext(ctx, switchSelect+" WHERE id=? AND user_id=?", id, userID)
	value, err := scanSwitch(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Switch{}, ErrNotFound
	}
	return value, err
}

func (s *Store) mutableSwitch(ctx context.Context, userID, id string) (model.Switch, error) {
	value, err := s.SwitchByID(ctx, userID, id)
	if err != nil {
		return model.Switch{}, err
	}
	if value.State == model.StateReleasing || value.State == model.StateReleased {
		return model.Switch{}, ErrInvalidState
	}
	return value, nil
}

// SwitchByIDInternal loads a switch without an owner filter for workers.
func (s *Store) SwitchByIDInternal(ctx context.Context, id string) (model.Switch, error) {
	value, err := scanSwitch(s.db.QueryRowContext(ctx, switchSelect+" WHERE id=?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Switch{}, ErrNotFound
	}
	return value, err
}

// DeleteSwitch permanently deletes an owned switch and its ciphertext.
func (s *Store) DeleteSwitch(ctx context.Context, userID, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM switches WHERE id=? AND user_id=?", id, userID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateSwitchSettings changes timing and reminders. Timing changes create a fresh deadline.
func (s *Store) UpdateSwitchSettings(ctx context.Context, userID, id, name string, intervalDays, graceDays int, reminders model.ReminderSchedule) (model.Switch, error) {
	now := s.clock.Now()
	encoded, err := json.Marshal(reminders)
	if err != nil {
		return model.Switch{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Switch{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var previousState string
	if err := tx.QueryRowContext(ctx, "SELECT state FROM switches WHERE id=? AND user_id=?", id, userID).Scan(&previousState); errors.Is(err, sql.ErrNoRows) {
		return model.Switch{}, ErrNotFound
	} else if err != nil {
		return model.Switch{}, err
	}
	if previousState == model.StateReleased || previousState == model.StateReleasing {
		return model.Switch{}, ErrInvalidState
	}
	result, err := tx.ExecContext(ctx, `UPDATE switches SET name=?,interval_days=?,grace_days=?,reminder_offsets_json=?,
      next_deadline_utc=CASE WHEN state IN ('ACTIVE','OVERDUE') THEN ? ELSE next_deadline_utc END,
	  state=CASE WHEN state='OVERDUE' THEN 'ACTIVE' ELSE state END,updated_at=? WHERE id=? AND user_id=? AND state NOT IN ('RELEASING','RELEASED')`,
		strings.TrimSpace(name), intervalDays, graceDays, string(encoded), formatTime(now.AddDate(0, 0, intervalDays)), formatTime(now), id, userID)
	if err != nil {
		return model.Switch{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return model.Switch{}, err
	}
	if count == 0 {
		return model.Switch{}, ErrInvalidState
	}
	if previousState == model.StateOverdue {
		if err := insertAudit(ctx, tx, id, "switch.reactivated_by_settings", map[string]any{"from": model.StateOverdue, "to": model.StateActive}, now); err != nil {
			return model.Switch{}, err
		}
	}
	if err := insertAudit(ctx, tx, id, "switch.settings_updated", map[string]any{"intervalDays": intervalDays, "graceDays": graceDays}, now); err != nil {
		return model.Switch{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Switch{}, err
	}
	return s.SwitchByID(ctx, userID, id)
}

// CheckIn resets a live switch to ACTIVE and records an audit entry transactionally.
func (s *Store) CheckIn(ctx context.Context, switchID, source string) (model.Switch, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Switch{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := scanSwitch(tx.QueryRowContext(ctx, switchSelect+" WHERE id=?", switchID))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Switch{}, ErrNotFound
	}
	if err != nil {
		return model.Switch{}, err
	}
	if current.State == model.StateReleased || current.State == model.StateReleasing || current.State == model.StatePaused {
		return model.Switch{}, ErrInvalidState
	}
	now := s.clock.Now()
	deadline := now.AddDate(0, 0, current.IntervalDays)
	result, err := tx.ExecContext(ctx, `UPDATE switches SET state='ACTIVE',last_checkin_utc=?,next_deadline_utc=?,updated_at=?
      WHERE id=? AND state IN ('ACTIVE','OVERDUE')`, formatTime(now), formatTime(deadline), formatTime(now), switchID)
	if err != nil {
		return model.Switch{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return model.Switch{}, err
	}
	if count == 0 {
		return model.Switch{}, ErrInvalidState
	}
	if err := insertAudit(ctx, tx, switchID, "switch.checked_in", map[string]any{"source": source}, now); err != nil {
		return model.Switch{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Switch{}, err
	}
	current.State = model.StateActive
	current.LastCheckin = now
	current.NextDeadline = deadline
	current.UpdatedAt = now
	return current, nil
}

// Transition changes state only when the expected state still holds and records it atomically.
func (s *Store) Transition(ctx context.Context, switchID, from, to, event string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	now := s.clock.Now()
	result, err := tx.ExecContext(ctx, "UPDATE switches SET state=?,updated_at=? WHERE id=? AND state=?", to, formatTime(now), switchID, from)
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
	if err := insertAudit(ctx, tx, switchID, event, map[string]any{"from": from, "to": to}, now); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// Pause ignores the switch until it is explicitly resumed.
func (s *Store) Pause(ctx context.Context, userID, id string) (model.Switch, error) {
	value, err := s.SwitchByID(ctx, userID, id)
	if err != nil {
		return model.Switch{}, err
	}
	if value.State != model.StateActive && value.State != model.StateOverdue {
		return model.Switch{}, ErrInvalidState
	}
	changed, err := s.Transition(ctx, id, value.State, model.StatePaused, "switch.paused")
	if err != nil {
		return model.Switch{}, err
	}
	if !changed {
		return model.Switch{}, ErrInvalidState
	}
	return s.SwitchByID(ctx, userID, id)
}

// Resume activates a paused switch with a new full interval.
func (s *Store) Resume(ctx context.Context, userID, id string) (model.Switch, error) {
	value, err := s.SwitchByID(ctx, userID, id)
	if err != nil {
		return model.Switch{}, err
	}
	if value.State != model.StatePaused {
		return model.Switch{}, ErrInvalidState
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Switch{}, err
	}
	defer func() { _ = tx.Rollback() }()
	now := s.clock.Now()
	deadline := now.AddDate(0, 0, value.IntervalDays)
	result, err := tx.ExecContext(ctx, "UPDATE switches SET state='ACTIVE',last_checkin_utc=?,next_deadline_utc=?,updated_at=? WHERE id=? AND state='PAUSED'", formatTime(now), formatTime(deadline), formatTime(now), id)
	if err != nil {
		return model.Switch{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return model.Switch{}, err
	}
	if count == 0 {
		return model.Switch{}, ErrInvalidState
	}
	if err := insertAudit(ctx, tx, id, "switch.resumed", map[string]any{"to": model.StateActive}, now); err != nil {
		return model.Switch{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Switch{}, err
	}
	return s.SwitchByID(ctx, userID, id)
}

// RotateAPIToken revokes the old long-lived token and returns a new one once.
func (s *Store) RotateAPIToken(ctx context.Context, userID, switchID string) (string, error) {
	if _, err := s.mutableSwitch(ctx, userID, switchID); err != nil {
		return "", err
	}
	token, err := NewToken(32)
	if err != nil {
		return "", err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	now := s.clock.Now()
	if _, err := tx.ExecContext(ctx, "DELETE FROM checkin_tokens WHERE switch_id=? AND kind='api'", switchID); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO checkin_tokens(token_hash,switch_id,kind,created_at) VALUES(?,?,?,?)", HashToken(token), switchID, "api", formatTime(now)); err != nil {
		return "", err
	}
	if err := insertAudit(ctx, tx, switchID, "checkin.api_token_rotated", nil, now); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return token, nil
}

// CreateMagicToken creates a one-use check-in token.
func (s *Store) CreateMagicToken(ctx context.Context, userID, switchID string, ttl time.Duration) (string, error) {
	if _, err := s.mutableSwitch(ctx, userID, switchID); err != nil {
		return "", err
	}
	token, err := NewToken(32)
	if err != nil {
		return "", err
	}
	now := s.clock.Now()
	_, err = s.db.ExecContext(ctx, "INSERT INTO checkin_tokens(token_hash,switch_id,kind,expires_at,created_at) VALUES(?,?,?,?,?)", HashToken(token), switchID, "magic", formatTime(now.Add(ttl)), formatTime(now))
	return token, err
}

// CheckInWithMagicToken consumes a one-use magic token and resets its switch in one transaction.
// Keeping both effects atomic prevents a valid token from being burned without a successful check-in.
func (s *Store) CheckInWithMagicToken(ctx context.Context, token string) (model.Switch, error) {
	hash := HashToken(token)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Switch{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var switchID string
	var expires, used sql.NullString
	err = tx.QueryRowContext(ctx, "SELECT switch_id,expires_at,used_at FROM checkin_tokens WHERE token_hash=? AND kind='magic'", hash).Scan(&switchID, &expires, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Switch{}, ErrNotFound
	}
	if err != nil {
		return model.Switch{}, err
	}
	if used.Valid {
		return model.Switch{}, ErrTokenUsed
	}
	now := s.clock.Now()
	if expires.Valid {
		expiry, err := parseTime(expires.String)
		if err != nil {
			return model.Switch{}, err
		}
		if !expiry.After(now) {
			return model.Switch{}, ErrTokenExpired
		}
	}
	current, err := scanSwitch(tx.QueryRowContext(ctx, switchSelect+" WHERE id=?", switchID))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Switch{}, ErrNotFound
	}
	if err != nil {
		return model.Switch{}, err
	}
	if current.State != model.StateActive && current.State != model.StateOverdue {
		return model.Switch{}, ErrInvalidState
	}
	deadline := now.AddDate(0, 0, current.IntervalDays)
	result, err := tx.ExecContext(ctx, `UPDATE switches SET state='ACTIVE',last_checkin_utc=?,next_deadline_utc=?,updated_at=?
      WHERE id=? AND state IN ('ACTIVE','OVERDUE')`, formatTime(now), formatTime(deadline), formatTime(now), switchID)
	if err != nil {
		return model.Switch{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return model.Switch{}, err
	}
	if count == 0 {
		return model.Switch{}, ErrInvalidState
	}
	result, err = tx.ExecContext(ctx, "UPDATE checkin_tokens SET used_at=? WHERE token_hash=? AND used_at IS NULL", formatTime(now), hash)
	if err != nil {
		return model.Switch{}, err
	}
	count, err = result.RowsAffected()
	if err != nil {
		return model.Switch{}, err
	}
	if count == 0 {
		return model.Switch{}, ErrTokenUsed
	}
	if err := insertAudit(ctx, tx, switchID, "switch.checked_in", map[string]any{"source": "magic_link"}, now); err != nil {
		return model.Switch{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Switch{}, err
	}
	current.State = model.StateActive
	current.LastCheckin = now
	current.NextDeadline = deadline
	current.UpdatedAt = now
	return current, nil
}

// ConsumeCheckinToken validates the long-lived API token without consuming it.
func (s *Store) ConsumeCheckinToken(ctx context.Context, token, kind string) (string, error) {
	if kind != "api" {
		return "", ErrInvalidState
	}
	hash := HashToken(token)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	var switchID string
	var expires, used sql.NullString
	err = tx.QueryRowContext(ctx, "SELECT switch_id,expires_at,used_at FROM checkin_tokens WHERE token_hash=? AND kind=?", hash, kind).Scan(&switchID, &expires, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if used.Valid {
		return "", ErrTokenUsed
	}
	if expires.Valid {
		value, err := parseTime(expires.String)
		if err != nil {
			return "", err
		}
		if !value.After(s.clock.Now()) {
			return "", ErrTokenExpired
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return switchID, nil
}

const switchSelect = `SELECT id,user_id,name,interval_days,grace_days,state,next_deadline_utc,last_checkin_utc,
  reminder_offsets_json,release_token,recovery_threshold,recovery_share_count,fire_drill_at,created_at,updated_at FROM switches`

type scanner interface{ Scan(...any) error }

func scanSwitch(row scanner) (model.Switch, error) {
	var value model.Switch
	var deadline, checkin, reminders, created, updated string
	var fireDrill sql.NullString
	err := row.Scan(&value.ID, &value.UserID, &value.Name, &value.IntervalDays, &value.GraceDays, &value.State,
		&deadline, &checkin, &reminders, &value.ReleaseToken, &value.RecoveryThreshold, &value.RecoveryShareCount,
		&fireDrill, &created, &updated)
	if err != nil {
		return model.Switch{}, err
	}
	if value.NextDeadline, err = parseTime(deadline); err != nil {
		return model.Switch{}, err
	}
	if value.LastCheckin, err = parseTime(checkin); err != nil {
		return model.Switch{}, err
	}
	if value.CreatedAt, err = parseTime(created); err != nil {
		return model.Switch{}, err
	}
	if value.UpdatedAt, err = parseTime(updated); err != nil {
		return model.Switch{}, err
	}
	if value.FireDrillAt, err = parseOptionalTime(fireDrill); err != nil {
		return model.Switch{}, err
	}
	if err := json.Unmarshal([]byte(reminders), &value.Reminders); err != nil {
		return model.Switch{}, err
	}
	return value, nil
}
