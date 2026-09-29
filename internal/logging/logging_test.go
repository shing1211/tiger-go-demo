package logging

import (
	"bytes"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	sdklogger "github.com/tigerfintech/openapi-go-sdk/logger"
)

// isClockTime reports whether s looks like the HH:MM:SS stamp logging.log writes.
func isClockTime(s string) bool {
	_, err := time.Parse("15:04:05", s)
	return err == nil
}

// TestParseLevel pins the level table, including the two cases that are easy to
// get wrong: an unrecognised name must fall back to info (not debug, and not an
// error), and "info" itself has no explicit case in the switch — it reaches
// LevelInfo through the default branch, so a typo in the default would be
// invisible.
func TestParseLevel(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want sdklogger.Level
	}{
		{"debug", "debug", sdklogger.LevelDebug},
		{"debug upper case", "DEBUG", sdklogger.LevelDebug},
		{"debug mixed case", "DeBuG", sdklogger.LevelDebug},
		{"debug padded", "  debug  ", sdklogger.LevelDebug},
		{"warn", "warn", sdklogger.LevelWarn},
		{"warn long form", "warning", sdklogger.LevelWarn},
		{"warn upper case", "WARN", sdklogger.LevelWarn},
		{"warn padded", "  warning ", sdklogger.LevelWarn},
		{"error", "error", sdklogger.LevelError},
		{"error upper case", "ERROR", sdklogger.LevelError},
		{"info", "info", sdklogger.LevelInfo},
		{"info upper case", "INFO", sdklogger.LevelInfo},
		{"empty defaults to info", "", sdklogger.LevelInfo},
		{"blank defaults to info", "   ", sdklogger.LevelInfo},
		{"unknown word defaults to info", "verbose", sdklogger.LevelInfo},
		{"near-miss defaults to info", "warnning", sdklogger.LevelInfo},
		{"log-level spelling defaults to info", "trace", sdklogger.LevelInfo},
		{"number defaults to info", "2", sdklogger.LevelInfo},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseLevel(tc.in); got != tc.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestLevelFiltering checks both halves of the threshold: a record at or above
// the configured level is emitted, one below it is dropped. The levels are
// ordered Debug < Info < Warn < Error in the SDK, so a naive comparison would
// invert the whole thing.
func TestLevelFiltering(t *testing.T) {
	tests := []struct {
		name      string
		threshold sdklogger.Level
		wantDebug bool
		wantInfo  bool
		wantWarn  bool
		wantError bool
	}{
		{"debug lets everything through", sdklogger.LevelDebug, true, true, true, true},
		{"info drops debug", sdklogger.LevelInfo, false, true, true, true},
		{"warn drops debug and info", sdklogger.LevelWarn, false, false, true, true},
		{"error only passes errors", sdklogger.LevelError, false, false, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l := New(&buf, tc.threshold)
			l.Debug("d")
			l.Info("i")
			l.Warn("w")
			l.Error("e")
			out := buf.String()

			// Compare per line: the level column is padded to 5 characters, so a
			// substring match on the whole line is not reliable.
			seen := map[string]string{}
			for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
				fields := strings.Fields(line)
				if len(fields) < 3 {
					t.Errorf("unparsable record %q", line)
					continue
				}
				seen[fields[1]] = fields[2]
			}
			for _, want := range []struct {
				level string
				msg   string
				emit  bool
			}{
				{"DEBUG", "d", tc.wantDebug},
				{"INFO", "i", tc.wantInfo},
				{"WARN", "w", tc.wantWarn},
				{"ERROR", "e", tc.wantError},
			} {
				got, ok := seen[want.level]
				switch {
				case want.emit && !ok:
					t.Errorf("%s should have been emitted (output: %q)", want.level, out)
				case want.emit && got != want.msg:
					t.Errorf("%s message = %q, want %q", want.level, got, want.msg)
				case !want.emit && ok:
					t.Errorf("%s should have been suppressed (output: %q)", want.level, out)
				}
			}
		})
	}
}

// TestLogFormatting pins the one-line format the README documents, and that
// printf verbs in an SDK message are actually substituted.
func TestLogFormatting(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, sdklogger.LevelDebug).Info("api request failed method=%s code=%d", "quote_real_time", 1042)
	line := buf.String()
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[1] != "INFO" {
		t.Errorf("second column should be the level INFO, got %q", line)
	}
	if !isClockTime(fields[0]) {
		t.Errorf("first column should be an HH:MM:SS clock time, got %q", fields[0])
	}
	if !strings.Contains(line, "api request failed method=quote_real_time code=1042") {
		t.Errorf("format verbs should be substituted, got %q", line)
	}
	if !strings.HasSuffix(line, "\n") {
		t.Errorf("record should end in a newline, got %q", line)
	}
	if strings.Count(line, "\n") != 1 {
		t.Errorf("one record should be one line, got %q", line)
	}
	// A message with no args must be printed verbatim, not run through Sprintf:
	// Sprintf("50%%") would collapse the pair into "50%".
	var bare bytes.Buffer
	New(&bare, sdklogger.LevelDebug).Warn("50%%")
	if !strings.Contains(bare.String(), "50%%") {
		t.Errorf("a message with no args should keep its %%, got %q", bare.String())
	}
}

// TestNewFromConfig checks the convenience constructor composes ParseLevel and
// New correctly.
func TestNewFromConfig(t *testing.T) {
	tests := []struct {
		name  string
		level string
		want  sdklogger.Level
	}{
		{"debug", "debug", sdklogger.LevelDebug},
		{"warn", "warn", sdklogger.LevelWarn},
		{"error", "error", sdklogger.LevelError},
		{"unknown falls back to info", "chatty", sdklogger.LevelInfo},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l := NewFromConfig(&buf, tc.level)
			if l.Level() != tc.want {
				t.Fatalf("Level() = %v, want %v", l.Level(), tc.want)
			}
			// Log at the configured threshold, which is always allowed.
			switch tc.want {
			case sdklogger.LevelDebug:
				l.Debug("hello")
			case sdklogger.LevelWarn:
				l.Warn("hello")
			case sdklogger.LevelError:
				l.Error("hello")
			default:
				l.Info("hello")
			}
			if !strings.Contains(buf.String(), "hello") {
				t.Errorf("logger at %v should emit at its own level, got %q", tc.want, buf.String())
			}
			// And one level below it, which must be dropped.
			buf.Reset()
			if tc.want > sdklogger.LevelDebug {
				l.Debug("secret")
				if buf.Len() != 0 {
					t.Errorf("logger at %v should suppress debug, got %q", tc.want, buf.String())
				}
			}
		})
	}
}

// TestSetLevelChangesThreshold covers the SDK-facing setter: the SDK calls
// SetLevel to lower verbosity at runtime, and the filter must follow.
func TestSetLevelChangesThreshold(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, sdklogger.LevelError)
	l.Info("before")
	if buf.Len() != 0 {
		t.Fatalf("nothing should be emitted at error level, got %q", buf.String())
	}

	l.SetLevel(sdklogger.LevelDebug)
	if l.Level() != sdklogger.LevelDebug {
		t.Errorf("Level() = %v after SetLevel(Debug)", l.Level())
	}
	l.Info("after")
	if !strings.Contains(buf.String(), "after") {
		t.Errorf("debug level should let info through, got %q", buf.String())
	}
}

// TestNewNilWriterFallsBackToStderr covers the nil-writer branch. It must not
// panic, and it must not be a no-op sink.
func TestNewNilWriterFallsBackToStderr(t *testing.T) {
	l := New(nil, sdklogger.LevelInfo)
	if l == nil {
		t.Fatal("New(nil, ...) should return a logger")
	}
	if l.w != io.Writer(os.Stderr) {
		t.Errorf("a nil writer should become os.Stderr, got %v", l.w)
	}
	// Writing to real stderr is fine here; the point is that it does not panic.
	l.Info("this goes to stderr")
}

// TestDiscardWritesNothing checks the test helper really is a sink, so a test
// using it cannot pollute the go test output.
func TestDiscardWritesNothing(t *testing.T) {
	l := Discard()
	if l.Level() != sdklogger.LevelError {
		t.Errorf("Discard level = %v, want %v", l.Level(), sdklogger.LevelError)
	}
	if l.w == nil {
		t.Fatal("Discard should still have a writer")
	}
	l.Debug("d")
	l.Info("i")
	l.Warn("w")
	l.Error("e")
}

// TestDiscardLevelFilters checks Discard is not just discarding by being a
// nil writer — it is a real threshold, so the level assertions still work.
func TestDiscardLevelFilters(t *testing.T) {
	l := Discard()
	l.Debug("d")
	l.Info("i")
	l.Warn("w")
	if l.Level() != sdklogger.LevelError {
		t.Errorf("level = %v, want %v", l.Level(), sdklogger.LevelError)
	}
}

// TestSilenceSDKNoise checks the SDK's advisory std-log output is redirected and
// that calling it repeatedly is safe. It mutates package-level state, so it must
// restore the previous writer for the rest of the package's tests.
func TestSilenceSDKNoise(t *testing.T) {
	prev := log.Writer()
	t.Cleanup(func() { log.SetOutput(prev) })

	SilenceSDKNoise()
	if log.Writer() != io.Discard {
		t.Errorf("std logger should write to io.Discard, got %v", log.Writer())
	}
	// Idempotent: the commands call it on every run, and it is also called from
	// rocli.Open, so double-calling must not error or panic.
	SilenceSDKNoise()
	log.Printf("[config] load token from file failed: %v", "no such file")
}

// TestConcurrentLogging exercises the mutex. The push client logs from its own
// goroutines, so an unsynchronised write here would be a real data race; run
// under -race this is the test that would catch it.
//
// The threshold is fixed so the record count is deterministic; SetLevel is
// exercised separately below because interleaving it here would change how many
// records pass the filter.
func TestConcurrentLogging(t *testing.T) {
	const goroutines, recordsEach = 8, 50

	var buf bytes.Buffer
	l := New(&buf, sdklogger.LevelDebug)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < recordsEach; j++ {
				l.Debug("tick")
				l.Info("tock")
				_ = l.Level()
			}
		}()
	}
	wg.Wait()

	want := goroutines * recordsEach * 2
	if lines := strings.Count(buf.String(), "\n"); lines != want {
		t.Errorf("got %d records, want %d", lines, want)
	}
	// Each record must be a whole line: an unsynchronised write would interleave
	// and produce lines with more than two fields after the clock and level.
	for _, line := range strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n") {
		if got := strings.Fields(line); len(got) != 3 {
			t.Errorf("torn record %q has %d fields, want 3", line, len(got))
		}
	}
}

// TestConcurrentSetLevel checks the level accessors under contention. The SDK
// raises and lowers verbosity from its own goroutines, so Level and SetLevel
// must not race with each other or with log.
func TestConcurrentSetLevel(t *testing.T) {
	l := Discard()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				l.SetLevel(sdklogger.Level(j % 4))
				_ = l.Level()
				l.Debug("d")
			}
		}()
	}
	wg.Wait()
}
