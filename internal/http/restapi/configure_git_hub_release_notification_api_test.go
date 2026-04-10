package restapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-openapi/loads"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dmytrovoron/github-release-notification/internal/http/restapi/operations"
)

func TestDocsEndpointServesSwaggerUI(t *testing.T) {
	spec, err := loads.Embedded(SwaggerJSON, FlatSwaggerJSON)
	require.NoError(t, err, "load embedded spec")

	api := operations.NewGitHubReleaseNotificationAPI(spec)
	handler := configureAPI(api)

	req := httptest.NewRequest(http.MethodGet, "/api/docs", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "unexpected status code")

	contentType := rec.Header().Get("Content-Type")
	assert.Contains(t, contentType, "text/html", "expected HTML content type")

	body := strings.ToLower(rec.Body.String())
	assert.Contains(t, body, "<html", "expected HTML response body")
	assert.Contains(t, body, "swagger", "expected swagger docs page content")
}
