package notification

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

const (
	testSMTPFrom = "alerts@example.com"
	testSMTPTo   = "ops@example.com"
)

type smtpBehavior struct {
	dataCodes       []int
	rejectRecipient bool
	closeAfterData  bool
	hangGreeting    bool
}

type smtpFixture struct {
	listener   net.Listener
	tlsConfig  *tls.Config
	sender     *smtpSender
	behavior   smtpBehavior
	wg         sync.WaitGroup
	mu         sync.Mutex
	messages   []string
	secureAuth int
}

func newSMTPFixture(t *testing.T, mode string, behavior smtpBehavior) *smtpFixture {
	t.Helper()
	certificateServer := httptest.NewTLSServer(http.NotFoundHandler())
	defer certificateServer.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig := &tls.Config{Certificates: certificateServer.TLS.Certificates, MinVersion: tls.VersionTLS12}
	if mode == "implicit" {
		listener = tls.NewListener(listener, tlsConfig)
	}
	sender, err := newSMTPSender(SMTPOptions{
		Address: listener.Addr().String(), TLSMode: mode, Timeout: time.Second,
		Username: "test-user", Password: "test-password", From: testSMTPFrom,
		To: []string{testSMTPTo, "other@example.com"}, MaxRetries: 2,
	})
	if err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	sender.tlsConfig.RootCAs = x509.NewCertPool()
	sender.tlsConfig.RootCAs.AddCert(certificateServer.Certificate())
	fixture := &smtpFixture{listener: listener, tlsConfig: tlsConfig, sender: sender, behavior: behavior}
	fixture.wg.Go(func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			fixture.wg.Go(func() {
				defer func() { _ = conn.Close() }()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				fixture.serve(conn)
			})
		}
	})
	t.Cleanup(func() { _ = listener.Close(); fixture.wg.Wait() })
	return fixture
}

func (s *smtpFixture) serve(conn net.Conn) {
	wire := textproto.NewConn(conn)
	if s.behavior.hangGreeting {
		_, _ = wire.ReadLine()
		return
	}
	_, secure := conn.(*tls.Conn)
	_ = wire.PrintfLine("220 localhost ESMTP")
	recipients := 0
	for {
		line, err := wire.ReadLine()
		if err != nil {
			return
		}
		command, _, _ := strings.Cut(line, " ")
		switch command {
		case "EHLO", "HELO":
			_ = wire.PrintfLine("250-localhost\r\n250-STARTTLS\r\n250 AUTH PLAIN")
		case "STARTTLS":
			_ = wire.PrintfLine("220 ready for TLS")
			secured := tls.Server(conn, s.tlsConfig)
			if err := secured.Handshake(); err != nil {
				return
			}
			conn, secure = secured, true
			wire = textproto.NewConn(conn)
		case "AUTH":
			if !secure {
				_ = wire.PrintfLine("530 TLS required")
				continue
			}
			s.mu.Lock()
			s.secureAuth++
			s.mu.Unlock()
			_ = wire.PrintfLine("235 authenticated")
		case "MAIL":
			_ = wire.PrintfLine("250 sender accepted")
		case "RCPT":
			recipients++
			if s.behavior.rejectRecipient && recipients == 2 {
				_ = wire.PrintfLine("550 recipient rejected")
				continue
			}
			_ = wire.PrintfLine("250 recipient accepted")
		case "DATA":
			_ = wire.PrintfLine("354 send message")
			message, err := io.ReadAll(wire.DotReader())
			if err != nil {
				return
			}
			s.mu.Lock()
			index := len(s.messages)
			s.messages = append(s.messages, string(message))
			s.mu.Unlock()
			if s.behavior.closeAfterData {
				return
			}
			code := 250
			if index < len(s.behavior.dataCodes) {
				code = s.behavior.dataCodes[index]
			}
			_ = wire.PrintfLine("%d message result", code)
		case "QUIT":
			_ = wire.PrintfLine("221 bye")
			return
		default:
			_ = wire.PrintfLine("500 unsupported command")
		}
	}
}

func TestSMTPDeliveryAndStableRetries(t *testing.T) {
	for _, mode := range []string{"starttls", "implicit"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newSMTPFixture(t, mode, smtpBehavior{dataCodes: []int{451, 250}})
			d := newDispatcher(nil, fixture.sender, 2, logr.Discard())
			d.wait = func(context.Context, time.Duration) bool { return true }
			event := testEvent(t)
			event.Type = eventTimeout
			event.Workflow = &WorkflowInfo{Name: "nightly", Node: "backup"}
			event.Execution.Message = "private-output"
			if state := d.deliver(context.Background(), event); state != stateDelivered {
				t.Fatalf("delivery state=%s", state)
			}
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if len(fixture.messages) != 2 || fixture.messages[0] != fixture.messages[1] || fixture.secureAuth != 2 {
				t.Fatal("retry changed the message or authentication preceded TLS")
			}
			message, err := mail.ReadMessage(strings.NewReader(fixture.messages[0]))
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(quotedprintable.NewReader(message.Body))
			if err != nil {
				t.Fatal(err)
			}
			if message.Header.Get("Message-ID") == "" || !strings.Contains(message.Header.Get("Subject"), eventTimeout) ||
				!strings.Contains(string(body), "Workflow: nightly") || strings.Contains(string(body), "private-output") {
				t.Fatal("invalid MIME or email content")
			}
		})
	}
}

func TestSMTPRejectionAndUnknownAcknowledgement(t *testing.T) {
	for _, tt := range []struct {
		name     string
		behavior smtpBehavior
		messages int
	}{
		{name: "permanent DATA rejection", behavior: smtpBehavior{dataCodes: []int{550}}, messages: 1},
		{name: "recipient rejection aborts all recipients", behavior: smtpBehavior{rejectRecipient: true}},
		{name: "unknown acknowledgement is not resent", behavior: smtpBehavior{closeAfterData: true}, messages: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newSMTPFixture(t, "starttls", tt.behavior)
			d := newDispatcher(nil, fixture.sender, 3, logr.Discard())
			d.wait = func(context.Context, time.Duration) bool { return true }
			if state := d.deliver(context.Background(), testEvent(t)); state != stateRejected {
				t.Fatalf("delivery state=%s", state)
			}
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if len(fixture.messages) != tt.messages || fixture.secureAuth != 1 {
				t.Fatal("rejected email was partially submitted or retried")
			}
		})
	}
}

func TestSMTPCancellationTimeoutAndCertificate(t *testing.T) {
	fixture := newSMTPFixture(t, "starttls", smtpBehavior{hangGreeting: true})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	result := fixture.sender.send(ctx, testEventID, nil)
	if result.delivered || time.Since(started) > time.Second {
		t.Fatal("SMTP did not stop on request cancellation")
	}
	fixture.sender.opts.Timeout = 50 * time.Millisecond
	result = fixture.sender.send(context.Background(), testEventID, nil)
	if !result.retry || result.delivered {
		t.Fatal("SMTP timeout should be retryable")
	}
	untrusted := newSMTPFixture(t, "implicit", smtpBehavior{})
	untrusted.sender.tlsConfig.RootCAs = x509.NewCertPool()
	result = untrusted.sender.send(context.Background(), testEventID, nil)
	if result.retry || result.delivered || result.reason != "smtp_certificate" {
		t.Fatalf("certificate result=%+v", result)
	}
}

func TestSMTPTemplatesAndChannelDeduplication(t *testing.T) {
	opts := SMTPOptions{Address: "localhost:465", From: testSMTPFrom, To: []string{testSMTPTo},
		SubjectTemplate: "告警 {{.Task.Name}}", BodyTemplate: "任务 {{.Task.Name}} {{if .Workflow}}{{.Workflow.Name}}{{end}}"}
	sender, err := newSMTPSender(opts)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := sender.encode(testEvent(t))
	if err != nil {
		t.Fatal(err)
	}
	message, err := mail.ReadMessage(strings.NewReader(string(encoded)))
	if err != nil {
		t.Fatal(err)
	}
	subject, err := (&mime.WordDecoder{}).DecodeHeader(message.Header.Get("Subject"))
	if err != nil || !strings.HasPrefix(subject, "告警 ") {
		t.Fatalf("invalid encoded subject %q: %v", subject, err)
	}
	opts.SubjectTemplate = "invalid {{"
	if _, err := newSMTPSender(opts); err == nil {
		t.Fatal("invalid subject template accepted")
	}
	opts.SubjectTemplate = "alert\r\nBcc: other@example.com"
	sender, err = newSMTPSender(opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sender.encode(testEvent(t)); err == nil {
		t.Fatal("email header injection accepted")
	}
	webhook := newDispatcher(nil, newWebhook(Options{}), 0, logr.Discard())
	email := newDispatcher(nil, sender, 0, logr.Discard())
	if !webhook.enqueue(testEvent(t)) || !email.enqueue(testEvent(t)) {
		t.Fatal("one channel suppressed the other channel")
	}
}
