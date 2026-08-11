package mail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/afterwords/afterwords/internal/store"
	mailclient "github.com/wneessen/go-mail"
)

// Sender drains the SQLite outbox and retries transient failures.
type Sender struct {
	store     *store.Store
	host      string
	port      int
	username  string
	password  string
	from      string
	tlsPolicy mailclient.TLSPolicy
	logger    *slog.Logger
}

func NewSender(s *store.Store, host string, port int, username, password, from, tlsPolicy string, logger *slog.Logger) (*Sender, error) {
	policy, err := parseTLSPolicy(tlsPolicy)
	if err != nil {
		return nil, err
	}
	return &Sender{store: s, host: host, port: port, username: username, password: password, from: from, tlsPolicy: policy, logger: logger}, nil
}

func parseTLSPolicy(value string) (mailclient.TLSPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "mandatory":
		return mailclient.TLSMandatory, nil
	case "opportunistic":
		return mailclient.TLSOpportunistic, nil
	case "none":
		return mailclient.NoTLS, nil
	default:
		return mailclient.TLSMandatory, fmt.Errorf("unsupported SMTP TLS policy %q", value)
	}
}

// Run polls until context cancellation.
func (s *Sender) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	s.deliverBatch(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.deliverBatch(ctx)
		}
	}
}

func (s *Sender) deliverBatch(ctx context.Context) {
	messages, err := s.store.PendingMessages(ctx, 20)
	if err != nil {
		s.logger.Error("load outbox", "error", err)
		return
	}
	for _, message := range messages {
		if err := s.deliver(ctx, message.ToEmail, message.Subject, message.BodyHTML, message.BodyText); err != nil {
			s.logger.Warn("email delivery failed", "message_id", message.ID, "kind", message.Kind)
			if markErr := s.store.MarkMessageFailed(ctx, message.ID, message.Attempts, err); markErr != nil {
				s.logger.Error("record email failure", "message_id", message.ID, "error", markErr)
			}
			continue
		}
		if err := s.store.MarkMessageSent(ctx, message.ID); err != nil {
			s.logger.Error("mark email sent", "message_id", message.ID, "error", err)
		}
	}
}

func (s *Sender) deliver(ctx context.Context, to, subject, htmlBody, textBody string) error {
	if s.host == "" {
		return errors.New("SMTP_HOST is not configured")
	}
	from, err := mail.ParseAddress(s.from)
	if err != nil {
		return fmt.Errorf("parse SMTP_FROM: %w", err)
	}
	options := []mailclient.Option{
		mailclient.WithPort(s.port),
		mailclient.WithTLSPolicy(s.tlsPolicy),
		mailclient.WithTimeout(10 * time.Second),
	}
	if s.username != "" {
		options = append(options, mailclient.WithSMTPAuth(mailclient.SMTPAuthPlain), mailclient.WithUsername(s.username), mailclient.WithPassword(s.password))
	}
	client, err := mailclient.NewClient(s.host, options...)
	if err != nil {
		return fmt.Errorf("create SMTP client: %w", err)
	}
	message := mailclient.NewMsg()
	if err := message.FromFormat(from.Name, from.Address); err != nil {
		return err
	}
	if err := message.To(to); err != nil {
		return err
	}
	message.Subject(subject)
	message.SetBodyString(mailclient.TypeTextPlain, textBody)
	message.AddAlternativeString(mailclient.TypeTextHTML, htmlBody)
	return client.DialAndSendWithContext(ctx, message)
}
