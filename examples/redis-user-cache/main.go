package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/redisclient"
)

func main() {
	address := flag.String("redis-addr", "127.0.0.1:6379", "Redis 地址")
	flag.Parse()
	if err := run(context.Background(), *address, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, address string, output io.Writer) (result error) {
	if ctx == nil || address == "" || output == nil {
		return errors.New("example requires context, Redis address and output")
	}
	client, err := redisclient.New(address)
	if err != nil {
		return err
	}
	defer func() {
		result = errors.Join(result, client.Close())
	}()
	cache, err := newUserHashCache(client, time.Minute, 300*time.Millisecond)
	if err != nil {
		return err
	}
	runCtx, finished := context.WithCancel(ctx)
	defer finished()
	entry := &mainService{cache: cache, output: output, finished: finished}
	return gsr.Run(runCtx, gsr.Config{Logging: logging.Config{Writer: io.Discard}}, entry)
}
