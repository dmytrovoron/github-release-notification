package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-openapi/loads"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jessevdk/go-flags"

	"github.com/dmytrovoron/github-release-notification/internal/config"
	"github.com/dmytrovoron/github-release-notification/internal/integration/github"
	"github.com/dmytrovoron/github-release-notification/internal/repository/postgres"
	"github.com/dmytrovoron/github-release-notification/internal/service/api"
	"github.com/dmytrovoron/github-release-notification/internal/service/api/restapi"
	"github.com/dmytrovoron/github-release-notification/internal/service/api/restapi/operations"
	"github.com/dmytrovoron/github-release-notification/internal/service/notifier"
	"github.com/dmytrovoron/github-release-notification/internal/service/scanner"
)

func main() {
	if err := server(); err != nil {
		if fe, ok := errors.AsType[*flags.Error](err); ok && fe.Type == flags.ErrHelp {
			os.Exit(0)
		}
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func server() error {
	baseLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(baseLogger)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database connection: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			baseLogger.Error("close db connection", "error", closeErr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DatabasePingTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	if err := migrationsRun(cfg.DatabaseURL, cfg.MigrationsPath); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	swaggerSpec, err := loads.Embedded(restapi.SwaggerJSON, restapi.FlatSwaggerJSON)
	if err != nil {
		return fmt.Errorf("load swagger spec: %w", err)
	}

	releaseNotificationAPI := operations.NewGitHubReleaseNotificationAPI(swaggerSpec)
	githubClient := github.NewClient(cfg.GitHubAuthToken, cfg.GitHubAPITimeout).WithBaseURL(cfg.GitHubAPIBaseURL)

	subscriptionRepo := postgres.NewSubscriptionRepository(db)

	notif := notifier.NewNotifier(
		baseLogger.With("appType", "notifier"),
		notifier.NotifierConfig{
			SMTPHost:     cfg.SMTPHost,
			SMTPPort:     cfg.SMTPPort,
			SMTPFrom:     cfg.SMTPFrom,
			SMTPUsername: cfg.SMTPUsername,
			SMTPPassword: cfg.SMTPPassword,
		},
	)

	// confirmURLBase points to the frontend confirm page (/confirm/{token}) so that
	// clicking the link in the confirmation email renders the HTML UI instead of raw JSON.
	confirmURLBase, err := url.JoinPath(cfg.AppBaseURL, "confirm")
	if err != nil {
		return fmt.Errorf("build confirm url base: %w", err)
	}
	// unsubscribeURLBase points to the frontend unsubscribe page (/unsubscribe/{token})
	// so unsubscribe links render HTML instead of returning an empty API response.
	unsubscribeURLBase, err := url.JoinPath(cfg.AppBaseURL, "unsubscribe")
	if err != nil {
		return fmt.Errorf("build unsubscribe url base: %w", err)
	}

	apiLogger := baseLogger.With("appType", "api")
	subscriptionService := api.NewSubscriptionService(subscriptionRepo, githubClient, notif, apiLogger, confirmURLBase)
	restapi.NewSubscriptionHandler(subscriptionService, apiLogger).Register(releaseNotificationAPI)

	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	var wg sync.WaitGroup

	scannerRepo := postgres.NewScannerRepository(db)
	scannerLogger := baseLogger.With("appType", "scanner")
	scannerRunner := scanner.NewRunner(scannerLogger, scannerRepo, githubClient, cfg.ScannerInterval)
	wg.Go(func() {
		scannerRunner.Start(appCtx)
	})

	notifierRepo := postgres.NewNotifierRepository(db)
	notifierLogger := baseLogger.With("appType", "notifier")
	notifierRunner := notifier.NewRunner(notifierLogger, notifierRepo, notif, cfg.NotifierInterval, unsubscribeURLBase)
	wg.Go(func() {
		notifierRunner.Start(appCtx)
	})

	server := restapi.NewServer(releaseNotificationAPI)
	server.EnabledListeners = []string{cfg.Scheme}
	server.SetHandler(restapi.NewHandler(releaseNotificationAPI, func(checkCtx context.Context) error {
		pingCtx, pingCancel := context.WithTimeout(checkCtx, 2*time.Second)
		defer pingCancel()

		return db.PingContext(pingCtx)
	}, apiLogger))
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	go func() {
		select {
		case sig := <-quit:
			baseLogger.Info("received signal, shutting down", "signal", sig)
			appCancel()
			if shutdownErr := server.Shutdown(); shutdownErr != nil {
				baseLogger.Error("http server shutdown", "error", shutdownErr)
			}
		case <-appCtx.Done():
		}
	}()

	server.ConfigureFlags() // inject API-specific custom flags. Must be called before args parsing

	parser := flags.NewParser(server, flags.Default)
	parser.ShortDescription = "GitHub Release Notification API"
	parser.LongDescription = "GitHub Release Notification API that allows users to subscribe to email notifications " +
		"about new releases of a chosen GitHub repository."

	for _, optsGroup := range releaseNotificationAPI.CommandLineOptionsGroups {
		_, err := parser.AddGroup(optsGroup.ShortDescription, optsGroup.LongDescription, optsGroup.Options)
		if err != nil {
			return fmt.Errorf("register command line options: %w", err)
		}
	}

	if _, err := parser.Parse(); err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	if err := server.Serve(); err != nil {
		appCancel()
		_ = server.Shutdown()
		wg.Wait()

		return fmt.Errorf("serve http server: %w", err)
	}

	wg.Wait()

	return nil
}

// migrationsRun applies all pending database migrations.
func migrationsRun(databaseURL, migrationsPath string) error {
	m, err := migrate.New(migrationsPath, databaseURL)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer func() {
		_, _ = m.Close()
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}
