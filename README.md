# gocrud

Package `gocrud` uses reflection to create CRUD operations for structs, on PostgreSQL and SQLite.

It fits applications whose data is mostly records of structs, e.g. the backend of a REST API, an internal tool or an admin panel: it creates and migrates the tables, validates and saves the objects, and gets them with filters, relations, pagination and reports, in transactions. It is not an SQL query builder: queries that do not fit, e.g. with window functions or joins of many tables, are better written as SQL, and run in the same transactions. Tables need an `ID` field, a number or a UUID, so tables with composite primary keys are not supported.

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

The GitHub workflows run the tests, the static analysis and the vulnerability check on pull requests and on `main`, and the vulnerability check also every Monday, as new vulnerabilities are found without any change of the code. Dependabot proposes updates of the dependencies and of the actions every week.
