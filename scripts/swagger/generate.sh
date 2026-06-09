#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../.."

args=(
  init
  -g scripts/swagger/api.go
  -o docs/swagger
  --parseInternal
  --parseDependency
  --parseDepth 2
)

if command -v swag >/dev/null 2>&1; then
  swag "${args[@]}"
else
  go run github.com/swaggo/swag/cmd/swag@v1.16.6 "${args[@]}"
fi
