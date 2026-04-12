package email

import (
	"bytes"
	"context"
	_ "embed"
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

// ReleaseMessage carries the data needed to send a release notification email.
type ReleaseMessage struct {
	Email          string
	Repository     string
	Tag            string
	UnsubscribeURL string
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

//go:embed templates/confirmation.html
var confirmationTmplContent string

//go:embed templates/release.html
var releaseTmplContent string

//nolint:gochecknoglobals // global template is fine here
var confirmationTmpl = template.Must(template.New("confirmation").Parse(confirmationTmplContent))

//nolint:gochecknoglobals // global template is fine here
var releaseTmpl = template.Must(template.New("release").Parse(releaseTmplContent))

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

	s.log.InfoContext(ctx, "Subscription confirmation email sent",
		"email", message.Email,
		"repository", message.Repository,
		"smtp_host", s.cfg.Host,
		"smtp_port", s.cfg.Port,
	)

	return nil
}

func (s *SMTPSender) SendRelease(ctx context.Context, message ReleaseMessage) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)

	var buf bytes.Buffer
	if err := releaseTmpl.Execute(&buf, struct {
		Repository     string
		Tag            string
		UnsubscribeURL string
	}{
		Repository:     message.Repository,
		Tag:            message.Tag,
		UnsubscribeURL: message.UnsubscribeURL,
	}); err != nil {
		return fmt.Errorf("render release email template: %w", err)
	}

	subject := "New release for " + message.Repository + ": " + message.Tag
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
		return fmt.Errorf("send release email via smtp: %w", err)
	}

	s.log.InfoContext(ctx, "Release notification email sent",
		"email", message.Email,
		"repository", message.Repository,
		"tag", message.Tag,
		"smtp_host", s.cfg.Host,
		"smtp_port", s.cfg.Port,
	)

	return nil
}
