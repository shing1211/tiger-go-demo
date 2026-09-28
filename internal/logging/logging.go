// Package logging provides a small leveled logger that satisfies the SDK's
// logger.Logger interface, so SDK internals and our own output share one
// format and one level threshold.
package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	sdklogger "github.com/tigerfintech/openapi-go-sdk/logger"
)

// ParseLevel maps a level name to an SDK level. Unknown names fall back to info.
func ParseLevel(s string) sdklogger.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return sdklogger.LevelDebug
	case "warn", "warning":
		return sdklogger.LevelWarn
	case "error":
		return sdklogger.LevelError
	default:
		return sdklogger.LevelInfo
	}
}

// Logger writes leveled messages to an io.Writer. Safe for concurrent use,
// which matters because the SDK logs from its push client's goroutines.
type Logger struct {
	mu    sync.Mutex
	w     io.Writer
	level sdklogger.Level
}

var _ sdklogger.Logger = (*Logger)(nil)

// New returns a Logger writing to w. A nil w means os.Stderr.
func New(w io.Writer, level sdklogger.Level) *Logger {
	if w == nil {
		w = os.Stderr
	}
	return &Logger{w: w, level: level}
}

// NewFromConfig is a convenience wrapper around New + ParseLevel.
func NewFromConfig(w io.Writer, level string) *Logger { return New(w, ParseLevel(level)) }

// SetLevel implements sdklogger.Logger.
func (l *Logger) SetLevel(level sdklogger.Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// Level returns the current threshold.
func (l *Logger) Level() sdklogger.Level {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.level
}

func (l *Logger) log(level sdklogger.Level, msg string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if level < l.level {
		return
	}
	// SDK messages contain printf-style verbs; ours contain %w wrapping.
	text := msg
	if len(args) > 0 {
		text = fmt.Sprintf(msg, args...)
	}
	fmt.Fprintf(l.w, "%s %-5s %s\n", time.Now().Format("15:04:05"), level, text)
}

// Debug implements sdklogger.Logger.
func (l *Logger) Debug(msg string, args ...interface{}) { l.log(sdklogger.LevelDebug, msg, args...) }

// Info implements sdklogger.Logger.
func (l *Logger) Info(msg string, args ...interface{}) { l.log(sdklogger.LevelInfo, msg, args...) }

// Warn implements sdklogger.Logger.
func (l *Logger) Warn(msg string, args ...interface{}) { l.log(sdklogger.LevelWarn, msg, args...) }

// Error implements sdklogger.Logger.
func (l *Logger) Error(msg string, args ...interface{}) { l.log(sdklogger.LevelError, msg, args...) }

// Discard is a logger that throws everything away, for tests.
func Discard() *Logger { return New(io.Discard, sdklogger.LevelError) }

// SilenceSDKNoise redirects the standard library logger, which the SDK uses
// directly (via log.Printf) for advisory notices such as "no token file
// found". Those are expected in this project — we never use a token file —
// and they would otherwise appear before our own output on every run.
//
// Our own logger writes with fmt.Fprintf to an explicit writer, so nothing we
// care about is lost. Real errors still surface through returned error values.
func SilenceSDKNoise() {
	log.SetOutput(io.Discard)
}
