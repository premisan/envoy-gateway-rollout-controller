package controller

import (
	"testing"
)

func TestParseNodeCount(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		totalNodes int
		expected   int
	}{
		{
			name:       "exact number 1",
			input:      "1",
			totalNodes: 10,
			expected:   1,
		},
		{
			name:       "exact number 3",
			input:      "3",
			totalNodes: 10,
			expected:   3,
		},
		{
			name:       "percentage 50%",
			input:      "50%",
			totalNodes: 6,
			expected:   3,
		},
		{
			name:       "percentage 100%",
			input:      "100%",
			totalNodes: 6,
			expected:   6,
		},
		{
			name:       "exceed total nodes clamp",
			input:      "20",
			totalNodes: 5,
			expected:   5,
		},
		{
			name:       "percentage zero clamp to 1",
			input:      "5%",
			totalNodes: 5,
			expected:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNodeCount(tt.input, tt.totalNodes)
			if got != tt.expected {
				t.Errorf("parseNodeCount(%q, %d) = %d, expected %d", tt.input, tt.totalNodes, got, tt.expected)
			}
		})
	}
}

func TestSanitizeResourceName(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"Node-01", "node-01"},
		{"ingress.worker.node_01", "ingress-worker-node-01"},
		{"desktop-control-plane", "desktop-control-plane"},
	}

	for _, c := range cases {
		got := sanitizeResourceName(c.input)
		if got != c.expected {
			t.Errorf("sanitizeResourceName(%q) = %q, expected %q", c.input, got, c.expected)
		}
	}
}
