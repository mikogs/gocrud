#!/usr/bin/env bash
# Runs the benchmarks of the bench module, and writes their results to $OUT, outside the repository:
#   bench.txt    the output of go test, which benchstat reads
#   summary.txt  gocrud next to database/sql, with how much more gocrud takes
#
# Environment:
#   BENCH      a regular expression of the benchmarks to run, e.g. 'Overhead/op=Get' (default: all)
#   COUNT      the number of runs of each benchmark, for benchstat to see the variation (default: 10)
#   BENCHTIME  the duration or the number of iterations of a run, e.g. 2s or 1000x (default: 1s)
#   OUT        the directory of the results (default: $TMPDIR/gocrud-bench, or /tmp/gocrud-bench)
#   BENCH_POSTGRES_URL  the PostgreSQL database of BenchmarkPostgres, whose tables with the prefix bench_ are replaced.
#              When it is not set, the script starts PostgreSQL in docker, or skips BenchmarkPostgres without docker.
set -euo pipefail

BENCH=${BENCH:-.}
COUNT=${COUNT:-10}
BENCHTIME=${BENCHTIME:-1s}
BENCHSTAT_VERSION=v0.0.0-20261009192801-be2c69fb417e

root=$(cd "$(dirname "$0")/.." && pwd)
OUT=${OUT:-${TMPDIR:-/tmp}/gocrud-bench}
mkdir -p "$OUT"
OUT=$(cd "$OUT" && pwd)

if [ -z "${BENCH_POSTGRES_URL:-}" ]; then
  if command -v docker >/dev/null; then
    container=$(docker run -d --rm -e POSTGRES_PASSWORD=bench -p 127.0.0.1::5432 postgres:17)
    trap 'docker stop "$container" >/dev/null' EXIT
    trap 'exit 1' INT TERM
    port=$(docker port "$container" 5432/tcp | head -1 | cut -d: -f2)
    # the server restarts once after the initialization of the database, so it is ready when it accepts queries
    until docker exec "$container" psql -U postgres -h 127.0.0.1 -c 'SELECT 1' >/dev/null 2>&1; do sleep 0.5; done
    export BENCH_POSTGRES_URL="postgres://postgres:bench@127.0.0.1:$port/postgres?sslmode=disable"
  else
    echo "docker is not available, BenchmarkPostgres is skipped" >&2
  fi
fi

cd "$root/bench"
go test -run '^$' -bench "$BENCH" -benchmem -benchtime "$BENCHTIME" -count "$COUNT" -timeout 60m ./... | tee "$OUT/bench.txt"

go run "golang.org/x/perf/cmd/benchstat@$BENCHSTAT_VERSION" -col "/lib@(sql gocrud)" "$OUT/bench.txt" > "$OUT/summary.txt"
echo
cat "$OUT/summary.txt"
