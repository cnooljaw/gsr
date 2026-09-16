package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cnooljaw/gsr/runtime"
)

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(output io.Writer) (result error) {
	local, err := runtime.NewRuntime(runtime.Config{})
	if err != nil {
		return err
	}
	defer func() {
		result = errors.Join(result, local.Close())
	}()
	if err := local.Start(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	entry := &MainService{output: output, done: make(chan struct{})}
	if _, err := local.CreateService(ctx, entry); err != nil {
		return err
	}
	// 这是有限演示的完成通知；服务器通常在宿主等待进程退出信号。
	select {
	case <-entry.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
