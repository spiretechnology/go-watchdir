package main

import (
	"fmt"
	"github.com/spiretechnology/watchdir"
	"os"
	"path/filepath"
	"time"
)

func main() {

	homedir, _ := os.UserHomeDir()
	wd := watchdir.WatchDir{
		Dir: filepath.Join(homedir, "Desktop"),
		MaxDepth: 10,
		PollTimeout: time.Second * 5,
	}
	chanStop := make(chan bool)
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
				fmt.Println("Removed file: ", event.File.Path, event.File.Info.Size())
			}
		}
	}

}
