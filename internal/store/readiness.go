package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Dtdkvn/afterwords/internal/model"
)

// BeginReleaseIfReady moves an overdue switch into RELEASING only when its
// encrypted payload and recovery/delivery paths are complete. Incomplete setup
// fails closed: the switch remains OVERDUE and can become ready later.
func (s *Store) BeginReleaseIfReady(ctx context.Context, switchID string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var state string
	var threshold, shareCount int
	var keyslots, vaultItems, recipients, trustees, delivered int
	err = tx.QueryRowContext(ctx, `SELECT state,recovery_threshold,recovery_share_count,
      (SELECT COUNT(*) FROM keyslots WHERE switch_id=switches.id),
      (SELECT COUNT(*) FROM vault_items WHERE switch_id=switches.id),
      (SELECT COUNT(*) FROM recipients WHERE switch_id=switches.id),
      (SELECT COUNT(*) FROM trustees WHERE switch_id=switches.id),
      (SELECT COUNT(*) FROM trustees WHERE switch_id=switches.id AND kit_delivered_at IS NOT NULL)
      FROM switches WHERE id=?`, switchID).
		Scan(&state, &threshold, &shareCount, &keyslots, &vaultItems, &recipients, &trustees, &delivered)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	if state != model.StateOverdue {
		return false, nil
	}
	ready := keyslots == 1 && vaultItems > 0 && recipients > 0 && trustees == shareCount && delivered == trustees &&
		shareCount > 0 && threshold > 0 && threshold <= shareCount
	if !ready {
		return false, nil
	}
	now := s.clock.Now()
	result, err := tx.ExecContext(ctx, "UPDATE switches SET state=?,updated_at=? WHERE id=? AND state=?", model.StateReleasing, formatTime(now), switchID, model.StateOverdue)
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
	if err := insertAudit(ctx, tx, switchID, "switch.releasing", map[string]any{"from": model.StateOverdue, "to": model.StateReleasing}, now); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
