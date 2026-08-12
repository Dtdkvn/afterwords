package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Dtdkvn/afterwords/internal/model"
)

// AddAudit appends a non-secret event record.
func (s *Store) AddAudit(ctx context.Context, switchID, event string, meta map[string]any) error {
	return insertAudit(ctx, s.db, switchID, event, meta, s.clock.Now())
}

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertAudit(ctx context.Context, db execer, switchID, event string, meta map[string]any, now time.Time) error {
	id, err := newID()
	if err != nil {
		return err
	}
	if meta == nil {
		meta = map[string]any{}
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "INSERT INTO audit_log(id,switch_id,event,meta_json,created_at) VALUES(?,?,?,?,?)", id, switchID, event, string(encoded), formatTime(now))
	if err != nil {
		return fmt.Errorf("insert audit: %w", err)
	}
	return nil
}

// ListAudit returns the newest entries first.
func (s *Store) ListAudit(ctx context.Context, userID, switchID string, limit int) ([]model.AuditEntry, error) {
	if _, err := s.SwitchByID(ctx, userID, switchID); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,switch_id,event,meta_json,created_at FROM audit_log
      WHERE switch_id=? ORDER BY created_at DESC,rowid DESC LIMIT ?`, switchID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]model.AuditEntry, 0)
	for rows.Next() {
		var value model.AuditEntry
		var meta, created string
		if err := rows.Scan(&value.ID, &value.SwitchID, &value.Event, &meta, &created); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(meta), &value.Meta); err != nil {
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
