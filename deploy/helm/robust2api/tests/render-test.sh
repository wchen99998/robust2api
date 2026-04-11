#!/bin/bash
set -euo pipefail

CHART_DIR="$(cd "$(dirname "$0")/.." && pwd)"

echo "=== Helm template render tests ==="

RENDERED=$(helm template test "$CHART_DIR" \
  --set secrets.jwtSecret=test-secret-that-is-at-least-32-bytes \
  --set secrets.totpEncryptionKey=test-totp-key-that-is-at-least-32-bytes-long \
  --set secrets.adminPassword=testpass 2>&1)
BOOTSTRAP_JOB=$(printf '%s\n' "$RENDERED" | awk '
BEGIN { RS="---\n"; ORS="" }
$0 ~ /kind: Job/ && $0 ~ /name: test-sub2api-bootstrap-r1/ { print; exit }
')
API_DEPLOYMENT=$(printf '%s\n' "$RENDERED" | awk '
BEGIN { RS="---\n"; ORS="" }
$0 ~ /kind: Deployment/ && $0 ~ /name: test-sub2api-api/ { print; exit }
')
WORKER_DEPLOYMENT=$(printf '%s\n' "$RENDERED" | awk '
BEGIN { RS="---\n"; ORS="" }
$0 ~ /kind: Deployment/ && $0 ~ /name: test-sub2api-worker/ { print; exit }
')

# Test 1: Bootstrap Job exists
echo -n "Bootstrap Job exists... "
[ -n "$BOOTSTRAP_JOB" ] && echo "PASS" || { echo "FAIL"; exit 1; }

# Test 2: Bootstrap Job is a normal chart resource, not a Helm hook
echo -n "Bootstrap Job is not a Helm hook... "
echo "$BOOTSTRAP_JOB" | grep -q 'helm.sh/hook' && { echo "FAIL — hook annotation still present"; exit 1; } || echo "PASS"

# Test 3: Bootstrap Job uses bootstrap command
echo -n "Bootstrap Job uses sub2api-bootstrap command... "
echo "$BOOTSTRAP_JOB" | grep -q 'sub2api-bootstrap' && echo "PASS" || { echo "FAIL"; exit 1; }

# Test 4: Bootstrap Job name includes release revision for reruns on upgrade
echo -n "Bootstrap Job name includes release revision... "
echo "$BOOTSTRAP_JOB" | grep -q 'name: test-sub2api-bootstrap-r1' && echo "PASS" || { echo "FAIL"; exit 1; }

# Test 5: No standalone PVC (volumeClaimTemplates inside StatefulSets are allowed)
echo -n "No PersistentVolumeClaim... "
echo "$RENDERED" | grep -q '^kind: PersistentVolumeClaim' && { echo "FAIL — PVC still exists"; exit 1; } || echo "PASS"

# Test 6: No AUTO_SETUP in ConfigMap
echo -n "No AUTO_SETUP in ConfigMap... "
echo "$RENDERED" | grep -q 'AUTO_SETUP' && { echo "FAIL — AUTO_SETUP still present"; exit 1; } || echo "PASS"

# Test 7: No UPDATE_PROXY_URL in ConfigMap
echo -n "No UPDATE_PROXY_URL in ConfigMap... "
echo "$RENDERED" | grep -q 'UPDATE_PROXY_URL' && { echo "FAIL — UPDATE_PROXY_URL still present"; exit 1; } || echo "PASS"

# Test 8: No /app/data volume mount in deployments
echo -n "No /app/data volumeMount in Deployment... "
echo "$RENDERED" | grep -q 'mountPath: /app/data' && { echo "FAIL — volume mount still present"; exit 1; } || echo "PASS"

# Test 9: API and Worker deployments exist
echo -n "API Deployment exists... "
[ -n "$API_DEPLOYMENT" ] && echo "PASS" || { echo "FAIL"; exit 1; }
echo -n "Worker Deployment exists... "
[ -n "$WORKER_DEPLOYMENT" ] && echo "PASS" || { echo "FAIL"; exit 1; }

# Test 10: Deployment images use API/worker repositories
echo -n "API Deployment uses api image... "
echo "$API_DEPLOYMENT" | grep -q 'image: "ghcr.io/wchen99998/robust2api/api:' && echo "PASS" || { echo "FAIL"; exit 1; }
echo -n "Worker Deployment uses worker image... "
echo "$WORKER_DEPLOYMENT" | grep -q 'image: "ghcr.io/wchen99998/robust2api/worker:' && echo "PASS" || { echo "FAIL"; exit 1; }

# Test 11: Bootstrap Job uses bootstrap image
echo -n "Bootstrap Job uses bootstrap image... "
echo "$BOOTSTRAP_JOB" | grep -q 'image: "ghcr.io/wchen99998/robust2api/bootstrap:' && echo "PASS" || { echo "FAIL"; exit 1; }

# Test 12: ConfigMap checksum annotation on API and Worker deployments
echo -n "API Deployment has checksum/configmap annotation... "
echo "$API_DEPLOYMENT" | grep -q 'checksum/configmap:' && echo "PASS" || { echo "FAIL"; exit 1; }
echo -n "Worker Deployment has checksum/configmap annotation... "
echo "$WORKER_DEPLOYMENT" | grep -q 'checksum/configmap:' && echo "PASS" || { echo "FAIL"; exit 1; }

# Test 13: existingSecret mode works
RENDERED_EXT=$(helm template test "$CHART_DIR" \
  --set existingSecret=my-secret \
  --set secrets.jwtSecret=test-secret-that-is-at-least-32-bytes \
  --set secrets.totpEncryptionKey=test-totp-key-that-is-at-least-32-bytes-long 2>&1)
echo -n "existingSecret mode renders... "
echo "$RENDERED_EXT" | grep -q 'my-secret' && echo "PASS" || { echo "FAIL"; exit 1; }

# Test 14: Production values include topology spread constraints for API deployment
RENDERED_PROD=$(helm template test "$CHART_DIR" \
  -f "$CHART_DIR/values-production.yaml" \
  --set secrets.jwtSecret=test-secret-that-is-at-least-32-bytes \
  --set secrets.totpEncryptionKey=test-totp-key-that-is-at-least-32-bytes-long \
  --set secrets.adminPassword=testpass 2>&1)
API_DEPLOYMENT_PROD=$(printf '%s\n' "$RENDERED_PROD" | awk '
BEGIN { RS="---\n"; ORS="" }
$0 ~ /kind: Deployment/ && $0 ~ /name: test-sub2api-api/ { print; exit }
')
echo -n "Production API topology spread constraints rendered... "
echo "$API_DEPLOYMENT_PROD" | grep -q 'topologySpreadConstraints:' && echo "PASS" || { echo "FAIL"; exit 1; }
echo -n "Production includes hostname spread key... "
echo "$API_DEPLOYMENT_PROD" | grep -q 'topologyKey: kubernetes.io/hostname' && echo "PASS" || { echo "FAIL"; exit 1; }
echo -n "Production includes zone spread key... "
echo "$API_DEPLOYMENT_PROD" | grep -q 'topologyKey: topology.kubernetes.io/zone' && echo "PASS" || { echo "FAIL"; exit 1; }

echo "=== All render tests passed ==="
