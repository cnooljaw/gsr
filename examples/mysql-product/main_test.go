package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestProductExampleRunsThroughService 验证入口通过业务 Service 完成产品 CRUD，成功路径不再故意查询已删除的记录。
func TestProductExampleRunsThroughService(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectExec("INSERT INTO products (name, price_cents, stock) VALUES (?, ?, ?)").
		WithArgs("药水", int64(199), int64(10)).WillReturnResult(sqlmock.NewResult(42, 1))
	mock.ExpectQuery("SELECT id, name, price_cents, stock FROM products WHERE id = ?").
		WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "price_cents", "stock"}).
		AddRow(int64(42), "药水", int64(199), int64(10)))
	mock.ExpectExec("UPDATE products SET name = ?, price_cents = ?, stock = ? WHERE id = ?").
		WithArgs("高级药水", int64(299), int64(8), int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT id, name, price_cents, stock FROM products WHERE id = ?").
		WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "price_cents", "stock"}).
		AddRow(int64(42), "高级药水", int64(299), int64(8)))
	mock.ExpectExec("DELETE FROM products WHERE id = ?").
		WithArgs(int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var output bytes.Buffer
	if err := runProductDemo(ctx, db, &output); err != nil {
		t.Fatalf("runProductDemo() error = %v", err)
	}
	if got := output.String(); got != "product 42: create/read/update/delete ok\n" {
		t.Fatalf("output = %q; want CRUD confirmation", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestProductExampleRejectsMissingConfiguration 验证示例入口在连接数据库前拒绝缺失 DSN 和不完整依赖。
func TestProductExampleRejectsMissingConfiguration(t *testing.T) {
	var output bytes.Buffer
	if err := run(context.Background(), "", &output); err == nil {
		t.Fatal("run(empty DSN) error = nil; want configuration error")
	}
	if err := run(context.Background(), "user:pass@tcp(127.0.0.1:3306)/", &output); err == nil || !strings.Contains(err.Error(), "database name") {
		t.Fatalf("run(DSN without database) error = %v; want database name error", err)
	}
	if err := runProductDemo(context.Background(), nil, &output); err == nil {
		t.Fatal("runProductDemo(nil database) error = nil; want invalid dependency")
	}
}
