package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/yewud/afterwords/internal/model"
)

const vaultQuotaBytes int64 = 25 * 1024 * 1024

// ReserveVaultItem allocates an item identifier before encryption so AAD can bind to it.
func (s *Store) ReserveVaultItem(ctx context.Context, userID, switchID string) (string, error) {
	if _, err := s.mutableSwitch(ctx, userID, switchID); err != nil {
		return "", err
	}
	id, err := newID()
	if err != nil {
		return "", err
	}
	now := s.clock.Now()
	_, err = s.db.ExecContext(ctx, "INSERT INTO vault_reservations(id,switch_id,expires_at,created_at) VALUES(?,?,?,?)", id, switchID, formatTime(now.Add(time.Hour)), formatTime(now))
	return id, err
}

// PutVaultItem stores an opaque encrypted envelope after validating its reservation, AAD, and quota.
func (s *Store) PutVaultItem(ctx context.Context, userID, switchID, itemID, iv, ciphertext, aad string, size int64) (model.VaultItem, error) {
	if _, err := s.mutableSwitch(ctx, userID, switchID); err != nil {
		return model.VaultItem{}, err
	}
	if aad != switchID+":"+itemID {
		return model.VaultItem{}, fmt.Errorf("aad must be switch_id:item_id")
	}
	if size < 0 || size > vaultQuotaBytes {
		return model.VaultItem{}, ErrQuotaExceeded
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.VaultItem{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var existing int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM vault_items WHERE id=? AND switch_id=?", itemID, switchID).Scan(&existing); err != nil {
		return model.VaultItem{}, err
	}
	if existing == 0 {
		var expires string
		err := tx.QueryRowContext(ctx, "SELECT expires_at FROM vault_reservations WHERE id=? AND switch_id=?", itemID, switchID).Scan(&expires)
		if errors.Is(err, sql.ErrNoRows) {
			return model.VaultItem{}, ErrReservation
		}
		if err != nil {
			return model.VaultItem{}, err
		}
		expiry, err := parseTime(expires)
		if err != nil || !expiry.After(s.clock.Now()) {
			return model.VaultItem{}, ErrReservation
		}
	}
	var used int64
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(SUM(size_bytes),0) FROM vault_items WHERE switch_id=? AND id<>?", switchID, itemID).Scan(&used); err != nil {
		return model.VaultItem{}, err
	}
	if used+size > vaultQuotaBytes {
		return model.VaultItem{}, ErrQuotaExceeded
	}
	now := s.clock.Now()
	_, err = tx.ExecContext(ctx, `INSERT INTO vault_items(id,switch_id,iv,ciphertext,aad,size_bytes,created_at,updated_at)
      VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET iv=excluded.iv,ciphertext=excluded.ciphertext,
      aad=excluded.aad,size_bytes=excluded.size_bytes,updated_at=excluded.updated_at`, itemID, switchID, iv, ciphertext, aad, size, formatTime(now), formatTime(now))
	if err != nil {
		return model.VaultItem{}, err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM vault_reservations WHERE id=?", itemID); err != nil {
		return model.VaultItem{}, err
	}
	event := "vault.item_created"
	if existing > 0 {
		event = "vault.item_updated"
	}
	if err := insertAudit(ctx, tx, switchID, event, map[string]any{"itemId": itemID, "size": size}, now); err != nil {
		return model.VaultItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.VaultItem{}, err
	}
	return model.VaultItem{ID: itemID, SwitchID: switchID, IV: iv, Ciphertext: ciphertext, AAD: aad, Size: size, CreatedAt: now, UpdatedAt: now}, nil
}

// ListVaultItems returns only ciphertext envelopes.
func (s *Store) ListVaultItems(ctx context.Context, userID, switchID string) ([]model.VaultItem, error) {
	if _, err := s.SwitchByID(ctx, userID, switchID); err != nil {
		return nil, err
	}
	return s.listVaultItems(ctx, switchID)
}

func (s *Store) listVaultItems(ctx context.Context, switchID string) ([]model.VaultItem, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,switch_id,iv,ciphertext,aad,size_bytes,created_at,updated_at FROM vault_items WHERE switch_id=? ORDER BY created_at", switchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.VaultItem, 0)
	for rows.Next() {
		var item model.VaultItem
		var created, updated string
		if err := rows.Scan(&item.ID, &item.SwitchID, &item.IV, &item.Ciphertext, &item.AAD, &item.Size, &created, &updated); err != nil {
			return nil, err
		}
		item.CreatedAt, err = parseTime(created)
		if err != nil {
			return nil, err
		}
		item.UpdatedAt, err = parseTime(updated)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// DeleteVaultItem removes one ciphertext item.
func (s *Store) DeleteVaultItem(ctx context.Context, userID, switchID, itemID string) error {
	if _, err := s.mutableSwitch(ctx, userID, switchID); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, "DELETE FROM vault_items WHERE id=? AND switch_id=?", itemID, switchID)
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
	if err := insertAudit(ctx, tx, switchID, "vault.item_deleted", map[string]any{"itemId": itemID}, s.clock.Now()); err != nil {
		return err
	}
	return tx.Commit()
}

// UpsertKeyslot persists the owner-wrapped Data Key and public KDF parameters.
func (s *Store) UpsertKeyslot(ctx context.Context, userID, switchID, wrappedDK, iv, salt string, params map[string]int) (model.Keyslot, error) {
	if _, err := s.mutableSwitch(ctx, userID, switchID); err != nil {
		return model.Keyslot{}, err
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return model.Keyslot{}, err
	}
	id, err := newID()
	if err != nil {
		return model.Keyslot{}, err
	}
	now := s.clock.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Keyslot{}, err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `INSERT INTO keyslots(id,switch_id,type,wrapped_dk,iv,salt,kdf_params_json,created_at,updated_at)
      VALUES(?,?,'owner_passphrase',?,?,?,?,?,?) ON CONFLICT(switch_id) DO UPDATE SET wrapped_dk=excluded.wrapped_dk,
      iv=excluded.iv,salt=excluded.salt,kdf_params_json=excluded.kdf_params_json,updated_at=excluded.updated_at`,
		id, switchID, wrappedDK, iv, salt, string(encoded), formatTime(now), formatTime(now))
	if err != nil {
		return model.Keyslot{}, err
	}
	if err := insertAudit(ctx, tx, switchID, "keyslot.updated", map[string]any{"type": "owner_passphrase"}, now); err != nil {
		return model.Keyslot{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Keyslot{}, err
	}
	return s.Keyslot(ctx, userID, switchID)
}

// Keyslot loads wrapped key material. It never contains the passphrase or unwrapped key.
func (s *Store) Keyslot(ctx context.Context, userID, switchID string) (model.Keyslot, error) {
	if _, err := s.SwitchByID(ctx, userID, switchID); err != nil {
		return model.Keyslot{}, err
	}
	return s.keyslotInternal(ctx, switchID)
}

func (s *Store) keyslotInternal(ctx context.Context, switchID string) (model.Keyslot, error) {
	var slot model.Keyslot
	var params, created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id,switch_id,wrapped_dk,iv,salt,kdf_params_json,created_at,updated_at
      FROM keyslots WHERE switch_id=?`, switchID).Scan(&slot.ID, &slot.SwitchID, &slot.WrappedDK, &slot.IV, &slot.Salt, &params, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Keyslot{}, ErrNotFound
	}
	if err != nil {
		return model.Keyslot{}, err
	}
	if err := json.Unmarshal([]byte(params), &slot.KDFParams); err != nil {
		return model.Keyslot{}, err
	}
	slot.CreatedAt, err = parseTime(created)
	if err != nil {
		return model.Keyslot{}, err
	}
	slot.UpdatedAt, err = parseTime(updated)
	return slot, err
}
