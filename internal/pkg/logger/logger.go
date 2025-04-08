package logger

import (
	"fmt"
	"log"
	"os"
	"sync"
)

// Logger defines the interface for logging messages.
type Logger interface {
	Error(msg string, err error)
	Warn(msg string)
	Info(msg string)
	Debug(msg string)
}

type simpleLogger struct {
	logger *log.Logger
}

var (
	loggerInstance *simpleLogger
	once           sync.Once
)

// New creates a new singleton instance of the simple logger.
func New() Logger {
	once.Do(func() {
		loggerInstance = &simpleLogger{
			logger: log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile),
		}
	})
	return loggerInstance
}

// Error logs an error message with the 🔴 emoji.
func (l *simpleLogger) Error(msg string, err error) {
	l.logger.Output(2, fmt.Sprintf("🔴 ERROR: %s - %v", msg, err))
}

// Warn logs a warning message with the ⚠️ emoji.
func (l *simpleLogger) Warn(msg string) {
	l.logger.Output(2, fmt.Sprintf("⚠️ WARN: %s", msg))
}

// Info logs an informational message.
func (l *simpleLogger) Info(msg string) {
	l.logger.Output(2, fmt.Sprintf("INFO: %s", msg))
}

// Debug logs a debug message.
func (l *simpleLogger) Debug(msg string) {
	// Simple logger doesn't differentiate debug, log as info for now
	// Could add a level check if needed later
	l.logger.Output(2, fmt.Sprintf("DEBUG: %s", msg))
}
