#!/usr/bin/env bash
# Generates shell completion scripts into completions/ for the release
# archives and packages (called by GoReleaser before building).
set -euo pipefail
rm -rf completions
mkdir -p completions
for shell in bash zsh fish; do
    go run ./cmd/ochq completion "$shell" > "completions/ochq.$shell"
done
