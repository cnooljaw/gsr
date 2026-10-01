package mysqlclient

import (
	"context"
	"testing"
)

// TestOpenRejectsInvalidConfiguration 验证无效配置在建立数据库连接前被拒绝。
func TestOpenRejectsInvalidConfiguration(t *testing.T) {
	for _, test := range []struct {
		name string
		ctx  context.Context
		dsn  string
	}{
		{name: "missing context", dsn: "user@tcp(127.0.0.1:3306)/demo"},
		{name: "missing DSN", ctx: context.Background()},
		{name: "missing database", ctx: context.Background(), dsn: "user@tcp(127.0.0.1:3306)/"},
		{name: "invalid DSN", ctx: context.Background(), dsn: "not-a-dsn"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if db, err := Open(test.ctx, test.dsn); err == nil {
				if db != nil {
					db.Close()
				}
				t.Fatal("Open() succeeded with invalid configuration")
			}
		})
	}
}
