// Package server exposes Afterwords' HTTP API and embedded browser application.
package server

import (
	"embed"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/yewud/afterwords/internal/auth"
	"github.com/yewud/afterwords/internal/clock"
	"github.com/yewud/afterwords/internal/store"
)

const sessionCookie = "afterwords_session"

//go:embed webdist
var webFiles embed.FS

type Server struct {
	store         *store.Store
	clock         clock.Clock
	baseURL       string
	secureCookies bool
	trustProxy    bool
	setupToken    string
	logger        *slog.Logger
	authLimiter   *rateLimiter
	dummyAuthHash string
}

func New(s *store.Store, c clock.Clock, baseURL string, secureCookies, trustProxy bool, setupToken string, logger *slog.Logger) *Server {
	dummyHash, _ := auth.HashPassword("constant timing placeholder")
	return &Server{store: s, clock: c, baseURL: strings.TrimRight(baseURL, "/"), secureCookies: secureCookies, trustProxy: trustProxy,
		setupToken: setupToken, logger: logger, authLimiter: newRateLimiter(c, 10, 5*time.Minute), dummyAuthHash: dummyHash}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	if s.trustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(s.securityHeaders)
	r.Use(s.sameOrigin)
	r.Get("/healthz", s.health)
	r.Get("/api/bootstrap", s.bootstrap)
	r.With(s.authLimiter.middleware).Post("/api/setup", s.setup)
	r.With(s.authLimiter.middleware).Post("/api/login", s.login)
	r.Get("/c/{token}", s.magicCheckinConfirmation)
	r.Post("/c/{token}", s.magicCheckin)
	r.Post("/api/checkin/{token}", s.apiCheckin)
	r.Get("/api/release/{token}/bundle", s.releaseBundle)

	r.Group(func(protected chi.Router) {
		protected.Use(s.authenticate)
		protected.Get("/api/session", s.currentSession)
		protected.Post("/api/logout", s.logout)
		protected.Get("/api/switches", s.listSwitches)
		protected.Post("/api/switches", s.createSwitch)
		protected.Get("/api/switches/{switchID}", s.getSwitch)
		protected.Patch("/api/switches/{switchID}", s.updateSwitch)
		protected.Delete("/api/switches/{switchID}", s.deleteSwitch)
		protected.Post("/api/switches/{switchID}/checkin", s.sessionCheckin)
		protected.Post("/api/switches/{switchID}/pause", s.pauseSwitch)
		protected.Post("/api/switches/{switchID}/resume", s.resumeSwitch)
		protected.Post("/api/switches/{switchID}/magic", s.sendMagicLink)
		protected.Post("/api/switches/{switchID}/rotate-api-token", s.rotateAPIToken)
		protected.Post("/api/switches/{switchID}/firedrill", s.fireDrill)
		protected.Get("/api/switches/{switchID}/vault", s.listVault)
		protected.Post("/api/switches/{switchID}/vault/reserve", s.reserveVault)
		protected.Put("/api/switches/{switchID}/vault/{itemID}", s.putVault)
		protected.Delete("/api/switches/{switchID}/vault/{itemID}", s.deleteVault)
		protected.Get("/api/switches/{switchID}/keyslot", s.getKeyslot)
		protected.Put("/api/switches/{switchID}/keyslot", s.putKeyslot)
		protected.Get("/api/switches/{switchID}/recipients", s.listRecipients)
		protected.Post("/api/switches/{switchID}/recipients", s.addRecipient)
		protected.Delete("/api/switches/{switchID}/recipients/{personID}", s.deleteRecipient)
		protected.Get("/api/switches/{switchID}/trustees", s.listTrustees)
		protected.Post("/api/switches/{switchID}/trustees", s.addTrustee)
		protected.Post("/api/switches/{switchID}/trustees/{personID}/delivered", s.markKitDelivered)
		protected.Delete("/api/switches/{switchID}/trustees/{personID}", s.deleteTrustee)
		protected.Get("/api/switches/{switchID}/audit", s.listAudit)
		protected.Post("/api/switches/{switchID}/outbox/{messageID}/retry", s.retryDeadLetter)
	})
	r.Handle("/*", s.spaHandler())
	return r
}

func (s *Server) setSessionCookie(w http.ResponseWriter, id string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: id, Path: "/", HttpOnly: true, Secure: s.secureCookies,
		SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(sessionTTL.Seconds())})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: s.secureCookies,
		SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}

func (s *Server) spaHandler() http.Handler {
	dist, err := fs.Sub(webFiles, "webdist")
	if err != nil {
		panic(err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if requested == "." || requested == "" {
			requested = "index.html"
		}
		if _, err := fs.Stat(dist, requested); err != nil {
			if path.Ext(requested) != "" {
				http.NotFound(w, r)
				return
			}
			requested = "index.html"
		}
		content, err := fs.ReadFile(dist, requested)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if contentType := mime.TypeByExtension(path.Ext(requested)); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		if requested == "index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		} else if strings.Contains(requested, "/assets/") || strings.HasPrefix(requested, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		_, _ = w.Write(content)
	})
}
