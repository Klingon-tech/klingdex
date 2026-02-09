// Package logging provides structured logging for the Klingon P2P node.
package logging

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/log"
)

// Level represents a log level.
type Level = log.Level

// Log levels.
const (
	DebugLevel = log.DebugLevel
	InfoLevel  = log.InfoLevel
	WarnLevel  = log.WarnLevel
	ErrorLevel = log.ErrorLevel
	FatalLevel = log.FatalLevel
)

// Logger wraps charmbracelet/log with additional functionality.
type Logger struct {
	*log.Logger
	timeFormat string
}

// Config holds logger configuration.
type Config struct {
	Level      string
	TimeFormat string
	Prefix     string
	Output     io.Writer
}

// DefaultConfig returns a default logging configuration.
func DefaultConfig() *Config {
	return &Config{
		Level:      "info",
		TimeFormat: time.TimeOnly,
		Prefix:     "",
		Output:     os.Stderr,
	}
}

// New creates a new logger with the given configuration.
func New(cfg *Config) *Logger {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	output := cfg.Output
	if output == nil {
		output = os.Stderr
	}

	logger := log.NewWithOptions(output, log.Options{
		ReportCaller:    false,
		ReportTimestamp: true,
		TimeFormat:      cfg.TimeFormat,
		Prefix:          cfg.Prefix,
	})

	logger.SetLevel(ParseLevel(cfg.Level))

	return &Logger{Logger: logger, timeFormat: cfg.TimeFormat}
}

// Default returns the default logger.
func Default() *Logger {
	return New(DefaultConfig())
}

// ParseLevel parses a string level into a log.Level.
func ParseLevel(level string) Level {
	switch strings.ToLower(level) {
	case "debug":
		return DebugLevel
	case "info":
		return InfoLevel
	case "warn", "warning":
		return WarnLevel
	case "error":
		return ErrorLevel
	case "fatal":
		return FatalLevel
	default:
		return InfoLevel
	}
}

// With returns a new logger with the given key-value pairs.
func (l *Logger) With(keyvals ...interface{}) *Logger {
	return &Logger{Logger: l.Logger.With(keyvals...), timeFormat: l.timeFormat}
}

// WithPrefix returns a new logger with the given prefix.
func (l *Logger) WithPrefix(prefix string) *Logger {
	timeFormat := l.timeFormat
	if timeFormat == "" {
		timeFormat = time.TimeOnly
	}
	newLogger := log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: true,
		TimeFormat:      timeFormat,
		Prefix:          prefix,
	})
	newLogger.SetLevel(l.GetLevel())
	return &Logger{Logger: newLogger, timeFormat: timeFormat}
}

// Component returns a logger for a specific component.
func (l *Logger) Component(name string) *Logger {
	return l.WithPrefix(name)
}

// Global default logger instance.
var defaultLogger = Default()

// SetDefault sets the default logger.
func SetDefault(l *Logger) {
	defaultLogger = l
}

// GetDefault returns the default logger.
func GetDefault() *Logger {
	return defaultLogger
}

// Package-level logging functions using the default logger.

func Debug(msg interface{}, keyvals ...interface{}) { defaultLogger.Debug(msg, keyvals...) }
func Info(msg interface{}, keyvals ...interface{})  { defaultLogger.Info(msg, keyvals...) }
func Warn(msg interface{}, keyvals ...interface{})  { defaultLogger.Warn(msg, keyvals...) }
func Error(msg interface{}, keyvals ...interface{}) { defaultLogger.Error(msg, keyvals...) }
func Fatal(msg interface{}, keyvals ...interface{}) { defaultLogger.Fatal(msg, keyvals...) }

func Debugf(format string, args ...interface{}) { defaultLogger.Debugf(format, args...) }
func Infof(format string, args ...interface{})  { defaultLogger.Infof(format, args...) }
func Warnf(format string, args ...interface{})  { defaultLogger.Warnf(format, args...) }
func Errorf(format string, args ...interface{}) { defaultLogger.Errorf(format, args...) }
func Fatalf(format string, args ...interface{}) { defaultLogger.Fatalf(format, args...) }
