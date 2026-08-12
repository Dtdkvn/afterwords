package server

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Dtdkvn/afterwords/internal/clock"
)

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		if s.secureCookies {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) sameOrigin(next http.Handler) http.Handler {
	base, _ := url.Parse(s.baseURL)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" {
			parsed, err := url.Parse(origin)
			if err != nil || !strings.EqualFold(parsed.Host, base.Host) || !strings.EqualFold(parsed.Scheme, base.Scheme) {
				writeError(w, http.StatusForbidden, "cross_origin_denied", "Cross-origin requests are not allowed.")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "authentication_required", "Please sign in.")
			return
		}
		session, err := s.store.Session(r.Context(), cookie.Value)
		if err != nil {
			s.clearSessionCookie(w)
			writeError(w, http.StatusUnauthorized, "session_expired", "Your session has expired. Please sign in again.")
			return
		}
		user, err := s.store.UserByID(r.Context(), session.UserID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication_required", "Please sign in.")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			provided := r.Header.Get("X-CSRF-Token")
			if len(provided) != len(session.CSRFToken) || subtle.ConstantTimeCompare([]byte(provided), []byte(session.CSRFToken)) != 1 {
				writeError(w, http.StatusForbidden, "csrf_failed", "Refresh the page and try again.")
				return
			}
		}
		next.ServeHTTP(w, withIdentity(r, user, session))
	})
}

type rateEntry struct {
	windowStart time.Time
	count       int
}

type rateLimiter struct {
	mu      sync.Mutex
	entries map[string]rateEntry
	clock   clock.Clock
	limit   int
	window  time.Duration
}

func newRateLimiter(c clock.Clock, limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{entries: make(map[string]rateEntry), clock: c, limit: limit, window: window}
}

func (l *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		key := fmt.Sprintf("%s:%s", host, r.URL.Path)
		now := l.clock.Now()
		l.mu.Lock()
		entry := l.entries[key]
		if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= l.window || now.Before(entry.windowStart) {
			entry = rateEntry{windowStart: now}
		}
		entry.count++
		l.entries[key] = entry
		allowed := entry.count <= l.limit
		if len(l.entries) > 10000 {
			for candidate, value := range l.entries {
				if now.Sub(value.windowStart) > 2*l.window {
					delete(l.entries, candidate)
				}
			}
		}
		l.mu.Unlock()
		if !allowed {
			w.Header().Set("Retry-After", "300")
			writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts. Please wait a few minutes.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
