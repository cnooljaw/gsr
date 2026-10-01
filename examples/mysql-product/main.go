package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/mysqlclient"
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
	db, err := mysqlclient.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer func() {
		result = errors.Join(result, db.Close())
	}()
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
