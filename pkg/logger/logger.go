package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity level of a log message
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

// String returns the string representation of log level
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Color codes for different log levels
var levelColors = map[LogLevel]string{
	DEBUG: "\033[36m", // Cyan
	INFO:  "\033[32m", // Green
	WARN:  "\033[33m", // Yellow
	ERROR: "\033[31m", // Red
	FATAL: "\033[35m", // Magenta
}

const colorReset = "\033[0m"

// Logger represents the main logger instance
type Logger struct {
	level          LogLevel
	output         io.Writer
	fileOutput     io.Writer
	enableColors   bool
	enableCaller   bool
	enableTime     bool
	timeFormat     string
	prefix         string
	mutex          sync.Mutex
	fields         map[string]interface{}
	hooks          []Hook
	rotateConfig   *RotateConfig
	currentLogFile *os.File
}

// Hook represents a function that gets called on each log entry
type Hook func(level LogLevel, message string, fields map[string]interface{})

// RotateConfig represents log rotation configuration
type RotateConfig struct {
	MaxSize    int64         // Maximum size in bytes before rotation
	MaxAge     time.Duration // Maximum age before rotation
	MaxBackups int           // Maximum number of backup files to keep
	Compress   bool          // Whether to compress rotated files
	LogDir     string        // Directory to store log files
	Filename   string        // Base filename for log files
}

// Config represents logger configuration
type Config struct {
	Level        string
	Output       string // "console", "file", "both"
	Format       string // "text", "json"
	EnableColors bool
	EnableCaller bool
	LogDir       string
	Filename     string
	MaxSize      int64
	MaxAge       time.Duration
	MaxBackups   int
}

var (
	// Default logger instance
	defaultLogger *Logger
	// Global mutex for thread safety
	globalMutex sync.RWMutex
)

// DefaultConfig returns default logger configuration
func DefaultConfig() *Config {
	return &Config{
		Level:        "INFO",
		Output:       "console",
		Format:       "text",
		EnableColors: true,
		EnableCaller: true,
		LogDir:       "./logs",
		Filename:     "chatapp.log",
		MaxSize:      100 * 1024 * 1024,  // 100MB
		MaxAge:       7 * 24 * time.Hour, // 7 days
		MaxBackups:   10,
	}
}

// Init initializes the global logger with default configuration
func Init() {
	config := DefaultConfig()
	InitWithConfig(config)
}

// InitWithConfig initializes the global logger with custom configuration
func InitWithConfig(config *Config) {
	logger, err := NewLogger(config)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	globalMutex.Lock()
	defaultLogger = logger
	globalMutex.Unlock()

	Info("Logger initialized successfully")
}

// NewLogger creates a new logger instance
func NewLogger(config *Config) (*Logger, error) {
	level := parseLogLevel(config.Level)

	logger := &Logger{
		level:        level,
		enableColors: config.EnableColors && config.Output != "file",
		enableCaller: config.EnableCaller,
		enableTime:   true,
		timeFormat:   "2006-01-02 15:04:05",
		fields:       make(map[string]interface{}),
		hooks:        make([]Hook, 0),
	}

	// Set output
	switch config.Output {
	case "console":
		logger.output = os.Stdout
	case "file":
		if err := logger.setupFileOutput(config); err != nil {
			return nil, fmt.Errorf("failed to setup file output: %w", err)
		}
	case "both":
		logger.output = os.Stdout
		if err := logger.setupFileOutput(config); err != nil {
			return nil, fmt.Errorf("failed to setup file output: %w", err)
		}
	default:
		logger.output = os.Stdout
	}

	return logger, nil
}

// setupFileOutput configures file output for the logger
func (l *Logger) setupFileOutput(config *Config) error {
	// Create log directory if it doesn't exist
	if err := os.MkdirAll(config.LogDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Setup rotation config
	l.rotateConfig = &RotateConfig{
		MaxSize:    config.MaxSize,
		MaxAge:     config.MaxAge,
		MaxBackups: config.MaxBackups,
		LogDir:     config.LogDir,
		Filename:   config.Filename,
	}

	// Open log file
	logPath := filepath.Join(config.LogDir, config.Filename)
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	l.currentLogFile = file
	l.fileOutput = file

	return nil
}

// parseLogLevel converts string to LogLevel
func parseLogLevel(level string) LogLevel {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN", "WARNING":
		return WARN
	case "ERROR":
		return ERROR
	case "FATAL":
		return FATAL
	default:
		return INFO
	}
}

// log writes a log message with the specified level
func (l *Logger) log(level LogLevel, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	message := fmt.Sprint(args...)
	entry := l.formatLogEntry(level, message)

	// Write to console output
	if l.output != nil {
		l.output.Write([]byte(entry))
	}

	// Write to file output
	if l.fileOutput != nil {
		// Remove colors for file output
		cleanEntry := l.removeColors(entry)
		l.fileOutput.Write([]byte(cleanEntry))

		// Check if rotation is needed
		if l.rotateConfig != nil {
			l.checkRotation()
		}
	}

	// Execute hooks
	for _, hook := range l.hooks {
		go hook(level, message, l.fields)
	}
}

// logf writes a formatted log message with the specified level
func (l *Logger) logf(level LogLevel, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	message := fmt.Sprintf(format, args...)
	entry := l.formatLogEntry(level, message)

	// Write to console output
	if l.output != nil {
		l.output.Write([]byte(entry))
	}

	// Write to file output
	if l.fileOutput != nil {
		cleanEntry := l.removeColors(entry)
		l.fileOutput.Write([]byte(cleanEntry))

		if l.rotateConfig != nil {
			l.checkRotation()
		}
	}

	// Execute hooks
	for _, hook := range l.hooks {
		go hook(level, message, l.fields)
	}
}

// formatLogEntry formats a log entry
func (l *Logger) formatLogEntry(level LogLevel, message string) string {
	var parts []string

	// Add timestamp
	if l.enableTime {
		timestamp := time.Now().Format(l.timeFormat)
		parts = append(parts, timestamp)
	}

	// Add log level with color
	levelStr := level.String()
	if l.enableColors {
		if color, ok := levelColors[level]; ok {
			levelStr = color + levelStr + colorReset
		}
	}
	parts = append(parts, fmt.Sprintf("[%s]", levelStr))

	// Add caller information
	if l.enableCaller {
		if caller := l.getCaller(); caller != "" {
			parts = append(parts, caller)
		}
	}

	// Add prefix if set
	if l.prefix != "" {
		parts = append(parts, l.prefix)
	}

	// Add fields
	if len(l.fields) > 0 {
		fieldsStr := l.formatFields()
		parts = append(parts, fieldsStr)
	}

	// Add message
	parts = append(parts, message)

	return strings.Join(parts, " ") + "\n"
}

// getCaller returns caller information
func (l *Logger) getCaller() string {
	_, file, line, ok := runtime.Caller(4) // Adjust depth as needed
	if !ok {
		return ""
	}

	// Get only the filename, not the full path
	filename := filepath.Base(file)
	return fmt.Sprintf("%s:%d", filename, line)
}

// formatFields formats logger fields
func (l *Logger) formatFields() string {
	if len(l.fields) == 0 {
		return ""
	}

	var parts []string
	for key, value := range l.fields {
		parts = append(parts, fmt.Sprintf("%s=%v", key, value))
	}

	return "[" + strings.Join(parts, " ") + "]"
}

// removeColors removes ANSI color codes from text
func (l *Logger) removeColors(text string) string {
	for _, color := range levelColors {
		text = strings.ReplaceAll(text, color, "")
	}
	text = strings.ReplaceAll(text, colorReset, "")
	return text
}

// checkRotation checks if log rotation is needed
func (l *Logger) checkRotation() {
	if l.currentLogFile == nil || l.rotateConfig == nil {
		return
	}

	info, err := l.currentLogFile.Stat()
	if err != nil {
		return
	}

	// Check size-based rotation
	if info.Size() >= l.rotateConfig.MaxSize {
		l.rotateLog()
		return
	}

	// Check age-based rotation
	if time.Since(info.ModTime()) >= l.rotateConfig.MaxAge {
		l.rotateLog()
		return
	}
}

// rotateLog performs log rotation
func (l *Logger) rotateLog() {
	if l.currentLogFile == nil || l.rotateConfig == nil {
		return
	}

	// Close current file
	l.currentLogFile.Close()

	// Create timestamp for backup file
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	backupPath := filepath.Join(
		l.rotateConfig.LogDir,
		fmt.Sprintf("%s.%s", l.rotateConfig.Filename, timestamp),
	)

	// Rename current log file
	currentPath := filepath.Join(l.rotateConfig.LogDir, l.rotateConfig.Filename)
	if err := os.Rename(currentPath, backupPath); err != nil {
		log.Printf("Failed to rotate log file: %v", err)
		return
	}

	// Create new log file
	file, err := os.OpenFile(currentPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Printf("Failed to create new log file: %v", err)
		return
	}

	l.currentLogFile = file
	l.fileOutput = file

	// Clean up old backup files
	go l.cleanupBackups()
}

// cleanupBackups removes old backup files
func (l *Logger) cleanupBackups() {
	if l.rotateConfig == nil {
		return
	}

	pattern := filepath.Join(l.rotateConfig.LogDir, l.rotateConfig.Filename+".*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return
	}

	if len(matches) <= l.rotateConfig.MaxBackups {
		return
	}

	// Sort by modification time and remove oldest files
	type fileInfo struct {
		path    string
		modTime time.Time
	}

	var files []fileInfo
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}
		files = append(files, fileInfo{
			path:    match,
			modTime: info.ModTime(),
		})
	}

	// Sort by modification time (oldest first)
	for i := 0; i < len(files)-1; i++ {
		for j := i + 1; j < len(files); j++ {
			if files[i].modTime.After(files[j].modTime) {
				files[i], files[j] = files[j], files[i]
			}
		}
	}

	// Remove oldest files
	toRemove := len(files) - l.rotateConfig.MaxBackups
	for i := 0; i < toRemove; i++ {
		os.Remove(files[i].path)
	}
}

// Logger methods

// Debug logs a message at DEBUG level
func (l *Logger) Debug(args ...interface{}) {
	l.log(DEBUG, args...)
}

// Debugf logs a formatted message at DEBUG level
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.logf(DEBUG, format, args...)
}

// Info logs a message at INFO level
func (l *Logger) Info(args ...interface{}) {
	l.log(INFO, args...)
}

// Infof logs a formatted message at INFO level
func (l *Logger) Infof(format string, args ...interface{}) {
	l.logf(INFO, format, args...)
}

// Warn logs a message at WARN level
func (l *Logger) Warn(args ...interface{}) {
	l.log(WARN, args...)
}

// Warnf logs a formatted message at WARN level
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.logf(WARN, format, args...)
}

// Error logs a message at ERROR level
func (l *Logger) Error(args ...interface{}) {
	l.log(ERROR, args...)
}

// Errorf logs a formatted message at ERROR level
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.logf(ERROR, format, args...)
}

// Fatal logs a message at FATAL level and exits
func (l *Logger) Fatal(args ...interface{}) {
	l.log(FATAL, args...)
	os.Exit(1)
}

// Fatalf logs a formatted message at FATAL level and exits
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.logf(FATAL, format, args...)
	os.Exit(1)
}

// WithField adds a field to the logger
func (l *Logger) WithField(key string, value interface{}) *Logger {
	newLogger := *l
	newLogger.fields = make(map[string]interface{})
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	newLogger.fields[key] = value
	return &newLogger
}

// WithFields adds multiple fields to the logger
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	newLogger := *l
	newLogger.fields = make(map[string]interface{})
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	for k, v := range fields {
		newLogger.fields[k] = v
	}
	return &newLogger
}

// SetLevel sets the minimum log level
func (l *Logger) SetLevel(level LogLevel) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.level = level
}

// SetPrefix sets a prefix for all log messages
func (l *Logger) SetPrefix(prefix string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.prefix = prefix
}

// AddHook adds a hook function
func (l *Logger) AddHook(hook Hook) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.hooks = append(l.hooks, hook)
}

// Close closes the logger and any open files
func (l *Logger) Close() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.currentLogFile != nil {
		return l.currentLogFile.Close()
	}
	return nil
}

// Global logger functions

// GetLogger returns the global logger instance
func GetLogger() *Logger {
	globalMutex.RLock()
	defer globalMutex.RUnlock()
	return defaultLogger
}

// Debug logs a message at DEBUG level using the global logger
func Debug(args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Debug(args...)
	}
}

// Debugf logs a formatted message at DEBUG level using the global logger
func Debugf(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Debugf(format, args...)
	}
}

// Info logs a message at INFO level using the global logger
func Info(args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Info(args...)
	}
}

// Infof logs a formatted message at INFO level using the global logger
func Infof(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Infof(format, args...)
	}
}

// Warn logs a message at WARN level using the global logger
func Warn(args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Warn(args...)
	}
}

// Warnf logs a formatted message at WARN level using the global logger
func Warnf(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Warnf(format, args...)
	}
}

// Error logs a message at ERROR level using the global logger
func Error(args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Error(args...)
	}
}

// Errorf logs a formatted message at ERROR level using the global logger
func Errorf(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Errorf(format, args...)
	}
}

// Fatal logs a message at FATAL level using the global logger and exits
func Fatal(args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Fatal(args...)
	}
	os.Exit(1)
}

// Fatalf logs a formatted message at FATAL level using the global logger and exits
func Fatalf(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Fatalf(format, args...)
	}
	os.Exit(1)
}

// WithField returns a logger with the specified field
func WithField(key string, value interface{}) *Logger {
	if defaultLogger != nil {
		return defaultLogger.WithField(key, value)
	}
	return nil
}

// WithFields returns a logger with the specified fields
func WithFields(fields map[string]interface{}) *Logger {
	if defaultLogger != nil {
		return defaultLogger.WithFields(fields)
	}
	return nil
}

// SetLevel sets the global logger level
func SetLevel(level string) {
	if defaultLogger != nil {
		defaultLogger.SetLevel(parseLogLevel(level))
	}
}

// Chat Application Specific Logging Helpers

// LogUserAction logs user actions for audit purposes
func LogUserAction(userID, action, resource string, metadata map[string]interface{}) {
	fields := map[string]interface{}{
		"user_id":  userID,
		"action":   action,
		"resource": resource,
		"type":     "user_action",
	}

	for k, v := range metadata {
		fields[k] = v
	}

	WithFields(fields).Info("User action performed")
}

// LogAPIRequest logs API requests
func LogAPIRequest(method, path, userID, ip string, duration time.Duration, statusCode int) {
	fields := map[string]interface{}{
		"method":      method,
		"path":        path,
		"user_id":     userID,
		"ip":          ip,
		"duration_ms": duration.Milliseconds(),
		"status_code": statusCode,
		"type":        "api_request",
	}

	if statusCode >= 400 {
		WithFields(fields).Error("API request failed")
	} else {
		WithFields(fields).Info("API request completed")
	}
}

// LogDatabaseOperation logs database operations
func LogDatabaseOperation(operation, collection string, duration time.Duration, err error) {
	fields := map[string]interface{}{
		"operation":   operation,
		"collection":  collection,
		"duration_ms": duration.Milliseconds(),
		"type":        "database_operation",
	}

	if err != nil {
		fields["error"] = err.Error()
		WithFields(fields).Error("Database operation failed")
	} else {
		WithFields(fields).Debug("Database operation completed")
	}
}

// LogSecurityEvent logs security-related events
func LogSecurityEvent(event, userID, ip string, metadata map[string]interface{}) {
	fields := map[string]interface{}{
		"event":   event,
		"user_id": userID,
		"ip":      ip,
		"type":    "security_event",
	}

	for k, v := range metadata {
		fields[k] = v
	}

	WithFields(fields).Warn("Security event detected")
}

// LogCallEvent logs call-related events
func LogCallEvent(callID, event, userID string, participants []string, metadata map[string]interface{}) {
	fields := map[string]interface{}{
		"call_id":      callID,
		"event":        event,
		"user_id":      userID,
		"participants": participants,
		"type":         "call_event",
	}

	for k, v := range metadata {
		fields[k] = v
	}

	WithFields(fields).Info("Call event")
}

// Performance monitoring

// LogPerformanceMetric logs performance metrics
func LogPerformanceMetric(metric string, value float64, unit string, tags map[string]string) {
	fields := map[string]interface{}{
		"metric": metric,
		"value":  value,
		"unit":   unit,
		"type":   "performance_metric",
	}

	for k, v := range tags {
		fields[k] = v
	}

	WithFields(fields).Debug("Performance metric")
}

// LogSystemHealth logs system health information
func LogSystemHealth(component string, status string, metadata map[string]interface{}) {
	fields := map[string]interface{}{
		"component": component,
		"status":    status,
		"type":      "system_health",
	}

	for k, v := range metadata {
		fields[k] = v
	}

	if status == "healthy" {
		WithFields(fields).Debug("System component healthy")
	} else {
		WithFields(fields).Warn("System component unhealthy")
	}
}
