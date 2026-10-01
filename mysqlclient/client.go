package mysqlclient

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Open 建立有限等待的 MySQL 连接池并检查首次连接。
// 返回的连接池由调用方在 Runtime 停止后关闭；需要自定义池参数时可继续使用 database/sql。
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	if ctx == nil || strings.TrimSpace(dsn) == "" {
		return nil, errors.New("mysqlclient: context and DSN are required")
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil || config.DBName == "" {
		return nil, errors.New("mysqlclient: valid DSN with database name is required")
	}
	if config.Timeout < 0 || config.ReadTimeout < 0 || config.WriteTimeout < 0 {
		return nil, errors.New("mysqlclient: connection timeouts must be finite")
	}
	if config.Timeout == 0 {
		config.Timeout = 2 * time.Second
	}
	if config.ReadTimeout == 0 {
		config.ReadTimeout = 2 * time.Second
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 2 * time.Second
	}
	connector, err := mysql.NewConnector(config)
	if err != nil {
		return nil, fmt.Errorf("mysqlclient: configure connector: %w", err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(3 * time.Minute)
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return nil, errors.Join(fmt.Errorf("mysqlclient: connect: %w", err), db.Close())
	}
	return db, nil
}
