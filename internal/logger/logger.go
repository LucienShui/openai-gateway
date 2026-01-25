package logger

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

type Logger struct {
	mu     sync.Mutex
	writer io.Writer
}

func New(writer io.Writer) *Logger {
	if writer == nil {
		writer = os.Stdout
	}
	return &Logger{writer: writer}
}

func (l *Logger) log(level string, data map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := make(map[string]any)
	entry["timestamp"] = time.Now().Format(time.RFC3339)
	entry["level"] = level
	for k, v := range data {
		entry[k] = v
	}

	b, _ := json.Marshal(entry)
	l.writer.Write(append(b, '\n'))
}

func (l *Logger) Info(data map[string]any) {
	l.log("INFO", data)
}

func (l *Logger) Error(data map[string]any) {
	l.log("ERROR", data)
}

func (l *Logger) Warn(data map[string]any) {
	l.log("WARN", data)
}
