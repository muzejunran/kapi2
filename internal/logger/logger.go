package logger

import (
	"context"
	"crypto/rand"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

type contextKey struct{}

// Setup configures the global logrus logger: text format, timestamp, log level, and file:line caller info.
// Call once at program startup before any logging.
func Setup() {
	logrus.SetReportCaller(true)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			// Show last 3 path segments: e.g. "internal/agent/agent.go:245"
			parts := strings.Split(f.File, "/")
			n := len(parts)
			if n > 3 {
				parts = parts[n-3:]
			}
			return "", strings.Join(parts, "/") + ":" + strconv.Itoa(f.Line)
		},
	})
}

// New returns a configured *logrus.Logger (same global instance, already configured by Setup).
func New() *logrus.Logger {
	return logrus.StandardLogger()
}

// NewID generates a short random hex trace ID (16 chars).
func NewID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%x", b)
}

// WithTraceID attaches a trace ID to the context.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, contextKey{}, traceID)
}

// IDFromContext retrieves the trace ID from the context (empty string if absent).
func IDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(contextKey{}).(string)
	return id
}

// FromContext returns a *logrus.Entry pre-seeded with trace_id from the context.
// If no trace ID is present, returns a plain entry from the global logger.
func FromContext(ctx context.Context) *logrus.Entry {
	id := IDFromContext(ctx)
	if id == "" {
		return logrus.NewEntry(logrus.StandardLogger())
	}
	return logrus.StandardLogger().WithField("trace_id", id)
}
