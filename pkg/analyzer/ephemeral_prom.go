package analyzer

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rolloutv1alpha1 "github.com/envoy-gateway-rollout-controller/api/v1alpha1"
)

const (
	DefaultPrometheusImage = "prom/prometheus:v2.54.0"
	EphemeralPromPrefix    = "egrc-prom-"
)

// EphemeralPrometheusManager handles on-demand lifecycle of a temporary Prometheus instance.
type EphemeralPrometheusManager struct {
	client client.Client
}

// NewEphemeralPrometheusManager creates a new EphemeralPrometheusManager.
func NewEphemeralPrometheusManager(c client.Client) *EphemeralPrometheusManager {
	return &EphemeralPrometheusManager{client: c}
}

// EnsureEphemeralPrometheus ensures an ephemeral Prometheus deployment, service, and configmap exist for this rollout.
func (m *EphemeralPrometheusManager) EnsureEphemeralPrometheus(
	ctx context.Context,
	rollout *rolloutv1alpha1.EnvoyGatewayRollout,
	targetPodIPs []string,
) (string, error) {
	ns := rollout.Spec.TargetNamespace
	if ns == "" {
		ns = "envoy-gateway-system"
	}
	baseName := EphemeralPromPrefix + rollout.Name

	image := rollout.Spec.Analysis.EphemeralPrometheus.Image
	if image == "" {
		image = DefaultPrometheusImage
	}
	scrapeInterval := rollout.Spec.Analysis.EphemeralPrometheus.ScrapeInterval
	if scrapeInterval == "" {
		scrapeInterval = "5s"
	}

	// Discover existing envoy proxy pods if targetPodIPs is empty
	if len(targetPodIPs) == 0 {
		podList := &corev1.PodList{}
		if err := m.client.List(ctx, podList, client.InNamespace(ns), client.MatchingLabels{"app.kubernetes.io/name": "envoy"}); err == nil {
			for _, p := range podList.Items {
				if p.Status.PodIP != "" {
					targetPodIPs = append(targetPodIPs, p.Status.PodIP)
				}
			}
		}
	}

	// 1. Create or Update ConfigMap
	var targetsYaml strings.Builder
	for _, ip := range targetPodIPs {
		targetsYaml.WriteString(fmt.Sprintf("          - '%s:19001'\n", ip))
	}
	if len(targetPodIPs) == 0 {
		targetsYaml.WriteString("          - '127.0.0.1:19001'\n")
	}

	configContent := fmt.Sprintf(`global:
  scrape_interval: %s
  evaluation_interval: %s

scrape_configs:
  - job_name: 'envoy-stats'
    metrics_path: '/stats/prometheus'
    static_configs:
      - targets:
%s
`, scrapeInterval, scrapeInterval, targetsYaml.String())

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      baseName + "-config",
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "ephemeral-prometheus",
				"app.kubernetes.io/managed-by": "envoy-gateway-rollout-controller",
				"rollout.envoyproxy.io/name":   rollout.Name,
			},
		},
		Data: map[string]string{
			"prometheus.yml": configContent,
		},
	}

	existingCM := &corev1.ConfigMap{}
	err := m.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: cm.Name}, existingCM)
	if errors.IsNotFound(err) {
		if err := m.client.Create(ctx, cm); err != nil {
			return "", fmt.Errorf("failed to create ephemeral prometheus configmap: %w", err)
		}
	} else if err == nil {
		existingCM.Data = cm.Data
		if err := m.client.Update(ctx, existingCM); err != nil {
			return "", fmt.Errorf("failed to update ephemeral prometheus configmap: %w", err)
		}
	} else {
		return "", err
	}

	configHash := fmt.Sprintf("%x", sha256.Sum256([]byte(configContent)))

	// 2. Create Deployment
	replicas := int32(1)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      baseName,
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "ephemeral-prometheus",
				"app.kubernetes.io/managed-by": "envoy-gateway-rollout-controller",
				"rollout.envoyproxy.io/name":   rollout.Name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": baseName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name": baseName,
					},
					Annotations: map[string]string{
						"rollout.envoyproxy.io/config-hash": configHash,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "prometheus",
							Image: image,
							Args: []string{
								"--config.file=/etc/prometheus/prometheus.yml",
								"--storage.tsdb.retention.time=2h",
								"--web.enable-lifecycle",
							},
							Ports: []corev1.ContainerPort{
								{Name: "web", ContainerPort: 9090},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config-volume",
									MountPath: "/etc/prometheus",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config-volume",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: cm.Name,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	existingDeploy := &appsv1.Deployment{}
	err = m.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: deploy.Name}, existingDeploy)
	if errors.IsNotFound(err) {
		if err := m.client.Create(ctx, deploy); err != nil {
			return "", fmt.Errorf("failed to create ephemeral prometheus deployment: %w", err)
		}
	} else if err == nil {
		if existingDeploy.Spec.Template.Annotations == nil {
			existingDeploy.Spec.Template.Annotations = make(map[string]string)
		}
		if existingDeploy.Spec.Template.Annotations["rollout.envoyproxy.io/config-hash"] != configHash {
			existingDeploy.Spec.Template.Annotations["rollout.envoyproxy.io/config-hash"] = configHash
			_ = m.client.Update(ctx, existingDeploy)
		}
	} else {
		return "", err
	}

	// 3. Create Service
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      baseName,
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/name": baseName,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app.kubernetes.io/name": baseName,
			},
			Ports: []corev1.ServicePort{
				{
					Port:       9090,
					TargetPort: intstr.FromInt(9090),
					Name:       "web",
				},
			},
		},
	}

	existingSvc := &corev1.Service{}
	err = m.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: svc.Name}, existingSvc)
	if errors.IsNotFound(err) {
		if err := m.client.Create(ctx, svc); err != nil {
			return "", fmt.Errorf("failed to create ephemeral prometheus service: %w", err)
		}
	} else if err != nil {
		return "", err
	}

	serviceURL := fmt.Sprintf("http://%s.%s.svc:9090", svc.Name, ns)
	return serviceURL, nil
}

// CleanupEphemeralPrometheus deletes the temporary deployment, service, and configmap.
func (m *EphemeralPrometheusManager) CleanupEphemeralPrometheus(ctx context.Context, rollout *rolloutv1alpha1.EnvoyGatewayRollout) error {
	ns := rollout.Spec.TargetNamespace
	if ns == "" {
		ns = "envoy-gateway-system"
	}
	baseName := EphemeralPromPrefix + rollout.Name

	// Delete Deployment
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: baseName, Namespace: ns}}
	_ = m.client.Delete(ctx, deploy)

	// Delete Service
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: baseName, Namespace: ns}}
	_ = m.client.Delete(ctx, svc)

	// Delete ConfigMap
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: baseName + "-config", Namespace: ns}}
	_ = m.client.Delete(ctx, cm)

	return nil
}
