package log

import (
	"context"
	"fmt"

	L "github.com/metacubex/sing/common/logger"
)

type singLogger struct{}

func (l singLogger) TraceContext(ctx context.Context, args ...any) {
	Debugln("%s", fmt.Sprint(args...))
}

func (l singLogger) DebugContext(ctx context.Context, args ...any) {
	Debugln("%s", fmt.Sprint(args...))
}

func (l singLogger) InfoContext(ctx context.Context, args ...any) {
	Infoln("%s", fmt.Sprint(args...))
}

func (l singLogger) WarnContext(ctx context.Context, args ...any) {
	Warnln("%s", fmt.Sprint(args...))
}

func (l singLogger) ErrorContext(ctx context.Context, args ...any) {
	Errorln("%s", fmt.Sprint(args...))
}

func (l singLogger) FatalContext(ctx context.Context, args ...any) {
	Fatalln("%s", fmt.Sprint(args...))
}

func (l singLogger) PanicContext(ctx context.Context, args ...any) {
	Fatalln("%s", fmt.Sprint(args...))
}

func (l singLogger) Trace(args ...any) {
	Debugln("%s", fmt.Sprint(args...))
}

func (l singLogger) Debug(args ...any) {
	Debugln("%s", fmt.Sprint(args...))
}

func (l singLogger) Info(args ...any) {
	Infoln("%s", fmt.Sprint(args...))
}

func (l singLogger) Warn(args ...any) {
	Warnln("%s", fmt.Sprint(args...))
}

func (l singLogger) Error(args ...any) {
	Errorln("%s", fmt.Sprint(args...))
}

func (l singLogger) Fatal(args ...any) {
	Fatalln("%s", fmt.Sprint(args...))
}

func (l singLogger) Panic(args ...any) {
	Fatalln("%s", fmt.Sprint(args...))
}

type singInfoToDebugLogger struct {
	singLogger
}

func (l singInfoToDebugLogger) InfoContext(ctx context.Context, args ...any) {
	Debugln("%s", fmt.Sprint(args...))
}

func (l singInfoToDebugLogger) Info(args ...any) {
	Debugln("%s", fmt.Sprint(args...))
}

var SingLogger L.ContextLogger = singLogger{}
var SingInfoToDebugLogger L.ContextLogger = singInfoToDebugLogger{}
