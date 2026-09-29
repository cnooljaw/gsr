package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/runtime"
	"github.com/go-sql-driver/mysql"
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

// TestProductExampleLiveMySQL 验证专用本机测试账号能完成产品 CRUD，并区分相同值更新与不存在的产品。
func TestProductExampleLiveMySQL(t *testing.T) {
	if os.Getenv("GSR_MYSQL_INTEGRATION") != "1" {
		t.Skip("set GSR_MYSQL_INTEGRATION=1 to use the dedicated local MySQL database")
	}
	dsn := os.Getenv("GSR_MYSQL_DSN")
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse test DSN: %v", err)
	}
	if config.User != "gsr_demo" || config.Net != "tcp" || config.Addr != "127.0.0.1:3306" ||
		config.DBName != "gsr_product_demo" {
		t.Fatal("live test requires gsr_demo on the dedicated local gsr_product_demo database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var output bytes.Buffer
	if err := run(ctx, dsn, &output); err != nil {
		t.Fatalf("run(live MySQL) error = %v", err)
	}
	if !strings.Contains(output.String(), "create/read/update/delete ok\n") {
		t.Fatalf("output = %q; want CRUD confirmation", output.String())
	}
	connector, err := mysql.NewConnector(config)
	if err != nil {
		t.Fatalf("configure live MySQL connector: %v", err)
	}
	db := sql.OpenDB(connector)
	t.Cleanup(func() { _ = db.Close() })
	err = runProductScenario(db, time.Second, func(ctx context.Context, self gsr.ServiceContext, id runtime.ServiceID) (result error) {
		value, err := self.Call(ctx, id, createProductCommand, product{Name: "原样更新测试", PriceCents: 100, Stock: 1})
		if err != nil {
			return fmt.Errorf("create unchanged-update fixture: %w", err)
		}
		created, ok := value.(product)
		if !ok || created.ID <= 0 {
			return fmt.Errorf("created product = %v; want valid ID", value)
		}
		deleted := false
		// 断言中途失败时，仍尽量通过同一 Service 删除测试创建的记录。
		defer func() {
			if !deleted {
				_, cleanupErr := self.Call(ctx, id, deleteProductCommand, created.ID)
				result = errors.Join(result, cleanupErr)
			}
		}()
		if _, err := self.Call(ctx, id, updateProductCommand, created); err != nil {
			return fmt.Errorf("update unchanged product: %w", err)
		}
		if _, err := self.Call(ctx, id, deleteProductCommand, created.ID); err != nil {
			return fmt.Errorf("delete unchanged-update fixture: %w", err)
		}
		deleted = true
		if _, err := self.Call(ctx, id, updateProductCommand, created); !errors.Is(err, errProductNotFound) {
			return fmt.Errorf("update deleted product = %v; want not found", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
