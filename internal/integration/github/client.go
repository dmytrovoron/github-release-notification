package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-github/v84/github"
)

// Client is a wrapper around the google/go-github client.
// The go-github library handles all response codes from GitHub,
// including rate limiting, authentication errors, and other non-2xx responses.
type Client struct {
	gh *github.Client
}

func NewClient(authToken string, timeout time.Duration) *Client {
	authToken = strings.TrimSpace(authToken)
	gh := github.NewClient(&http.Client{Timeout: timeout}).WithAuthToken(authToken)

	return &Client{gh: gh}
}

func (c *Client) WithBaseURL(baseURL string) *Client {
	if baseURL != "" {
		gh, err := c.gh.WithEnterpriseURLs(baseURL, baseURL)
		if err == nil {
			c.gh = gh
		}
	}

	return c
}

func (c *Client) RepositoryExists(ctx context.Context, owner, repo string) (bool, error) {
	_, _, err := c.gh.Repositories.Get(ctx, owner, repo)
	if err == nil {
		return true, nil
	}

	errResp, ok := errors.AsType[*github.ErrorResponse](err)
	if !ok {
		return false, fmt.Errorf("unexpected error type: %w", err)
	}

	return errResp.Response.StatusCode != http.StatusNotFound, nil
}
