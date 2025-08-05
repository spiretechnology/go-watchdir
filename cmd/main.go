package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/spiretechnology/go-watchdir/v2"
	"golang.org/x/sync/errgroup"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalln("Please provide a watch directory argument")
	}

	dir := os.Args[1]

	// Create a context that is cancelled on SIGINT/SIGTERM (Ctrl+C)
	ctx := context.Background()
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	wd := watchdir.New(
		os.DirFS(dir),
		watchdir.WithWriteStabilityThreshold(time.Second),
		watchdir.WithFileFilter(watchdir.FilterFunc(func(ctx context.Context, filename string) (bool, error) {
			if strings.HasPrefix(path.Base(filename), ".") {
				return false, nil
			}
			return true, nil
		})),
	)
	eg, ctx := errgroup.WithContext(ctx)

	chanEvents := make(chan watchdir.Event)
	eg.Go(func() error {
		defer close(chanEvents)
		return watchdir.Watch(ctx, wd, 0, chanEvents)
	})
	eg.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case event, ok := <-chanEvents:
				if !ok {
					return nil // Channel closed
				}
				switch event.Type {
				case watchdir.FileAdded:
					log.Printf("[+] %s\n", event.File)
				case watchdir.FileRemoved:
					log.Printf("[-] %s\n", event.File)
				}
			}
		}
	})
	if err := eg.Wait(); err != nil {
		log.Fatal(err)
	}
}
