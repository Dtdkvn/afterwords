package server

import (
	"context"
	"net/http"

	"github.com/Dtdkvn/afterwords/internal/model"
)

type contextKey string

const (
	userKey    contextKey = "user"
	sessionKey contextKey = "session"
)

func withIdentity(r *http.Request, user model.User, session model.Session) *http.Request {
	ctx := context.WithValue(r.Context(), userKey, user)
	ctx = context.WithValue(ctx, sessionKey, session)
	return r.WithContext(ctx)
}

func userFrom(r *http.Request) model.User { return r.Context().Value(userKey).(model.User) }

func sessionFrom(r *http.Request) model.Session { return r.Context().Value(sessionKey).(model.Session) }
