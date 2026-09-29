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
	"github.com/redis/go-redis/v9"
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
	client := redis.NewClient(&redis.Options{
		Addr:                  address,
		Protocol:              2,
		DisableIdentity:       true,
		ContextTimeoutEnabled: true,
		MaxRetries:            -1,
		DialTimeout:           time.Second,
		ReadTimeout:           time.Second,
		WriteTimeout:          time.Second,
	})
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
