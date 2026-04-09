package charttest

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type helmRelease struct {
	Spec struct {
		Values map[string]any `yaml:"values"`
	} `yaml:"spec"`
}

func TestHelmTemplate_MergesIngressAnnotationsAndSetsComponentServiceNames(t *testing.T) {
	overlayPath := writeValuesFile(t, map[string]any{
		"observability": map[string]any{
			"enabled": true,
			"otel": map[string]any{
				"serviceName": "sub2api",
				"environment": "staging",
			},
		},
		"ingress": map[string]any{
			"annotations": map[string]any{
				"cert-manager.io/cluster-issuer": "letsencrypt-staging",
			},
			"gateway": map[string]any{
				"annotations": map[string]any{
					"nginx.ingress.kubernetes.io/proxy-read-timeout": "600",
				},
			},
			"control": map[string]any{
				"annotations": map[string]any{
					"nginx.ingress.kubernetes.io/proxy-send-timeout": "30",
				},
			},
		},
	})

	manifests := renderChart(t, overlayPath)

	gatewayIngress := findManifest(t, manifests, "Ingress", "sub2api-gateway")
	controlIngress := findManifest(t, manifests, "Ingress", "sub2api-control")

	gatewayAnnotations := nestedStringMap(t, gatewayIngress, "metadata", "annotations")
	require.Equal(t, "letsencrypt-staging", gatewayAnnotations["cert-manager.io/cluster-issuer"])
	require.Equal(t, "600", gatewayAnnotations["nginx.ingress.kubernetes.io/proxy-read-timeout"])

	controlAnnotations := nestedStringMap(t, controlIngress, "metadata", "annotations")
	require.Equal(t, "letsencrypt-staging", controlAnnotations["cert-manager.io/cluster-issuer"])
	require.Equal(t, "30", controlAnnotations["nginx.ingress.kubernetes.io/proxy-send-timeout"])
	_, inheritedGatewayTimeout := controlAnnotations["nginx.ingress.kubernetes.io/proxy-read-timeout"]
	require.False(t, inheritedGatewayTimeout)

	gatewayDeployment := findManifest(t, manifests, "Deployment", "sub2api-gateway")
	controlDeployment := findManifest(t, manifests, "Deployment", "sub2api-control")
	workerDeployment := findManifest(t, manifests, "Deployment", "sub2api-worker")

	require.Equal(t, "sub2api-gateway", containerEnvValue(t, gatewayDeployment, "OTEL_SERVICE_NAME"))
	require.Equal(t, "sub2api-control", containerEnvValue(t, controlDeployment, "OTEL_SERVICE_NAME"))
	require.Equal(t, "sub2api-worker", containerEnvValue(t, workerDeployment, "OTEL_SERVICE_NAME"))
	require.Contains(t, containerEnvValue(t, gatewayDeployment, "OTEL_RESOURCE_ATTRIBUTES"), "deployment.environment=staging")
	require.Contains(t, containerEnvValue(t, gatewayDeployment, "OTEL_RESOURCE_ATTRIBUTES"), "sub2api.component=gateway")
}

func TestHelmTemplate_ProductionValuesEnableGatewayAvailabilitySettings(t *testing.T) {
	valuesPath := productionValuesPath(t)
	secretsPath := writeValuesFile(t, map[string]any{
		"postgresql": map[string]any{
			"auth": map[string]any{
				"password":         "test-password",
				"postgresPassword": "test-postgres-password",
			},
		},
		"redis": map[string]any{
			"auth": map[string]any{
				"password": "test-redis-password",
			},
		},
		"grafanaProvisioning": map[string]any{
			"reader": map[string]any{
				"username": "grafana-reader",
				"password": "grafana-password",
			},
		},
	})
	manifests := renderChart(t, valuesPath, secretsPath)

	gatewayIngress := findManifest(t, manifests, "Ingress", "sub2api-gateway")
	gatewayAnnotations := nestedStringMap(t, gatewayIngress, "metadata", "annotations")
	require.Equal(t, "10", gatewayAnnotations["nginx.ingress.kubernetes.io/proxy-connect-timeout"])
	require.Equal(t, "600", gatewayAnnotations["nginx.ingress.kubernetes.io/proxy-read-timeout"])
	require.Equal(t, "600", gatewayAnnotations["nginx.ingress.kubernetes.io/proxy-send-timeout"])

	controlIngress := findManifest(t, manifests, "Ingress", "sub2api-control")
	controlAnnotations := nestedStringMap(t, controlIngress, "metadata", "annotations")
	_, controlHasLongReadTimeout := controlAnnotations["nginx.ingress.kubernetes.io/proxy-read-timeout"]
	require.False(t, controlHasLongReadTimeout)

	findManifest(t, manifests, "HorizontalPodAutoscaler", "sub2api-gateway")
	findManifest(t, manifests, "PodDisruptionBudget", "sub2api-gateway")

	gatewayDeployment := findManifest(t, manifests, "Deployment", "sub2api-gateway")
	topologySpread := nestedSlice(t, gatewayDeployment, "spec", "template", "spec", "topologySpreadConstraints")
	require.Len(t, topologySpread, 2)
	require.Equal(t, "sub2api-gateway", containerEnvValue(t, gatewayDeployment, "OTEL_SERVICE_NAME"))
}

func renderChart(t *testing.T, valuesFiles ...string) []map[string]any {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm binary not available")
	}

	chartPath := filepath.Join(repoRoot(t), "deploy", "helm", "sub2api")
	args := []string{"template", "sub2api", chartPath, "--namespace", "sub2api"}
	for _, valuesFile := range valuesFiles {
		args = append(args, "-f", valuesFile)
	}

	cmd := exec.Command("helm", args...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	return decodeManifestStream(t, output)
}

func productionValuesPath(t *testing.T) string {
	t.Helper()

	releasePath := filepath.Join(repoRoot(t), "clusters", "production", "apps", "sub2api.yaml")
	raw, err := os.ReadFile(releasePath)
	require.NoError(t, err)

	var release helmRelease
	require.NoError(t, yaml.Unmarshal(raw, &release))
	return writeValuesFile(t, release.Spec.Values)
}

func writeValuesFile(t *testing.T, values map[string]any) string {
	t.Helper()

	raw, err := yaml.Marshal(values)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "values.yaml")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	return path
}

func decodeManifestStream(t *testing.T, raw []byte) []map[string]any {
	t.Helper()

	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var manifests []map[string]any
	for {
		var doc map[string]any
		err := decoder.Decode(&doc)
		if err != nil {
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
		}
		if len(doc) == 0 {
			continue
		}
		manifests = append(manifests, doc)
	}
	return manifests
}

func findManifest(t *testing.T, manifests []map[string]any, kind, name string) map[string]any {
	t.Helper()

	for _, manifest := range manifests {
		if stringValue(manifest["kind"]) != kind {
			continue
		}
		metadata, ok := manifest["metadata"].(map[string]any)
		if !ok {
			continue
		}
		if stringValue(metadata["name"]) == name {
			return manifest
		}
	}
	t.Fatalf("manifest %s/%s not found", kind, name)
	return nil
}

func containerEnvValue(t *testing.T, manifest map[string]any, envName string) string {
	t.Helper()

	containers := nestedSlice(t, manifest, "spec", "template", "spec", "containers")
	require.NotEmpty(t, containers)
	container, ok := containers[0].(map[string]any)
	require.True(t, ok)
	envList, ok := container["env"].([]any)
	require.True(t, ok)
	for _, item := range envList {
		env, ok := item.(map[string]any)
		require.True(t, ok)
		if stringValue(env["name"]) == envName {
			return stringValue(env["value"])
		}
	}
	t.Fatalf("env %q not found", envName)
	return ""
}

func nestedStringMap(t *testing.T, manifest map[string]any, path ...string) map[string]string {
	t.Helper()

	current := nestedMap(t, manifest, path...)
	out := make(map[string]string, len(current))
	for key, value := range current {
		out[key] = stringValue(value)
	}
	return out
}

func nestedMap(t *testing.T, manifest map[string]any, path ...string) map[string]any {
	t.Helper()

	current := manifest
	for _, segment := range path {
		next, ok := current[segment].(map[string]any)
		require.True(t, ok, "path %v missing map segment %q", path, segment)
		current = next
	}
	return current
}

func nestedSlice(t *testing.T, manifest map[string]any, path ...string) []any {
	t.Helper()

	current := manifest
	for _, segment := range path[:len(path)-1] {
		next, ok := current[segment].(map[string]any)
		require.True(t, ok, "path %v missing map segment %q", path, segment)
		current = next
	}
	out, ok := current[path[len(path)-1]].([]any)
	require.True(t, ok, "path %v missing slice", path)
	return out
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}
