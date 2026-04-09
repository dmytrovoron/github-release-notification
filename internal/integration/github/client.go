package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

func NewClient(baseURL string, timeout time.Duration) (*Client, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse github api base url: %w", err)
	}

	return &Client{
		baseURL: parsedURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (c *Client) RepositoryExists(ctx context.Context, repository string) (bool, error) {
	relativePath := "/repos/" + strings.TrimSpace(repository)
	endpoint := c.baseURL.ResolveReference(&url.URL{Path: relativePath})

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), http.NoBody)
	if err != nil {
		return false, fmt.Errorf("build github request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "github-release-notification")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("execute github request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("github api returned status %d", resp.StatusCode)
	}
}
