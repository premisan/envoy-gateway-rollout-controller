package envoy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AdminClient interacts with an Envoy Proxy's admin interface (default port 19001).
type AdminClient struct {
	httpClient *http.Client
}

// NewAdminClient creates a new AdminClient with timeout.
func NewAdminClient() *AdminClient {
	return &AdminClient{
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// DrainListeners triggers graceful draining of inbound listeners on the target Envoy pod.
// This allows existing active connections to finish while refusing new incoming connections.
func (c *AdminClient) DrainListeners(ctx context.Context, envoyHost string, port int) error {
	if port <= 0 {
		port = 19001
	}
	url := fmt.Sprintf("http://%s:%d/drain_listeners?inboundonly", envoyHost, port)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create drain request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call drain_listeners on %s: %w", envoyHost, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("drain_listeners returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// CheckLiveness queries /server_info to verify if the Envoy instance is healthy.
func (c *AdminClient) CheckLiveness(ctx context.Context, envoyHost string, port int) (bool, string, error) {
	if port <= 0 {
		port = 19001
	}
	url := fmt.Sprintf("http://%s:%d/server_info", envoyHost, port)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, "", err
	}

	content := string(body)
	isLive := resp.StatusCode == http.StatusOK && strings.Contains(content, "state: LIVE")
	return isLive, content, nil
}
