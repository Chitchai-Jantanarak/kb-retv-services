#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

matches="$(
  grep -RIn --include='*.go' 'context\.TODO()' internal pkg 2>/dev/null \
    | grep -v '_test\.go:' || true
)"

if [[ -n "$matches" ]]; then
  printf '%s\n' "$matches"
  exit 1
fi
