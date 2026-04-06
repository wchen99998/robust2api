#!/bin/bash
set -euo pipefail

CHART_DIR="$(cd "$(dirname "$0")/.." && pwd)"

echo "=== Helm template render tests ==="

RENDERED=$(helm template test "$CHART_DIR" --set secrets.jwtSecret=test-secret-that-is-at-least-32-bytes --set secrets.totpEncryptionKey=test-totp-key-that-is-at-least-32-bytes-long --set secrets.adminPassword=testpass 2>&1)
BOOTSTRAP_JOB=$(printf '%s\n' "$RENDERED" | awk '
BEGIN { RS="---\n"; ORS="" }
$0 ~ /kind: Job/ && $0 ~ /name: test-sub2api-bootstrap-r1/ { print; exit }
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

# Test 7: No volumeMounts in Deployment
echo -n "No /app/data volumeMount in Deployment... "
echo "$RENDERED" | grep -q 'mountPath: /app/data' && { echo "FAIL — volume mount still present"; exit 1; } || echo "PASS"

# Test 8: Deployment still exists
echo -n "Server Deployment exists... "
echo "$RENDERED" | grep -q 'kind: Deployment' && echo "PASS" || { echo "FAIL"; exit 1; }

# Test 9: existingSecret mode works
RENDERED_EXT=$(helm template test "$CHART_DIR" --set existingSecret=my-secret --set secrets.jwtSecret=test-secret-that-is-at-least-32-bytes --set secrets.totpEncryptionKey=test-totp-key-that-is-at-least-32-bytes-long 2>&1)
echo -n "existingSecret mode renders... "
echo "$RENDERED_EXT" | grep -q 'my-secret' && echo "PASS" || { echo "FAIL"; exit 1; }

echo "=== All render tests passed ==="
