package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Dtdkvn/afterwords/internal/model"
)

// SetupRequired reports whether the first owner still needs to be created.
func (s *Store) SetupRequired(ctx context.Context) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return false, fmt.Errorf("count users: %w", err)
	}
	return count == 0, nil
}

// CreateOwner atomically creates the only v0.1 account.
func (s *Store) CreateOwner(ctx context.Context, email, passwordHash string) (model.User, error) {
	id, err := newID()
	if err != nil {
		return model.User{}, err
	}
	now := s.clock.Now()
	email = strings.ToLower(strings.TrimSpace(email))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.User{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return model.User{}, fmt.Errorf("count owners: %w", err)
	}
	if count != 0 {
		return model.User{}, ErrAlreadySetup
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO users(id,email,password_hash,created_at) VALUES(?,?,?,?)", id, email, passwordHash, formatTime(now)); err != nil {
		return model.User{}, fmt.Errorf("insert owner: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return model.User{}, err
	}
	return model.User{ID: id, Email: email, CreatedAt: now}, nil
}

// UserCredentials returns the owner and password hash for login verification.
func (s *Store) UserCredentials(ctx context.Context, email string) (model.User, string, error) {
	var user model.User
	var created, hash string
	err := s.db.QueryRowContext(ctx, "SELECT id,email,password_hash,created_at FROM users WHERE email = ? COLLATE NOCASE", strings.TrimSpace(email)).Scan(&user.ID, &user.Email, &hash, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return model.User{}, "", ErrNotFound
	}
	if err != nil {
		return model.User{}, "", fmt.Errorf("load user: %w", err)
	}
	user.CreatedAt, err = parseTime(created)
	return user, hash, err
}

// UserByID loads a public user record.
func (s *Store) UserByID(ctx context.Context, id string) (model.User, error) {
	var user model.User
	var created string
	err := s.db.QueryRowContext(ctx, "SELECT id,email,created_at FROM users WHERE id=?", id).Scan(&user.ID, &user.Email, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return model.User{}, ErrNotFound
	}
	if err != nil {
		return model.User{}, err
	}
	user.CreatedAt, err = parseTime(created)
	return user, err
}

// CreateSession persists an opaque session and CSRF token.
func (s *Store) CreateSession(ctx context.Context, userID string, ttl time.Duration) (model.Session, error) {
	id, err := NewToken(32)
	if err != nil {
		return model.Session{}, err
	}
	csrf, err := NewToken(24)
	if err != nil {
		return model.Session{}, err
	}
	now := s.clock.Now()
	session := model.Session{ID: id, UserID: userID, CSRFToken: csrf, ExpiresAt: now.Add(ttl)}
	_, err = s.db.ExecContext(ctx, "INSERT INTO sessions(id,user_id,csrf_token,expires_at,created_at) VALUES(?,?,?,?,?)", session.ID, userID, csrf, formatTime(session.ExpiresAt), formatTime(now))
	if err != nil {
		return model.Session{}, fmt.Errorf("create session: %w", err)
	}
	return session, nil
}

// Session validates and loads an unexpired session.
func (s *Store) Session(ctx context.Context, id string) (model.Session, error) {
	var session model.Session
	var expires string
	err := s.db.QueryRowContext(ctx, "SELECT id,user_id,csrf_token,expires_at FROM sessions WHERE id=?", id).Scan(&session.ID, &session.UserID, &session.CSRFToken, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Session{}, ErrNotFound
	}
	if err != nil {
		return model.Session{}, err
	}
	session.ExpiresAt, err = parseTime(expires)
	if err != nil {
		return model.Session{}, err
	}
	if !session.ExpiresAt.After(s.clock.Now()) {
		_, _ = s.db.ExecContext(ctx, "DELETE FROM sessions WHERE id=?", id)
		return model.Session{}, ErrTokenExpired
	}
	return session, nil
}

// DeleteSession logs out one browser session.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE id=?", id)
	return err
}

// DeleteExpiredSessions performs routine cleanup.
func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", formatTime(s.clock.Now()))
	return err
}
