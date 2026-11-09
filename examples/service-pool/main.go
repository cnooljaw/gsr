package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/logging"
)

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(output io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	entry := newMainService(output, cancel)
	if err := gsr.Run(ctx, gsr.Config{Logging: logging.Config{Writer: io.Discard}}, entry); err != nil {
		return err
	}
	// Run 已等待 Service 收尾，宿主此时才读取演示结果。
	if entry.received != 2 {
		return errors.Join(entry.result, ctx.Err())
	}
	return entry.result
}
