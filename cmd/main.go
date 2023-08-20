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
		watchdir.WithPollInterval(0),
		watchdir.WithWriteStabilityThreshold(time.Second),
		watchdir.WithFileFilter(watchdir.FilterFunc(func(ctx context.Context, filename string) (bool, error) {
			if strings.HasPrefix(path.Base(filename), ".") {
				return false, nil
			}
			return true, nil
		})),
	)
	err := wd.Watch(ctx, watchdir.HandlerFunc(func(ctx context.Context, event watchdir.Event) error {
		switch event.Type {
		case watchdir.FileAdded:
			log.Printf("[+] %s\n", event.File)
		case watchdir.FileRemoved:
			log.Printf("[-] %s\n", event.File)
		}
		return nil
	}))
	if err != nil {
		log.Fatal(err)
	}
}
