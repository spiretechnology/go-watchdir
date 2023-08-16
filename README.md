# go-watchdir

A Go library for monitoring changes to files and folders.

## Installation

```sh
go get github.com/spiretechnology/go-watchdir/v2
```

## Example Usage

```go
wd := watchdir.New(os.DirFS("/path/to/dir"))

wd.Watch(ctx, watchdir.HandlerFunc(func(ctx context.Context, event watchdir.Event) {
    switch event.Type {
    case watchdir.FileAdded:
        fmt.Println("Added file: ", event.File)
    case watchdir.FileRemoved:
        fmt.Println("Removed file: ", event.File)
    }
}))
```

## How does it work?

This library polls the provided file system and all subdirectories recursively, then sleeps for a configurable amount of time, then repeats the process. File events are emitted to the provided handler.

## Contributing

Contributions are encouraged, particularly for optimizations, tests, and bug fixes. Please submit a PR if you want to contribute a change.
