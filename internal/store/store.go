// Package store implements Afterwords persistence on SQLite.
package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/Dtdkvn/afterwords/internal/clock"
	"github.com/Dtdkvn/afterwords/migrations"
	_ "modernc.org/sqlite"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadySetup  = errors.New("owner account already exists")
	ErrQuotaExceeded = errors.New("vault exceeds the 25 MB limit")
	ErrInvalidState  = errors.New("operation is not valid in the current state")
	ErrTokenUsed     = errors.New("token has already been used")
	ErrTokenExpired  = errors.New("token has expired")
	ErrReservation   = errors.New("vault item reservation is invalid or expired")
	ErrEventClaimed  = errors.New("scheduled event already claimed")
	ErrConflict      = errors.New("conflicts with an existing record")
)

const timestampLayout = time.RFC3339Nano

// Store is safe for concurrent use.
type Store struct {
	db    *sql.DB
	clock clock.Clock
}

// Open connects to SQLite, applies migrations, and configures connection limits.
func Open(ctx context.Context, path string, c clock.Clock) (*Store, error) {
	if c == nil {
		return nil, fmt.Errorf("clock is required")
	}
	dsn := path
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		dsn = "file:" + path
	}
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	dsn += separator + "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db, clock: c}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		var applied int
		err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&applied)
		if err != nil {
			return fmt.Errorf("inspect schema: %w", err)
		}
		if applied > 0 {
			err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = ?", name).Scan(&applied)
			if err != nil {
				return fmt.Errorf("inspect migration %s: %w", name, err)
			}
			if applied > 0 {
				continue
			}
		}
		script, err := migrations.Files.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}
		if _, err = tx.ExecContext(ctx, string(script)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(?, ?)", name, formatTime(s.clock.Now())); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

// Ping checks database availability.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// DB is intentionally exposed only for diagnostic tests and health checks.
func (s *Store) DB() *sql.DB { return s.db }

// NewToken returns a cryptographically random URL-safe token.
func NewToken(bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

// HashToken hashes an opaque bearer token before persistence.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newID() (string, error) { return NewToken(18) }

func formatTime(value time.Time) string { return value.UTC().Format(timestampLayout) }

func parseTime(value string) (time.Time, error) { return time.Parse(timestampLayout, value) }

func parseOptionalTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid || value.String == "" {
		return nil, nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func isUniqueError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
