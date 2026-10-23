package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=egrc;egr;egrollout
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Target-Version",type="string",JSONPath=".spec.version"
// +kubebuilder:printcolumn:name="Current-Step",type="integer",JSONPath=".status.currentStep"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EnvoyGatewayRollout is the Schema for the Envoy Gateway zero-downtime rollout API.
type EnvoyGatewayRollout struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EnvoyGatewayRolloutSpec   `json:"spec,omitempty"`
	Status EnvoyGatewayRolloutStatus `json:"status,omitempty"`
}

// EnvoyGatewayRolloutSpec defines the desired state of Envoy Gateway and rollout behavior.
type EnvoyGatewayRolloutSpec struct {
	// Version is the desired Envoy Gateway version (e.g. "v1.6.1", "v1.7.0")
	Version string `json:"version"`

	// TargetNamespace is the namespace where Envoy Gateway should be deployed (defaults to "envoy-gateway-system")
	// +kubebuilder:default="envoy-gateway-system"
	TargetNamespace string `json:"targetNamespace,omitempty"`

	// Gateway configuration
	Gateway GatewayConfig `json:"gateway"`

	// EnvoyProxy configuration for DataPlane DaemonSet
	EnvoyProxy EnvoyProxyConfig `json:"envoyProxy"`

	// Strategy defines the node-level canary rollout stages
	Strategy RolloutStrategy `json:"strategy"`

	// Analysis defines the Prometheus metric evaluation and ephemeral instance setup
	Analysis AnalysisConfig `json:"analysis"`

	// Control allows manual operator interventions (via Web UI or CLI)
	Control ControlConfig `json:"control,omitempty"`
}

// GatewayConfig specifies the Gateway resource parameters
type GatewayConfig struct {
	Name             string           `json:"name"`
	GatewayClassName string           `json:"gatewayClassName,omitempty"`
	Listeners        []ListenerConfig `json:"listeners,omitempty"`
}

// ListenerConfig specifies ports and TLS configurations
type ListenerConfig struct {
	Name            string `json:"name"`
	Protocol        string `json:"protocol"` // HTTP, HTTPS
	Port            int32  `json:"port"`
	TLSSecretName   string `json:"tlsSecretName,omitempty"`
}

// EnvoyProxyConfig defines DataPlane DaemonSet settings
type EnvoyProxyConfig struct {
	Name         string              `json:"name,omitempty"`
	NodeSelector map[string]string   `json:"nodeSelector,omitempty"`
	Tolerations  []corev1.Toleration `json:"tolerations,omitempty"`
	Annotations  map[string]string   `json:"annotations,omitempty"`
}

// RolloutStrategy defines canary steps per node
type RolloutStrategy struct {
	Steps []RolloutStep `json:"steps"`
}

// RolloutStep defines a single stage in the rollout process
type RolloutStep struct {
	// Nodes can be a count (e.g. "1", "3") or percentage (e.g. "50%", "100%")
	Nodes string `json:"nodes"`

	// PauseDuration is the metric observation wait time (e.g. "3m", "5m")
	PauseDuration string `json:"pauseDuration"`
}

// AnalysisConfig configures Prometheus metrics and ephemeral instance provisioning
type AnalysisConfig struct {
	// EphemeralPrometheus spawns a lightweight temporary Prometheus during rollout
	EphemeralPrometheus EphemeralPrometheusConfig `json:"ephemeralPrometheus,omitempty"`

	// PrometheusURL allows specifying an existing Prometheus server instead
	PrometheusURL string `json:"prometheusUrl,omitempty"`

	// Interval for metric evaluation (e.g. "15s")
	// +kubebuilder:default="15s"
	Interval string `json:"interval,omitempty"`

	// FailureLimit is consecutive failures before automatic rollback triggers
	// +kubebuilder:default=3
	FailureLimit int `json:"failureLimit,omitempty"`

	// Metrics to evaluate
	Metrics []MetricRule `json:"metrics,omitempty"`
}

// EphemeralPrometheusConfig configures the on-demand Prometheus instance
type EphemeralPrometheusConfig struct {
	// Enabled creates an ephemeral Prometheus pod during rollout and cleans up when finished
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// Retention duration for ephemeral metrics (e.g. "2h")
	// +kubebuilder:default="2h"
	Retention string `json:"retention,omitempty"`

	// ScrapeInterval (e.g. "5s", "10s")
	// +kubebuilder:default="5s"
	ScrapeInterval string `json:"scrapeInterval,omitempty"`

	// Image for Prometheus (defaults to prom/prometheus:v2.54.0)
	Image string `json:"image,omitempty"`
}

// MetricRule defines threshold and PromQL query
type MetricRule struct {
	Name      string  `json:"name"`
	Threshold float64 `json:"threshold"`
	Query     string  `json:"query"`
}

// ControlConfig enables manual control flags injected via Web UI
type ControlConfig struct {
	// Paused freezes the step timer and waits indefinitely
	Paused bool `json:"paused,omitempty"`

	// Abort triggers an immediate graceful rollback
	Abort bool `json:"abort,omitempty"`

	// ForcePromote skips remaining observation time and moves to next step
	ForcePromote bool `json:"forcePromote,omitempty"`
}

// RolloutPhase represents the lifecycle state
type RolloutPhase string

const (
	PhasePending       RolloutPhase = "Pending"
	PhaseDeploying     RolloutPhase = "Deploying"
	PhaseCanaryRunning RolloutPhase = "CanaryRunning"
	PhaseObserving     RolloutPhase = "Observing"
	PhasePromoting     RolloutPhase = "Promoting"
	PhaseDraining      RolloutPhase = "Draining"
	PhaseCompleted     RolloutPhase = "Completed"
	PhaseRollingBack   RolloutPhase = "RollingBack"
	PhaseAborted       RolloutPhase = "Aborted"
	PhasePaused        RolloutPhase = "Paused"
)

// NodeRolloutStatus describes a single node's state
type NodeRolloutStatus struct {
	NodeName       string `json:"nodeName"`
	NodeIP         string `json:"nodeIP"`
	Phase          string `json:"phase"` // Stable, Canary, Draining, Completed, RolledBack
	ActiveVersion  string `json:"activeVersion"`
	CanaryVersion  string `json:"canaryVersion,omitempty"`
	DrainProgress  int    `json:"drainProgress"` // 0 to 100%
	ActiveSessions int    `json:"activeSessions"`
	ErrorCount5xx  int    `json:"errorCount5xx"`
}

// EnvoyGatewayRolloutStatus defines the observed state
type EnvoyGatewayRolloutStatus struct {
	Phase                 RolloutPhase        `json:"phase,omitempty"`
	CurrentVersion        string              `json:"currentVersion,omitempty"`
	TargetVersion         string              `json:"targetVersion,omitempty"`
	CurrentStep           int                 `json:"currentStep,omitempty"`
	TotalSteps            int                 `json:"totalSteps,omitempty"`
	StepTimeRemaining     string              `json:"stepTimeRemaining,omitempty"`
	EphemeralPrometheusURL string              `json:"ephemeralPrometheusUrl,omitempty"`
	ConsecutiveFailures   int                 `json:"consecutiveFailures,omitempty"`
	Nodes                 []NodeRolloutStatus `json:"nodes,omitempty"`
	LastMetricSnapshot    map[string]float64  `json:"lastMetricSnapshot,omitempty"`
	Message               string              `json:"message,omitempty"`
	LastUpdateTime        metav1.Time         `json:"lastUpdateTime,omitempty"`
}

// +kubebuilder:object:root=true

// EnvoyGatewayRolloutList contains a list of EnvoyGatewayRollout
type EnvoyGatewayRolloutList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EnvoyGatewayRollout `json:"items"`
}
