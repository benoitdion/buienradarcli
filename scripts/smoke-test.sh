#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd -- "$script_dir/.." && pwd)

cd "$repo_root"

echo "Running live Buienradar smoke test..."
echo "This forces a fresh API-key extraction in a temporary cache."
go test -tags=e2e -run '^TestCLIColdStartKeyExtraction$' -count=1 -v ./internal/cli
