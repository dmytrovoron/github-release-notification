//go:build e2e

package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gh "github.com/google/go-github/v84/github"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	subclient "github.com/dmytrovoron/github-release-notification/tests/http/client"
)

func setup(t *testing.T) e2e {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	githubAuthToken := requireGitHubAuthToken(t)

	configureTestcontainersDockerEnv(t)
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx := t.Context()
	repoRoot := findRepoRoot(t)

	nw, err := network.New(ctx, network.WithAttachable())
	require.NoError(t, err, "create docker network")
	t.Cleanup(func() { _ = nw.Remove(ctx) })

	dbC := startPostgresContainer(t, nw)
	smtpC := startSMTPContainer(t, nw)

	dbHost, err := dbC.Host(ctx)
	require.NoError(t, err, "resolve db host")
	dbPort, err := dbC.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err, "resolve db port")

	appC := startAppContainer(t, repoRoot, nw, githubAuthToken)
	attachContainerLogsOnFailure(t, appC, "app")

	appHost, err := appC.Host(ctx)
	require.NoError(t, err, "resolve app host")
	appPort, err := appC.MappedPort(ctx, "8080/tcp")
	require.NoError(t, err, "resolve app port")
	smtpHost, err := smtpC.Host(ctx)
	require.NoError(t, err, "resolve smtp host")
	smtpHTTPPort, err := smtpC.MappedPort(ctx, "8025/tcp")
	require.NoError(t, err, "resolve smtp http port")

	databaseURL := fmt.Sprintf("postgres://app:app@%s/app?sslmode=disable", net.JoinHostPort(dbHost, dbPort.Port()))

	db, err := sql.Open("pgx", databaseURL)
	require.NoError(t, err, "open db")
	t.Cleanup(func() { _ = db.Close() })

	client := newTestAPIClient(t, fmt.Sprintf("http://%s/api", net.JoinHostPort(appHost, appPort.Port())))

	return e2e{
		client:         client,
		db:             db,
		smtpAPIBaseURL: fmt.Sprintf("http://%s", net.JoinHostPort(smtpHost, smtpHTTPPort.Port())),
	}
}

func attachContainerLogsOnFailure(t *testing.T, c testcontainers.Container, containerName string) {
	t.Helper()

	t.Cleanup(func() {
		if !t.Failed() {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		reader, err := c.Logs(ctx)
		if err != nil {
			t.Logf("failed to read %s container logs: %v", containerName, err)

			return
		}
		defer func() { _ = reader.Close() }()

		logBytes, err := io.ReadAll(reader)
		if err != nil {
			t.Logf("failed to consume %s container logs: %v", containerName, err)

			return
		}

		const maxLogBytes = 200_000
		if len(logBytes) > maxLogBytes {
			t.Logf(
				"%s container logs (truncated, showing last %d of %d bytes):\n%s",
				containerName,
				maxLogBytes,
				len(logBytes),
				string(logBytes[len(logBytes)-maxLogBytes:]),
			)

			return
		}

		t.Logf("%s container logs:\n%s", containerName, string(logBytes))
	})
}

func requireGitHubAuthToken(t *testing.T) string {
	t.Helper()

	token := strings.TrimSpace(os.Getenv("GITHUB_AUTH_TOKEN"))
	if token == "" {
		// https://github.com/settings/personal-access-tokens/new
		t.Skip("set GITHUB_AUTH_TOKEN with access to dmytrovoron/github-release-notification-e2e-test to run scanner e2e")
	}

	return token
}

type e2eScanner struct {
	e2e

	gh *gh.Client
}

func setupScanner(t *testing.T) e2eScanner {
	t.Helper()

	t.Setenv("SCANNER_INTERVAL", "2s")

	githubAuthToken := requireGitHubAuthToken(t)
	ghClient := gh.NewClient(&http.Client{Timeout: 20 * time.Second}).WithAuthToken(githubAuthToken)
	e2eScan := e2eScanner{gh: ghClient}
	e2eScan.requireRepositoryAccess(t, scannerE2ERepository)
	e2eScan.e2e = setup(t)

	return e2eScan
}

func configureTestcontainersDockerEnv(t *testing.T) {
	t.Helper()

	dockerHost := os.Getenv("DOCKER_HOST")
	if dockerHost == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			colimaSocket := filepath.Join(homeDir, ".colima", "default", "docker.sock")
			if _, statErr := os.Stat(colimaSocket); statErr == nil {
				dockerHost = "unix://" + colimaSocket
				t.Setenv("DOCKER_HOST", dockerHost)
			}
		}
	}

	// On Colima, Ryuk must mount an in-VM socket path, not the host user path.
	if strings.Contains(dockerHost, "/.colima/") {
		t.Setenv("TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE", "/var/run/docker.sock")
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	require.NoError(t, err, "get working directory")

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
			require.Failf(t, "resolve repo root", "could not find directory containing both go.mod and Dockerfile from %q", wd)
		}
		current = parent
	}
}

func startPostgresContainer(t *testing.T, nw *testcontainers.DockerNetwork) testcontainers.Container {
	t.Helper()

	c, err := testcontainers.Run(
		t.Context(),
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
	require.NoError(t, err, "start db container")

	t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })

	return c
}

func startSMTPContainer(t *testing.T, nw *testcontainers.DockerNetwork) testcontainers.Container {
	t.Helper()

	c, err := testcontainers.Run(
		t.Context(),
		"axllent/mailpit:v1.27",
		network.WithNetwork([]string{"smtp"}, nw),
		testcontainers.WithExposedPorts("1025/tcp", "8025/tcp"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/api/v1/info").WithPort("8025/tcp").WithStartupTimeout(20*time.Second)),
	)
	require.NoError(t, err, "start smtp container")

	t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })

	return c
}

func startAppContainer(
	t *testing.T,
	repoRoot string,
	nw *testcontainers.DockerNetwork,
	githubAuthToken string,
) testcontainers.Container {
	t.Helper()

	appEnv := map[string]string{
		"DATABASE_URL":          "postgres://app:app@db:5432/app?sslmode=disable",
		"MIGRATIONS_PATH":       "file:///app/migrations",
		"DATABASE_PING_TIMEOUT": "10s",
		"GITHUB_API_TIMEOUT":    "5s",
		"GITHUB_AUTH_TOKEN":     githubAuthToken,
		"SMTP_HOST":             "smtp",
		"SMTP_PORT":             "1025",
	}
	if scannerInterval := strings.TrimSpace(os.Getenv("SCANNER_INTERVAL")); scannerInterval != "" {
		appEnv["SCANNER_INTERVAL"] = scannerInterval
	}

	appReq := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			FromDockerfile: testcontainers.FromDockerfile{
				Context:    repoRoot,
				Dockerfile: "Dockerfile",
			},
			Env:          appEnv,
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

	c, err := testcontainers.GenericContainer(t.Context(), appReq)
	require.NoError(t, err, "start app container")

	t.Cleanup(func() { _ = c.Terminate(t.Context()) })

	return c
}

func newTestAPIClient(t *testing.T, baseURL string) *subclient.GitHubReleaseNotificationAPI {
	t.Helper()

	parsedURL, err := url.Parse(baseURL)
	require.NoError(t, err, "parse api base url")
	require.NotEmpty(t, parsedURL.Host, "api base url host must not be empty")

	basePath := parsedURL.Path
	if basePath == "" {
		basePath = "/"
	}

	cfg := subclient.DefaultTransportConfig().
		WithHost(parsedURL.Host).
		WithBasePath(basePath).
		WithSchemes([]string{parsedURL.Scheme})

	return subclient.NewHTTPClientWithConfig(nil, cfg)
}
