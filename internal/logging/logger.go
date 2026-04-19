package logging

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

// Level represents the logging verbosity threshold.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// Logger writes structured JSON logs.
type Logger struct {
	std   *log.Logger
	level Level
	debug bool
}

// New creates a structured logger writing to stdout.
func New(level string, debug bool) *Logger {
	return NewWithWriter(os.Stdout, level, debug)
}

// NewWithWriter creates a structured logger writing to a custom writer.
func NewWithWriter(w io.Writer, level string, debug bool) *Logger {
	return &Logger{
		std:   log.New(w, "", 0),
		level: parseLevel(level),
		debug: debug,
	}
}

func parseLevel(level string) Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return LevelDebug
	case "warn":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

func (l *Logger) log(level Level, label, message string, fields map[string]any) {
	if l == nil {
		return
	}
	if level < l.level {
		return
	}

	record := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"level": label,
		"msg":   message,
	}
	if len(fields) > 0 {
		record["fields"] = fields
	}
	if l.debug {
		record["debug"] = true
	}
	bytes, err := json.Marshal(record)
	if err != nil {
		l.std.Printf("{\"level\":\"error\",\"msg\":\"failed to encode log record\",\"error\":%q}", err.Error())
		return
	}
	l.std.Println(string(bytes))
}

// Debug logs a debug message.
func (l *Logger) Debug(message string, fields map[string]any) {
	l.log(LevelDebug, "debug", message, fields)
}

// Info logs an informational message.
func (l *Logger) Info(message string, fields map[string]any) {
	l.log(LevelInfo, "info", message, fields)
}

// Warn logs a warning message.
func (l *Logger) Warn(message string, fields map[string]any) {
	l.log(LevelWarn, "warn", message, fields)
}

// Error logs an error message.
func (l *Logger) Error(message string, fields map[string]any) {
	l.log(LevelError, "error", message, fields)
}
