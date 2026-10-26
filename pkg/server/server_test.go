package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rolloutv1alpha1 "github.com/envoy-gateway-rollout-controller/api/v1alpha1"
)

func TestDiscoverExistingConfigAndTemplate(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rolloutv1alpha1.AddToScheme(scheme)

	// Create a fake custom EnvoyProxy
	envoyProxy := &unstructured.Unstructured{}
	envoyProxy.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "gateway.envoyproxy.io",
		Version: "v1alpha1",
		Kind:    "EnvoyProxy",
	})
	envoyProxy.SetName("custom-proxy-config")
	envoyProxy.SetNamespace("envoy-gateway-system")
	_ = unstructured.SetNestedMap(envoyProxy.Object, map[string]interface{}{
		"type": "Kubernetes",
		"kubernetes": map[string]interface{}{
			"envoyDaemonSet": map[string]interface{}{
				"pod": map[string]interface{}{
					"nodeSelector": map[string]interface{}{
						"dkosv3.9rum.cc/node-group": "envoy-gateway",
					},
					"tolerations": []interface{}{
						map[string]interface{}{
							"key":      "node-role.dkosv3.9rum.cc/lb",
							"operator": "Equal",
							"value":    "true",
							"effect":   "NoExecute",
						},
					},
				},
			},
		},
	}, "spec", "provider")

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(envoyProxy).Build()
	srv := NewServer(fakeClient, ":8080", nil)

	ctx := context.Background()
	cfg := srv.discoverExistingConfig(ctx, "envoy-gateway-system")

	if cfg.EnvoyProxyName != "custom-proxy-config" {
		t.Fatalf("expected EnvoyProxyName custom-proxy-config, got %q", cfg.EnvoyProxyName)
	}
	if cfg.NodeSelector["dkosv3.9rum.cc/node-group"] != "envoy-gateway" {
		t.Errorf("expected nodeSelector dkosv3.9rum.cc/node-group, got %+v", cfg.NodeSelector)
	}
	if len(cfg.Tolerations) != 1 || cfg.Tolerations[0].Key != "node-role.dkosv3.9rum.cc/lb" {
		t.Errorf("expected toleration node-role.dkosv3.9rum.cc/lb, got %+v", cfg.Tolerations)
	}

	tmpl := srv.generateRolloutTemplate("v1.8.0", "envoy-gateway-system", cfg)
	if !strings.Contains(tmpl, "custom-proxy-config") {
		t.Errorf("expected template to contain custom-proxy-config: %s", tmpl)
	}
	if !strings.Contains(tmpl, "node-role.dkosv3.9rum.cc/lb") {
		t.Errorf("expected template to contain tolerations: %s", tmpl)
	}
}

func TestHandleBootstrapRolloutCustomYAML(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rolloutv1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	srv := NewServer(fakeClient, ":8080", nil)

	customYAML := `
apiVersion: rollout.envoyproxy.io/v1alpha1
kind: EnvoyGatewayRollout
metadata:
  name: my-custom-rollout
  namespace: envoy-gateway-system
spec:
  version: v1.8.0
  targetNamespace: envoy-gateway-system
  gateway:
    name: my-gw
  envoyProxy:
    name: custom-proxy-config
  strategy:
    steps:
      - nodes: "1"
`
	reqBody := map[string]string{
		"yaml":          customYAML,
		"targetVersion": "v1.8.0",
		"namespace":     "envoy-gateway-system",
	}
	jsonBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/rollouts/bootstrap", strings.NewReader(string(jsonBytes)))
	rec := httptest.NewRecorder()

	srv.handleBootstrapRollout(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify created resource
	var created rolloutv1alpha1.EnvoyGatewayRollout
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Namespace: "envoy-gateway-system", Name: "my-custom-rollout"}, &created); err != nil {
		t.Fatalf("failed to get created rollout: %v", err)
	}

	if created.Spec.Gateway.Name != "my-gw" {
		t.Errorf("expected Gateway.Name my-gw, got %q", created.Spec.Gateway.Name)
	}
	if created.Spec.EnvoyProxy.Name != "custom-proxy-config" {
		t.Errorf("expected EnvoyProxy.Name custom-proxy-config, got %q", created.Spec.EnvoyProxy.Name)
	}
}
