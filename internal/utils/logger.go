package utils

import (
	"log"
	"os"
)

type Logger struct {
	*log.Logger
	level string
}

func NewLogger(level string) *Logger {
	return &Logger{Logger: log.New(os.Stdout, "", log.LstdFlags), level: level}
}

func (l *Logger) Debugf(format string, args ...any) {
	if l.level == "debug" {
		l.Printf("DEBUG: "+format, args...)
	}
}

func (l *Logger) Infof(format string, args ...any) {
	l.Printf("INFO: "+format, args...)
}

func (l *Logger) Warnf(format string, args ...any) {
	l.Printf("WARN: "+format, args...)
}

func (l *Logger) Errorf(format string, args ...any) {
	l.Printf("ERROR: "+format, args...)
}
