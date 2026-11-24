package postgres

import (
	"github.com/jackc/pgx/v5"

	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
)

func txFrom(tx portshared.Tx) pgx.Tx { return tx.(pgx.Tx) }
