package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

type LoggingConfig struct {
	Level      string `json:"level"`
	Format     string `json:"format"`
	Output     string `json:"output"`
	MaxSize    int    `json:"max_size"`
	MaxBackups int    `json:"max_backups"`
	MaxAge     int    `json:"max_age"`
}

type Logger struct {
	logger *log.Logger
	config *LoggingConfig
	mutex  sync.RWMutex
	level  LogLevel
}

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

var levelMap = map[string]LogLevel{
	"debug": DEBUG,
	"info":  INFO,
	"warn":  WARN,
	"error": ERROR,
	"fatal": FATAL,
}

func NewLogger(config *LoggingConfig) (*Logger, error) {
	if config == nil {
		config = &LoggingConfig{
			Level:  "info",
			Format: "text",
			Output: "stdout",
		}
	}

	level, exists := levelMap[config.Level]
	if !exists {
		level = INFO
	}

	var output io.Writer
	switch config.Output {
	case "stdout":
		output = os.Stdout
	case "stderr":
		output = os.Stderr
	default:
		file, err := os.OpenFile(config.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		output = file
	}

	return &Logger{
		logger: log.New(output, "", log.LstdFlags),
		config: config,
		level:  level,
	}, nil
}

func (l *Logger) Debug(format string, args ...interface{}) {
	if l.level <= DEBUG {
		l.logger.Printf("[DEBUG] "+format, args...)
	}
}

func (l *Logger) Info(format string, args ...interface{}) {
	if l.level <= INFO {
		l.logger.Printf("[INFO] "+format, args...)
	}
}

func (l *Logger) Warn(format string, args ...interface{}) {
	if l.level <= WARN {
		l.logger.Printf("[WARN] "+format, args...)
	}
}

func (l *Logger) Error(format string, args ...interface{}) {
	if l.level <= ERROR {
		l.logger.Printf("[ERROR] "+format, args...)
	}
}

func (l *Logger) Fatal(format string, args ...interface{}) {
	l.logger.Printf("[FATAL] "+format, args...)
	os.Exit(1)
}

func (l *Logger) Close() error {
	return nil
}
