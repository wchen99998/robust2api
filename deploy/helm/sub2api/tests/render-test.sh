#!/bin/bash
set -euo pipefail

CHART_DIR="$(cd "$(dirname "$0")/.." && pwd)"

echo "=== Helm template render tests ==="

RENDERED=$(helm template test "$CHART_DIR" --set secrets.jwtSecret=test-secret-that-is-at-least-32-bytes --set secrets.totpEncryptionKey=test-totp-key-that-is-at-least-32-bytes-long --set secrets.adminPassword=testpass 2>&1)

# Test 1: Bootstrap Job exists
echo -n "Bootstrap Job exists... "
echo "$RENDERED" | grep -q 'kind: Job' && echo "PASS" || { echo "FAIL"; exit 1; }

# Test 2: Bootstrap Job has hook annotations
echo -n "Bootstrap Job has pre-install hook... "
echo "$RENDERED" | grep -q 'helm.sh/hook.*pre-install' && echo "PASS" || { echo "FAIL"; exit 1; }

# Test 3: Bootstrap Job uses bootstrap command
echo -n "Bootstrap Job uses sub2api-bootstrap command... "
echo "$RENDERED" | grep -q 'sub2api-bootstrap' && echo "PASS" || { echo "FAIL"; exit 1; }

# Test 4: No standalone PVC (volumeClaimTemplates inside StatefulSets are allowed)
echo -n "No PersistentVolumeClaim... "
echo "$RENDERED" | grep -q '^kind: PersistentVolumeClaim' && { echo "FAIL — PVC still exists"; exit 1; } || echo "PASS"

# Test 5: No AUTO_SETUP in ConfigMap
echo -n "No AUTO_SETUP in ConfigMap... "
echo "$RENDERED" | grep -q 'AUTO_SETUP' && { echo "FAIL — AUTO_SETUP still present"; exit 1; } || echo "PASS"

# Test 6: No volumeMounts in Deployment
echo -n "No /app/data volumeMount in Deployment... "
echo "$RENDERED" | grep -q 'mountPath: /app/data' && { echo "FAIL — volume mount still present"; exit 1; } || echo "PASS"

# Test 7: Deployment still exists
echo -n "Server Deployment exists... "
echo "$RENDERED" | grep -q 'kind: Deployment' && echo "PASS" || { echo "FAIL"; exit 1; }

# Test 8: existingSecret mode works
RENDERED_EXT=$(helm template test "$CHART_DIR" --set existingSecret=my-secret --set secrets.jwtSecret=test-secret-that-is-at-least-32-bytes --set secrets.totpEncryptionKey=test-totp-key-that-is-at-least-32-bytes-long 2>&1)
echo -n "existingSecret mode renders... "
echo "$RENDERED_EXT" | grep -q 'my-secret' && echo "PASS" || { echo "FAIL"; exit 1; }

echo "=== All render tests passed ==="
