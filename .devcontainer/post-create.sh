#!/usr/bin/env bash
set -euo pipefail

corepack enable
corepack prepare pnpm@9 --activate

if ! command -v golangci-lint >/dev/null 2>&1; then
  curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sudo sh -s -- -b /usr/local/bin v2.9.0
fi

cd /workspaces/sub2api/backend
go mod download

cd /workspaces/sub2api/frontend
pnpm install --frozen-lockfile
