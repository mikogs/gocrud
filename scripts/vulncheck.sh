#!/usr/bin/env bash
# Checks gocrud and its bench module, with their tests, for known vulnerabilities with govulncheck. It fails when
# the code calls a vulnerable function, of a dependency or of the standard library of the Go version in use.
set -euo pipefail

GOVULNCHECK_VERSION=v1.9.0

root=$(cd "$(dirname "$0")/.." && pwd)

for module in "$root" "$root/bench" "$root/otelhook"; do
  echo "== $module"
  cd "$module"
  go run "golang.org/x/vuln/cmd/govulncheck@$GOVULNCHECK_VERSION" -test ./...
done
