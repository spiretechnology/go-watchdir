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
    Dir:         "/path/to/dir",
    MaxDepth:    10,
    PollTimeout: time.Second * 5,
}

// Make a stop channel so we can end the watch dir
chanStop := make(chan bool)

// Start watching the directory. Returns a channel for reading file events,
// and another channel for reading error messages
chanFiles, chanError := wd.Watch(chanStop)

for {
    select {
    case err := <-chanError:
        fmt.Println("Error: ", err)
    case event := <-chanFiles:
        switch event.Operation {
        case watchdir.Add:
            fmt.Println("Added file: ", event.File.Path)
        case watchdir.Remove:
            fmt.Println("Removed file: ", event.File.Path)
        }
    }
}
```

## How does it work?

The `watchdir.WatchDir` struct polls the directory and all subdirectories recursively, then sleeps for an amount of time specified by the `PollTimeout` option, then repeats the process.

Recursive polling is not ideal for performance and memory usage, but one of the most common use cases for this library is monitoring network drives, which necessitates polling.

## Contributing

Contributions are encouraged, particularly optimizations, tests, and bug fixes. Please submit a PR if you want to contribute a change.
