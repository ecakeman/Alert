package postgres

import (
	"github.com/jackc/pgx/v5"

	"github.com/ecakeman/alert/internal/application/port"
)

func txFrom(tx port.Tx) pgx.Tx { return tx.(pgx.Tx) }

// scannable 同时被 pgx.Row 与 pgx.Rows 满足。
type scannable interface {
	Scan(dest ...any) error
}
