package main

import (
	"context"
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// 这是有限演示的完成通知；服务器通常在宿主等待进程退出信号。
	entry := &MainService{output: output, finished: cancel}
	return gsr.Run(ctx, gsr.Config{Logging: logging.Config{Writer: io.Discard}}, entry)
}
