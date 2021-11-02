package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spiretechnology/go-watchdir"
)

func main() {

	if len(os.Args) < 2 {
		log.Fatalln("Please provide a watch directory argument")
	}

	dir := os.Args[1]

	wd := watchdir.WatchDir{
		Dir:         dir,
		MaxDepth:    10,
		PollTimeout: time.Second * 5,
		//Buffering: true,
		//BufferingSorter: func(a, b *watchdir.FoundFile) bool {
		//	return a.Info.ModTime().After(b.Info.ModTime())
		//},
	}

	// Create a context that is cancelled on SIGINT/SIGTERM (Ctrl+C)
	ctx := context.Background()
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Channel that will receive all the watch directory events
	chanFiles := make(chan watchdir.Event)

	// In the background, watch for file changes
	go func() {
		err := wd.Watch(ctx, chanFiles)
		if err != nil && err != context.Canceled {
			panic(err)
		}
		close(chanFiles)
	}()

	// In the foreground, handle all the file events
	for event := range chanFiles {
		switch event.Operation {
		case watchdir.Add:
			fmt.Println("Added file: ", event.File.Path)
		case watchdir.Remove:
			fmt.Println("Removed file: ", event.File.Path, event.File.Info.Size())
		}
	}

}
