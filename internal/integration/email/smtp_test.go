package email_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dmytrovoron/github-release-notification/internal/integration/email"
)

func TestSMTPSender_SendConfirmation_Mail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		message     email.ConfirmationMessage
		wantSubject string
		wantFrom    string
		wantTo      string
		wantInBody  []string
	}{
		{
			name: "all fields rendered correctly",
			message: email.ConfirmationMessage{
				Email:        "user@example.com",
				Repository:   "golang/go",
				ConfirmToken: "0123456789abcdef",
				ConfirmURL:   "https://example.com/confirm/0123456789abcdef",
			},
			wantSubject: "Subject: Confirm your GitHub release subscription for golang/go",
			wantFrom:    "From: noreply@example.com",
			wantTo:      "To: user@example.com",
			wantInBody: []string{
				"golang/go",
				"0123456789abcdef",
				"https://example.com/confirm/0123456789abcdef",
				"Content-Type: text/html; charset=UTF-8",
			},
		},
		{
			name: "different repository and email",
			message: email.ConfirmationMessage{
				Email:        "another@test.org",
				Repository:   "owner/my-repo",
				ConfirmToken: "aabbccddeeff0011",
				ConfirmURL:   "https://host.io/confirm/aabbccddeeff0011",
			},
			wantSubject: "Subject: Confirm your GitHub release subscription for owner/my-repo",
			wantFrom:    "From: noreply@example.com",
			wantTo:      "To: another@test.org",
			wantInBody: []string{
				"owner/my-repo",
				"aabbccddeeff0011",
				"https://host.io/confirm/aabbccddeeff0011",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			host, port, getCapture := startFakeSMTP(t, false)
			cfg := email.SMTPConfig{Host: host, Port: port, From: "noreply@example.com"}
			sender := email.NewSMTPSender(slog.New(slog.DiscardHandler), cfg)

			err := sender.SendConfirmation(t.Context(), tc.message)
			require.NoError(t, err)

			got := getCapture()
			assert.Contains(t, got.data, tc.wantSubject)
			assert.Contains(t, got.data, tc.wantFrom)
			assert.Contains(t, got.data, tc.wantTo)
			for _, want := range tc.wantInBody {
				assert.Contains(t, got.data, want)
			}
		})
	}
}

func TestSMTPSender_SendRelease_Mail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		message     email.ReleaseMessage
		wantSubject string
		wantInBody  []string
	}{
		{
			name: "all fields rendered correctly",
			message: email.ReleaseMessage{
				Email:          "user@example.com",
				Repository:     "golang/go",
				Tag:            "v1.22.0",
				UnsubscribeURL: "https://example.com/unsubscribe/token123",
			},
			wantSubject: "Subject: New release for golang/go: v1.22.0",
			wantInBody: []string{
				"golang/go",
				"v1.22.0",
				"https://example.com/unsubscribe/token123",
				"Content-Type: text/html; charset=UTF-8",
			},
		},
		{
			name: "different repository and tag",
			message: email.ReleaseMessage{
				Email:          "dev@company.io",
				Repository:     "torvalds/linux",
				Tag:            "v6.8",
				UnsubscribeURL: "https://example.com/unsubscribe/xyz",
			},
			wantSubject: "Subject: New release for torvalds/linux: v6.8",
			wantInBody: []string{
				"torvalds/linux",
				"v6.8",
				"https://example.com/unsubscribe/xyz",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			host, port, getCapture := startFakeSMTP(t, false)
			cfg := email.SMTPConfig{Host: host, Port: port, From: "noreply@example.com"}
			sender := email.NewSMTPSender(slog.New(slog.DiscardHandler), cfg)

			err := sender.SendRelease(t.Context(), tc.message)
			require.NoError(t, err)

			got := getCapture()
			assert.Contains(t, got.data, tc.wantSubject)
			for _, want := range tc.wantInBody {
				assert.Contains(t, got.data, want)
			}
		})
	}
}

func TestSMTPSender_Auth_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		username     string
		password     string
		wantAuthUsed bool
	}{
		{
			name:         "no username - auth skipped",
			username:     "",
			password:     "",
			wantAuthUsed: false,
		},
		{
			name:         "username set - auth used",
			username:     "smtpuser",
			password:     "secret",
			wantAuthUsed: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Always advertise AUTH so smtp.SendMail won't error when auth != nil.
			host, port, getCapture := startFakeSMTP(t, true)
			cfg := email.SMTPConfig{
				Host:     host,
				Port:     port,
				From:     "noreply@example.com",
				Username: tc.username,
				Password: tc.password,
			}
			sender := email.NewSMTPSender(slog.New(slog.DiscardHandler), cfg)

			msg := email.ConfirmationMessage{
				Email:        "user@example.com",
				Repository:   "owner/repo",
				ConfirmToken: "0123456789abcdef",
				ConfirmURL:   "https://example.com/confirm/0123456789abcdef",
			}
			err := sender.SendConfirmation(t.Context(), msg)
			require.NoError(t, err)

			got := getCapture()
			assert.Equal(t, tc.wantAuthUsed, got.authAttempted)
		})
	}
}
