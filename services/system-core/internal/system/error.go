package system

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

// Error 是可返回给调用方的业务错误。
type Error struct {
	Code    string
	Message string
	HTTP    int
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func errCode(code, message string, http int) *Error {
	return &Error{Code: code, Message: message, HTTP: http}
}

// mapDBErr 把数据库层的典型冲突翻译成对外错误码：
// 唯一约束冲突（如重复 service_key）应为 409，而不是落到 500 INTERNAL。
func mapDBErr(err error, code, message string) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return errCode(code, message, 409)
	}
	return err
}

func IsCode(err error, code string) bool {
	if err == nil {
		return false
	}
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

func wrap(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(format+": %w", append(args, err)...)
}
