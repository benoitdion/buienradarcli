#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd -- "$script_dir/.." && pwd)

cd "$repo_root"

echo "Running live Buienradar smoke test..."
echo "This checks fresh API-key extraction, rain graphs, and forecast temperature coverage."
go test -tags=e2e -run '^TestCLIEndToEnd$' -count=1 -v ./internal/cli
