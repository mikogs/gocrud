#!/usr/bin/env bash
# Runs the tests of gocrud and of its bench module with the race detector, which needs cgo, and checks that no
# goroutines are still running after the tests of each package, see pkg/test/leaks.go. The benchmarks run once, to
# check their code too. The tests start PostgreSQL in docker.
set -euo pipefail

export LEAKCHECK=1 CGO_ENABLED=1

root=$(cd "$(dirname "$0")/.." && pwd)

for module in "$root" "$root/bench"; do
  echo "== $module"
  cd "$module"
  go test -race -count=1 -timeout 20m ./...
done

echo "== $root/bench, benchmarks"
cd "$root/bench"
go test -race -count=1 -run '^$' -bench . -benchtime 1x ./...
