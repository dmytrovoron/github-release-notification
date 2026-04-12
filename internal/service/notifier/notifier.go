package notifier

import (
	"context"
	"fmt"
	"log/slog"

	integrationemail "github.com/dmytrovoron/github-release-notification/internal/integration/email"
)

// ConfirmationEmail carries the data needed to ask a new subscriber to confirm.
type ConfirmationEmail struct {
	Email        string
	Repository   string
	ConfirmToken string
	ConfirmURL   string
}

// ReleaseEmail carries the data needed to notify about a new release.
type ReleaseEmail struct {
	Email          string
	Repository     string
	Tag            string
	UnsubscribeURL string
}

// NotifierConfig defines SMTP settings used by notifier.
type NotifierConfig struct {
	SMTPHost     string
	SMTPPort     int
	SMTPFrom     string
	SMTPUsername string
	SMTPPassword string
}

// Notifier sends notification requests through the email integration.
type Notifier struct {
	sender *integrationemail.SMTPSender
}

func NewNotifier(log *slog.Logger, cfg NotifierConfig) *Notifier {
	return &Notifier{
		sender: integrationemail.NewSMTPSender(log, integrationemail.SMTPConfig{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			From:     cfg.SMTPFrom,
			Username: cfg.SMTPUsername,
			Password: cfg.SMTPPassword,
		}),
	}
}

func (n *Notifier) SendConfirmation(ctx context.Context, email ConfirmationEmail) error {
	err := n.sender.SendConfirmation(ctx, integrationemail.ConfirmationMessage{
		Email:        email.Email,
		Repository:   email.Repository,
		ConfirmToken: email.ConfirmToken,
		ConfirmURL:   email.ConfirmURL,
	})
	if err != nil {
		return fmt.Errorf("send confirmation email: %w", err)
	}

	return nil
}

func (n *Notifier) SendRelease(ctx context.Context, email ReleaseEmail) error {
	err := n.sender.SendRelease(ctx, integrationemail.ReleaseMessage{
		Email:          email.Email,
		Repository:     email.Repository,
		Tag:            email.Tag,
		UnsubscribeURL: email.UnsubscribeURL,
	})
	if err != nil {
		return fmt.Errorf("send release email: %w", err)
	}

	return nil
}
