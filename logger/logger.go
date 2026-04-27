// Package logger defines the logging interface used by the Synthetix SDK.
//
// The SDK is BYO-logger: callers can pass any implementation that
// satisfies this interface. A Nop adapter is provided for tests and
// for callers that do not need SDK output.
package logger

// Structured key/value logger. Matches the slog-style signature used
// by most Go logging libraries (zerolog/slog/zap can all wrap into it).
type Logger interface {
	Debug(msg string, keyVals ...any)
	Info(msg string, keyVals ...any)
	Warn(msg string, keyVals ...any)
	Error(msg string, keyVals ...any)
	With(keyVals ...any) Logger
}

// Nop returns a Logger that discards all output. Safe for tests and
// for embedding when the caller does not want SDK chatter.
func Nop() Logger { return nopLogger{} }

type nopLogger struct{}

func (nopLogger) Debug(string, ...any)  {}
func (nopLogger) Info(string, ...any)   {}
func (nopLogger) Warn(string, ...any)   {}
func (nopLogger) Error(string, ...any)  {}
func (nopLogger) With(...any) Logger    { return nopLogger{} }
