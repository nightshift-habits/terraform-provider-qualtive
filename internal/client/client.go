package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const APIVersion = "2026-10-18"

var (
	ErrNotFound          = errors.New("not found")
	ErrSlugAlreadyExists = errors.New("slug already exists")
)

type Client struct {
	baseURL    string
	apiKey     string
	userAgent  string
	httpClient *http.Client
}

func New(baseURL, apiKey, userAgent string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		userAgent:  userAgent,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

type APIError struct {
	StatusCode int
	Reason     string
}

func (e *APIError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("qualtive admin API returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("qualtive admin API returned HTTP %d: %s", e.StatusCode, e.Reason)
}

func (e *APIError) Unwrap() error {
	switch e.StatusCode {
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusConflict:
		return ErrSlugAlreadyExists
	default:
		return nil
	}
}

type requestOptions struct {
	container   string
	workspaceID *int64
	body        any
	out         any
}

func (c *Client) do(ctx context.Context, method, path string, opts requestOptions) error {
	var body io.Reader
	if opts.body != nil {
		data, err := json.Marshal(opts.body)
		if err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("API-Version", APIVersion)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("X-Container", opts.container)
	if opts.workspaceID != nil {
		req.Header.Set("X-Workspace", strconv.FormatInt(*opts.workspaceID, 10))
	}
	if opts.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &APIError{StatusCode: resp.StatusCode}
		var errBody struct {
			Reason string `json:"reason"`
		}
		if json.Unmarshal(data, &errBody) == nil {
			apiErr.Reason = errBody.Reason
		}
		return apiErr
	}

	if opts.out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, opts.out); err != nil {
		return fmt.Errorf("decoding response body: %w", err)
	}
	return nil
}
