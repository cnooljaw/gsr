package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/runtime"
)

// productMainService 演示从普通业务入口创建 Service，并只通过 Command 执行产品 CRUD。
type productMainService struct {
	db       *sql.DB
	output   io.Writer
	finished context.CancelFunc
}

var _ gsr.Service = (*productMainService)(nil)

func (service *productMainService) OnStart(ctx context.Context, self gsr.ServiceContext) error {
	products, err := newProductService(service.db, 500*time.Millisecond)
	if err != nil {
		return err
	}
	id, err := self.CreateService(ctx, products)
	if err != nil {
		return err
	}
	value, err := self.Call(ctx, id, createProductCommand, product{Name: "药水", PriceCents: 199, Stock: 10})
	if err != nil {
		return err
	}
	created, ok := value.(product)
	if !ok || created.ID <= 0 {
		return errors.New("example received an invalid created product")
	}
	value, err = self.Call(ctx, id, getProductCommand, created.ID)
	if err != nil {
		return fmt.Errorf("read created product: %w", err)
	}
	if value != created {
		return fmt.Errorf("read created product = %+v; want %+v", value, created)
	}
	updated := created
	updated.Name = "高级药水"
	updated.PriceCents = 299
	updated.Stock = 8
	if _, err := self.Call(ctx, id, updateProductCommand, updated); err != nil {
		return err
	}
	value, err = self.Call(ctx, id, getProductCommand, created.ID)
	if err != nil {
		return fmt.Errorf("read updated product: %w", err)
	}
	if value != updated {
		return fmt.Errorf("read updated product = %+v; want %+v", value, updated)
	}
	if _, err := self.Call(ctx, id, deleteProductCommand, created.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(service.output, "product %d: create/read/update/delete ok\n", created.ID); err != nil {
		return err
	}
	self.Exit()
	return nil
}

func (service *productMainService) Handle(_ gsr.CommandContext, _ runtime.CommandID, _ ...any) (any, error) {
	return nil, runtime.ErrUnknownCommand
}

func (service *productMainService) OnStop(_ context.Context) error {
	service.finished()
	return nil
}
