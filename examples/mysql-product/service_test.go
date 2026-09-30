package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

// TestProductServiceCRUD 验证 MainService 只通过 Command 调用产品 Service，数据库增删改查均在目标 Handler 中执行。
func TestProductServiceCRUD(t *testing.T) {
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
	mock.ExpectExec("DELETE FROM products WHERE id = ?").
		WithArgs(int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT id, name, price_cents, stock FROM products WHERE id = ?").
		WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "price_cents", "stock"}))

	err = runProductScenario(db, time.Second, func(ctx context.Context, self gsr.ServiceContext, id runtime.ServiceID) error {
		value, err := self.Call(ctx, id, createProductCommand, product{Name: "药水", PriceCents: 199, Stock: 10})
		created, ok := value.(product)
		if err != nil || !ok || created != (product{ID: 42, Name: "药水", PriceCents: 199, Stock: 10}) {
			return fmt.Errorf("Call(create) = %+v, %v; want product 42", value, err)
		}
		value, err = self.Call(ctx, id, getProductCommand, created.ID)
		if err != nil || value != created {
			return fmt.Errorf("Call(get) = %+v, %v; want %+v", value, err, created)
		}
		updated := product{ID: 42, Name: "高级药水", PriceCents: 299, Stock: 8}
		if value, err := self.Call(ctx, id, updateProductCommand, updated); err != nil || value != nil {
			return fmt.Errorf("Call(update) = %v, %v; want nil, nil", value, err)
		}
		if value, err := self.Call(ctx, id, deleteProductCommand, updated.ID); err != nil || value != nil {
			return fmt.Errorf("Call(delete) = %v, %v; want nil, nil", value, err)
		}
		if value, err := self.Call(ctx, id, getProductCommand, updated.ID); value != nil || !errors.Is(err, errProductNotFound) {
			return fmt.Errorf("Call(get deleted) = %v, %v; want not found", value, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestProductServiceDistinguishesUnchangedAndMissing 验证相同内容的更新成功，真正不存在的产品才返回未找到。
func TestProductServiceDistinguishesUnchangedAndMissing(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	value := product{ID: 42, Name: "药水", PriceCents: 199, Stock: 10}
	for _, exists := range []bool{true, false} {
		mock.ExpectExec("UPDATE products SET name = ?, price_cents = ?, stock = ? WHERE id = ?").
			WithArgs(value.Name, value.PriceCents, value.Stock, value.ID).
			WillReturnResult(sqlmock.NewResult(0, 0))
		rows := sqlmock.NewRows([]string{"1"})
		if exists {
			rows.AddRow(1)
		}
		mock.ExpectQuery("SELECT 1 FROM products WHERE id = ?").WithArgs(value.ID).WillReturnRows(rows)
	}
	mock.ExpectExec("DELETE FROM products WHERE id = ?").
		WithArgs(value.ID).WillReturnResult(sqlmock.NewResult(0, 0))
	err = runProductScenario(db, time.Second, func(ctx context.Context, self gsr.ServiceContext, id runtime.ServiceID) error {
		if _, err := self.Call(ctx, id, updateProductCommand, value); err != nil {
			return fmt.Errorf("Call(update unchanged): %w", err)
		}
		if _, err := self.Call(ctx, id, updateProductCommand, value); !errors.Is(err, errProductNotFound) {
			return fmt.Errorf("Call(update missing) error = %v; want not found", err)
		}
		if _, err := self.Call(ctx, id, deleteProductCommand, value.ID); !errors.Is(err, errProductNotFound) {
			return fmt.Errorf("Call(delete missing) error = %v; want not found", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestProductServiceMasksDatabaseFailure 验证数据库内部错误不会通过业务 Call 结果泄露给调用者。
func TestProductServiceMasksDatabaseFailure(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery("SELECT id, name, price_cents, stock FROM products WHERE id = ?").
		WithArgs(int64(42)).WillReturnError(errors.New("internal database detail"))
	err = runProductScenario(db, time.Second, func(ctx context.Context, self gsr.ServiceContext, id runtime.ServiceID) error {
		value, callErr := self.Call(ctx, id, getProductCommand, int64(42))
		if value != nil || !errors.Is(callErr, errProductStoreUnavailable) {
			return fmt.Errorf("Call(get on database failure) = %v, %v; want store unavailable", value, callErr)
		}
		if strings.Contains(callErr.Error(), "internal database detail") {
			return fmt.Errorf("Call(get) exposed database error: %v", callErr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestProductServiceRecoversAfterSQLTimeout 验证一次有界 SQL 等待失败后，同一 Service 仍能处理后续查询。
func TestProductServiceRecoversAfterSQLTimeout(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery("SELECT id, name, price_cents, stock FROM products WHERE id = ?").
		WithArgs(int64(42)).WillDelayFor(200 * time.Millisecond).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "price_cents", "stock"}).
			AddRow(int64(42), "药水", int64(199), int64(10)))
	mock.ExpectQuery("SELECT id, name, price_cents, stock FROM products WHERE id = ?").
		WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "price_cents", "stock"}).
		AddRow(int64(42), "药水", int64(199), int64(10)))
	err = runProductScenario(db, 50*time.Millisecond, func(ctx context.Context, self gsr.ServiceContext, id runtime.ServiceID) error {
		if value, err := self.Call(ctx, id, getProductCommand, int64(42)); value != nil || !errors.Is(err, errProductStoreUnavailable) {
			return fmt.Errorf("Call(get during SQL delay) = %v, %v; want store unavailable", value, err)
		}
		value, err := self.Call(ctx, id, getProductCommand, int64(42))
		if err != nil || value != (product{ID: 42, Name: "药水", PriceCents: 199, Stock: 10}) {
			return fmt.Errorf("Call(get after timeout) = %+v, %v; want product 42", value, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestProductServiceRejectsInvalidCommands 验证非法产品、ID 和未知命令在访问数据库前被拒绝。
func TestProductServiceRejectsInvalidCommands(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	service, err := newProductService(db, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	current := productTestCommandContext{}
	for _, check := range []struct {
		command runtime.CommandID
		args    []any
		want    error
	}{
		{createProductCommand, []any{product{ID: 1, Name: "药水", PriceCents: 1}}, runtime.ErrInvalidCommandArguments},
		{createProductCommand, []any{product{Name: " ", PriceCents: 1}}, runtime.ErrInvalidCommandArguments},
		{createProductCommand, []any{product{Name: "药水", PriceCents: -1}}, runtime.ErrInvalidCommandArguments},
		{updateProductCommand, []any{product{Name: "药水", PriceCents: 1}}, runtime.ErrInvalidCommandArguments},
		{getProductCommand, []any{int64(0)}, runtime.ErrInvalidCommandArguments},
		{deleteProductCommand, []any{"42"}, runtime.ErrInvalidCommandArguments},
		{runtime.CommandID(99), nil, runtime.ErrUnknownCommand},
	} {
		if _, err := service.Handle(current, check.command, check.args...); !errors.Is(err, check.want) {
			t.Fatalf("Handle(%d, %v) error = %v; want %v", check.command, check.args, err, check.want)
		}
	}
}

type productTestCommandContext struct{ gsr.CommandContext }

// TestProductServiceRequiresBoundedDatabase 验证产品 Service 拒绝空数据库和无限等待配置。
func TestProductServiceRequiresBoundedDatabase(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, check := range []struct {
		db      *sql.DB
		timeout time.Duration
	}{
		{nil, time.Second},
		{db, 0},
		{db, -time.Second},
	} {
		if _, err := newProductService(check.db, check.timeout); err == nil {
			t.Fatalf("newProductService(%v, %v) error = nil; want invalid config", check.db, check.timeout)
		}
	}
}

func runProductScenario(db *sql.DB, timeout time.Duration, check func(context.Context, gsr.ServiceContext, runtime.ServiceID) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	entry := &productTestMainService{db: db, timeout: timeout, check: check, finished: cancel}
	return gsr.Run(ctx, gsr.Config{Logging: logging.Config{Writer: io.Discard}}, entry)
}

type productTestMainService struct {
	db       *sql.DB
	timeout  time.Duration
	check    func(context.Context, gsr.ServiceContext, runtime.ServiceID) error
	finished context.CancelFunc
}

func (service *productTestMainService) OnStart(ctx context.Context, self gsr.ServiceContext) error {
	products, err := newProductService(service.db, service.timeout)
	if err != nil {
		return err
	}
	id, err := self.CreateService(ctx, products)
	if err != nil {
		return err
	}
	if err := service.check(ctx, self, id); err != nil {
		return err
	}
	self.Exit()
	return nil
}

func (service *productTestMainService) Handle(_ gsr.CommandContext, _ runtime.CommandID, _ ...any) (any, error) {
	return nil, runtime.ErrUnknownCommand
}

func (service *productTestMainService) OnStop(_ context.Context) error {
	service.finished()
	return nil
}
