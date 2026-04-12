//go:build e2e

package e2e

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v84/github"
	"github.com/subosito/gotenv"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	apiclient "github.com/dmytrovoron/github-release-notification/tests/http/client"
)

func TestMain(m *testing.M) {
	run(m)
}

func run(m *testing.M) int {
	repoRoot := mustFindRepoRoot()
	envPath := filepath.Join(repoRoot, ".env")
	env, err := gotenv.Read(envPath)
	if err != nil {
		log.Fatalf("Read %s: %v", envPath, err)
	}

	setupDockerEnv()

	if err := checkDockerProvider(); err != nil {
		log.Fatalf("Docker provider not healthy: %v", err)
	}

	ctx := context.Background()

	nw, err := network.New(ctx, network.WithAttachable())
	if err != nil {
		log.Fatalf("Create docker network: %v", err)
	}

	defer func() { _ = nw.Remove(ctx) }()

	dbC := mustStartPostgresContainer(ctx, nw)
	defer func() { _ = testcontainers.TerminateContainer(dbC) }()

	smtpC := mustStartSMTPContainer(ctx, nw)
	defer func() { _ = testcontainers.TerminateContainer(smtpC) }()

	appC := mustStartAppContainer(ctx, repoRoot, nw, env)
	defer func() { _ = appC.Terminate(ctx) }()

	dbHost, err := dbC.Host(ctx)
	if err != nil {
		log.Fatalf("Resolve db host: %v", err)
	}

	dbPort, err := dbC.MappedPort(ctx, "5432/tcp")
	if err != nil {
		log.Fatalf("Resolve db port: %v", err)
	}

	appHost, err := appC.Host(ctx)
	if err != nil {
		log.Fatalf("Resolve app host: %v", err)
	}

	appPort, err := appC.MappedPort(ctx, "8080/tcp")
	if err != nil {
		log.Fatalf("Resolve app port: %v", err)
	}

	smtpHost, err := smtpC.Host(ctx)
	if err != nil {
		log.Fatalf("Resolve smtp host: %v", err)
	}

	smtpHTTPPort, err := smtpC.MappedPort(ctx, "8025/tcp")
	if err != nil {
		log.Fatalf("Resolve smtp http port: %v", err)
	}

	databaseURL := fmt.Sprintf("postgres://app:app@%s/app?sslmode=disable", net.JoinHostPort(dbHost, dbPort.Port()))

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatalf("Open db: %v", err)
	}

	defer func() { _ = db.Close() }()

	ghClient := github.NewClient(&http.Client{Timeout: 20 * time.Second}).WithAuthToken(env["GITHUB_AUTH_TOKEN"])

	apiClient := mustNewAPIClient(fmt.Sprintf("http://%s/api", net.JoinHostPort(appHost, appPort.Port())))
	smtpAPIBaseURL := "http://" + net.JoinHostPort(smtpHost, smtpHTTPPort.Port())

	eAPI = e2eAPI{
		client: apiClient,
		db:     db,
	}

	eScanner = e2eScanner{
		client: apiClient,
		db:     db,
		gh:     ghClient,
	}

	eNotifier = e2eNotifier{
		smtpAPIBaseURL: smtpAPIBaseURL,
		db:             db,
	}

	if err := requireRepositoryAccess(ctx, ghClient, scannerE2ERepository); err != nil {
		log.Fatalf("repository access check failed: %v", err)
	}

	code := m.Run()

	if code != 0 {
		dumpContainerLogs(ctx, appC, "app")
	}

	return code
}

func setupDockerEnv() {
	dockerHost := os.Getenv("DOCKER_HOST")
	if dockerHost == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			colimaSocket := filepath.Join(homeDir, ".colima", "default", "docker.sock")
			if _, statErr := os.Stat(colimaSocket); statErr == nil {
				dockerHost = "unix://" + colimaSocket
				_ = os.Setenv("DOCKER_HOST", dockerHost)
			}
		}
	}

	// On Colima, Ryuk must mount an in-VM socket path, not the host user path.
	if strings.Contains(dockerHost, "/.colima/") {
		_ = os.Setenv("TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE", "/var/run/docker.sock")
	}
}

func checkDockerProvider() error {
	p, err := testcontainers.NewDockerProvider()
	if err != nil {
		return err
	}

	defer func() { _ = p.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return p.Health(ctx)
}

func mustFindRepoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("get working directory: %v", err)
	}

	current := wd
	for {
		goMod := filepath.Join(current, "go.mod")
		dockerfile := filepath.Join(current, "Dockerfile")

		if _, err := os.Stat(goMod); err == nil {
			if _, err := os.Stat(dockerfile); err == nil {
				return current
			}
		}

		parent := filepath.Dir(current)
		if parent == current {
			log.Fatalf("could not find directory containing both go.mod and Dockerfile from %q", wd)
		}

		current = parent
	}
}

func mustStartPostgresContainer(ctx context.Context, nw *testcontainers.DockerNetwork) testcontainers.Container {
	c, err := testcontainers.Run(
		ctx,
		"postgres:18-alpine",
		network.WithNetwork([]string{"db"}, nw),
		testcontainers.WithEnv(map[string]string{
			"POSTGRES_DB":       "app",
			"POSTGRES_USER":     "app",
			"POSTGRES_PASSWORD": "app",
		}),
		testcontainers.WithExposedPorts("5432/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(1).WithStartupTimeout(10*time.Second),
		),
	)
	if err != nil {
		log.Fatalf("start db container: %v", err)
	}

	return c
}

func mustStartSMTPContainer(ctx context.Context, nw *testcontainers.DockerNetwork) testcontainers.Container {
	c, err := testcontainers.Run(
		ctx,
		"axllent/mailpit:v1.27",
		network.WithNetwork([]string{"smtp"}, nw),
		testcontainers.WithExposedPorts("1025/tcp", "8025/tcp"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/api/v1/info").WithPort("8025/tcp").WithStartupTimeout(20*time.Second)),
	)
	if err != nil {
		log.Fatalf("start smtp container: %v", err)
	}

	return c
}

func mustStartAppContainer(
	ctx context.Context,
	repoRoot string,
	nw *testcontainers.DockerNetwork,
	env map[string]string,
) testcontainers.Container {
	env["MIGRATIONS_PATH"] = "file:///app/migrations"
	env["DATABASE_URL"] = "postgres://app:app@db:5432/app?sslmode=disable"
	env["SCANNER_INTERVAL"] = "2s"
	env["NOTIFIER_INTERVAL"] = "2s"
	env["SMTP_HOST"] = "smtp"

	appReq := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			FromDockerfile: testcontainers.FromDockerfile{
				Context:    repoRoot,
				Dockerfile: "Dockerfile",
			},
			Env:          env,
			ExposedPorts: []string{"8080/tcp"},
			WaitingFor: wait.ForHTTP("/healthz").
				WithPort("8080/tcp").
				WithStatusCodeMatcher(func(code int) bool { return code == http.StatusOK }).
				WithStartupTimeout(120 * time.Second),
			Networks: []string{nw.Name},
			NetworkAliases: map[string][]string{
				nw.Name: {"app"},
			},
		},
		Started: true,
	}

	c, err := testcontainers.GenericContainer(ctx, appReq)
	if err != nil {
		log.Fatalf("start app container: %v", err)
	}

	return c
}

func mustNewAPIClient(baseURL string) *apiclient.GitHubReleaseNotificationAPI {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		log.Fatalf("parse api base url: %v", err)
	}

	if parsedURL.Host == "" {
		log.Fatalf("api base url host must not be empty")
	}

	basePath := parsedURL.Path
	if basePath == "" {
		basePath = "/"
	}

	cfg := apiclient.DefaultTransportConfig().
		WithHost(parsedURL.Host).
		WithBasePath(basePath).
		WithSchemes([]string{parsedURL.Scheme})

	return apiclient.NewHTTPClientWithConfig(nil, cfg)
}

func dumpContainerLogs(ctx context.Context, c testcontainers.Container, containerName string) {
	logCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	reader, err := c.Logs(logCtx)
	if err != nil {
		log.Printf("failed to read %s container logs: %v", containerName, err)

		return
	}

	defer func() { _ = reader.Close() }()

	logBytes, err := io.ReadAll(reader)
	if err != nil {
		log.Printf("failed to consume %s container logs: %v", containerName, err)

		return
	}

	const maxLogBytes = 200_000
	if len(logBytes) > maxLogBytes {
		log.Printf(
			"%s container logs (truncated, showing last %d of %d bytes):\n%s",
			containerName,
			maxLogBytes,
			len(logBytes),
			string(logBytes[len(logBytes)-maxLogBytes:]),
		)

		return
	}

	log.Printf("%s container logs:\n%s", containerName, string(logBytes))
}

func requireRepositoryAccess(ctx context.Context, ghClient *github.Client, repositoryName string) error {
	owner, repo, ok := strings.Cut(repositoryName, "/")
	if !ok {
		return fmt.Errorf("repository name must be owner/repo, got %q", repositoryName)
	}

	_, resp, err := ghClient.Repositories.Get(ctx, owner, repo)
	if err == nil {
		return nil
	}

	if ghErr, ok := errors.AsType[*github.ErrorResponse](err); ok {
		switch ghErr.Response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
			return fmt.Errorf(
				"GITHUB_AUTH_TOKEN must have read and write access to %s (github status %d)",
				repositoryName,
				ghErr.Response.StatusCode,
			)
		default:
		}
	}

	if resp != nil {
		return fmt.Errorf("verify repository access for %s: %w (status %d)", repositoryName, err, resp.StatusCode)
	}

	return fmt.Errorf("verify repository access for %s: %w", repositoryName, err)
}
