package main

import (
	"fmt"
	"log"
	"os"
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
