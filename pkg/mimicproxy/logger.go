package mimicproxy

import (
	"fmt"
	"log"
	"os"
)

// Logger is the interface for structured logging in the proxy.
// This interface is compatible with multiple logging backends including zap.Logger.
type Logger interface {
	// Debug logs a debug message with optional key-value pairs.
	Debug(msg string, keysAndValues ...interface{})
	// Info logs an info message with optional key-value pairs.
	Info(msg string, keysAndValues ...interface{})
	// Warn logs a warning message with optional key-value pairs.
	Warn(msg string, keysAndValues ...interface{})
	// Error logs an error message with optional key-value pairs.
	Error(msg string, keysAndValues ...interface{})
}

// NoOpLogger is a logger that discards all log messages.
type NoOpLogger struct{}

// Debug implements Logger.
func (n *NoOpLogger) Debug(msg string, keysAndValues ...interface{}) {}

// Info implements Logger.
func (n *NoOpLogger) Info(msg string, keysAndValues ...interface{}) {}

// Warn implements Logger.
func (n *NoOpLogger) Warn(msg string, keysAndValues ...interface{}) {}

// Error implements Logger.
func (n *NoOpLogger) Error(msg string, keysAndValues ...interface{}) {}

// StandardLogger wraps Go's standard logger to implement the Logger interface.
type StandardLogger struct {
	logger *log.Logger
	level  LogLevel
}

// LogLevel represents the logging level.
type LogLevel int

const (
	// LogLevelDebug enables debug and above.
	LogLevelDebug LogLevel = iota
	// LogLevelInfo enables info and above.
	LogLevelInfo
	// LogLevelWarn enables warn and above.
	LogLevelWarn
	// LogLevelError enables error only.
	LogLevelError
)

// NewStandardLogger creates a new StandardLogger with the specified level.
func NewStandardLogger(level LogLevel) (logger *StandardLogger) {
	logger = &StandardLogger{
		logger: log.New(os.Stdout, "[mimic-proxy] ", log.LstdFlags),
		level:  level,
	}

	return logger
}

// Debug implements Logger.
func (s *StandardLogger) Debug(msg string, keysAndValues ...interface{}) {
	if s.level <= LogLevelDebug {
		s.log("DEBUG", msg, keysAndValues...)
	}
}

// Info implements Logger.
func (s *StandardLogger) Info(msg string, keysAndValues ...interface{}) {
	if s.level <= LogLevelInfo {
		s.log("INFO", msg, keysAndValues...)
	}
}

// Warn implements Logger.
func (s *StandardLogger) Warn(msg string, keysAndValues ...interface{}) {
	if s.level <= LogLevelWarn {
		s.log("WARN", msg, keysAndValues...)
	}
}

// Error implements Logger.
func (s *StandardLogger) Error(msg string, keysAndValues ...interface{}) {
	if s.level <= LogLevelError {
		s.log("ERROR", msg, keysAndValues...)
	}
}

func (s *StandardLogger) log(level, msg string, keysAndValues ...interface{}) {
	formatted := fmt.Sprintf("%s: %s", level, msg)

	if len(keysAndValues) > 0 {
		formatted += " "

		for i := 0; i < len(keysAndValues); i += 2 {
			if i+1 < len(keysAndValues) {
				formatted += fmt.Sprintf("%v=%v ", keysAndValues[i], keysAndValues[i+1])
			} else {
				formatted += fmt.Sprintf("%v=<missing> ", keysAndValues[i])
			}
		}
	}

	s.logger.Println(formatted)
}
