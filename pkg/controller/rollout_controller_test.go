package controller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rolloutv1alpha1 "github.com/envoy-gateway-rollout-controller/api/v1alpha1"
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

func TestResolveEnvoyImageForVersion(t *testing.T) {
	cases := []struct {
		gatewayVersion string
		expectedEnvoy  string
	}{
		{"v1.9.1", "docker.io/envoyproxy/envoy:distroless-v1.39.1"},
		{"v1.9.0", "docker.io/envoyproxy/envoy:distroless-v1.39.0"},
		{"v1.8.5", "docker.io/envoyproxy/envoy:distroless-v1.38.4"},
		{"v1.2.3", "docker.io/envoyproxy/envoy:distroless-v1.32.3"},
	}

	for _, c := range cases {
		got := resolveEnvoyImageForVersion(c.gatewayVersion)
		if got != c.expectedEnvoy {
			t.Errorf("resolveEnvoyImageForVersion(%q) = %q, expected %q", c.gatewayVersion, got, c.expectedEnvoy)
		}
	}
}

func TestDetectEnvoyGatewayVersionAndPendingUpgrade(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rolloutv1alpha1.AddToScheme(scheme)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "envoy-gateway",
			Namespace: "envoy-gateway-system",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "envoy-gateway",
							Image: "docker.io/envoyproxy/gateway:v1.7.4",
						},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deploy).Build()
	reconciler := NewEnvoyGatewayRolloutReconciler(fakeClient, scheme)

	ctx := context.Background()

	// 1. Test detectEnvoyGatewayVersion
	detected := reconciler.detectEnvoyGatewayVersion(ctx, "envoy-gateway-system")
	if detected != "v1.7.4" {
		t.Fatalf("expected detected version v1.7.4, got %q", detected)
	}

	// 2. Test handlePending triggers upgrade when target is v1.8.0
	rollout := &rolloutv1alpha1.EnvoyGatewayRollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "eg",
			Namespace: "envoy-gateway-system",
		},
		Spec: rolloutv1alpha1.EnvoyGatewayRolloutSpec{
			Version:         "v1.8.0",
			TargetNamespace: "envoy-gateway-system",
			Strategy: rolloutv1alpha1.RolloutStrategy{
				Steps: []rolloutv1alpha1.RolloutStep{
					{Nodes: "1"},
				},
			},
		},
	}

	res, err := reconciler.handlePending(ctx, rollout)
	if err != nil {
		t.Fatalf("handlePending returned unexpected error: %v", err)
	}
	if !res.Requeue {
		t.Errorf("expected Requeue to be true for upgrade transition")
	}
	if rollout.Status.CurrentVersion != "v1.7.4" {
		t.Errorf("expected rollout.Status.CurrentVersion = v1.7.4, got %q", rollout.Status.CurrentVersion)
	}
	if rollout.Status.Phase != rolloutv1alpha1.PhaseDeploying {
		t.Errorf("expected rollout.Status.Phase = %s, got %s", rolloutv1alpha1.PhaseDeploying, rollout.Status.Phase)
	}
	if rollout.Status.CurrentStep != 1 {
		t.Errorf("expected CurrentStep = 1, got %d", rollout.Status.CurrentStep)
	}
}
