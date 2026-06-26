package utils

import (
	"fmt"
	"sync"
	"time"
)

type UILogger struct {
	mu     sync.Mutex
	writer func(string)
}

func NewUILogger() *UILogger {
	return &UILogger{}
}

func (l *UILogger) SetWriter(w func(string)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writer = w
}

func (l *UILogger) log(level string, color string, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	line := fmt.Sprintf("[%s][%s]%s[-][-] %s\n",
		color,
		time.Now().Format("15:04:05"),
		level,
		msg,
	)

	if l.writer != nil {
		l.writer(line)
		return
	}

	fmt.Print(line)
}

func (l *UILogger) Info(msg string) {
	l.log(" INFO ", "white", msg)
}

func (l *UILogger) Warn(msg string) {
	l.log(" WARN ", "yellow", msg)
}

func (l *UILogger) Error(msg string) {
	l.log(" ERROR ", "red", msg)
}

func (l *UILogger) Success(msg string) {
	l.log(" OK ", "green", msg)
}
