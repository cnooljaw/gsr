package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/logging"
	"github.com/go-sql-driver/mysql"
)

func main() {
	if err := run(context.Background(), os.Getenv("GSR_MYSQL_DSN"), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, dsn string, output io.Writer) (result error) {
	if ctx == nil || strings.TrimSpace(dsn) == "" || output == nil {
		return errors.New("example requires context, GSR_MYSQL_DSN and output")
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil || config.DBName == "" {
		return errors.New("example requires a valid MySQL DSN with a database name")
	}
	config.Timeout = 2 * time.Second
	config.ReadTimeout = 2 * time.Second
	config.WriteTimeout = 2 * time.Second
	connector, err := mysql.NewConnector(config)
	if err != nil {
		return fmt.Errorf("configure MySQL connector: %w", err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(3 * time.Minute)
	defer func() {
		result = errors.Join(result, db.Close())
	}()
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return fmt.Errorf("connect to MySQL: %w", err)
	}
	return runProductDemo(ctx, db, output)
}

func runProductDemo(ctx context.Context, db *sql.DB, output io.Writer) error {
	if ctx == nil || db == nil || output == nil {
		return errors.New("product demo requires context, database and output")
	}
	runCtx, finished := context.WithCancel(ctx)
	defer finished()
	entry := &productMainService{db: db, output: output, finished: finished}
	return gsr.Run(runCtx, gsr.Config{Logging: logging.Config{Writer: os.Stderr}}, entry)
}
