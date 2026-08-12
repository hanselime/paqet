package flog

import (
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"time"
)

type Level int32

const None Level = -1
const (
	Debug Level = iota
	Info
	Warn
	Error
	Fatal
)

var (
	lv  atomic.Int32
	ch  = make(chan string, 1024)
	out io.Writer
)

func init() {
	lv.Store(int32(Info))
	out = os.Stdout
	go func() {
		for l := range ch {
			fmt.Fprint(out, l)
		}
	}()
}

func SetLevel(l int) { lv.Store(int32(l)) }

func logf(l Level, format string, args ...any) {
	mlv := Level(lv.Load())
	if mlv == None || l < mlv {
		return
	}
	if mlv > Debug {
		for _, a := range args {
			if err, ok := a.(error); ok && WErr(err) == nil {
				return
			}
		}
	}

	now := time.Now().Format("2006-01-02 15:04:05.000")
	line := fmt.Sprintf("%s [%s] %s\n", now, l, escape(fmt.Sprintf(format, args...)))

	select {
	case ch <- line:
	default:
	}
}

var levelNames = [...]string{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"}

func (l Level) String() string {
	switch {
	case l == None:
		return "NONE"
	case l < Debug || l > Fatal:
		return "UNKNOWN"
	}
	return levelNames[l]
}

func Debugf(format string, args ...any) { logf(Debug, format, args...) }
func Infof(format string, args ...any)  { logf(Info, format, args...) }
func Warnf(format string, args ...any)  { logf(Warn, format, args...) }
func Errorf(format string, args ...any) { logf(Error, format, args...) }
func Fatalf(format string, args ...any) {
	if Level(lv.Load()) != None {
		now := time.Now().Format("2006-01-02 15:04:05.000")
		fmt.Fprintf(os.Stderr, "%s [%s] %s\n", now, Fatal, escape(fmt.Sprintf(format, args...)))
	}
	os.Exit(1)
}
