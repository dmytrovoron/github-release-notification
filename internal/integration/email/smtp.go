package email

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/smtp"
	"strings"
)

// ConfirmationMessage carries the data needed to send a confirmation email.
type ConfirmationMessage struct {
	Email        string
	Repository   string
	ConfirmToken string
	ConfirmURL   string
}

// SMTPConfig defines SMTP transport settings.
type SMTPConfig struct {
	Host     string
	Port     int
	From     string
	Username string
	Password string
}

type SMTPSender struct {
	log *slog.Logger
	cfg SMTPConfig
}

//nolint:gochecknoglobals // global template is fine here
var confirmationTmpl = template.Must(template.New("confirmation").Parse(`<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>Confirm your subscription</title></head>
<body style="font-family:sans-serif;color:#222;max-width:600px;margin:40px auto;padding:0 16px">
  <h2>Confirm your GitHub release subscription</h2>
  <p>Hello,</p>
  <p>You subscribed to release notifications for <strong><a href="https://github.com/{{.Repository}}">{{.Repository}}</a></strong>.</p>
  <p>Click the button below to confirm your subscription:</p>
  <p>
    <a href="{{.ConfirmURL}}"
       style="display:inline-block;padding:12px 24px;background:#238636;color:#fff;text-decoration:none;border-radius:6px;font-weight:bold">
      Confirm subscription
    </a>
  </p>
  <p>Or copy and paste this token into the confirmation endpoint:</p>
  <pre style="background:#f6f8fa;border:1px solid #d0d7de;border-radius:6px;padding:12px 16px;font-size:14px;overflow-wrap:break-word;white-space:pre-wrap;display:inline-block">{{.ConfirmToken}}</pre>
  <p style="color:#888;font-size:12px">If you did not request this, you can safely ignore this email.</p>
</body>
</html>`))

func NewSMTPSender(log *slog.Logger, cfg SMTPConfig) *SMTPSender {
	return &SMTPSender{
		log: log,
		cfg: cfg,
	}
}

func (s *SMTPSender) SendConfirmation(ctx context.Context, message ConfirmationMessage) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)

	var buf bytes.Buffer
	if err := confirmationTmpl.Execute(&buf, struct {
		Repository   string
		ConfirmURL   string
		ConfirmToken string
	}{
		Repository:   message.Repository,
		ConfirmURL:   message.ConfirmURL,
		ConfirmToken: message.ConfirmToken,
	}); err != nil {
		return fmt.Errorf("render confirmation email template: %w", err)
	}

	subject := "Confirm your GitHub release subscription for " + message.Repository
	mailBody := strings.Join([]string{
		"From: " + s.cfg.From,
		"To: " + message.Email,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		"",
		buf.String(),
	}, "\r\n")

	var auth smtp.Auth
	if s.cfg.Username != "" {
		auth = smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	}

	if err := smtp.SendMail(addr, auth, s.cfg.From, []string{message.Email}, []byte(mailBody)); err != nil {
		return fmt.Errorf("send confirmation email via smtp: %w", err)
	}

	s.log.InfoContext(ctx, "subscription confirmation email sent",
		"email", message.Email,
		"repository", message.Repository,
		"smtp_host", s.cfg.Host,
		"smtp_port", s.cfg.Port,
	)

	return nil
}
