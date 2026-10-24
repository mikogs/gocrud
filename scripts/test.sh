#!/usr/bin/env bash
# Checks the formatting and runs go vet and the tests of gocrud and of its bench module, and the tests of gocrud again
# with the pgx driver of PostgreSQL instead of lib/pq. The tests start PostgreSQL in docker, so docker has to be running.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)

for module in "$root" "$root/bench"; do
  echo "== $module"
  cd "$module"

  unformatted=$(gofmt -l .)
  if [ -n "$unformatted" ]; then
    echo "files not formatted with gofmt:" >&2
    echo "$unformatted" >&2
    exit 1
  fi

  go vet ./...
  go test -count=1 -timeout 10m ./...
done

echo "== $root, PostgreSQL with pgx"
cd "$root"
GOCRUD_POSTGRES_DRIVER=pgx go test -count=1 -timeout 10m -skip SQLite ./...
