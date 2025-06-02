package logger

import (
	"io"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Logger represents the application logger
type Logger struct {
	*logrus.Logger
}

// Fields type for structured logging
type Fields map[string]interface{}

var (
	appLogger *Logger
)

// Config represents logger configuration
type Config struct {
	Level        string
	Format       string // "json" or "text"
	Output       string // "stdout", "stderr", or file path
	EnableCaller bool
	EnableColors bool
}

// DefaultConfig returns default logger configuration
func DefaultConfig() *Config {
	return &Config{
		Level:        "info",
		Format:       "json",
		Output:       "stdout",
		EnableCaller: true,
		EnableColors: false,
	}
}

// Init initializes the logger with default configuration
func Init() {
	config := DefaultConfig()
	InitWithConfig(config)
}

// InitWithConfig initializes the logger with custom configuration
func InitWithConfig(config *Config) {
	logger := logrus.New()

	// Set log level
	level, err := logrus.ParseLevel(config.Level)
	if err != nil {
		level = logrus.InfoLevel
		log.Printf("Invalid log level '%s', using 'info'", config.Level)
	}
	logger.SetLevel(level)

	// Set output
	var output io.Writer
	switch config.Output {
	case "stdout":
		output = os.Stdout
	case "stderr":
		output = os.Stderr
	default:
		if config.Output != "" {
			file, err := os.OpenFile(config.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if err != nil {
				log.Printf("Failed to open log file '%s': %v", config.Output, err)
				output = os.Stdout
			} else {
				output = file
			}
		} else {
			output = os.Stdout
		}
	}
	logger.SetOutput(output)

	// Set formatter
	if config.Format == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339,
			ForceColors:     config.EnableColors,
		})
	}

	// Enable caller reporting
	if config.EnableCaller {
		logger.SetReportCaller(true)
	}

	appLogger = &Logger{logger}

	Info("Logger initialized successfully")
}

// GetLogger returns the global logger instance
func GetLogger() *Logger {
	if appLogger == nil {
		Init() // Initialize with default config if not already initialized
	}
	return appLogger
}

// SetLevel sets the logging level
func SetLevel(level string) {
	if appLogger == nil {
		Init()
	}

	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		Errorf("Invalid log level '%s': %v", level, err)
		return
	}

	appLogger.SetLevel(logLevel)
	Infof("Log level set to %s", level)
}

// WithFields creates a logger entry with fields
func WithFields(fields Fields) *logrus.Entry {
	if appLogger == nil {
		Init()
	}
	return appLogger.WithFields(logrus.Fields(fields))
}

// WithField creates a logger entry with a single field
func WithField(key string, value interface{}) *logrus.Entry {
	if appLogger == nil {
		Init()
	}
	return appLogger.WithField(key, value)
}

// Logging methods

// Debug logs a debug message
func Debug(args ...interface{}) {
	if appLogger == nil {
		Init()
	}
	appLogger.Debug(args...)
}

// Debugf logs a formatted debug message
func Debugf(format string, args ...interface{}) {
	if appLogger == nil {
		Init()
	}
	appLogger.Debugf(format, args...)
}

// Info logs an info message
func Info(args ...interface{}) {
	if appLogger == nil {
		Init()
	}
	appLogger.Info(args...)
}

// Infof logs a formatted info message
func Infof(format string, args ...interface{}) {
	if appLogger == nil {
		Init()
	}
	appLogger.Infof(format, args...)
}

// Warn logs a warning message
func Warn(args ...interface{}) {
	if appLogger == nil {
		Init()
	}
	appLogger.Warn(args...)
}

// Warnf logs a formatted warning message
func Warnf(format string, args ...interface{}) {
	if appLogger == nil {
		Init()
	}
	appLogger.Warnf(format, args...)
}

// Error logs an error message
func Error(args ...interface{}) {
	if appLogger == nil {
		Init()
	}
	appLogger.Error(args...)
}

// Errorf logs a formatted error message
func Errorf(format string, args ...interface{}) {
	if appLogger == nil {
		Init()
	}
	appLogger.Errorf(format, args...)
}

// Fatal logs a fatal message and exits
func Fatal(args ...interface{}) {
	if appLogger == nil {
		Init()
	}
	appLogger.Fatal(args...)
}

// Fatalf logs a formatted fatal message and exits
func Fatalf(format string, args ...interface{}) {
	if appLogger == nil {
		Init()
	}
	appLogger.Fatalf(format, args...)
}

// Panic logs a panic message and panics
func Panic(args ...interface{}) {
	if appLogger == nil {
		Init()
	}
	appLogger.Panic(args...)
}

// Panicf logs a formatted panic message and panics
func Panicf(format string, args ...interface{}) {
	if appLogger == nil {
		Init()
	}
	appLogger.Panicf(format, args...)
}

// Specialized logging methods

// LogHTTPRequest logs HTTP request details
func LogHTTPRequest(method, path, userAgent, ip string, statusCode int, duration time.Duration) {
	WithFields(Fields{
		"method":      method,
		"path":        path,
		"user_agent":  userAgent,
		"ip":          ip,
		"status_code": statusCode,
		"duration_ms": duration.Milliseconds(),
		"type":        "http_request",
	}).Info("HTTP request processed")
}

// LogUserAction logs user actions for audit
func LogUserAction(userID, action, resource string, metadata Fields) {
	fields := Fields{
		"user_id":  userID,
		"action":   action,
		"resource": resource,
		"type":     "user_action",
	}

	// Merge metadata
	for k, v := range metadata {
		fields[k] = v
	}

	WithFields(fields).Info("User action logged")
}

// LogSecurityEvent logs security-related events
func LogSecurityEvent(event, userID, ip string, details Fields) {
	fields := Fields{
		"event":   event,
		"user_id": userID,
		"ip":      ip,
		"type":    "security_event",
	}

	// Merge details
	for k, v := range details {
		fields[k] = v
	}

	WithFields(fields).Warn("Security event detected")
}

// LogSystemEvent logs system events
func LogSystemEvent(event, component string, details Fields) {
	fields := Fields{
		"event":     event,
		"component": component,
		"type":      "system_event",
	}

	// Merge details
	for k, v := range details {
		fields[k] = v
	}

	WithFields(fields).Info("System event logged")
}

// LogDatabaseOperation logs database operations
func LogDatabaseOperation(operation, collection string, duration time.Duration, err error) {
	fields := Fields{
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

// LogWebSocketEvent logs WebSocket events
func LogWebSocketEvent(event, userID, connectionID string, details Fields) {
	fields := Fields{
		"event":         event,
		"user_id":       userID,
		"connection_id": connectionID,
		"type":          "websocket_event",
	}

	// Merge details
	for k, v := range details {
		fields[k] = v
	}

	WithFields(fields).Debug("WebSocket event logged")
}

// LogWebRTCEvent logs WebRTC events
func LogWebRTCEvent(event, callID, userID string, details Fields) {
	fields := Fields{
		"event":   event,
		"call_id": callID,
		"user_id": userID,
		"type":    "webrtc_event",
	}

	// Merge details
	for k, v := range details {
		fields[k] = v
	}

	WithFields(fields).Info("WebRTC event logged")
}

// LogAPICall logs API calls
func LogAPICall(method, endpoint, userID string, requestSize, responseSize int64, duration time.Duration, statusCode int) {
	fields := Fields{
		"method":        method,
		"endpoint":      endpoint,
		"user_id":       userID,
		"request_size":  requestSize,
		"response_size": responseSize,
		"duration_ms":   duration.Milliseconds(),
		"status_code":   statusCode,
		"type":          "api_call",
	}

	level := logrus.InfoLevel
	if statusCode >= 400 {
		level = logrus.ErrorLevel
	} else if statusCode >= 300 {
		level = logrus.WarnLevel
	}

	appLogger.WithFields(logrus.Fields(fields)).Log(level, "API call processed")
}

// LogFileOperation logs file operations
func LogFileOperation(operation, filename, userID string, fileSize int64, err error) {
	fields := Fields{
		"operation": operation,
		"filename":  filename,
		"user_id":   userID,
		"file_size": fileSize,
		"type":      "file_operation",
	}

	if err != nil {
		fields["error"] = err.Error()
		WithFields(fields).Error("File operation failed")
	} else {
		WithFields(fields).Info("File operation completed")
	}
}

// LogPushNotification logs push notification events
func LogPushNotification(provider, userID, deviceToken string, success bool, err error) {
	fields := Fields{
		"provider":     provider,
		"user_id":      userID,
		"device_token": maskToken(deviceToken),
		"success":      success,
		"type":         "push_notification",
	}

	if err != nil {
		fields["error"] = err.Error()
		WithFields(fields).Error("Push notification failed")
	} else {
		WithFields(fields).Info("Push notification sent")
	}
}

// LogSMSOperation logs SMS operations
func LogSMSOperation(provider, phoneNumber string, success bool, err error) {
	fields := Fields{
		"provider":     provider,
		"phone_number": maskPhoneNumber(phoneNumber),
		"success":      success,
		"type":         "sms_operation",
	}

	if err != nil {
		fields["error"] = err.Error()
		WithFields(fields).Error("SMS operation failed")
	} else {
		WithFields(fields).Info("SMS sent successfully")
	}
}

// LogEmailOperation logs email operations
func LogEmailOperation(provider, recipient string, subject string, success bool, err error) {
	fields := Fields{
		"provider":  provider,
		"recipient": maskEmail(recipient),
		"subject":   subject,
		"success":   success,
		"type":      "email_operation",
	}

	if err != nil {
		fields["error"] = err.Error()
		WithFields(fields).Error("Email operation failed")
	} else {
		WithFields(fields).Info("Email sent successfully")
	}
}

// Performance logging

// LogPerformance logs performance metrics
func LogPerformance(operation string, duration time.Duration, metadata Fields) {
	fields := Fields{
		"operation":   operation,
		"duration_ms": duration.Milliseconds(),
		"type":        "performance",
	}

	// Merge metadata
	for k, v := range metadata {
		fields[k] = v
	}

	level := logrus.InfoLevel
	if duration > 5*time.Second {
		level = logrus.WarnLevel
	} else if duration > 10*time.Second {
		level = logrus.ErrorLevel
	}

	appLogger.WithFields(logrus.Fields(fields)).Log(level, "Performance metric logged")
}

// LogMemoryUsage logs memory usage
func LogMemoryUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	WithFields(Fields{
		"alloc_mb":       bToMb(m.Alloc),
		"total_alloc_mb": bToMb(m.TotalAlloc),
		"sys_mb":         bToMb(m.Sys),
		"num_gc":         m.NumGC,
		"type":           "memory_usage",
	}).Debug("Memory usage logged")
}

// LogGoroutineCount logs current goroutine count
func LogGoroutineCount() {
	count := runtime.NumGoroutine()
	WithFields(Fields{
		"goroutine_count": count,
		"type":            "goroutine_count",
	}).Debug("Goroutine count logged")
}

// Utility functions

// maskToken masks sensitive token for logging
func maskToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "***" + token[len(token)-4:]
}

// maskPhoneNumber masks phone number for logging
func maskPhoneNumber(phone string) string {
	if len(phone) <= 4 {
		return "***"
	}
	return phone[:2] + "***" + phone[len(phone)-2:]
}

// maskEmail masks email for logging
func maskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "***"
	}

	username := parts[0]
	domain := parts[1]

	if len(username) <= 2 {
		return "***@" + domain
	}

	return username[:1] + "***" + username[len(username)-1:] + "@" + domain
}

// bToMb converts bytes to megabytes
func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}

// Context-aware logging

// RequestLogger creates a logger with request context
func RequestLogger(requestID, userID, ip string) *logrus.Entry {
	return WithFields(Fields{
		"request_id": requestID,
		"user_id":    userID,
		"ip":         ip,
	})
}

// ErrorWithStack logs error with stack trace
func ErrorWithStack(err error, message string) {
	if appLogger == nil {
		Init()
	}

	stack := make([]byte, 4096)
	length := runtime.Stack(stack, false)

	WithFields(Fields{
		"error": err.Error(),
		"stack": string(stack[:length]),
		"type":  "error_with_stack",
	}).Error(message)
}

// LogStartup logs application startup information
func LogStartup(appName, version, environment string, port int) {
	WithFields(Fields{
		"app_name":    appName,
		"version":     version,
		"environment": environment,
		"port":        port,
		"type":        "startup",
	}).Info("Application started")
}

// LogShutdown logs application shutdown
func LogShutdown(reason string) {
	WithFields(Fields{
		"reason": reason,
		"type":   "shutdown",
	}).Info("Application shutting down")
}

// Configuration helpers

// SetJSONFormat sets JSON formatter
func SetJSONFormat() {
	if appLogger == nil {
		Init()
	}
	appLogger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339,
	})
}

// SetTextFormat sets text formatter
func SetTextFormat(colors bool) {
	if appLogger == nil {
		Init()
	}
	appLogger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: time.RFC3339,
		ForceColors:     colors,
	})
}

// SetOutput sets logger output
func SetOutput(output io.Writer) {
	if appLogger == nil {
		Init()
	}
	appLogger.SetOutput(output)
}

// GetLevel returns current log level
func GetLevel() logrus.Level {
	if appLogger == nil {
		Init()
	}
	return appLogger.GetLevel()
}

// IsDebugEnabled returns true if debug logging is enabled
func IsDebugEnabled() bool {
	return GetLevel() >= logrus.DebugLevel
}

// Close closes the logger (if logging to file)
func Close() {
	if appLogger != nil {
		Info("Logger shutting down")
		// If we were logging to a file, we could close it here
		// For now, just log the shutdown
	}
}
