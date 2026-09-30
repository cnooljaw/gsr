package main

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/runtime"
)

const (
	createProductCommand runtime.CommandID = iota + 1
	getProductCommand
	updateProductCommand
	deleteProductCommand
)

const (
	insertProductSQL = "INSERT INTO products (name, price_cents, stock) VALUES (?, ?, ?)"
	selectProductSQL = "SELECT id, name, price_cents, stock FROM products WHERE id = ?"
	updateProductSQL = "UPDATE products SET name = ?, price_cents = ?, stock = ? WHERE id = ?"
	deleteProductSQL = "DELETE FROM products WHERE id = ?"
	productExistsSQL = "SELECT 1 FROM products WHERE id = ?"
)

var (
	errProductNotFound         = runtime.NewStatusError(runtime.ErrorClassDomain, "product.not_found", "product not found")
	errProductStoreUnavailable = runtime.NewStatusError(
		runtime.ErrorClassDomain, "product.store_unavailable", "product storage is unavailable",
	)
)

// product 是示例中的产品快照；价格用分表示，避免浮点舍入。
type product struct {
	ID         int64
	Name       string
	PriceCents int64
	Stock      int64
}

// productService 独占产品命令的执行路径，直接借用应用管理的数据库连接池。
type productService struct {
	dispatcher *gsr.Dispatcher
	db         *sql.DB
	timeout    time.Duration
}

func newProductService(db *sql.DB, timeout time.Duration) (*productService, error) {
	if db == nil || timeout <= 0 {
		return nil, errors.New("product service requires a database and finite timeout")
	}
	service := &productService{db: db, timeout: timeout}
	service.dispatcher = gsr.MustDispatcher(map[runtime.CommandID]any{
		createProductCommand: service.handleCreate,
		getProductCommand:    service.handleGet,
		updateProductCommand: service.handleUpdate,
		deleteProductCommand: service.handleDelete,
	})
	return service, nil
}

// Handle 是产品 Service 的命令入口，具体命令由 dispatcher 分发。
func (service *productService) Handle(ctx gsr.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	return service.dispatcher.Handle(ctx, command, args...)
}

func (service *productService) OnStart(_ context.Context, _ gsr.ServiceContext) error {
	return nil
}

// handleCreate 校验产品详情后写入数据库；命令参数类型由 Dispatcher 校验。
func (service *productService) handleCreate(ctx gsr.CommandContext, value product) (product, error) {
	if value.ID != 0 || !validProductDetails(value) {
		return product{}, runtime.ErrInvalidCommandArguments
	}
	created, err := service.create(value)
	if err != nil {
		return product{}, service.storeFailure(ctx, "create", err)
	}
	return created, nil
}

// handleGet 保留“未找到”和数据库故障的不同业务结果。
func (service *productService) handleGet(ctx gsr.CommandContext, id int64) (product, error) {
	if id <= 0 {
		return product{}, runtime.ErrInvalidCommandArguments
	}
	found, err := service.get(id)
	if errors.Is(err, sql.ErrNoRows) {
		return product{}, errProductNotFound
	}
	if err != nil {
		return product{}, service.storeFailure(ctx, "get", err)
	}
	return found, nil
}

// handleUpdate 校验业务字段，并区分相同值更新与产品不存在。
func (service *productService) handleUpdate(ctx gsr.CommandContext, value product) error {
	if value.ID <= 0 || !validProductDetails(value) {
		return runtime.ErrInvalidCommandArguments
	}
	err := service.update(value)
	if errors.Is(err, sql.ErrNoRows) {
		return errProductNotFound
	}
	if err != nil {
		return service.storeFailure(ctx, "update", err)
	}
	return nil
}

// handleDelete 只接受有效 ID，并保留产品不存在的业务错误。
func (service *productService) handleDelete(ctx gsr.CommandContext, id int64) error {
	if id <= 0 {
		return runtime.ErrInvalidCommandArguments
	}
	err := service.delete(id)
	if errors.Is(err, sql.ErrNoRows) {
		return errProductNotFound
	}
	if err != nil {
		return service.storeFailure(ctx, "delete", err)
	}
	return nil
}

// OnStop 不关闭借用的数据库连接池；应用在 Runtime 收尾后负责关闭它。
func (service *productService) OnStop(_ context.Context) error {
	return nil
}

func (service *productService) create(value product) (product, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.timeout)
	defer cancel()
	result, err := service.db.ExecContext(ctx, insertProductSQL, value.Name, value.PriceCents, value.Stock)
	if err != nil {
		return product{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return product{}, err
	}
	if id <= 0 {
		return product{}, errors.New("product insert returned an invalid ID")
	}
	value.ID = id
	return value, nil
}

func (service *productService) get(id int64) (product, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.timeout)
	defer cancel()
	var value product
	err := service.db.QueryRowContext(ctx, selectProductSQL, id).
		Scan(&value.ID, &value.Name, &value.PriceCents, &value.Stock)
	return value, err
}

func (service *productService) update(value product) error {
	ctx, cancel := context.WithTimeout(context.Background(), service.timeout)
	defer cancel()
	result, err := service.db.ExecContext(
		ctx, updateProductSQL, value.Name, value.PriceCents, value.Stock, value.ID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}
	// MySQL 默认只计实际变更行数；零行还可能是对已有产品写入相同值。
	var exists int
	err = service.db.QueryRowContext(ctx, productExistsSQL, value.ID).Scan(&exists)
	return err
}

func (service *productService) delete(id int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), service.timeout)
	defer cancel()
	result, err := service.db.ExecContext(ctx, deleteProductSQL, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (service *productService) storeFailure(ctx gsr.CommandContext, operation string, err error) error {
	if ctx != nil {
		_ = ctx.Logger().Error("product database operation failed", "operation", operation, "error", err.Error())
	}
	return errProductStoreUnavailable
}

func validProductDetails(value product) bool {
	return strings.TrimSpace(value.Name) != "" && utf8.ValidString(value.Name) &&
		utf8.RuneCountInString(value.Name) <= 100 && value.PriceCents >= 0 && value.Stock >= 0
}
