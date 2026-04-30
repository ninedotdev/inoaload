#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

wails build -clean -platform darwin/universal "$@"

echo
echo "Built: build/bin/iNoaload.app"
