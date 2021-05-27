package watchdir

import "os"

// FoundFile represents a file found within a watch directory
type FoundFile struct {
	// Dir is the root directory being watched
	Dir string
	// Path is the absolute path to the found file
	Path string
	// Info is the stat info about the file
	Info os.FileInfo
}
