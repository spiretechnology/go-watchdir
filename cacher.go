package watchdir

// Cacher handles caching of which files in a watch directory have been indexed already
type Cacher interface {
	// Found is called when a file has been found in the watch directory. It is up to the Found implementation
	// to return true if the file is new, or false if the file has been indexed already. If an error is returned
	// the file will be skipped, and the error will be logged.
	Found(file *FoundFile) (bool, error)

	// Cleanup performs final cleanup when the directory watcher is shut down. This gives the cacher an opportunity
	// to save its work, or whatever else is needed.
	Cleanup() error
}
