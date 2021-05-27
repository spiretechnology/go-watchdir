package watchdir

import "os"

// FileSystem is an interface for the common file system functions needed by the watch directory
type FileSystem interface {

	// ReadDir receives a directory string, relative to the root of the file system. It then returns a listing of all
	// the base names contained in that directory (only the name of the file including extension, but not the directory
	// path leading up to it).
	ReadDir(dir string) ([]string, error)

	// Stat returns the file info for the file at the given path.
	Stat(path string) (os.FileInfo, error)
}

// DefaultFileSystem is an implementation of FileSystem that uses the machine's native OS/runtime file system
type DefaultFileSystem struct{}

// ReadDir lists the files in a directory on the file system
func (fs *DefaultFileSystem) ReadDir(dir string) ([]string, error) {
	file, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	list, err := file.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	err = file.Close()
	if err != nil {
		return nil, err
	}
	return list, nil
}

// Stat returns the file info for the file at the given path
func (fs *DefaultFileSystem) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
