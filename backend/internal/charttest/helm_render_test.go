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
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
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
	gatewayValuesPath := productionValuesPath(t, "sub2api-gateway")
	sharedSecretsPath := writeValuesFile(t, map[string]any{
		"externalDatabase": map[string]any{
			"password": "test-password",
		},
		"externalRedis": map[string]any{
			"password": "test-redis-password",
		},
	})
	manifests := renderChart(t, gatewayValuesPath, sharedSecretsPath)

	gatewayIngress := findManifest(t, manifests, "Ingress", "sub2api-gateway")
	gatewayAnnotations := nestedStringMap(t, gatewayIngress, "metadata", "annotations")
	require.Equal(t, "10", gatewayAnnotations["nginx.ingress.kubernetes.io/proxy-connect-timeout"])
	require.Equal(t, "600", gatewayAnnotations["nginx.ingress.kubernetes.io/proxy-read-timeout"])
	require.Equal(t, "600", gatewayAnnotations["nginx.ingress.kubernetes.io/proxy-send-timeout"])
	requireManifestAbsent(t, manifests, "Ingress", "sub2api-control")

	findManifest(t, manifests, "HorizontalPodAutoscaler", "sub2api-gateway")
	findManifest(t, manifests, "PodDisruptionBudget", "sub2api-gateway")

	gatewayDeployment := findManifest(t, manifests, "Deployment", "sub2api-gateway")
	topologySpread := nestedSlice(t, gatewayDeployment, "spec", "template", "spec", "topologySpreadConstraints")
	require.Len(t, topologySpread, 2)
	require.Equal(t, "sub2api-gateway", containerEnvValue(t, gatewayDeployment, "OTEL_SERVICE_NAME"))

	controlValuesPath := productionValuesPath(t, "sub2api-control")
	controlSecretsPath := writeValuesFile(t, map[string]any{
		"secrets": map[string]any{
			"jwtSecret":         "jwt-secret",
			"totpEncryptionKey": "totp-key",
			"adminPassword":     "admin-password",
		},
		"externalDatabase": map[string]any{
			"password": "test-password",
		},
		"externalRedis": map[string]any{
			"password": "test-redis-password",
		},
		"grafanaProvisioning": map[string]any{
			"reader": map[string]any{
				"username": "grafana-reader",
				"password": "grafana-password",
			},
		},
	})
	controlManifests := renderChart(t, controlValuesPath, controlSecretsPath)
	controlIngress := findManifest(t, controlManifests, "Ingress", "sub2api-control")
	controlAnnotations := nestedStringMap(t, controlIngress, "metadata", "annotations")
	_, controlHasLongReadTimeout := controlAnnotations["nginx.ingress.kubernetes.io/proxy-read-timeout"]
	require.False(t, controlHasLongReadTimeout)
}

func TestHelmTemplate_DefaultValuesRenderLegacySingleReleaseTopology(t *testing.T) {
	manifests := renderChart(t)

	findManifest(t, manifests, "Deployment", "sub2api-gateway")
	controlDeployment := findManifest(t, manifests, "Deployment", "sub2api-control")
	findManifest(t, manifests, "Deployment", "sub2api-worker")
	frontendDeployment := findManifest(t, manifests, "Deployment", "sub2api-frontend")

	findManifest(t, manifests, "Service", "sub2api-gateway")
	frontendService := findManifest(t, manifests, "Service", "sub2api-control")
	findManifest(t, manifests, "Service", "sub2api-control-api")

	findManifest(t, manifests, "Ingress", "sub2api-gateway")
	findManifest(t, manifests, "Ingress", "sub2api-control")

	bootstrapJob := findManifest(t, manifests, "Job", "sub2api-bootstrap")
	require.Equal(t, "sub2api-bootstrap", stringValue(nestedMap(t, bootstrapJob, "metadata")["name"]))

	controlContainers := nestedSlice(t, controlDeployment, "spec", "template", "spec", "containers")
	require.Len(t, controlContainers, 1)
	controlContainer, ok := controlContainers[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "control", stringValue(controlContainer["name"]))

	require.Equal(t, "http://sub2api-control-api:8080", containerEnvValue(t, frontendDeployment, "CONTROL_UPSTREAM"))
	require.Equal(t, "http://sub2api-gateway:80", containerEnvValue(t, frontendDeployment, "GATEWAY_UPSTREAM"))

	frontendSelector := nestedMap(t, frontendService, "spec", "selector")
	require.Equal(t, "frontend", stringValue(frontendSelector["app.kubernetes.io/component"]))
}

func TestHelmTemplate_ComponentSelectiveControlReleaseRendersWithoutGatewayAndWorker(t *testing.T) {
	valuesPath := writeValuesFile(t, map[string]any{
		"gateway": map[string]any{
			"enabled": false,
		},
		"control": map[string]any{
			"enabled": true,
		},
		"worker": map[string]any{
			"enabled": false,
		},
		"bootstrap": map[string]any{
			"enabled": false,
		},
		"frontend": map[string]any{
			"enabled":            true,
			"gatewayServiceName": "sub2api-gateway-gateway",
		},
		"ingress": map[string]any{
			"enabled": true,
			"gateway": map[string]any{
				"enabled": false,
			},
			"control": map[string]any{
				"enabled": true,
			},
		},
		"postgresql": map[string]any{
			"enabled": false,
		},
		"redis": map[string]any{
			"enabled": false,
		},
		"externalDatabase": map[string]any{
			"host":     "postgres.internal",
			"password": "db-password",
		},
		"externalRedis": map[string]any{
			"host":     "redis.internal",
			"password": "redis-password",
		},
		"secrets": map[string]any{
			"jwtSecret":         "jwt-secret",
			"totpEncryptionKey": "totp-key",
			"adminPassword":     "admin-password",
		},
	})
	manifests := renderChart(t, valuesPath)

	findManifest(t, manifests, "Deployment", "sub2api-control")
	frontendDeployment := findManifest(t, manifests, "Deployment", "sub2api-frontend")
	findManifest(t, manifests, "Service", "sub2api-control")
	findManifest(t, manifests, "Service", "sub2api-control-api")
	findManifest(t, manifests, "Ingress", "sub2api-control")

	requireManifestAbsent(t, manifests, "Deployment", "sub2api-gateway")
	requireManifestAbsent(t, manifests, "Deployment", "sub2api-worker")
	requireManifestAbsent(t, manifests, "Job", "sub2api-bootstrap")
	requireManifestAbsent(t, manifests, "Ingress", "sub2api-gateway")

	require.Equal(t, "http://sub2api-gateway-gateway:80", containerEnvValue(t, frontendDeployment, "GATEWAY_UPSTREAM"))
}

func TestHelmTemplate_BootstrapChecksumTracksOnlyBootstrapInputs(t *testing.T) {
	baseValues := map[string]any{
		"gateway": map[string]any{
			"enabled": false,
		},
		"control": map[string]any{
			"enabled": false,
		},
		"worker": map[string]any{
			"enabled": false,
		},
		"frontend": map[string]any{
			"enabled": false,
		},
		"bootstrap": map[string]any{
			"enabled":          true,
			"manualRerunToken": "",
		},
		"postgresql": map[string]any{
			"enabled": false,
		},
		"redis": map[string]any{
			"enabled": false,
		},
		"externalDatabase": map[string]any{
			"host":     "postgres.internal",
			"port":     5432,
			"user":     "sub2api",
			"database": "sub2api",
			"sslmode":  "require",
			"password": "db-password",
		},
		"externalRedis": map[string]any{
			"host":      "redis.internal",
			"port":      6379,
			"password":  "redis-password",
			"enableTLS": true,
		},
		"secrets": map[string]any{
			"jwtSecret":         "jwt-secret",
			"totpEncryptionKey": "totp-key",
			"adminPassword":     "admin-password",
		},
	}

	checksumA := renderBootstrapChecksum(t, baseValues)

	unrelatedValues := cloneMap(t, baseValues)
	unrelatedValues["ingress"] = map[string]any{
		"enabled": false,
	}
	checksumUnrelated := renderBootstrapChecksum(t, unrelatedValues)
	require.Equal(t, checksumA, checksumUnrelated)

	tokenValues := cloneMap(t, baseValues)
	tokenValues["bootstrap"] = map[string]any{
		"enabled":          true,
		"manualRerunToken": "manual-rerun-1",
	}
	checksumWithToken := renderBootstrapChecksum(t, tokenValues)
	require.NotEqual(t, checksumA, checksumWithToken)
}

func TestHelmTemplate_ProductionMultiReleaseValuesRender(t *testing.T) {
	tests := []struct {
		releaseName    string
		secrets        map[string]any
		expectedKind   string
		expectedObject string
	}{
		{
			releaseName: "sub2api-bootstrap",
			secrets: map[string]any{
				"secrets": map[string]any{
					"jwtSecret":         "jwt-secret",
					"totpEncryptionKey": "totp-key",
					"adminPassword":     "admin-password",
				},
				"externalDatabase": map[string]any{
					"password": "db-password",
				},
				"externalRedis": map[string]any{
					"password": "redis-password",
				},
			},
			expectedKind:   "Job",
			expectedObject: "sub2api-bootstrap-bootstrap",
		},
		{
			releaseName: "sub2api-gateway",
			secrets: map[string]any{
				"externalDatabase": map[string]any{
					"password": "db-password",
				},
				"externalRedis": map[string]any{
					"password": "redis-password",
				},
			},
			expectedKind:   "Deployment",
			expectedObject: "sub2api-gateway-gateway",
		},
		{
			releaseName: "sub2api-control",
			secrets: map[string]any{
				"secrets": map[string]any{
					"jwtSecret":         "jwt-secret",
					"totpEncryptionKey": "totp-key",
					"adminPassword":     "admin-password",
				},
				"externalDatabase": map[string]any{
					"password": "db-password",
				},
				"externalRedis": map[string]any{
					"password": "redis-password",
				},
				"grafanaProvisioning": map[string]any{
					"reader": map[string]any{
						"username": "grafana-reader",
						"password": "grafana-password",
					},
				},
			},
			expectedKind:   "Deployment",
			expectedObject: "sub2api-control-frontend",
		},
		{
			releaseName: "sub2api-worker",
			secrets: map[string]any{
				"externalDatabase": map[string]any{
					"password": "db-password",
				},
				"externalRedis": map[string]any{
					"password": "redis-password",
				},
			},
			expectedKind:   "Deployment",
			expectedObject: "sub2api-worker-worker",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.releaseName, func(t *testing.T) {
			valuesPath := productionValuesPath(t, tc.releaseName)
			secretsPath := writeValuesFile(t, tc.secrets)
			manifests := renderChartWithReleaseName(t, tc.releaseName, valuesPath, secretsPath)
			findManifest(t, manifests, tc.expectedKind, tc.expectedObject)
		})
	}
}

func renderChart(t *testing.T, valuesFiles ...string) []map[string]any {
	t.Helper()
	return renderChartWithReleaseName(t, "sub2api", valuesFiles...)
}

func renderChartWithReleaseName(t *testing.T, releaseName string, valuesFiles ...string) []map[string]any {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm binary not available")
	}

	chartPath := filepath.Join(repoRoot(t), "deploy", "helm", "sub2api")
	args := []string{"template", releaseName, chartPath, "--namespace", "sub2api"}
	for _, valuesFile := range valuesFiles {
		args = append(args, "-f", valuesFile)
	}

	cmd := exec.Command("helm", args...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	return decodeManifestStream(t, output)
}

func productionValuesPath(t *testing.T, releaseName string) string {
	t.Helper()

	releases := productionReleaseValues(t)
	values, ok := releases[releaseName]
	require.True(t, ok, "production HelmRelease %q not found", releaseName)
	return writeValuesFile(t, values)
}

func productionReleaseValues(t *testing.T) map[string]map[string]any {
	t.Helper()

	releasePath := filepath.Join(repoRoot(t), "clusters", "production", "apps", "sub2api.yaml")
	raw, err := os.ReadFile(releasePath)
	require.NoError(t, err)

	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	releases := map[string]map[string]any{}
	for {
		var release helmRelease
		err := decoder.Decode(&release)
		if err != nil {
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
		}
		if release.Kind != "HelmRelease" || release.Metadata.Name == "" {
			continue
		}
		releases[release.Metadata.Name] = release.Spec.Values
	}

	return releases
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

func requireManifestAbsent(t *testing.T, manifests []map[string]any, kind, name string) {
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
			t.Fatalf("manifest %s/%s unexpectedly rendered", kind, name)
		}
	}
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

func renderBootstrapChecksum(t *testing.T, values map[string]any) string {
	t.Helper()

	overlayPath := writeValuesFile(t, values)
	manifests := renderChart(t, overlayPath)
	job := findManifest(t, manifests, "Job", "sub2api-bootstrap")

	metadataAnnotations := nestedStringMap(t, job, "metadata", "annotations")
	templateAnnotations := nestedStringMap(t, job, "spec", "template", "metadata", "annotations")
	require.Equal(t, metadataAnnotations["sub2api.io/bootstrap-inputs-checksum"], templateAnnotations["sub2api.io/bootstrap-inputs-checksum"])

	return metadataAnnotations["sub2api.io/bootstrap-inputs-checksum"]
}

func cloneMap(t *testing.T, src map[string]any) map[string]any {
	t.Helper()

	raw, err := yaml.Marshal(src)
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &out))

	return out
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
