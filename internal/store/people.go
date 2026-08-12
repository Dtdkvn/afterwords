package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/Dtdkvn/afterwords/internal/model"
)

func (s *Store) AddRecipient(ctx context.Context, userID, switchID, name, email string) (model.Recipient, error) {
	if _, err := s.mutableSwitch(ctx, userID, switchID); err != nil {
		return model.Recipient{}, err
	}
	name = strings.TrimSpace(name)
	email = strings.ToLower(strings.TrimSpace(email))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Recipient{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var existing model.Recipient
	var created string
	err = tx.QueryRowContext(ctx, "SELECT id,switch_id,name,email,created_at FROM recipients WHERE switch_id=? AND email=?", switchID, email).
		Scan(&existing.ID, &existing.SwitchID, &existing.Name, &existing.Email, &created)
	if err == nil {
		if existing.Name != name {
			return model.Recipient{}, ErrConflict
		}
		existing.CreatedAt, err = parseTime(created)
		return existing, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return model.Recipient{}, err
	}
	id, err := newID()
	if err != nil {
		return model.Recipient{}, err
	}
	now := s.clock.Now()
	value := model.Recipient{ID: id, SwitchID: switchID, Name: name, Email: email, CreatedAt: now}
	_, err = tx.ExecContext(ctx, "INSERT INTO recipients(id,switch_id,name,email,created_at) VALUES(?,?,?,?,?)", id, switchID, value.Name, value.Email, formatTime(now))
	if err != nil {
		if isUniqueError(err) {
			return model.Recipient{}, ErrConflict
		}
		return model.Recipient{}, err
	}
	if err := insertAudit(ctx, tx, switchID, "recipient.added", map[string]any{"recipientId": id}, now); err != nil {
		return model.Recipient{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Recipient{}, err
	}
	return value, nil
}

func (s *Store) ListRecipients(ctx context.Context, userID, switchID string) ([]model.Recipient, error) {
	if _, err := s.SwitchByID(ctx, userID, switchID); err != nil {
		return nil, err
	}
	return s.listRecipients(ctx, switchID)
}

func (s *Store) listRecipients(ctx context.Context, switchID string) ([]model.Recipient, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,switch_id,name,email,created_at FROM recipients WHERE switch_id=? ORDER BY created_at", switchID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]model.Recipient, 0)
	for rows.Next() {
		var value model.Recipient
		var created string
		if err := rows.Scan(&value.ID, &value.SwitchID, &value.Name, &value.Email, &created); err != nil {
			return nil, err
		}
		value.CreatedAt, err = parseTime(created)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) DeleteRecipient(ctx context.Context, userID, switchID, id string) error {
	if _, err := s.mutableSwitch(ctx, userID, switchID); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, "DELETE FROM recipients WHERE id=? AND switch_id=?", id, switchID)
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
	if err := insertAudit(ctx, tx, switchID, "recipient.removed", map[string]any{"recipientId": id}, s.clock.Now()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AddTrustee(ctx context.Context, userID, switchID, name, email string, shareIndex int, fingerprint string, threshold, shareCount int) (model.Trustee, error) {
	if _, err := s.mutableSwitch(ctx, userID, switchID); err != nil {
		return model.Trustee{}, err
	}
	name = strings.TrimSpace(name)
	email = strings.ToLower(strings.TrimSpace(email))
	fingerprint = strings.ToLower(fingerprint)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Trustee{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var currentThreshold, currentShareCount, trusteeCount int
	if err := tx.QueryRowContext(ctx, `SELECT recovery_threshold,recovery_share_count,
      (SELECT COUNT(*) FROM trustees WHERE switch_id=switches.id) FROM switches WHERE id=? AND user_id=?`, switchID, userID).
		Scan(&currentThreshold, &currentShareCount, &trusteeCount); errors.Is(err, sql.ErrNoRows) {
		return model.Trustee{}, ErrNotFound
	} else if err != nil {
		return model.Trustee{}, err
	}
	var existing model.Trustee
	var delivered sql.NullString
	var created string
	err = tx.QueryRowContext(ctx, `SELECT id,switch_id,name,email,share_index,share_fingerprint,kit_delivered_at,created_at
      FROM trustees WHERE switch_id=? AND (share_index=? OR share_fingerprint=?) LIMIT 1`, switchID, shareIndex, fingerprint).
		Scan(&existing.ID, &existing.SwitchID, &existing.Name, &existing.Email, &existing.ShareIndex, &existing.ShareFingerprint, &delivered, &created)
	if err == nil {
		if existing.Name != name || existing.Email != email || existing.ShareIndex != shareIndex || existing.ShareFingerprint != fingerprint ||
			currentThreshold != threshold || currentShareCount != shareCount {
			return model.Trustee{}, ErrConflict
		}
		existing.KitDeliveredAt, err = parseOptionalTime(delivered)
		if err != nil {
			return model.Trustee{}, err
		}
		existing.CreatedAt, err = parseTime(created)
		return existing, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return model.Trustee{}, err
	}
	if trusteeCount > 0 && (currentThreshold != threshold || currentShareCount != shareCount) {
		return model.Trustee{}, ErrConflict
	}
	id, err := newID()
	if err != nil {
		return model.Trustee{}, err
	}
	now := s.clock.Now()
	value := model.Trustee{ID: id, SwitchID: switchID, Name: name, Email: email, ShareIndex: shareIndex, ShareFingerprint: fingerprint, CreatedAt: now}
	if _, err := tx.ExecContext(ctx, "UPDATE switches SET recovery_threshold=?,recovery_share_count=?,updated_at=? WHERE id=? AND user_id=?", threshold, shareCount, formatTime(now), switchID, userID); err != nil {
		return model.Trustee{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO trustees(id,switch_id,name,email,share_index,share_fingerprint,created_at)
      VALUES(?,?,?,?,?,?,?)`, id, switchID, value.Name, value.Email, shareIndex, value.ShareFingerprint, formatTime(now))
	if err != nil {
		if isUniqueError(err) {
			return model.Trustee{}, ErrConflict
		}
		return model.Trustee{}, err
	}
	if err := insertAudit(ctx, tx, switchID, "trustee.added", map[string]any{"trusteeId": id, "shareIndex": shareIndex}, now); err != nil {
		return model.Trustee{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Trustee{}, err
	}
	return value, nil
}

func (s *Store) ListTrustees(ctx context.Context, userID, switchID string) ([]model.Trustee, error) {
	if _, err := s.SwitchByID(ctx, userID, switchID); err != nil {
		return nil, err
	}
	return s.listTrustees(ctx, switchID)
}

func (s *Store) listTrustees(ctx context.Context, switchID string) ([]model.Trustee, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,switch_id,name,email,share_index,share_fingerprint,kit_delivered_at,created_at
      FROM trustees WHERE switch_id=? ORDER BY share_index`, switchID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]model.Trustee, 0)
	for rows.Next() {
		var value model.Trustee
		var delivered sql.NullString
		var created string
		if err := rows.Scan(&value.ID, &value.SwitchID, &value.Name, &value.Email, &value.ShareIndex, &value.ShareFingerprint, &delivered, &created); err != nil {
			return nil, err
		}
		value.KitDeliveredAt, err = parseOptionalTime(delivered)
		if err != nil {
			return nil, err
		}
		value.CreatedAt, err = parseTime(created)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) MarkKitDelivered(ctx context.Context, userID, switchID, trusteeID string) error {
	if _, err := s.mutableSwitch(ctx, userID, switchID); err != nil {
		return err
	}
	now := s.clock.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, "UPDATE trustees SET kit_delivered_at=? WHERE id=? AND switch_id=?", formatTime(now), trusteeID, switchID)
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
	if err := insertAudit(ctx, tx, switchID, "trustee.kit_marked_delivered", map[string]any{"trusteeId": trusteeID}, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteTrustee(ctx context.Context, userID, switchID, id string) error {
	if _, err := s.mutableSwitch(ctx, userID, switchID); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, "DELETE FROM trustees WHERE id=? AND switch_id=?", id, switchID)
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
	if err := insertAudit(ctx, tx, switchID, "trustee.removed", map[string]any{"trusteeId": id}, s.clock.Now()); err != nil {
		return err
	}
	return tx.Commit()
}

// OwnerEmailForSwitch is used only for operational mail addressing.
func (s *Store) OwnerEmailForSwitch(ctx context.Context, switchID string) (string, error) {
	var email string
	err := s.db.QueryRowContext(ctx, `SELECT users.email FROM users JOIN switches ON switches.user_id=users.id WHERE switches.id=?`, switchID).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return email, err
}
