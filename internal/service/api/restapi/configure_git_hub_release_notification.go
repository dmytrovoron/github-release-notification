package restapi

import (
	"context"
	"crypto/tls"
	_ "embed"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"

	"github.com/dmytrovoron/github-release-notification/internal/service/api/restapi/operations"
	"github.com/dmytrovoron/github-release-notification/internal/service/api/restapi/operations/subscription"
)

//go:generate go tool -modfile=../../../../tools/go.mod github.com/go-swagger/go-swagger/cmd/swagger generate server --spec ../../../../api/swagger.yaml --target ../ --name GitHubReleaseNotification --exclude-main

//go:embed static/index.html
var indexHTML []byte

//go:embed static/style.css
var styleCSS []byte

//go:embed static/app.js
var appJS []byte

func configureFlags(api *operations.GitHubReleaseNotificationAPI) {
	_ = api
}

func configureAPI(api *operations.GitHubReleaseNotificationAPI) http.Handler {
	return configureAPIWithHealthChecker(api, nil, nil)
}

// NewHandler configures API handlers and returns middleware-wrapped handler with an injected health checker.
func NewHandler(api *operations.GitHubReleaseNotificationAPI, healthChecker func(context.Context) error, logger *slog.Logger) http.Handler {
	return configureAPIWithHealthChecker(api, healthChecker, logger)
}

func configureAPIWithHealthChecker(
	api *operations.GitHubReleaseNotificationAPI,
	healthChecker func(context.Context) error,
	logger *slog.Logger,
) http.Handler {
	api.ServeError = errors.ServeError

	api.Logger = func(msg string, args ...any) {
		logger.Info(fmt.Sprintf(msg, args...))
	}

	api.UseSwaggerUI()
	api.UseRedoc()

	api.JSONConsumer = runtime.JSONConsumer()
	api.UrlformConsumer = runtime.DiscardConsumer

	api.JSONProducer = runtime.JSONProducer()

	if api.SubscriptionConfirmSubscriptionHandler == nil {
		api.SubscriptionConfirmSubscriptionHandler = subscription.ConfirmSubscriptionHandlerFunc(
			func(params subscription.ConfirmSubscriptionParams) middleware.Responder {
				_ = params

				return middleware.NotImplemented("operation subscription.ConfirmSubscription has not yet been implemented")
			},
		)
	}
	if api.SubscriptionGetSubscriptionsHandler == nil {
		api.SubscriptionGetSubscriptionsHandler = subscription.GetSubscriptionsHandlerFunc(
			func(params subscription.GetSubscriptionsParams) middleware.Responder {
				_ = params

				return middleware.NotImplemented("operation subscription.GetSubscriptions has not yet been implemented")
			},
		)
	}
	if api.SubscriptionSubscribeHandler == nil {
		api.SubscriptionSubscribeHandler = subscription.SubscribeHandlerFunc(
			func(params subscription.SubscribeParams) middleware.Responder {
				_ = params

				return middleware.NotImplemented("operation subscription.Subscribe has not yet been implemented")
			},
		)
	}
	if api.SubscriptionUnsubscribeHandler == nil {
		api.SubscriptionUnsubscribeHandler = subscription.UnsubscribeHandlerFunc(
			func(params subscription.UnsubscribeParams) middleware.Responder {
				_ = params

				return middleware.NotImplemented("operation subscription.Unsubscribe has not yet been implemented")
			},
		)
	}

	api.PreServerShutdown = func() {}

	api.ServerShutdown = func() {}

	return setupGlobalMiddleware(api.Serve(setupMiddlewares), healthChecker, logger)
}

// The TLS configuration before HTTPS server starts.
func configureTLS(tlsConfig *tls.Config) {
	_ = tlsConfig
}

// As soon as server is initialized but not run yet, this function will be called.
// If you need to modify a config, store server instance to stop it individually later, this is the place.
// This function can be called multiple times, depending on the number of serving schemes.
// The scheme value will be set accordingly: "http", "https" or "unix".
func configureServer(server *http.Server, scheme, addr string) {
	_ = server
	_ = scheme
	_ = addr
}

// The middleware configuration is for the handler executors. These do not apply to the swagger.json document.
// The middleware executes after routing but before authentication, binding and validation.
func setupMiddlewares(handler http.Handler) http.Handler {
	return handler
}

// The middleware configuration happens before anything, this middleware also applies to serving the swagger.json document.
// So this is a good place to plug in a panic handling middleware, logging and metrics.
func setupGlobalMiddleware(handler http.Handler, healthChecker func(context.Context) error, logger *slog.Logger) http.Handler {
	metrics := metricsMiddleware(newMetricsRegistry())

	return metrics(healthcheckMiddleware(healthChecker, logger)(loggingMiddleware(logger)(staticPagesMiddleware(handler))))
}

// staticPagesMiddleware serves the frontend HTML page for the root path and the
// /confirm/* and /unsubscribe/* paths, and style.css and app.js.
// All other requests are forwarded to the next handler.
func staticPagesMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			path := r.URL.Path
			switch {
			case path == "/style.css":
				w.Header().Set("Content-Type", "text/css; charset=utf-8")
				_, _ = w.Write(styleCSS)

				return
			case path == "/app.js":
				w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
				_, _ = w.Write(appJS)

				return
			case path == "/" || path == "" || strings.HasPrefix(path, "/confirm/") || strings.HasPrefix(path, "/unsubscribe/"):
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write(indexHTML)

				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
