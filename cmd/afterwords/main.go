// Command afterwords runs the self-hosted Afterwords server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	appclock "github.com/Dtdkvn/afterwords/internal/clock"
	"github.com/Dtdkvn/afterwords/internal/config"
	"github.com/Dtdkvn/afterwords/internal/engine"
	appmail "github.com/Dtdkvn/afterwords/internal/mail"
	"github.com/Dtdkvn/afterwords/internal/server"
	"github.com/Dtdkvn/afterwords/internal/store"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "afterwords:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	level := slog.LevelInfo
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	if cfg.DBPath != ":memory:" && !strings.HasPrefix(cfg.DBPath, "file:") {
		directory := filepath.Dir(cfg.DBPath)
		if directory != "." {
			if err := os.MkdirAll(directory, 0o700); err != nil {
				return fmt.Errorf("create database directory: %w", err)
			}
		}
	}
	virtualClock := appclock.NewScaled(cfg.TimeScale)
	if cfg.TimeScale != 1 {
		logger.Warn("TIME_SCALE DEMO MODE IS ACTIVE; DO NOT USE THIS SETTING FOR REAL SAFETY WORKFLOWS", "time_scale", cfg.TimeScale)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	database, err := store.Open(ctx, cfg.DBPath, virtualClock)
	if err != nil {
		return err
	}
	defer database.Close()

	stateEngine := engine.New(database, virtualClock, cfg.BaseURL, logger)
	mailSender, err := appmail.NewSender(database, cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom, cfg.SMTPTLSPolicy, logger)
	if err != nil {
		return fmt.Errorf("configure mail sender: %w", err)
	}
	go stateEngine.Run(ctx, cfg.TimeScale)
	go mailSender.Run(ctx)

	app := server.New(database, virtualClock, cfg.BaseURL, cfg.SecureCookies, cfg.TrustProxy, cfg.SetupToken, logger)
	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       35 * time.Second,
		WriteTimeout:      35 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    64 * 1024,
	}
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("Afterwords started", "address", httpServer.Addr, "base_url", cfg.BaseURL, "version", version, "commit", commit, "built_at", date)
		serverErrors <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer shutdownCancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return nil
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
