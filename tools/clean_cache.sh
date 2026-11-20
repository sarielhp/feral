#!/usr/bin/env bash
set -euo pipefail

# Ensure we are in the repository root
cd "$(dirname "$0")/.."

# Check if feral binary exists, compile if missing
if [ ! -f "./feral" ]; then
    make
fi

echo "Cleaning local email cache using built-in cache command..."
./feral cache prune --wipe
