package restapi_test

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/dmytrovoron/github-release-notification/internal"
	serviceapi "github.com/dmytrovoron/github-release-notification/internal/service/api"
	"github.com/dmytrovoron/github-release-notification/internal/service/api/models"
	"github.com/dmytrovoron/github-release-notification/internal/service/api/restapi"
	"github.com/dmytrovoron/github-release-notification/internal/service/api/restapi/operations"
	"github.com/dmytrovoron/github-release-notification/internal/service/api/restapi/operations/subscription"
)

func newTestAPI(t *testing.T, svc *fakeSubscriptionService) *operations.GitHubReleaseNotificationAPI {
	t.Helper()

	notifAPI := operations.NewGitHubReleaseNotificationAPI(nil)
	handler := restapi.NewSubscriptionHandler(svc, slog.New(slog.DiscardHandler))
	handler.Register(notifAPI)

	return notifAPI
}

func responderStatus(t *testing.T, resp middleware.Responder) int {
	t.Helper()

	rec := httptest.NewRecorder()
	resp.WriteResponse(rec, runtime.JSONProducer())

	return rec.Code
}

func TestSubscriptionHandler_Subscribe_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "ok", err: nil, wantStatus: http.StatusOK},
		{name: "invalid email", err: serviceapi.ErrInvalidEmail, wantStatus: http.StatusBadRequest},
		{name: "invalid repository", err: serviceapi.ErrInvalidRepository, wantStatus: http.StatusBadRequest},
		{name: "repository not found", err: serviceapi.ErrRepositoryNotFound, wantStatus: http.StatusNotFound},
		{name: "subscription conflict", err: serviceapi.ErrSubscriptionConflict, wantStatus: http.StatusConflict},
		{name: "internal error", err: errors.New("boom"), wantStatus: http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := &fakeSubscriptionService{subscribeErr: tc.err}
			apiObj := newTestAPI(t, svc)

			params := subscription.SubscribeParams{
				HTTPRequest: httptest.NewRequest(http.MethodPost, "/subscribe", http.NoBody),
				Email:       "user@example.com",
				Repo:        "owner/repo",
			}
			resp := apiObj.SubscriptionSubscribeHandler.Handle(params)

			assert.Equal(t, tc.wantStatus, responderStatus(t, resp))
			assert.Equal(t, "user@example.com", svc.subscribeEmail)
			assert.Equal(t, "owner/repo", svc.subscribeRepo)
		})
	}
}

func TestSubscriptionHandler_Confirm_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "ok", err: nil, wantStatus: http.StatusOK},
		{name: "invalid token", err: serviceapi.ErrInvalidToken, wantStatus: http.StatusBadRequest},
		{name: "token not found", err: serviceapi.ErrTokenNotFound, wantStatus: http.StatusNotFound},
		{name: "internal error", err: errors.New("boom"), wantStatus: http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := &fakeSubscriptionService{confirmErr: tc.err}
			apiObj := newTestAPI(t, svc)

			params := subscription.ConfirmSubscriptionParams{
				HTTPRequest: httptest.NewRequest(http.MethodPost, "/confirm/abc", http.NoBody),
				Token:       "0123456789abcdef",
			}
			resp := apiObj.SubscriptionConfirmSubscriptionHandler.Handle(params)

			assert.Equal(t, tc.wantStatus, responderStatus(t, resp))
			assert.Equal(t, "0123456789abcdef", svc.confirmToken)
		})
	}
}

func TestSubscriptionHandler_Unsubscribe_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "ok", err: nil, wantStatus: http.StatusOK},
		{name: "invalid token", err: serviceapi.ErrInvalidToken, wantStatus: http.StatusBadRequest},
		{name: "token not found", err: serviceapi.ErrTokenNotFound, wantStatus: http.StatusNotFound},
		{name: "internal error", err: errors.New("boom"), wantStatus: http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := &fakeSubscriptionService{unsubscribeErr: tc.err}
			apiObj := newTestAPI(t, svc)

			params := subscription.UnsubscribeParams{
				HTTPRequest: httptest.NewRequest(http.MethodDelete, "/unsubscribe/abc", http.NoBody),
				Token:       "0123456789abcdef",
			}
			resp := apiObj.SubscriptionUnsubscribeHandler.Handle(params)

			assert.Equal(t, tc.wantStatus, responderStatus(t, resp))
			assert.Equal(t, "0123456789abcdef", svc.unsubToken)
		})
	}
}

func TestSubscriptionHandler_GetSubscriptions_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		listErr    error
		listItems  []app.Subscription
		wantStatus int
		assertOK   func(t *testing.T, resp middleware.Responder)
	}{
		{
			name:       "invalid email",
			listErr:    serviceapi.ErrInvalidEmail,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "internal error",
			listErr:    errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "ok with mapped payload",
			listItems: []app.Subscription{
				{Email: "a@example.com", Repository: "owner/a", Confirmed: true, LastSeenTag: "v1.0.0"},
				{Email: "b@example.com", Repository: "owner/b", Confirmed: false, LastSeenTag: ""},
			},
			wantStatus: http.StatusOK,
			assertOK: func(t *testing.T, resp middleware.Responder) {
				t.Helper()

				okResp, ok := resp.(*subscription.GetSubscriptionsOK)
				require.True(t, ok)

				expected := []*models.Subscription{
					{Email: new("a@example.com"), Repo: new("owner/a"), Confirmed: true, LastSeenTag: "v1.0.0"},
					{Email: new("b@example.com"), Repo: new("owner/b"), Confirmed: false, LastSeenTag: ""},
				}
				assert.Equal(t, expected, okResp.Payload)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := &fakeSubscriptionService{listErr: tc.listErr, listItems: tc.listItems}
			apiObj := newTestAPI(t, svc)

			params := subscription.GetSubscriptionsParams{
				HTTPRequest: httptest.NewRequest(http.MethodGet, "/subscriptions?email=user@example.com", http.NoBody),
				Email:       "user@example.com",
			}
			resp := apiObj.SubscriptionGetSubscriptionsHandler.Handle(params)

			assert.Equal(t, tc.wantStatus, responderStatus(t, resp))
			assert.Equal(t, "user@example.com", svc.listEmail)
			if tc.assertOK != nil {
				tc.assertOK(t, resp)
			}
		})
	}
}
