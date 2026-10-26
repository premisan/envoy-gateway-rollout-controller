package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rolloutv1alpha1 "github.com/envoy-gateway-rollout-controller/api/v1alpha1"
)

// Server provides REST API and real-time SSE streaming for the Web UI.
type Server struct {
	client     client.Client
	listenAddr string
	uiFS       fs.FS
	clients    map[chan string]bool
	clientsMu  sync.RWMutex
}

// NewServer creates a new Server instance.
func NewServer(c client.Client, listenAddr string, uiFS fs.FS) *Server {
	return &Server{
		client:     c,
		listenAddr: listenAddr,
		uiFS:       uiFS,
		clients:    make(map[chan string]bool),
	}
}

// Start begins HTTP serving and SSE broadcast loop.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// REST API Endpoints
	mux.HandleFunc("/api/rollouts/bootstrap", s.handleBootstrapRollout)
	mux.HandleFunc("/api/rollouts", s.handleRollouts)
	mux.HandleFunc("/api/rollouts/", s.handleRolloutActions)
	mux.HandleFunc("/api/envoy-gateway/status", s.handleEnvoyGatewayStatus)
	mux.HandleFunc("/api/releases", s.handleReleases)
	mux.HandleFunc("/api/stream", s.handleSSE)
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"version": "v1.0.18",
			"image":   "premisan0/envoy-gateway-rollout-controller:v1.0.18",
		})
	})

	// Frontend UI static files serving with anti-caching for index.html
	if s.uiFS != nil {
		fsHandler := http.FileServer(http.FS(s.uiFS))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" || r.URL.Path == "/index.html" || strings.HasSuffix(r.URL.Path, ".html") {
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
				w.Header().Set("Pragma", "no-cache")
				w.Header().Set("Expires", "0")
			}
			fsHandler.ServeHTTP(w, r)
		})
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte("<h1>Envoy Gateway Rollout Controller API is running. (UI static assets not embedded)</h1>"))
		})
	}

	server := &http.Server{
		Addr:    s.listenAddr,
		Handler: withCORS(mux),
	}

	// Background SSE broadcaster
	go s.startBroadcaster(ctx)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	fmt.Printf("[Server] Dashboard & Control API listening on http://0.0.0.0%s\n", s.listenAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) handleRollouts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	if r.Method == http.MethodGet {
		list := &rolloutv1alpha1.EnvoyGatewayRolloutList{}
		if err := s.client.List(ctx, list); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, list.Items)
		return
	}

	// POST: Create a new rollout or initial bootstrap
	var newRollout rolloutv1alpha1.EnvoyGatewayRollout
	if err := json.NewDecoder(r.Body).Decode(&newRollout); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if newRollout.Namespace == "" {
		newRollout.Namespace = "envoy-gateway-system"
	}
	if newRollout.Spec.TargetNamespace == "" {
		newRollout.Spec.TargetNamespace = "envoy-gateway-system"
	}

	if err := s.client.Create(ctx, &newRollout); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create rollout: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, newRollout)
}

func (s *Server) handleRolloutActions(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// /api/rollouts/{name} or /api/rollouts/{name}/{action}
	if len(parts) < 3 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	name := parts[2]
	action := ""
	if len(parts) >= 4 {
		action = parts[3]
	}

	ctx := r.Context()
	rollout := &rolloutv1alpha1.EnvoyGatewayRollout{}
	// Look up by name across namespaces or in default namespace
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = "envoy-gateway-system"
	}

	if err := s.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, rollout); err != nil {
		if errors.IsNotFound(err) {
			http.Error(w, "Rollout not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if action == "" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, rollout)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Action dispatching with optimistic concurrency retry (RetryOnConflict)
	switch action {
	case "pause":
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			current := &rolloutv1alpha1.EnvoyGatewayRollout{}
			if err := s.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, current); err != nil {
				return err
			}
			current.Spec.Control.Paused = true
			return s.client.Update(ctx, current)
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to pause rollout: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "Rollout paused"})
	case "resume":
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			current := &rolloutv1alpha1.EnvoyGatewayRollout{}
			if err := s.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, current); err != nil {
				return err
			}
			current.Spec.Control.Paused = false
			return s.client.Update(ctx, current)
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to resume rollout: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "Rollout resumed"})
	case "abort", "rollback":
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			current := &rolloutv1alpha1.EnvoyGatewayRollout{}
			if err := s.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, current); err != nil {
				return err
			}
			current.Spec.Control.Abort = true
			return s.client.Update(ctx, current)
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to trigger rollback: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "Rollback triggered"})
	case "force-promote":
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			current := &rolloutv1alpha1.EnvoyGatewayRollout{}
			if err := s.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, current); err != nil {
				return err
			}
			if current.Status.Phase != rolloutv1alpha1.PhaseObserving {
				return fmt.Errorf("force promote is only allowed during %s phase, current phase is %s", rolloutv1alpha1.PhaseObserving, current.Status.Phase)
			}
			if current.Spec.Control.Paused {
				return fmt.Errorf("cannot promote step while rollout is paused")
			}
			current.Spec.Control.ForcePromote = true
			return s.client.Update(ctx, current)
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to force promote: %v", err), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "Force promote triggered"})
	case "upgrade":
		var req struct {
			TargetVersion string `json:"targetVersion"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.TargetVersion) == "" {
			http.Error(w, "Invalid targetVersion", http.StatusBadRequest)
			return
		}
		cleanTarget := strings.TrimSpace(req.TargetVersion)

		// 1. Update Spec using RetryOnConflict to avoid conflict with active reconciler
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			current := &rolloutv1alpha1.EnvoyGatewayRollout{}
			if err := s.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, current); err != nil {
				return err
			}
			current.Spec.Version = cleanTarget
			current.Spec.Control.Abort = false
			current.Spec.Control.Paused = false
			return s.client.Update(ctx, current)
		})
		if err != nil {
			fmt.Printf("[Server] Error updating rollout spec: %v\n", err)
			http.Error(w, fmt.Sprintf("Failed to update rollout spec: %v", err), http.StatusInternalServerError)
			return
		}

		// 2. Best-effort optimistic status update with RetryOnConflict
		// Note: The controller reconciler automatically triggers on spec change and transitions status to PhaseDeploying.
		_ = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			current := &rolloutv1alpha1.EnvoyGatewayRollout{}
			if err := s.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, current); err != nil {
				return err
			}
			if current.Status.CurrentVersion != cleanTarget && current.Status.Phase != rolloutv1alpha1.PhaseDeploying && current.Status.Phase != rolloutv1alpha1.PhaseObserving {
				current.Status.TargetVersion = cleanTarget
				current.Status.Phase = rolloutv1alpha1.PhaseDeploying
				current.Status.CurrentStep = 1
				current.Status.ConsecutiveFailures = 0
				current.Status.Message = fmt.Sprintf("Upgrade to %s initiated by operator", cleanTarget)
				current.Status.LastUpdateTime = metav1.Now()
				return s.client.Status().Update(ctx, current)
			}
			return nil
		})

		fmt.Printf("[Server] Successfully initiated upgrade of %s to %s\n", name, cleanTarget)
		writeJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Upgrade to %s initiated", cleanTarget)})
		writeJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Upgrade to %s initiated", cleanTarget)})
	default:
		http.Error(w, "Unknown action: "+action, http.StatusBadRequest)
	}
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientChan := make(chan string, 10)
	s.clientsMu.Lock()
	s.clients[clientChan] = true
	s.clientsMu.Unlock()

	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, clientChan)
		close(clientChan)
		s.clientsMu.Unlock()
	}()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case msg := <-clientChan:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

func (s *Server) startBroadcaster(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.clientsMu.RLock()
			clientCount := len(s.clients)
			s.clientsMu.RUnlock()

			if clientCount == 0 {
				continue
			}

			list := &rolloutv1alpha1.EnvoyGatewayRolloutList{}
			if err := s.client.List(ctx, list); err == nil {
				data, err := json.Marshal(list.Items)
				if err == nil {
					s.broadcast(string(data))
				}
			}
		}
	}
}

func (s *Server) broadcast(msg string) {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	for ch := range s.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// ReleaseInfo describes official Envoy Gateway releases from Helm OCI or GitHub.
type ReleaseInfo struct {
	Version     string `json:"version"`
	HelmChart   string `json:"helmChart"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"publishedAt"`
}

func (s *Server) handleReleases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Try fetching live releases from GitHub API with 2s timeout
	client := &http.Client{Timeout: 2 * time.Second}
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://api.github.com/repos/envoyproxy/gateway/releases?per_page=15", nil)
	req.Header.Set("User-Agent", "envoy-gateway-rollout-controller")

	if resp, err := client.Do(req); err == nil && resp.StatusCode == http.StatusOK {
		var ghReleases []struct {
			TagName     string `json:"tag_name"`
			Prerelease  bool   `json:"prerelease"`
			PublishedAt string `json:"published_at"`
		}
		if json.NewDecoder(resp.Body).Decode(&ghReleases) == nil && len(ghReleases) > 0 {
			results := make([]ReleaseInfo, 0, len(ghReleases))
			for _, rel := range ghReleases {
				if !rel.Prerelease && strings.HasPrefix(rel.TagName, "v") {
					results = append(results, ReleaseInfo{
						Version:     rel.TagName,
						HelmChart:   fmt.Sprintf("oci://docker.io/envoyproxy/gateway-helm:%s", rel.TagName),
						Prerelease:  rel.Prerelease,
						PublishedAt: rel.PublishedAt,
					})
				}
			}
			resp.Body.Close()
			if len(results) > 0 {
				writeJSON(w, http.StatusOK, results)
				return
			}
		}
		resp.Body.Close()
	}

	// Fallback to verified official stable releases
	fallbackReleases := []ReleaseInfo{
		{Version: "v1.9.2", HelmChart: "oci://docker.io/envoyproxy/gateway-helm:v1.9.2", PublishedAt: "2024-10-18T00:00:00Z"},
		{Version: "v1.9.1", HelmChart: "oci://docker.io/envoyproxy/gateway-helm:v1.9.1", PublishedAt: "2024-10-10T00:00:00Z"},
		{Version: "v1.9.0", HelmChart: "oci://docker.io/envoyproxy/gateway-helm:v1.9.0", PublishedAt: "2024-09-25T00:00:00Z"},
		{Version: "v1.8.5", HelmChart: "oci://docker.io/envoyproxy/gateway-helm:v1.8.5", PublishedAt: "2024-09-12T00:00:00Z"},
		{Version: "v1.8.4", HelmChart: "oci://docker.io/envoyproxy/gateway-helm:v1.8.4", PublishedAt: "2024-08-30T00:00:00Z"},
		{Version: "v1.8.0", HelmChart: "oci://docker.io/envoyproxy/gateway-helm:v1.8.0", PublishedAt: "2024-07-15T00:00:00Z"},
		{Version: "v1.7.2", HelmChart: "oci://docker.io/envoyproxy/gateway-helm:v1.7.2", PublishedAt: "2024-06-20T00:00:00Z"},
		{Version: "v1.7.0", HelmChart: "oci://docker.io/envoyproxy/gateway-helm:v1.7.0", PublishedAt: "2024-05-10T00:00:00Z"},
		{Version: "v1.6.1", HelmChart: "oci://docker.io/envoyproxy/gateway-helm:v1.6.1", PublishedAt: "2024-04-05T00:00:00Z"},
	}
	writeJSON(w, http.StatusOK, fallbackReleases)
}

// EnvoyGatewayStatusResponse represents detected Envoy Gateway deployment in the cluster
type EnvoyGatewayStatusResponse struct {
	Installed      bool   `json:"installed"`
	Version        string `json:"version"`
	Namespace      string `json:"namespace"`
	DeploymentName string `json:"deploymentName,omitempty"`
	ReadyReplicas  int32  `json:"readyReplicas"`
}

func (s *Server) detectEnvoyGateway(ctx context.Context, targetNS string) EnvoyGatewayStatusResponse {
	if targetNS == "" {
		targetNS = "envoy-gateway-system"
	}

	resp := EnvoyGatewayStatusResponse{
		Installed: false,
		Version:   "",
		Namespace: targetNS,
	}

	// 1. Try finding deployment 'envoy-gateway'
	deploy := &appsv1.Deployment{}
	err := s.client.Get(ctx, client.ObjectKey{Namespace: targetNS, Name: "envoy-gateway"}, deploy)
	if err == nil {
		resp.Installed = true
		resp.DeploymentName = deploy.Name
		resp.ReadyReplicas = deploy.Status.ReadyReplicas

		for _, container := range deploy.Spec.Template.Spec.Containers {
			if container.Name == "envoy-gateway" || strings.Contains(container.Image, "gateway") {
				parts := strings.Split(container.Image, ":")
				if len(parts) >= 2 {
					tag := parts[len(parts)-1]
					resp.Version = strings.Split(tag, "@")[0]
				}
				break
			}
		}
	} else {
		// 2. Try listing pods with label app.kubernetes.io/name=envoy-gateway
		podList := &corev1.PodList{}
		if listErr := s.client.List(ctx, podList, client.InNamespace(targetNS), client.MatchingLabels{"app.kubernetes.io/name": "envoy-gateway"}); listErr == nil && len(podList.Items) > 0 {
			pod := podList.Items[0]
			resp.Installed = true
			resp.ReadyReplicas = 1
			for _, container := range pod.Spec.Containers {
				if container.Name == "envoy-gateway" || strings.Contains(container.Image, "gateway") {
					parts := strings.Split(container.Image, ":")
					if len(parts) >= 2 {
						tag := parts[len(parts)-1]
						resp.Version = strings.Split(tag, "@")[0]
					}
					break
				}
			}
		}
	}

	return resp
}

func (s *Server) handleEnvoyGatewayStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	targetNS := r.URL.Query().Get("namespace")
	resp := s.detectEnvoyGateway(r.Context(), targetNS)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleBootstrapRollout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TargetVersion string `json:"targetVersion"`
		Namespace     string `json:"namespace"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	targetVer := strings.TrimSpace(req.TargetVersion)
	if targetVer == "" {
		http.Error(w, "targetVersion is required", http.StatusBadRequest)
		return
	}

	ns := strings.TrimSpace(req.Namespace)
	if ns == "" {
		ns = "envoy-gateway-system"
	}

	rollout := &rolloutv1alpha1.EnvoyGatewayRollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "eg",
			Namespace: ns,
		},
		Spec: rolloutv1alpha1.EnvoyGatewayRolloutSpec{
			Version:         targetVer,
			TargetNamespace: ns,
			Gateway: rolloutv1alpha1.GatewayConfig{
				Name: "eg",
			},
			EnvoyProxy: rolloutv1alpha1.EnvoyProxyConfig{
				Name: "eg-proxy",
			},
			Strategy: rolloutv1alpha1.RolloutStrategy{
				Steps: []rolloutv1alpha1.RolloutStep{
					{Nodes: "1", Pause: &rolloutv1alpha1.RolloutPause{Duration: "3m"}},
					{Nodes: "3", Pause: &rolloutv1alpha1.RolloutPause{Duration: "5m"}},
					{Nodes: "100%", Pause: &rolloutv1alpha1.RolloutPause{Duration: "5m"}},
				},
			},
			Analysis: rolloutv1alpha1.AnalysisConfig{
				EphemeralPrometheus: rolloutv1alpha1.EphemeralPrometheusConfig{
					Enabled:        true,
					ScrapeInterval: "5s",
					Retention:      "2h",
				},
				FailureLimit: 3,
				Interval:     "15s",
				Metrics: []rolloutv1alpha1.MetricRule{
					{
						Name:      "http-5xx-rate",
						Threshold: 1.0,
						Query:     "sum(rate(envoy_cluster_upstream_rq_xx{response_code_class=\"5\", instance=~\"$CANARY_NODES.*\"}[1m])) / clamp_min(sum(rate(envoy_cluster_upstream_rq_xx{instance=~\"$CANARY_NODES.*\"}[1m])), 0.001) * 100",
					},
					{
						Name:      "http-4xx-rate",
						Threshold: 5.0,
						Query:     "(sum(rate(envoy_http_downstream_rq_xx{envoy_response_code_class=\"4\", instance=~\"$CANARY_NODES.*\"}[1m])) or sum(rate(envoy_cluster_upstream_rq_xx{envoy_response_code_class=\"4\", instance=~\"$CANARY_NODES.*\"}[1m])) or sum(rate(envoy_cluster_upstream_rq_xx{response_code_class=\"4\", instance=~\"$CANARY_NODES.*\"}[1m]))) / clamp_min(sum(rate(envoy_http_downstream_rq_xx{instance=~\"$CANARY_NODES.*\"}[1m])) or sum(rate(envoy_cluster_upstream_rq_xx{instance=~\"$CANARY_NODES.*\"}[1m])), 0.001) * 100",
					},
					{
						Name:      "connection-errors",
						Threshold: 5.0,
						Query:     "sum(rate(envoy_server_total_connections_error{instance=~\"$CANARY_NODES.*\"}[1m]))",
					},
					{
						Name:      "p99-latency",
						Threshold: 100.0,
						Query:     "(histogram_quantile(0.99, sum(rate(envoy_http_downstream_rq_time_bucket{instance=~\"$CANARY_NODES.*\"}[1m])) by (le)) or histogram_quantile(0.99, sum(rate(envoy_http_downstream_rq_time_bucket[1m])) by (le)))",
					},
				},
			},
		},
	}

	egStatus := s.detectEnvoyGateway(r.Context(), ns)

	if err := s.client.Create(r.Context(), rollout); err != nil {
		if errors.IsAlreadyExists(err) {
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				current := &rolloutv1alpha1.EnvoyGatewayRollout{}
				if err := s.client.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: "eg"}, current); err != nil {
					return err
				}
				current.Spec.Version = targetVer
				current.Spec.Control.Paused = false
				current.Spec.Control.Abort = false

				if egStatus.Installed && egStatus.Version != "" {
					current.Status.CurrentVersion = egStatus.Version
				}
				current.Status.Phase = rolloutv1alpha1.PhasePending
				current.Status.CurrentStep = 0
				current.Status.Message = fmt.Sprintf("Upgrade to %s initiated", targetVer)

				if updateErr := s.client.Update(r.Context(), current); updateErr != nil {
					return updateErr
				}
				return s.client.Status().Update(r.Context(), current)
			})
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to update existing rollout: %v", err), http.StatusInternalServerError)
				return
			}
		} else {
			http.Error(w, fmt.Sprintf("Failed to create rollout: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		// New rollout created: if actual Envoy Gateway exists in cluster, initialize status immediately
		if egStatus.Installed && egStatus.Version != "" {
			_ = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				current := &rolloutv1alpha1.EnvoyGatewayRollout{}
				if err := s.client.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: "eg"}, current); err != nil {
					return err
				}
				current.Status.CurrentVersion = egStatus.Version
				current.Status.Phase = rolloutv1alpha1.PhasePending
				current.Status.CurrentStep = 0
				current.Status.Message = fmt.Sprintf("Upgrade from %s to %s initiated", egStatus.Version, targetVer)
				return s.client.Status().Update(r.Context(), current)
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "Rollout successfully initiated",
		"name":    "eg",
	})
}
