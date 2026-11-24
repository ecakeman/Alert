package postgres

import (
	"github.com/jackc/pgx/v5"

	"github.com/ecakeman/alert/internal/application/port"
)

func txFrom(tx port.Tx) pgx.Tx { return tx.(pgx.Tx) }
