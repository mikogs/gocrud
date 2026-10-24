package registry

import (
	"context"
	"database/sql"

	"github.com/mikogs/gocrud"
)

// WithTx returns a copy of the registry that runs all its queries in the tx transaction.
// The caller is responsible for committing or rolling back the transaction.
func (c *CRUD) WithTx(tx *sql.Tx) *CRUD {
	return &CRUD{
		registry: c.registry,
		crud:     c.crud.WithTx(tx),
	}
}

// Tx returns the transaction that the registry runs its queries in, or nil if it does not run in a transaction.
func (c *CRUD) Tx() *sql.Tx {
	return c.crud.Tx()
}

// RunInTx runs fn in a transaction, see gocrud.CRUD.RunInTx.
func (c *CRUD) RunInTx(ctx context.Context, fn func(tx *CRUD) error) error {
	return c.RunInTxWithOptions(ctx, nil, fn)
}

// RunInTxWithOptions runs fn in a transaction started with opts, see gocrud.CRUD.RunInTxWithOptions.
func (c *CRUD) RunInTxWithOptions(ctx context.Context, opts *sql.TxOptions, fn func(tx *CRUD) error) error {
	return c.crud.RunInTxWithOptions(ctx, opts, func(txCRUD *gocrud.CRUD) error {
		return fn(&CRUD{
			registry: c.registry,
			crud:     txCRUD,
		})
	})
}
