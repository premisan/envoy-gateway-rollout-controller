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
	mux.HandleFunc("/api/rollouts", s.handleRollouts)
	mux.HandleFunc("/api/rollouts/", s.handleRolloutActions)
	mux.HandleFunc("/api/releases", s.handleReleases)
	mux.HandleFunc("/api/stream", s.handleSSE)
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"version": "v1.0.14",
			"image":   "premisan0/envoy-gateway-rollout-controller:v1.0.14",
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
			current.Spec.Control.ForcePromote = true
			return s.client.Update(ctx, current)
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to force promote: %v", err), http.StatusInternalServerError)
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
