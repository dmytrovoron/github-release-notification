package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"time"

	"github.com/go-openapi/loads"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jessevdk/go-flags"

	"github.com/dmytrovoron/github-release-notification/internal/config"
	"github.com/dmytrovoron/github-release-notification/internal/http/restapi"
	"github.com/dmytrovoron/github-release-notification/internal/http/restapi/operations"
	"github.com/dmytrovoron/github-release-notification/internal/integration/github"
	"github.com/dmytrovoron/github-release-notification/internal/migrations"
	"github.com/dmytrovoron/github-release-notification/internal/notifier"
	"github.com/dmytrovoron/github-release-notification/internal/repository/postgres"
	"github.com/dmytrovoron/github-release-notification/internal/scanner"
	"github.com/dmytrovoron/github-release-notification/internal/service"
)

func main() {
	server()
}

func server() {
	baseLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(baseLogger)

	httpLogger := baseLogger.With("appType", "httpServer")
	scannerLogger := baseLogger.With("appType", "scanner")
	notifierLogger := baseLogger.With("appType", "notifier")

	cfg, err := config.Load()
	if err != nil {
		exitWithError(baseLogger, "load config", err)
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		exitWithError(baseLogger, "open database connection", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			baseLogger.Error("close db connection", "error", closeErr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DatabasePingTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		exitWithError(baseLogger, "ping database", err)
	}

	if err := migrations.Run(cfg.DatabaseURL, cfg.MigrationsPath); err != nil {
		exitWithError(baseLogger, "run migrations", err)
	}

	swaggerSpec, err := loads.Embedded(restapi.SwaggerJSON, restapi.FlatSwaggerJSON)
	if err != nil {
		exitWithError(baseLogger, "load swagger spec", err)
	}

	api := operations.NewGitHubReleaseNotificationAPI(swaggerSpec)
	githubClient := github.NewClient(cfg.GitHubAuthToken, cfg.GitHubAPITimeout).WithBaseURL(cfg.GitHubAPIBaseURL)

	subscriptionRepo := postgres.NewSubscriptionRepository(db)
	notif := notifier.NewNotifier(
		notifierLogger,
		notifier.NotifierConfig{
			SMTPHost:     cfg.SMTPHost,
			SMTPPort:     cfg.SMTPPort,
			SMTPFrom:     cfg.SMTPFrom,
			SMTPUsername: cfg.SMTPUsername,
			SMTPPassword: cfg.SMTPPassword,
		},
	)

	// TODO: this is a dirty workaround to build the confirm URL base.
	// Ideally, the confirm path ("/confirm") should be derived from the generated
	// ConfirmSubscriptionURL, but it requires a non-empty token to produce a valid URL.
	// Instead, we manually replicate the path segment and join it with the swagger base path.
	confirmURLBase, err := url.JoinPath(cfg.AppBaseURL, swaggerSpec.BasePath(), "confirm")
	if err != nil {
		exitWithError(baseLogger, "build confirm url base", err)
	}
	unsubscribeURLBase, err := url.JoinPath(cfg.AppBaseURL, swaggerSpec.BasePath(), "unsubscribe")
	if err != nil {
		exitWithError(baseLogger, "build unsubscribe url base", err)
	}
	subscriptionService := service.NewSubscriptionService(subscriptionRepo, githubClient, notif, httpLogger, confirmURLBase)
	restapi.NewSubscriptionHandler(subscriptionService, httpLogger).Register(api)

	scannerRepo := postgres.NewScannerRepository(db)
	scannerRunner := scanner.NewRunner(scannerLogger, scannerRepo, githubClient, cfg.ScannerInterval)
	scannerCtx, scannerCancel := context.WithCancel(context.Background())
	defer scannerCancel()
	go scannerRunner.Start(scannerCtx)

	notifierRepo := postgres.NewNotifierRepository(db)
	notifierRunner := notifier.NewRunner(notifierLogger, notifierRepo, notif, cfg.NotifierInterval, unsubscribeURLBase)
	notifierCtx, notifierCancel := context.WithCancel(context.Background())
	defer notifierCancel()
	go notifierRunner.Start(notifierCtx)

	server := restapi.NewServer(api)
	server.EnabledListeners = []string{cfg.Scheme}
	server.SetHandler(restapi.NewHandler(api, func(checkCtx context.Context) error {
		pingCtx, pingCancel := context.WithTimeout(checkCtx, 2*time.Second)
		defer pingCancel()

		return db.PingContext(pingCtx)
	}, httpLogger))
	server.ConfigureFlags() // inject API-specific custom flags. Must be called before args parsing

	parser := flags.NewParser(server, flags.Default)
	parser.ShortDescription = "GitHub Release Notification API"
	parser.LongDescription = "GitHub Release Notification API that allows users to subscribe to email notifications " +
		"about new releases of a chosen GitHub repository."

	for _, optsGroup := range api.CommandLineOptionsGroups {
		_, err := parser.AddGroup(optsGroup.ShortDescription, optsGroup.LongDescription, optsGroup.Options)
		if err != nil {
			exitWithError(baseLogger, "register command line options", err)
		}
	}

	if _, err := parser.Parse(); err != nil {
		code := 1
		fe := new(flags.Error)
		if errors.As(err, &fe) {
			if fe.Type == flags.ErrHelp {
				code = 0
			}
		}
		os.Exit(code)
	}

	if err := server.Serve(); err != nil {
		scannerCancel()
		_ = server.Shutdown()

		exitWithError(httpLogger, "serve http server", err)
	}
}

func exitWithError(logger *slog.Logger, message string, err error) {
	logger.Error(message, "error", err)
	os.Exit(1)
}
