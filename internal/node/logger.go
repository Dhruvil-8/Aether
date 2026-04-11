package node

import (
	"log"
	"os"
	"strings"
)

type LogLevel int

const (
	LogDebug LogLevel = iota
	LogInfo
	LogWarn
	LogError
)

type Logger struct {
	level  LogLevel
	logger *log.Logger
}

func NewLogger(level string) *Logger {
	return &Logger{
		level:  parseLogLevel(level),
		logger: log.New(os.Stdout, "", log.LstdFlags),
	}
}

func parseLogLevel(level string) LogLevel {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return LogDebug
	case "warn", "warning":
		return LogWarn
	case "error":
		return LogError
	default:
		return LogInfo
	}
}

func (l *Logger) Debugf(component, format string, args ...any) {
	l.logf(LogDebug, "DEBUG", component, format, args...)
}

func (l *Logger) Infof(component, format string, args ...any) {
	l.logf(LogInfo, "INFO", component, format, args...)
}

func (l *Logger) Warnf(component, format string, args ...any) {
	l.logf(LogWarn, "WARN", component, format, args...)
}

func (l *Logger) Errorf(component, format string, args ...any) {
	l.logf(LogError, "ERROR", component, format, args...)
}

func (l *Logger) logf(level LogLevel, levelLabel, component, format string, args ...any) {
	if l == nil || l.logger == nil || level < l.level {
		return
	}
	prefix := "[" + levelLabel + "]"
	if component != "" {
		prefix += " [" + component + "]"
	}
	l.logger.Printf(prefix+" "+format, args...)
}
