# go-watchdir

A Go library for monitoring changes to files and folders.

## Installation

```sh
go get github.com/spiretechnology/go-watchdir
```

## Example Usage

```go
// Create a watch directory
wd := watchdir.WatchDir{
    FS:         os.DirFS("/path/to/dir"),
    MaxDepth:    10,
    PollTimeout: time.Second * 5,
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
```

## How does it work?

The `watchdir.WatchDir` struct polls the directory and all subdirectories recursively, then sleeps for an amount of time specified by the `PollTimeout` option, then repeats the process.

Recursive polling is not ideal for performance and memory usage, but one of the most common use cases for this library is monitoring network drives, which necessitates polling.

## Contributing

Contributions are encouraged, particularly optimizations, tests, and bug fixes. Please submit a PR if you want to contribute a change.
