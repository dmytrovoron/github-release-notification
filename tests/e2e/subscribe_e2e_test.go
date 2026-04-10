//go:build e2e

package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

type e2eEnv struct {
	client             *http.Client
	baseURL            string
	databaseURLForTest string
}

func TestSubscribeEndpointE2E(t *testing.T) {
	env := setupE2EEnv(t)
	email := "alice@example.com"
	repo := "owner/repo"

	postSubscribe(t, env.client, env.baseURL, email, repo, http.StatusOK)
	postSubscribe(t, env.client, env.baseURL, email, repo, http.StatusConflict)

	activateSubscriptionByEmail(t, env.databaseURLForTest, email)

	assertSingleConfirmedSubscription(t, getSubscriptions(t, env.client, env.baseURL, email), email, repo)
}

func setupE2EEnv(t *testing.T) e2eEnv {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	configureTestcontainersDockerEnv(t)
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx := t.Context()
	repoRoot := findRepoRoot(t)
	githubBaseURL := startGitHubStub(t)

	nw, err := network.New(ctx, network.WithAttachable())
	if err != nil {
		t.Fatalf("create docker network: %v", err)
	}
	t.Cleanup(func() {
		_ = nw.Remove(ctx)
	})

	dbC, err := startPostgresContainer(ctx, nw)
	if err != nil {
		t.Fatalf("start db container: %v", err)
	}
	t.Cleanup(func() {
		_ = testcontainers.TerminateContainer(dbC)
	})

	dbHost, err := dbC.Host(ctx)
	if err != nil {
		t.Fatalf("resolve db host: %v", err)
	}
	dbPort, err := dbC.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatalf("resolve db port: %v", err)
	}

	appC, err := startAppContainer(ctx, repoRoot, nw, githubBaseURL)
	if err != nil {
		t.Fatalf("start app container: %v", err)
	}
	t.Cleanup(func() {
		_ = appC.Terminate(ctx)
	})

	appHost, err := appC.Host(ctx)
	if err != nil {
		t.Fatalf("resolve app host: %v", err)
	}
	appPort, err := appC.MappedPort(ctx, "8080/tcp")
	if err != nil {
		t.Fatalf("resolve app port: %v", err)
	}

	return e2eEnv{
		client:             &http.Client{Timeout: 10 * time.Second},
		baseURL:            fmt.Sprintf("http://%s/api", net.JoinHostPort(appHost, appPort.Port())),
		databaseURLForTest: fmt.Sprintf("postgres://app:app@%s/app?sslmode=disable", net.JoinHostPort(dbHost, dbPort.Port())),
	}
}

func startPostgresContainer(ctx context.Context, nw *testcontainers.DockerNetwork) (testcontainers.Container, error) {
	return testcontainers.Run(
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
}

func startAppContainer(
	ctx context.Context,
	repoRoot string,
	nw *testcontainers.DockerNetwork,
	githubBaseURL string,
) (testcontainers.Container, error) {
	appReq := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			FromDockerfile: testcontainers.FromDockerfile{
				Context:    repoRoot,
				Dockerfile: "Dockerfile",
			},
			Env: map[string]string{
				"DATABASE_URL":          "postgres://app:app@db:5432/app?sslmode=disable",
				"MIGRATIONS_PATH":       "file:///app/migrations",
				"DATABASE_PING_TIMEOUT": "10s",
				"GITHUB_API_BASE_URL":   githubBaseURL,
				"GITHUB_API_TIMEOUT":    "5s",
			},
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

	return testcontainers.GenericContainer(ctx, appReq)
}

func assertSingleConfirmedSubscription(t *testing.T, items []subscriptionDTO, expectedEmail, expectedRepo string) {
	t.Helper()

	if len(items) != 1 {
		t.Fatalf("expected 1 active subscription, got %d", len(items))
	}
	if items[0].Email != expectedEmail {
		t.Fatalf("expected email %q, got %q", expectedEmail, items[0].Email)
	}
	if items[0].Repo != expectedRepo {
		t.Fatalf("expected repo %q, got %q", expectedRepo, items[0].Repo)
	}
	if !items[0].Confirmed {
		t.Fatalf("expected confirmed=true")
	}
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
	if err != nil {
		t.Fatalf("get working directory: %v", err)
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
			t.Fatalf("resolve repo root: could not find directory containing both go.mod and Dockerfile from %q", wd)
		}
		current = parent
	}
}

type subscriptionDTO struct {
	Email     string `json:"email"`
	Repo      string `json:"repo"`
	Confirmed bool   `json:"confirmed"`
}

func postSubscribe(t *testing.T, client *http.Client, baseURL, email, repo string, expectedCode int) {
	t.Helper()

	form := url.Values{}
	form.Set("email", email)
	form.Set("repo", repo)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/subscribe", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("build subscribe request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do subscribe request: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != expectedCode {
		t.Fatalf("expected status %d, got %d", expectedCode, resp.StatusCode)
	}
}

func getSubscriptions(t *testing.T, client *http.Client, baseURL, email string) []subscriptionDTO {
	t.Helper()

	query := url.Values{}
	query.Set("email", email)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/subscriptions?"+query.Encode(), http.NoBody)
	if err != nil {
		t.Fatalf("build subscriptions request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do subscriptions request: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var payload []subscriptionDTO
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode subscriptions response: %v", err)
	}

	return payload
}

func activateSubscriptionByEmail(t *testing.T, databaseURL, email string) {
	t.Helper()

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	ctx := t.Context()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping db: %v", err)
	}

	if _, err := db.ExecContext(ctx, "UPDATE subscriptions SET status='active' WHERE email=$1", email); err != nil {
		t.Fatalf("activate subscription: %v", err)
	}
}

func startGitHubStub(t *testing.T) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := &http.Server{Handler: mux}
	var lcfg net.ListenConfig
	ln, err := lcfg.Listen(t.Context(), "tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen github stub: %v", err)
	}

	go func() {
		_ = srv.Serve(ln)
	}()

	t.Cleanup(func() {
		_ = srv.Shutdown(t.Context())
	})

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("resolve github stub port")
	}
	host := os.Getenv("E2E_GITHUB_STUB_HOST")
	if host == "" {
		host = "host.docker.internal"
	}

	return "http://" + net.JoinHostPort(host, strconv.Itoa(addr.Port))
}
