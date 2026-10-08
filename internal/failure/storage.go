package failure

import (
	"context"
	"database/sql/driver"
	"errors"
	"net"

	"github.com/go-sql-driver/mysql"
)

func Storage(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return Wrap(op, err)
	}
	var n net.Error
	var m *mysql.MySQLError
	temporary := errors.Is(err, driver.ErrBadConn) || errors.Is(err, mysql.ErrInvalidConn) || errors.Is(err, context.DeadlineExceeded) || errors.As(err, &n)
	// 以下 MySQL 错误码表示连接、锁等待或死锁等可重试情形。
	if errors.As(err, &m) {
		switch m.Number {
		case 1040, 1053, 1205, 1213, 2006, 2013:
			temporary = true
		}
	}
	if temporary {
		return &Error{Code: Unavailable, Message: "服务暂不可用，请稍后重试。", Op: op, Retryable: true, Cause: err}
	}
	return Wrap(op, err)
}
