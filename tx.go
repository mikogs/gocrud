package gocrud

import (
	"context"
	"database/sql"
	"errors"
)

// DBTX is the database connection that CRUD runs its queries on. It is implemented by *sql.DB, *sql.Conn and *sql.Tx.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// WithTx returns a copy of the CRUD that runs all its queries in the tx transaction.
// The caller is responsible for committing or rolling back the transaction.
func (c *CRUD) WithTx(tx *sql.Tx) *CRUD {
	txCRUD := *c
	txCRUD.tx = tx
	txCRUD.conn = c.hooked(tx)

	return &txCRUD
}

// Tx returns the transaction that the CRUD runs its queries in, or nil if it does not run in a transaction.
func (c *CRUD) Tx() *sql.Tx {
	return c.tx
}

// RunInTx runs fn in a transaction. The transaction is committed when fn returns nil, and rolled back when fn
// returns an error or panics. All the queries in fn must be run using the tx CRUD passed to it.
// When the CRUD already runs in a transaction, fn joins that transaction instead of starting a new one.
func (c *CRUD) RunInTx(ctx context.Context, fn func(tx *CRUD) error) error {
	return c.RunInTxWithOptions(ctx, nil, fn)
}

// RunInTxWithOptions is RunInTx with options, such as isolation level, for starting the transaction.
// The options are ignored when the CRUD already runs in a transaction.
func (c *CRUD) RunInTxWithOptions(ctx context.Context, opts *sql.TxOptions, fn func(tx *CRUD) error) (err error) {
	if c.tx != nil {
		return fn(c)
	}

	tx, err := c.db.BeginTx(ctx, opts)
	if err != nil {
		return getDBFuncCRUDError("begin transaction", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	err = fn(c.WithTx(tx))
	if err != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return errors.Join(err, getDBFuncCRUDError("rollback transaction", rollbackErr))
		}

		return err
	}

	err = tx.Commit()
	if err != nil {
		return getDBFuncCRUDError("commit transaction", err)
	}

	return nil
}
