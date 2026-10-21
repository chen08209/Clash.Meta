package log

import (
	"fmt"
	"os"
	"sync/atomic"

	"github.com/metacubex/mihomo/common/observable"

	log "github.com/sirupsen/logrus"
)

var (
	logCh  = make(chan Event)
	source = observable.NewObservable[Event](logCh)
	level  atomic.Int32
)

func init() {
	level.Store(int32(INFO))
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:             true,
		TimestampFormat:           "2006-01-02T15:04:05.000000000Z07:00",
		EnvironmentOverrideColors: true,
	})
}

type Event struct {
	LogLevel LogLevel
	Payload  string
}

func (e *Event) Type() string {
	return e.LogLevel.String()
}

func Infoln(format string, v ...any) {
	if !shouldLog(INFO) {
		return
	}
	event := newLog(INFO, format, v...)
	logCh <- event
	print(event)
}

func Warnln(format string, v ...any) {
	if !shouldLog(WARNING) {
		return
	}
	event := newLog(WARNING, format, v...)
	logCh <- event
	print(event)
}

func Errorln(format string, v ...any) {
	if !shouldLog(ERROR) {
		return
	}
	event := newLog(ERROR, format, v...)
	logCh <- event
	print(event)
}

func Debugln(format string, v ...any) {
	if !shouldLog(DEBUG) {
		return
	}
	event := newLog(DEBUG, format, v...)
	logCh <- event
	print(event)
}

func Fatalln(format string, v ...any) {
	log.Fatalf(format, v...)
}

func Subscribe() observable.Subscription[Event] {
	sub, _ := source.Subscribe()
	return sub
}

func UnSubscribe(sub observable.Subscription[Event]) {
	source.UnSubscribe(sub)
}

func Level() LogLevel {
	return LogLevel(level.Load())
}

// DebugEnabled lets hot paths skip building log arguments that would otherwise
// be evaluated before the level gate inside Debugln runs.
func DebugEnabled() bool {
	return shouldLog(DEBUG)
}

func InfoEnabled() bool {
	return shouldLog(INFO)
}

func SetLevel(newLevel LogLevel) {
	level.Store(int32(newLevel))
}

func shouldLog(logLevel LogLevel) bool {
	return logLevel >= Level()
}

func print(data Event) {
	if !shouldLog(data.LogLevel) {
		return
	}

	switch data.LogLevel {
	case INFO:
		log.StandardLogger().Log(log.InfoLevel, data.Payload)
	case WARNING:
		log.StandardLogger().Log(log.WarnLevel, data.Payload)
	case ERROR:
		log.StandardLogger().Log(log.ErrorLevel, data.Payload)
	case DEBUG:
		log.StandardLogger().Log(log.DebugLevel, data.Payload)
	}
}

func newLog(logLevel LogLevel, format string, v ...any) Event {
	return Event{
		LogLevel: logLevel,
		Payload:  fmt.Sprintf(format, v...),
	}
}
