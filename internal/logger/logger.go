package logger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var currentLevel zapcore.Level

// LoggingConfig matches the Python implementation
type LoggingConfig struct {
	LogToFile bool   `json:"log_to_file"`
	Dir       string `json:"dir"`
	Pattern   string `json:"pattern"`
	KeepCount *int   `json:"keep_count"`
}

var sharedLoggingConfig LoggingConfig
var fileHandlerInstance *fileHandler

func init() {
	sharedLoggingConfig = parseLoggingConfig()
	if sharedLoggingConfig.LogToFile {
		fileHandlerInstance = newFileHandler(sharedLoggingConfig)
	}
}

// New creates a console-only logger for server lifecycle logs
func New() *zap.Logger {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.RFC3339TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	currentLevel = parseLogLevel(os.Getenv("LOG_LEVEL"))

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(os.Stdout),
		currentLevel,
	)

	return zap.New(core)
}

// NewAPILogger creates a logger that writes to both console and file
// for API call logging
func NewAPILogger(consoleLogger *zap.Logger) *zap.Logger {
	if !sharedLoggingConfig.LogToFile || fileHandlerInstance == nil {
		return consoleLogger
	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.RFC3339TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	fileCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(fileHandlerInstance),
		currentLevel,
	)

	// Create a tee that writes to both console (from the passed logger) and file
	return zap.New(zapcore.NewTee(consoleLogger.Core(), fileCore))
}

func parseLoggingConfig() LoggingConfig {
	config := LoggingConfig{
		Dir:     "logs",
		Pattern: "%Y%m%d.log.jsonl",
	}

	configJSON := os.Getenv("LOGGING_CONFIG")
	if configJSON == "" {
		return config
	}

	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		// If parsing fails, return defaults
		return LoggingConfig{
			Dir:     "logs",
			Pattern: "%Y%m%d.log.jsonl",
		}
	}

	// Set defaults if empty
	if config.Dir == "" {
		config.Dir = "logs"
	}
	if config.Pattern == "" {
		config.Pattern = "%Y%m%d.log.jsonl"
	}

	return config
}

func IsDebug() bool {
	return currentLevel == zapcore.DebugLevel
}

func parseLogLevel(levelStr string) zapcore.Level {
	switch strings.ToLower(levelStr) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// fileHandler implements date-based log rotation
type fileHandler struct {
	mu            sync.Mutex
	logDir        string
	pattern       string
	keepCount     *int
	currentFile   *os.File
	currentDate   string
}

func newFileHandler(config LoggingConfig) *fileHandler {
	// Create log directory if it doesn't exist
	if err := os.MkdirAll(config.Dir, 0755); err != nil {
		return nil
	}

	return &fileHandler{
		logDir:    config.Dir,
		pattern:   config.Pattern,
		keepCount: config.KeepCount,
	}
}

// Write implements io.Writer with date-based rotation
func (h *fileHandler) Write(p []byte) (n int, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	dateStr := h.formatDate(now)

	// Check if we need to rotate
	if dateStr != h.currentDate || h.currentFile == nil {
		if err := h.rotate(dateStr); err != nil {
			return 0, err
		}
	}

	return h.currentFile.Write(p)
}

// Sync implements zapcore.WriteSyncer
func (h *fileHandler) Sync() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.currentFile != nil {
		return h.currentFile.Sync()
	}
	return nil
}

// rotate switches to a new log file
func (h *fileHandler) rotate(dateStr string) error {
	// Close current file if open
	if h.currentFile != nil {
		h.currentFile.Close()
	}

	h.currentDate = dateStr
	filename := filepath.Join(h.logDir, dateStr)

	// Open or create log file
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	h.currentFile = file

	// Clean up old files if keep_count is set
	if h.keepCount != nil && *h.keepCount > 0 {
		h.cleanupOldFiles()
	}

	return nil
}

// formatDate converts Python strftime pattern to Go time format
func (h *fileHandler) formatDate(t time.Time) string {
	// Convert Python strftime directives to Go time format
	goFormat := h.pattern

	// Map Python strftime directives to Go time format
	replacements := map[string]string{
		"%Y": "2006",
		"%y": "06",
		"%m": "01",
		"%d": "02",
		"%H": "15",
		"%M": "04",
		"%S": "05",
		"%f": "000000",
	}

	for py, gofmt := range replacements {
		goFormat = strings.ReplaceAll(goFormat, py, gofmt)
	}

	return t.Format(goFormat)
}

// cleanupOldFiles removes old log files based on keep_count
func (h *fileHandler) cleanupOldFiles() {
	if h.keepCount == nil || *h.keepCount <= 0 {
		return
	}

	// Get all files in log directory
	entries, err := os.ReadDir(h.logDir)
	if err != nil {
		return
	}

	// Build regex pattern to match log files
	// Convert pattern to regex by escaping special chars and replacing date directives
	regexPattern := h.pattern
	replacements := map[string]string{
		"%Y": `\d{4}`,
		"%y": `\d{2}`,
		"%m": `\d{2}`,
		"%d": `\d{2}`,
		"%H": `\d{2}`,
		"%M": `\d{2}`,
		"%S": `\d{2}`,
		"%f": `\d{6}`,
		".":  `\.`,
	}

	for old, new := range replacements {
		regexPattern = strings.ReplaceAll(regexPattern, old, new)
	}

	re, err := regexp.Compile("^" + regexPattern + "$")
	if err != nil {
		return
	}

	// Collect matching files with their mod times
	type fileInfo struct {
		name    string
		modTime time.Time
	}

	var files []fileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if re.MatchString(entry.Name()) {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			files = append(files, fileInfo{name: entry.Name(), modTime: info.ModTime()})
		}
	}

	// Sort by modification time (newest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	// Remove old files beyond keep_count
	if len(files) > *h.keepCount {
		for _, f := range files[*h.keepCount:] {
			os.Remove(filepath.Join(h.logDir, f.name))
		}
	}
}
