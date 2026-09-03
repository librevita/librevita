package database

import (
	"context"

	"github.com/cockroachdb/errors"
	"librevita.org/internal/database/record"
)

// WithTx executes the given operation inside an ent database transaction.
// Automatically commits on success, and safely rolls back if an error occurs or a panic is raised.
func WithTx(ctx context.Context, client *record.Client, fn func(tx *record.Tx) error) error {
	tx, err := client.Tx(ctx)
	if err != nil {
		return errors.Wrap(err, "database: begin transaction")
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rerr := tx.Rollback(); rerr != nil {
			return errors.CombineErrors(err, errors.Wrap(rerr, "database: rollback transaction"))
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "database: commit transaction")
	}
	return nil
}
