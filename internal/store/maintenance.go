package store

import "context"

// PruneExpired removes a bounded batch of ephemeral records using the injected clock.
func (s *Store) PruneExpired(ctx context.Context, limit int) (int64, error) {
	if limit < 1 || limit > 5000 {
		limit = 500
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	now := formatTime(s.clock.Now())
	statements := []struct {
		query string
		args  []any
	}{
		{query: "DELETE FROM sessions WHERE id IN (SELECT id FROM sessions WHERE expires_at<=? LIMIT ?)", args: []any{now, limit}},
		{query: "DELETE FROM checkin_tokens WHERE token_hash IN (SELECT token_hash FROM checkin_tokens WHERE kind='magic' AND (expires_at<=? OR used_at IS NOT NULL) LIMIT ?)", args: []any{now, limit}},
		{query: "DELETE FROM fire_drills WHERE token IN (SELECT token FROM fire_drills WHERE expires_at<=? LIMIT ?)", args: []any{now, limit}},
		{query: "DELETE FROM vault_reservations WHERE id IN (SELECT id FROM vault_reservations WHERE expires_at<=? LIMIT ?)", args: []any{now, limit}},
	}
	var removed int64
	for _, statement := range statements {
		result, err := tx.ExecContext(ctx, statement.query, statement.args...)
		if err != nil {
			return 0, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		removed += count
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return removed, nil
}
