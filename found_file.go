package watchdir

import "os"

// FoundFile represents a file found within a watch directory
type FoundFile struct {
	// Path is the relative path to the file within the file system
	Path string
	// Info is the stat info about the file
	Info os.FileInfo
}
