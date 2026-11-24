package postgres

import (
	"github.com/jackc/pgx/v5"

	"github.com/ecakeman/alert/internal/application/port"
	"github.com/ecakeman/alert/internal/infrastructure/persistence/postgres/model"
)

func txFrom(tx port.Tx) pgx.Tx { return tx.(pgx.Tx) }

// scannable 已移交 postgres/model（Scannable）；行模型的 Scan/ToApp/FromApp 见 model 包。

// scannable 过渡别名（S1.8 清理）：同包其余仓储仍引用，迁移完成后移除。
type scannable = model.Scannable
