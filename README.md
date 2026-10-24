# gocrud

Package `gocrud` uses reflection to create CRUD operations for structs.

> ⚠️ The project might be good for small prototyping and showcase of reflection package. It is not recommended to use it in production. Please use a more mature library instead.

## Documentation

* [Overview](docs/README.md)
* [Demo](docs/demo.md)
* [Struct Definition](docs/struct.md)
* CRUD object
    * [Initialization](docs/crud/init.md), and the [query hook](docs/crud/init.md#query-hook)
    * [Operations](docs/crud/ops.md)
    * [Typed functions](docs/crud/generics.md)
    * [Transactions](docs/crud/transactions.md)
    * [Migrations](docs/crud/migrations.md)
    * [Versioned migrations](docs/crud/versioned-migrations.md)
    * [Relations](docs/crud/relations.md)
    * [Reports](docs/crud/reports.md)
    * [Custom queries](docs/crud/custom-queries.md)
* Additional packages
    * [Registry](docs/pkgs/registry.md)
* [Helper functions](docs/helper-functions.md)

## Development

* `scripts/test.sh` checks the formatting, and runs `go vet` and the tests, also of the `bench` module, and the tests of PostgreSQL again with the pgx driver. The tests start PostgreSQL in docker.
* `scripts/concurcheck.sh` runs the tests with the race detector, and checks that no goroutines are still running after the tests of each package.
* `scripts/lint.sh` runs the static analysis: golangci-lint, with the linters of `.golangci.yml`, which include gosec, and gosec itself, also on the tests.
* `scripts/vulncheck.sh` checks the code and the dependencies for known vulnerabilities with `govulncheck`.
* `scripts/bench.sh` runs the benchmarks of gocrud, next to the same queries written with `database/sql`.

The GitHub workflows run the tests, the static analysis and the vulnerability check on pull requests and on `main`, and Dependabot proposes updates of the dependencies and of the actions every week.
