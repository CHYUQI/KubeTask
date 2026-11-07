package notification

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"text/template"
	"time"
)

// SMTPOptions contains validated global email configuration.
type SMTPOptions struct {
	Address         string
	TLSMode         string
	Username        string
	Password        string
	From            string
	To              []string
	Timeout         time.Duration
	MaxRetries      int
	SubjectTemplate string
	BodyTemplate    string
}

type smtpSender struct {
	opts      SMTPOptions
	tlsConfig *tls.Config
	subject   *template.Template
	body      *template.Template
}

const defaultSMTPSubject = `[KubeTask] {{.Type}} - {{.Task.Name}}`

const defaultSMTPBody = `Event: {{.Type}}
Task: {{.Task.Name}}
Namespace: {{.Execution.Namespace}}
Job: {{.Execution.JobName}}
Reason: {{.Execution.Reason}}
Event ID: {{.ID}}
Observed at: {{.ObservedAt.UTC.Format "2006-01-02T15:04:05Z07:00"}}
{{if .Workflow}}Workflow: {{.Workflow.Name}}
Node: {{.Workflow.Node}}
{{end}}`

func newSMTPSender(opts SMTPOptions) (*smtpSender, error) {
	if opts.SubjectTemplate == "" {
		opts.SubjectTemplate = defaultSMTPSubject
	}
	if opts.BodyTemplate == "" {
		opts.BodyTemplate = defaultSMTPBody
	}
	subject, err := template.New("subject").Option("missingkey=error").Parse(opts.SubjectTemplate)
	if err != nil {
		return nil, fmt.Errorf("invalid smtp-subject-template")
	}
	body, err := template.New("body").Option("missingkey=error").Parse(opts.BodyTemplate)
	if err != nil {
		return nil, fmt.Errorf("invalid smtp-body-template")
	}
	host, _, _ := net.SplitHostPort(opts.Address)
	return &smtpSender{opts: opts, subject: subject, body: body, tlsConfig: &tls.Config{
		ServerName: host, MinVersion: tls.VersionTLS12,
	}}, nil
}

func (*smtpSender) close() {}

func (s *smtpSender) encode(e Event) ([]byte, error) {
	var subject, body strings.Builder
	if err := s.subject.Execute(&subject, e); err != nil {
		return nil, err
	}
	if strings.ContainsAny(subject.String(), "\r\n") || strings.TrimSpace(subject.String()) == "" {
		return nil, fmt.Errorf("email subject must be a non-empty single line")
	}
	if err := s.body.Execute(&body, e); err != nil {
		return nil, err
	}
	if strings.TrimSpace(body.String()) == "" {
		return nil, fmt.Errorf("email body must not be empty")
	}
	var message strings.Builder
	fmt.Fprintf(&message, "From: %s\r\nTo: %s\r\nSubject: %s\r\n", s.opts.From, strings.Join(s.opts.To, ",\r\n "), mime.QEncoding.Encode("UTF-8", subject.String()))
	fmt.Fprintf(&message, "Date: %s\r\nMessage-ID: <%x@kubetask.local>\r\n", e.ObservedAt.UTC().Format(time.RFC1123Z), sha256.Sum256([]byte(e.ID)))
	message.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n")
	writer := quotedprintable.NewWriter(&message)
	if _, err := io.WriteString(writer, body.String()); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return []byte(message.String()), nil
}

func (s *smtpSender) send(ctx context.Context, _ string, body []byte) attemptResult {
	ctx, cancel := context.WithTimeout(ctx, s.opts.Timeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", s.opts.Address)
	if err != nil {
		return smtpFailure(err, "smtp_connect")
	}
	defer func() { _ = conn.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	transport := conn
	if s.opts.TLSMode == "implicit" {
		transport = tls.Client(conn, s.tlsConfig)
	}
	client, err := smtp.NewClient(transport, s.tlsConfig.ServerName)
	if err != nil {
		return smtpFailure(err, "smtp_greeting")
	}
	if s.opts.TLSMode == "starttls" {
		if err := client.StartTLS(s.tlsConfig); err != nil {
			return smtpFailure(err, "smtp_starttls")
		}
	}
	if s.opts.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.opts.Username, s.opts.Password, s.tlsConfig.ServerName)); err != nil {
			return smtpFailure(err, "smtp_auth")
		}
	}
	if err := client.Mail(s.opts.From); err != nil {
		return smtpFailure(err, "smtp_sender")
	}
	for _, recipient := range s.opts.To {
		if err := client.Rcpt(recipient); err != nil {
			return smtpFailure(err, "smtp_recipient")
		}
	}
	writer, err := client.Data()
	if err != nil {
		return smtpFailure(err, "smtp_data")
	}
	if _, err := writer.Write(body); err != nil {
		return smtpFailure(err, "smtp_write")
	}
	if err := writer.Close(); err != nil {
		// An absent acknowledgement after DATA can mean the server accepted the
		// email. Only an explicit transient SMTP reply is safe to retry here.
		return smtpFailure(err, "smtp_ack")
	}
	_ = client.Quit()
	return attemptResult{status: 250, delivered: true, reason: "smtp_accepted"}
}

func smtpFailure(err error, stage string) attemptResult {
	if reply, ok := errors.AsType[*textproto.Error](err); ok {
		return attemptResult{status: reply.Code, retry: reply.Code >= 400 && reply.Code < 500, reason: stage}
	}
	if stage == "smtp_ack" {
		return attemptResult{reason: "smtp_ack_unknown"}
	}
	if _, ok := errors.AsType[*tls.CertificateVerificationError](err); ok {
		return attemptResult{reason: "smtp_certificate"}
	}
	_, networkError := errors.AsType[net.Error](err)
	return attemptResult{retry: networkError || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF), reason: stage}
}
