package esphome

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client calls the ESPHome sidecar HTTP service.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a sidecar client targeting baseURL (e.g. "http://esphome-svc:6052").
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{}, // no global timeout; callers use context
	}
}

// Compile calls POST /compile/{device} on the sidecar, streaming log lines to logWriter.
func (c *Client) Compile(ctx context.Context, device string, logWriter io.Writer) error {
	url := fmt.Sprintf("%s/compile/%s", c.baseURL, device)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sidecar unavailable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("sidecar busy: another compile is running")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sidecar HTTP %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		var msg struct {
			Log    string `json:"log"`
			Result string `json:"result"`
			Code   int    `json:"code"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.Log != "" {
			fmt.Fprintln(logWriter, msg.Log)
		}
		if msg.Result == "ok" {
			return nil
		}
		if msg.Result == "error" {
			return fmt.Errorf("esphome compile failed (exit code %d)", msg.Code)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read sidecar response: %w", err)
	}
	return fmt.Errorf("sidecar stream ended without result")
}

// Cancel calls DELETE /compile on the sidecar to SIGTERM the running compile.
func (c *Client) Cancel() error {
	url := fmt.Sprintf("%s/compile", c.baseURL)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// Ping checks if the sidecar is reachable.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check HTTP %d", resp.StatusCode)
	}
	return nil
}
