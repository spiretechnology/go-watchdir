package watchdir

import (
	"path"
	"strings"
)

// normalizePath removes leading and trailing slashes from a path, and returns an empty string if the path is ".".
func normalizePath(name string) string {
	trimmed := path.Clean(name)
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "." {
		trimmed = ""
	}
	return trimmed
}
