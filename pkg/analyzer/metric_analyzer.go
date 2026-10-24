package analyzer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	rolloutv1alpha1 "github.com/envoy-gateway-rollout-controller/api/v1alpha1"
)

// MetricAnalyzer executes PromQL queries against Prometheus and checks thresholds.
type MetricAnalyzer struct {
	httpClient *http.Client
}

// NewMetricAnalyzer creates a new MetricAnalyzer.
func NewMetricAnalyzer() *MetricAnalyzer {
	return NewMetricAnalyzerWithClient(&http.Client{
		Timeout: 10 * time.Second,
	})
}

// NewMetricAnalyzerWithClient creates a new MetricAnalyzer with a custom HTTP client.
func NewMetricAnalyzerWithClient(client *http.Client) *MetricAnalyzer {
	return &MetricAnalyzer{
		httpClient: client,
	}
}

// PromQueryResponse represents standard Prometheus instant query API response.
type PromQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"` // [timestamp, "value_string"]
		} `json:"result"`
	} `json:"data"`
	ErrorType string `json:"errorType,omitempty"`
	Error     string `json:"error,omitempty"`
}

// AnalysisResult holds metric query results and health status.
type AnalysisResult struct {
	AllHealthy          bool
	MetricsSnapshot     map[string]float64
	ViolationMessages   []string
	ConsecutiveFailures int
}

// Evaluate performs threshold checks for all defined metrics.
func (a *MetricAnalyzer) Evaluate(
	ctx context.Context,
	prometheusURL string,
	rules []rolloutv1alpha1.MetricRule,
	canaryNodes []string,
	currentConsecutiveFailures int,
	failureLimit int,
) (*AnalysisResult, error) {
	if failureLimit <= 0 {
		failureLimit = 3
	}

	result := &AnalysisResult{
		AllHealthy:          true,
		MetricsSnapshot:     make(map[string]float64),
		ViolationMessages:   make([]string, 0),
		ConsecutiveFailures: currentConsecutiveFailures,
	}

	canaryNodesRegex := strings.Join(canaryNodes, "|")
	if canaryNodesRegex == "" {
		canaryNodesRegex = ".*"
	}

	anyViolation := false

	for _, rule := range rules {
		query := strings.ReplaceAll(rule.Query, "$CANARY_NODES", canaryNodesRegex)

		val, err := a.queryPrometheus(ctx, prometheusURL, query)
		if err != nil {
			// Prometheus query failure counts as anomaly warning
			result.ViolationMessages = append(result.ViolationMessages, fmt.Sprintf("metric %s query error: %v", rule.Name, err))
			continue
		}

		result.MetricsSnapshot[rule.Name] = val

		if val > rule.Threshold {
			anyViolation = true
			result.ViolationMessages = append(result.ViolationMessages,
				fmt.Sprintf("metric %s breached threshold: current=%.2f > limit=%.2f", rule.Name, val, rule.Threshold),
			)
		}
	}

	if anyViolation {
		result.ConsecutiveFailures++
		if result.ConsecutiveFailures >= failureLimit {
			result.AllHealthy = false
		}
	} else {
		// Reset on healthy sample
		result.ConsecutiveFailures = 0
		result.AllHealthy = true
	}

	return result, nil
}

func (a *MetricAnalyzer) queryPrometheus(ctx context.Context, promURL, query string) (float64, error) {
	endpoint := fmt.Sprintf("%s/api/v1/query?query=%s", strings.TrimRight(promURL, "/"), url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("prometheus request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("prometheus returned status %d: %s", resp.StatusCode, string(body))
	}

	var pResp PromQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&pResp); err != nil {
		return 0, fmt.Errorf("failed to decode prometheus response: %w", err)
	}

	if pResp.Status != "success" {
		return 0, fmt.Errorf("prometheus error: %s (%s)", pResp.Error, pResp.ErrorType)
	}

	if len(pResp.Data.Result) == 0 {
		// Zero data returned usually means 0 error rate or no traffic
		return 0.0, nil
	}

	// Read first metric value
	rawVal := pResp.Data.Result[0].Value
	if len(rawVal) < 2 {
		return 0.0, nil
	}

	valStr, ok := rawVal[1].(string)
	if !ok {
		return 0.0, fmt.Errorf("unexpected value format: %v", rawVal[1])
	}

	floatVal, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return 0.0, fmt.Errorf("failed to parse metric float value: %w", err)
	}

	return floatVal, nil
}

// QueryActiveConnections queries Prometheus for active downstream connection counts grouped by pod IP.
func (a *MetricAnalyzer) QueryActiveConnections(ctx context.Context, promURL string) (map[string]int, error) {
	if promURL == "" {
		return nil, nil
	}
	query := "sum by (instance) (envoy_listener_downstream_cx_active)"
	endpoint := fmt.Sprintf("%s/api/v1/query?query=%s", strings.TrimRight(promURL, "/"), url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus returned status %d", resp.StatusCode)
	}

	var pResp PromQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&pResp); err != nil {
		return nil, err
	}

	result := make(map[string]int)
	for _, item := range pResp.Data.Result {
		inst := item.Metric["instance"]
		ip := strings.Split(inst, ":")[0]
		if len(item.Value) >= 2 {
			if valStr, ok := item.Value[1].(string); ok {
				if f, err := strconv.ParseFloat(valStr, 64); err == nil {
					result[ip] += int(f)
				}
			}
		}
	}
	return result, nil
}

// QueryPodActiveConnections queries envoy pods directly via their admin metrics port (19001).
// This serves as an immediate fallback whenever Prometheus is not running (e.g. idle/completed state).
func (a *MetricAnalyzer) QueryPodActiveConnections(ctx context.Context, podIPs []string) map[string]int {
	result := make(map[string]int)
	client := &http.Client{Timeout: 2 * time.Second}

	for _, ip := range podIPs {
		if ip == "" {
			continue
		}
		url := fmt.Sprintf("http://%s:19001/stats/prometheus", ip)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}

		scanner := bufio.NewScanner(bytes.NewReader(body))
		sum := 0
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "#") {
				continue
			}
			if strings.HasPrefix(line, "envoy_listener_downstream_cx_active{") {
				if !strings.Contains(line, "19001") && !strings.Contains(line, "19000") && !strings.Contains(line, "19003") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						if val, err := strconv.Atoi(fields[1]); err == nil {
							sum += val
						}
					}
				}
			}
		}
		result[ip] = sum
	}
	return result
}
