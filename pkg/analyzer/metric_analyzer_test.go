package analyzer

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	rolloutv1alpha1 "github.com/envoy-gateway-rollout-controller/api/v1alpha1"
)

type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func TestMetricAnalyzer_Evaluate_Healthy(t *testing.T) {
	mockClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) *http.Response {
			jsonResp := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"0.05"]}]}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(jsonResp)),
				Header:     make(http.Header),
			}
		}),
	}

	analyzer := NewMetricAnalyzerWithClient(mockClient)
	rules := []rolloutv1alpha1.MetricRule{
		{
			Name:      "http-5xx-rate",
			Threshold: 1.0,
			Query:     `sum(rate(envoy_cluster_upstream_rq_xx{response_code_class="5"}[1m]))`,
		},
	}

	result, err := analyzer.Evaluate(context.Background(), "http://mock-prom:9090", rules, []string{"node-01"}, 0, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.AllHealthy {
		t.Errorf("expected healthy evaluation, got unhealthy")
	}
	if result.MetricsSnapshot["http-5xx-rate"] != 0.05 {
		t.Errorf("expected 0.05, got %f", result.MetricsSnapshot["http-5xx-rate"])
	}
	if result.ConsecutiveFailures != 0 {
		t.Errorf("expected 0 consecutive failures, got %d", result.ConsecutiveFailures)
	}
}

func TestMetricAnalyzer_Evaluate_ThresholdBreach(t *testing.T) {
	mockClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) *http.Response {
			jsonResp := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"2.50"]}]}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(jsonResp)),
				Header:     make(http.Header),
			}
		}),
	}

	analyzer := NewMetricAnalyzerWithClient(mockClient)
	rules := []rolloutv1alpha1.MetricRule{
		{
			Name:      "http-5xx-rate",
			Threshold: 1.0,
			Query:     `sum(rate(envoy_cluster_upstream_rq_xx{response_code_class="5"}[1m]))`,
		},
	}

	// 2 previous failures + 1 current breach = 3 (failureLimit reached)
	result, err := analyzer.Evaluate(context.Background(), "http://mock-prom:9090", rules, []string{"node-01"}, 2, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.AllHealthy {
		t.Errorf("expected unhealthy state due to failureLimit reached")
	}
	if result.ConsecutiveFailures != 3 {
		t.Errorf("expected 3 consecutive failures, got %d", result.ConsecutiveFailures)
	}
	if len(result.ViolationMessages) == 0 {
		t.Errorf("expected violation message, got none")
	}
}
