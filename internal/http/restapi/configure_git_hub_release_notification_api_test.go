package restapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-openapi/loads"

	"github.com/dmytrovoron/github-release-notification/internal/http/restapi/operations"
)

func TestDocsEndpointServesSwaggerUI(t *testing.T) {
	spec, err := loads.Embedded(SwaggerJSON, FlatSwaggerJSON)
	if err != nil {
		t.Fatalf("load embedded spec: %v", err)
	}

	api := operations.NewGitHubReleaseNotificationAPI(spec)
	handler := configureAPI(api)

	req := httptest.NewRequest(http.MethodGet, "/api/docs", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Fatalf("expected HTML content type, got %q", contentType)
	}

	body := strings.ToLower(rec.Body.String())
	if !strings.Contains(body, "<html") {
		t.Fatal("expected HTML response body")
	}
	if !strings.Contains(body, "swagger") {
		t.Fatal("expected swagger docs page content")
	}
}
