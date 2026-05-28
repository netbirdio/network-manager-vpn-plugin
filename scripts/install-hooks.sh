#!/usr/bin/env sh
set -eu

mkdir -p .git/hooks
cp scripts/hooks/pre-commit .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit

echo "Installed pre-commit hook: task quality"
