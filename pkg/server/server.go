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
	mux.HandleFunc("/api/stream", s.handleSSE)

	// Frontend UI static files serving
	if s.uiFS != nil {
		mux.Handle("/", http.FileServer(http.FS(s.uiFS)))
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

	// Action dispatching
	switch action {
	case "pause":
		rollout.Spec.Control.Paused = true
		_ = s.client.Update(ctx, rollout)
		writeJSON(w, http.StatusOK, map[string]string{"message": "Rollout paused"})
	case "resume":
		rollout.Spec.Control.Paused = false
		if rollout.Status.Phase == rolloutv1alpha1.PhasePaused {
			rollout.Status.Phase = rolloutv1alpha1.PhaseObserving
		}
		_ = s.client.Update(ctx, rollout)
		_ = s.client.Status().Update(ctx, rollout)
		writeJSON(w, http.StatusOK, map[string]string{"message": "Rollout resumed"})
	case "abort", "rollback":
		rollout.Spec.Control.Abort = true
		_ = s.client.Update(ctx, rollout)
		writeJSON(w, http.StatusOK, map[string]string{"message": "Rollback triggered"})
	case "force-promote":
		rollout.Spec.Control.ForcePromote = true
		_ = s.client.Update(ctx, rollout)
		writeJSON(w, http.StatusOK, map[string]string{"message": "Force promote triggered"})
	case "upgrade":
		var req struct {
			TargetVersion string `json:"targetVersion"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TargetVersion == "" {
			http.Error(w, "Invalid targetVersion", http.StatusBadRequest)
			return
		}
		rollout.Spec.Version = req.TargetVersion
		rollout.Spec.Control.Abort = false
		rollout.Spec.Control.Paused = false
		rollout.Status.Phase = rolloutv1alpha1.PhaseDeploying
		rollout.Status.CurrentStep = 1
		rollout.Status.ConsecutiveFailures = 0
		_ = s.client.Update(ctx, rollout)
		_ = s.client.Status().Update(ctx, rollout)
		writeJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Upgrade to %s initiated", req.TargetVersion)})
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
	ticker := time.NewTicker(2 * time.Second)
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
