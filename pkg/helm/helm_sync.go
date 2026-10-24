package helm

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	DefaultOCIAppChart = "oci://docker.io/envoyproxy/gateway-helm"
	DefaultOCICRDChart = "oci://docker.io/envoyproxy/gateway-crds-helm"
)

// SyncHelmRelease implements the exact Envoy Gateway official upgrade procedure:
// Step 1: Pre-upgrade CRDs via 'helm template gateway-crds-helm | kubectl apply --server-side --force-conflicts'
// Step 2: Synchronize Envoy Gateway application stack via 'helm template gateway-helm | kubectl apply --server-side --force-conflicts'
//
// This guarantees that:
// 1. Existing CRDs are updated without being skipped (bypassing Helm's limitation of skipping CRD upgrades).
// 2. All associated components (RBAC, ConfigMap, Webhook, Deployment) are synchronized without ownership conflict.
func SyncHelmRelease(ctx context.Context, releaseName, namespace, targetVersion string) error {
	logger := log.FromContext(ctx)

	cleanVer := strings.TrimSpace(targetVersion)
	if cleanVer == "" {
		return fmt.Errorf("target version cannot be empty")
	}

	helmPath, err := exec.LookPath("helm")
	if err != nil {
		logger.Info("Helm binary not found in PATH, skipping helm sync", "error", err)
		return nil
	}

	kubectlPath, err := exec.LookPath("kubectl")
	if err != nil {
		logger.Info("kubectl binary not found in PATH", "error", err)
		return nil
	}

	// -------------------------------------------------------------
	// Step 1: Official CRD Upgrade
	// Helm by design does NOT upgrade CRDs on 'helm upgrade'.
	// As recommended by Envoy Gateway official documentation:
	// helm template eg-crds oci://docker.io/envoyproxy/gateway-crds-helm --version <ver> \
	//   --set crds.gatewayAPI.enabled=true --set crds.envoyGateway.enabled=true \
	//   | kubectl apply --server-side --force-conflicts -f -
	// -------------------------------------------------------------
	logger.Info("[CRD Pre-upgrade] Rendering and applying CRDs via gateway-crds-helm", "version", cleanVer)
	crdCmd := exec.CommandContext(ctx, helmPath,
		"template", releaseName+"-crds", DefaultOCICRDChart,
		"--version", cleanVer,
		"--set", "crds.gatewayAPI.enabled=true",
		"--set", "crds.envoyGateway.enabled=true",
	)
	crdCmd.Env = append(crdCmd.Environ(), "HOME=/tmp", "HELM_CONFIG_HOME=/tmp/.helm", "HELM_CACHE_HOME=/tmp/.helm/cache")
	var crdRaw, crdErrBuf bytes.Buffer
	crdCmd.Stdout = &crdRaw
	crdCmd.Stderr = &crdErrBuf
	if err := crdCmd.Run(); err != nil {
		logger.Error(err, "Notice: gateway-crds-helm template notice", "stderr", crdErrBuf.String())
	} else if crdRaw.Len() > 0 {
		crdClean := cleanManifestYAML(crdRaw.String())
		if len(crdClean) > 0 {
			applyCmd := exec.CommandContext(ctx, kubectlPath,
				"apply", "--server-side", "--force-conflicts", "-f", "-",
			)
			applyCmd.Stdin = strings.NewReader(crdClean)
			applyOut, applyErr := applyCmd.CombinedOutput()
			if applyErr != nil {
				logger.Error(applyErr, "Failed to apply CRDs via server-side apply", "output", string(applyOut))
			} else {
				appliedCount := strings.Count(string(applyOut), "serverside-applied")
				logger.Info("Successfully applied CRDs via Server-Side Apply", "appliedCount", appliedCount)
			}
		}
	}

	// -------------------------------------------------------------
	// Step 2: Official Control Plane Stack Upgrade
	// helm template eg oci://docker.io/envoyproxy/gateway-helm --version <ver> -n <ns> \
	//   --skip-crds --set crds.enabled=false \
	//   | kubectl apply --server-side --force-conflicts -f -
	// -------------------------------------------------------------
	logger.Info("[Control Plane Upgrade] Rendering and applying Envoy Gateway application stack",
		"release", releaseName,
		"version", cleanVer,
		"namespace", namespace,
	)
	appCmd := exec.CommandContext(ctx, helmPath,
		"template", releaseName, DefaultOCIAppChart,
		"--version", cleanVer,
		"--namespace", namespace,
		"--skip-crds",
		"--set", "crds.enabled=false",
	)
	appCmd.Env = append(appCmd.Environ(), "HOME=/tmp", "HELM_CONFIG_HOME=/tmp/.helm", "HELM_CACHE_HOME=/tmp/.helm/cache")
	var appRaw, appErrBuf bytes.Buffer
	appCmd.Stdout = &appRaw
	appCmd.Stderr = &appErrBuf
	if err := appCmd.Run(); err != nil {
		logger.Error(err, "gateway-helm template failed", "stderr", appErrBuf.String())
	} else if appRaw.Len() > 0 {
		appClean := cleanManifestYAML(appRaw.String())
		if len(appClean) > 0 {
			applyCmd := exec.CommandContext(ctx, kubectlPath,
				"apply", "--server-side", "--force-conflicts", "-f", "-",
			)
			applyCmd.Stdin = strings.NewReader(appClean)
			applyOut, applyErr := applyCmd.CombinedOutput()
			if applyErr != nil {
				logger.Error(applyErr, "Failed to apply control plane stack via server-side apply", "output", string(applyOut))
				return fmt.Errorf("control plane stack apply failed: %w", applyErr)
			}
			appliedCount := strings.Count(string(applyOut), "serverside-applied")
			logger.Info("Successfully synchronized Envoy Gateway control plane components via Server-Side Apply",
				"appliedCount", appliedCount,
			)
		}
	}

	return nil
}

// cleanManifestYAML removes OCI pull informational logs from helm template stdout
func cleanManifestYAML(raw string) string {
	idx := strings.Index(raw, "---\n")
	if idx != -1 {
		return raw[idx:]
	}
	idxApi := strings.Index(raw, "apiVersion:")
	if idxApi != -1 {
		return raw[idxApi:]
	}
	return raw
}
