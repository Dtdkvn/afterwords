package mail

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Dtdkvn/afterwords/internal/clock"
	"github.com/Dtdkvn/afterwords/internal/model"
	"github.com/Dtdkvn/afterwords/internal/store"
	mailclient "github.com/wneessen/go-mail"
)

func TestParseTLSPolicy(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  mailclient.TLSPolicy
	}{
		{name: "mandatory", value: "mandatory", want: mailclient.TLSMandatory},
		{name: "opportunistic", value: "opportunistic", want: mailclient.TLSOpportunistic},
		{name: "none", value: "none", want: mailclient.NoTLS},
		{name: "normalized", value: " MANDATORY ", want: mailclient.TLSMandatory},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseTLSPolicy(test.value)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("parseTLSPolicy(%q) = %v, want %v", test.value, got, test.want)
			}
		})
	}
	if _, err := parseTLSPolicy("invalid"); err == nil {
		t.Fatal("expected invalid policy to fail")
	}
	if _, err := NewSender(nil, "localhost", 25, "", "", "sender@example.com", "invalid", slog.Default()); err == nil {
		t.Fatal("NewSender accepted an invalid TLS policy")
	}
}

func TestDeliverBuildsMultipartMessage(t *testing.T) {
	server := startPlainSMTP(t)
	sender, err := NewSender(nil, server.host, server.port, "", "", "Afterwords <valid-from@domain.tld>", "none", slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := sender.deliver(ctx, "valid-to@domain.tld", "A calm subject", "<p>HTML body</p>", "plain body"); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-server.messages:
		for _, marker := range []string{"Subject: A calm subject", "multipart/alternative", "text/plain", "text/html", "plain body", "HTML body"} {
			if !strings.Contains(message, marker) {
				t.Fatalf("SMTP message did not contain %q:\n%s", marker, message)
			}
		}
	case <-ctx.Done():
		t.Fatal("SMTP server did not receive a message")
	}
}

func TestDeliverFailsClosedWhenSTARTTLSIsUnavailable(t *testing.T) {
	server := startPlainSMTP(t)
	sender, err := NewSender(nil, server.host, server.port, "", "", "Afterwords <valid-from@domain.tld>", "mandatory", slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = sender.deliver(ctx, "valid-to@domain.tld", "Subject", "<p>body</p>", "body")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "starttls") {
		t.Fatalf("mandatory TLS error=%v, want STARTTLS refusal", err)
	}
	select {
	case message := <-server.messages:
		t.Fatalf("message was delivered without mandatory TLS: %s", message)
	default:
	}
}

func TestDeliverValidatesConfigurationAndAddresses(t *testing.T) {
	tests := []struct {
		name string
		host string
		from string
		to   string
		want string
	}{
		{name: "missing host", from: "valid-from@domain.tld", to: "valid-to@domain.tld", want: "SMTP_HOST"},
		{name: "invalid sender", host: "localhost", from: "bad<", to: "valid-to@domain.tld", want: "SMTP_FROM"},
		{name: "invalid recipient", host: "localhost", from: "valid-from@domain.tld", to: "bad<", want: "mail address"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sender, err := NewSender(nil, test.host, 1, "", "", test.from, "none", slog.Default())
			if err != nil {
				t.Fatal(err)
			}
			err = sender.deliver(context.Background(), test.to, "subject", "html", "text")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("deliver error=%v, want substring %q", err, test.want)
			}
		})
	}
}

func TestDeliverBatchMarksSentAndDeadLetteredMessages(t *testing.T) {
	t.Run("sent", func(t *testing.T) {
		server := startPlainSMTP(t)
		database, ownerID, switchID, messageID := mailStoreFixture(t)
		var logs bytes.Buffer
		sender, err := NewSender(database, server.host, server.port, "", "", "Afterwords <valid-from@domain.tld>", "none", slog.New(slog.NewTextHandler(&logs, nil)))
		if err != nil {
			t.Fatal(err)
		}
		sender.deliverBatch(context.Background())
		var sentAt *string
		var attempts int
		if err := database.DB().QueryRow("SELECT sent_at,attempts FROM outbox WHERE id=?", messageID).Scan(&sentAt, &attempts); err != nil {
			t.Fatal(err)
		}
		if sentAt == nil || attempts != 0 {
			t.Fatalf("sent_at=%v attempts=%d, want sent and zero retries", sentAt, attempts)
		}
		assertAuditEvent(t, database, ownerID, switchID, "email.sent")
	})

	t.Run("dead letter", func(t *testing.T) {
		database, ownerID, switchID, messageID := mailStoreFixture(t)
		if _, err := database.DB().Exec("UPDATE outbox SET attempts=9 WHERE id=?", messageID); err != nil {
			t.Fatal(err)
		}
		var logs bytes.Buffer
		sender, err := NewSender(database, "", 25, "", "", "valid-from@domain.tld", "mandatory", slog.New(slog.NewTextHandler(&logs, nil)))
		if err != nil {
			t.Fatal(err)
		}
		sender.deliverBatch(context.Background())
		var attempts int
		if err := database.DB().QueryRow("SELECT attempts FROM outbox WHERE id=?", messageID).Scan(&attempts); err != nil {
			t.Fatal(err)
		}
		if attempts != 10 {
			t.Fatalf("attempts=%d, want 10", attempts)
		}
		if !strings.Contains(logs.String(), "moved to dead letter") || strings.Contains(logs.String(), "recipient@example.com") {
			t.Fatalf("terminal log was missing or exposed recipient data: %s", logs.String())
		}
		assertAuditEvent(t, database, ownerID, switchID, "email.dead_lettered")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		sender.Run(ctx)
	})
}

func mailStoreFixture(t *testing.T) (*store.Store, string, string, string) {
	t.Helper()
	ctx := context.Background()
	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	database, err := store.Open(ctx, "file:"+name+"?mode=memory&cache=shared", clock.NewManual(time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	owner, err := database.CreateOwner(ctx, name+"@example.com", "unused")
	if err != nil {
		t.Fatal(err)
	}
	value, _, err := database.CreateSwitch(ctx, owner.ID, store.CreateSwitchParams{Name: "Letters", IntervalDays: 7, GraceDays: 3, Reminders: model.DefaultReminderSchedule()})
	if err != nil {
		t.Fatal(err)
	}
	messageID, err := database.EnqueueMessage(ctx, model.OutboxMessage{SwitchID: value.ID, Kind: "checkin_reminder", ToEmail: "valid-to@domain.tld", Subject: "Reminder", BodyHTML: "<p>HTML body</p>", BodyText: "plain body"})
	if err != nil {
		t.Fatal(err)
	}
	return database, owner.ID, value.ID, messageID
}

func assertAuditEvent(t *testing.T, database *store.Store, ownerID, switchID, want string) {
	t.Helper()
	entries, err := database.ListAudit(context.Background(), ownerID, switchID, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Event == want {
			return
		}
	}
	t.Fatalf("audit event %q not found in %#v", want, entries)
}

type plainSMTPServer struct {
	host        string
	port        int
	messages    chan string
	listener    net.Listener
	done        chan struct{}
	wg          sync.WaitGroup
	connections sync.Map
}

func startPlainSMTP(t *testing.T) *plainSMTPServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host, rawPort, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatal(err)
	}
	server := &plainSMTPServer{host: host, port: port, messages: make(chan string, 4), listener: listener, done: make(chan struct{})}
	server.wg.Add(1)
	go func() {
		defer server.wg.Done()
		for {
			connection, err := listener.Accept()
			if err != nil {
				select {
				case <-server.done:
					return
				default:
					return
				}
			}
			server.wg.Add(1)
			server.connections.Store(connection, struct{}{})
			go func() {
				defer server.wg.Done()
				defer server.connections.Delete(connection)
				handleSMTPConnection(connection, server.messages)
			}()
		}
	}()
	t.Cleanup(func() {
		close(server.done)
		_ = server.listener.Close()
		server.connections.Range(func(key, _ any) bool {
			_ = key.(net.Conn).Close()
			return true
		})
		server.wg.Wait()
	})
	return server
}

func handleSMTPConnection(connection net.Conn, messages chan<- string) {
	defer func() { _ = connection.Close() }()
	reader := bufio.NewReader(connection)
	writer := bufio.NewWriter(connection)
	writeLine := func(line string) bool {
		if _, err := fmt.Fprintf(writer, "%s\r\n", line); err != nil {
			return false
		}
		return writer.Flush() == nil
	}
	if !writeLine("220 afterwords test SMTP ready") {
		return
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		command := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(command, "EHLO"), strings.HasPrefix(command, "HELO"):
			if !writeLine("250-localhost") || !writeLine("250 8BITMIME") {
				return
			}
		case strings.HasPrefix(command, "MAIL FROM:"), strings.HasPrefix(command, "RCPT TO:"), command == "RSET", command == "NOOP":
			if !writeLine("250 OK") {
				return
			}
		case command == "DATA":
			if !writeLine("354 End data with <CR><LF>.<CR><LF>") {
				return
			}
			var data strings.Builder
			for {
				line, err = reader.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(line, "\r\n") == "." {
					break
				}
				_, _ = io.WriteString(&data, line)
			}
			messages <- data.String()
			if !writeLine("250 queued") {
				return
			}
		case command == "QUIT":
			_ = writeLine("221 bye")
			return
		default:
			if !writeLine("500 unsupported") {
				return
			}
		}
	}
}
