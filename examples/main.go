package main

import (
	"fmt"
	"github.com/spiretechnology/watchdir"
	"time"
)

func main() {

	wd := watchdir.WatchDir{
		Dir: "/Users/conner/Desktop",
		MaxDepth: 10,
		PollTimeout: time.Second * 30,
	}
	chanStop := make(chan bool)
	chanFiles, chanError := wd.Watch(chanStop)

	for {
		select {
		case err := <-chanError:
			fmt.Println("Error: ", err)
		case file := <-chanFiles:
			switch file.Operation {
			case watchdir.Add:
				fmt.Println("Added file: ", file.File.Path)
			case watchdir.Remove:
				fmt.Println("Removed file: ", file.File.Path)
			}
		}
	}

}
