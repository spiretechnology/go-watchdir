package watchdir

import (
	"fmt"
	"log"
)

// log prints a log to the logger
func (wd *WatchDir) log(level LogLevel, args ...interface{}) {

	// Get the minimum level that gets logged
	minLevel := wd.LogLevel
	if minLevel == 0 {
		minLevel = WARN
	}

	// If the log level is below the threshold
	if level < minLevel {
		return
	}

	// Get the logger to log into
	logger := wd.Logger
	if logger == nil {
		logger = log.Default()
	}

	// Send the output to the logger
	logger.Println(args...)

}

// logf prints a log with a format string and args
func (wd *WatchDir) logf(level LogLevel, format string, args ...interface{}) {
	wd.log(level, fmt.Sprintf(format, args...))
}
