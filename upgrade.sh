#!/bin/sh
set -eu

echo "Upgrading devkit..."

if [ -f VERSION ]; then
  old="$(cat VERSION)"
else
  old="unknown"
fi

go install github.com/89jobrien/devkit/cmd/devkit@latest
go install github.com/89jobrien/devkit/cmd/ci-agent@latest

echo "Upgraded from $old to latest"

# Re-copy CI templates if local ci/ dir exists
if [ -d ci ]; then
  if [ -d .github/workflows ] && [ -f ci/github.yml ]; then
    cp ci/github.yml .github/workflows/ci.yml
    echo "Updated .github/workflows/ci.yml"
  fi
  if [ -d .gitea/workflows ] && [ -f ci/gitea.yml ]; then
    cp ci/gitea.yml .gitea/workflows/ci.yml
    echo "Updated .gitea/workflows/ci.yml"
  fi
fi
