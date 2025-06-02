package utils

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Standard API Response Structures

// APIResponse represents a standard API response
type APIResponse struct {
	Success   bool        `json:"success"`
	Message   string      `json:"message,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Error     *APIError   `json:"error,omitempty"`
	Meta      *Meta       `json:"meta,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

// APIError represents an error in API response
type APIError struct {
	Code       string                 `json:"code"`
	Message    string                 `json:"message"`
	Details    string                 `json:"details,omitempty"`
	Fields     map[string]string      `json:"fields,omitempty"`
	Validation map[string]interface{} `json:"validation,omitempty"`
	TraceID    string                 `json:"trace_id,omitempty"`
}

// Meta represents metadata for responses (pagination, etc.)
type Meta struct {
	Page       int   `json:"page,omitempty"`
	Limit      int   `json:"limit,omitempty"`
	Total      int64 `json:"total,omitempty"`
	TotalPages int   `json:"total_pages,omitempty"`
	HasNext    bool  `json:"has_next,omitempty"`
	HasPrev    bool  `json:"has_prev,omitempty"`
}

// PaginationRequest represents pagination parameters
type PaginationRequest struct {
	Page     int    `form:"page" json:"page"`
	Limit    int    `form:"limit" json:"limit"`
	SortBy   string `form:"sort_by" json:"sort_by"`
	SortDir  string `form:"sort_dir" json:"sort_dir"`
	Search   string `form:"search" json:"search"`
	Filter   string `form:"filter" json:"filter"`
	DateFrom string `form:"date_from" json:"date_from"`
	DateTo   string `form:"date_to" json:"date_to"`
}

// PaginationResult represents paginated results
type PaginationResult struct {
	Data       interface{} `json:"data"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	Limit      int         `json:"limit"`
	TotalPages int         `json:"total_pages"`
	HasNext    bool        `json:"has_next"`
	HasPrev    bool        `json:"has_prev"`
}

// Error Codes
const (
	ErrCodeValidation     = "VALIDATION_ERROR"
	ErrCodeAuthentication = "AUTHENTICATION_ERROR"
	ErrCodeAuthorization  = "AUTHORIZATION_ERROR"
	ErrCodeNotFound       = "NOT_FOUND"
	ErrCodeConflict       = "CONFLICT_ERROR"
	ErrCodeInternal       = "INTERNAL_ERROR"
	ErrCodeBadRequest     = "BAD_REQUEST"
	ErrCodeTooManyRequest = "TOO_MANY_REQUESTS"
	ErrCodeServiceUnavail = "SERVICE_UNAVAILABLE"
	ErrCodeTimeout        = "TIMEOUT_ERROR"
	ErrCodeInvalidToken   = "INVALID_TOKEN"
	ErrCodeExpiredToken   = "EXPIRED_TOKEN"
	ErrCodeInvalidOTP     = "INVALID_OTP"
	ErrCodeExpiredOTP     = "EXPIRED_OTP"
	ErrCodeFileTooBig     = "FILE_TOO_BIG"
	ErrCodeInvalidFile    = "INVALID_FILE_TYPE"
	ErrCodeQuotaExceeded  = "QUOTA_EXCEEDED"
)

// Success Response Helpers

// Success sends a successful response
func Success(c *gin.Context, data interface{}) {
	response := APIResponse{
		Success:   true,
		Data:      data,
		Timestamp: time.Now().Unix(),
	}
	c.JSON(http.StatusOK, response)
}

// SuccessWithMessage sends a successful response with message
func SuccessWithMessage(c *gin.Context, message string, data interface{}) {
	response := APIResponse{
		Success:   true,
		Message:   message,
		Data:      data,
		Timestamp: time.Now().Unix(),
	}
	c.JSON(http.StatusOK, response)
}

// SuccessWithMeta sends a successful response with metadata
func SuccessWithMeta(c *gin.Context, data interface{}, meta *Meta) {
	response := APIResponse{
		Success:   true,
		Data:      data,
		Meta:      meta,
		Timestamp: time.Now().Unix(),
	}
	c.JSON(http.StatusOK, response)
}

// Created sends a 201 created response
func Created(c *gin.Context, data interface{}) {
	response := APIResponse{
		Success:   true,
		Message:   "Resource created successfully",
		Data:      data,
		Timestamp: time.Now().Unix(),
	}
	c.JSON(http.StatusCreated, response)
}

// NoContent sends a 204 no content response
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// Error Response Helpers

// Error sends an error response
func Error(c *gin.Context, statusCode int, code, message string) {
	response := APIResponse{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
		},
		Timestamp: time.Now().Unix(),
	}
	c.JSON(statusCode, response)
}

// ErrorWithDetails sends an error response with details
func ErrorWithDetails(c *gin.Context, statusCode int, code, message, details string) {
	response := APIResponse{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
			Details: details,
		},
		Timestamp: time.Now().Unix(),
	}
	c.JSON(statusCode, response)
}

// ValidationErrorResponse sends a validation error response using ValidationError from validation.go
func ValidationErrorResponse(c *gin.Context, errors map[string]ValidationError) {
	fields := make(map[string]string)
	for field, err := range errors {
		fields[field] = err.Message
	}

	response := APIResponse{
		Success: false,
		Error: &APIError{
			Code:    ErrCodeValidation,
			Message: "Validation failed",
			Fields:  fields,
		},
		Timestamp: time.Now().Unix(),
	}
	c.JSON(http.StatusBadRequest, response)
}

// SendValidationError sends a validation error response (legacy method)
func SendValidationError(c *gin.Context, fields map[string]string) {
	response := APIResponse{
		Success: false,
		Error: &APIError{
			Code:    ErrCodeValidation,
			Message: "Validation failed",
			Fields:  fields,
		},
		Timestamp: time.Now().Unix(),
	}
	c.JSON(http.StatusBadRequest, response)
}

// BadRequest sends a 400 bad request response
func BadRequest(c *gin.Context, message string) {
	Error(c, http.StatusBadRequest, ErrCodeBadRequest, message)
}

// Unauthorized sends a 401 unauthorized response
func Unauthorized(c *gin.Context, message string) {
	if message == "" {
		message = "Authentication required"
	}
	Error(c, http.StatusUnauthorized, ErrCodeAuthentication, message)
}

// Forbidden sends a 403 forbidden response
func Forbidden(c *gin.Context, message string) {
	if message == "" {
		message = "Access denied"
	}
	Error(c, http.StatusForbidden, ErrCodeAuthorization, message)
}

// NotFound sends a 404 not found response
func NotFound(c *gin.Context, message string) {
	if message == "" {
		message = "Resource not found"
	}
	Error(c, http.StatusNotFound, ErrCodeNotFound, message)
}

// Conflict sends a 409 conflict response
func Conflict(c *gin.Context, message string) {
	Error(c, http.StatusConflict, ErrCodeConflict, message)
}

// TooManyRequests sends a 429 too many requests response
func TooManyRequests(c *gin.Context, message string) {
	if message == "" {
		message = "Too many requests"
	}
	Error(c, http.StatusTooManyRequests, ErrCodeTooManyRequest, message)
}

// InternalServerError sends a 500 internal server error response
func InternalServerError(c *gin.Context, message string) {
	if message == "" {
		message = "Internal server error"
	}
	Error(c, http.StatusInternalServerError, ErrCodeInternal, message)
}

// ServiceUnavailable sends a 503 service unavailable response
func ServiceUnavailable(c *gin.Context, message string) {
	if message == "" {
		message = "Service temporarily unavailable"
	}
	Error(c, http.StatusServiceUnavailable, ErrCodeServiceUnavail, message)
}

// Specific Error Helpers

// InvalidToken sends an invalid token error
func InvalidToken(c *gin.Context) {
	Error(c, http.StatusUnauthorized, ErrCodeInvalidToken, "Invalid or malformed token")
}

// ExpiredToken sends an expired token error
func ExpiredToken(c *gin.Context) {
	Error(c, http.StatusUnauthorized, ErrCodeExpiredToken, "Token has expired")
}

// InvalidOTP sends an invalid OTP error
func InvalidOTP(c *gin.Context) {
	Error(c, http.StatusBadRequest, ErrCodeInvalidOTP, "Invalid OTP code")
}

// ExpiredOTP sends an expired OTP error
func ExpiredOTP(c *gin.Context) {
	Error(c, http.StatusBadRequest, ErrCodeExpiredOTP, "OTP code has expired")
}

// FileTooBig sends a file too big error
func FileTooBig(c *gin.Context, maxSize string) {
	message := fmt.Sprintf("File size exceeds maximum allowed size of %s", maxSize)
	Error(c, http.StatusBadRequest, ErrCodeFileTooBig, message)
}

// InvalidFileType sends an invalid file type error
func InvalidFileType(c *gin.Context, allowedTypes []string) {
	message := fmt.Sprintf("Invalid file type. Allowed types: %s", strings.Join(allowedTypes, ", "))
	Error(c, http.StatusBadRequest, ErrCodeInvalidFile, message)
}

// QuotaExceeded sends a quota exceeded error
func QuotaExceeded(c *gin.Context, quota string) {
	message := fmt.Sprintf("Quota exceeded: %s", quota)
	Error(c, http.StatusForbidden, ErrCodeQuotaExceeded, message)
}

// Pagination Helpers

// GetPaginationParams extracts pagination parameters from request
func GetPaginationParams(c *gin.Context) PaginationRequest {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	// Ensure valid pagination values
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	sortDir := c.DefaultQuery("sort_dir", "desc")
	if sortDir != "asc" && sortDir != "desc" {
		sortDir = "desc"
	}

	return PaginationRequest{
		Page:     page,
		Limit:    limit,
		SortBy:   c.DefaultQuery("sort_by", "created_at"),
		SortDir:  sortDir,
		Search:   c.Query("search"),
		Filter:   c.Query("filter"),
		DateFrom: c.Query("date_from"),
		DateTo:   c.Query("date_to"),
	}
}

// CreatePaginationMeta creates pagination metadata
func CreatePaginationMeta(page, limit int, total int64) *Meta {
	totalPages := int((total + int64(limit) - 1) / int64(limit))

	return &Meta{
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
		HasNext:    page < totalPages,
		HasPrev:    page > 1,
	}
}

// PaginatedResponse sends a paginated response
func PaginatedResponse(c *gin.Context, data interface{}, page, limit int, total int64) {
	meta := CreatePaginationMeta(page, limit, total)
	SuccessWithMeta(c, data, meta)
}

// JSON Utilities

// ParseJSON parses JSON from request body
func ParseJSON(c *gin.Context, v interface{}) error {
	if err := c.ShouldBindJSON(v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

// ParseQuery parses query parameters
func ParseQuery(c *gin.Context, v interface{}) error {
	if err := c.ShouldBindQuery(v); err != nil {
		return fmt.Errorf("invalid query parameters: %w", err)
	}
	return nil
}

// ParseForm parses form data
func ParseForm(c *gin.Context, v interface{}) error {
	if err := c.ShouldBind(v); err != nil {
		return fmt.Errorf("invalid form data: %w", err)
	}
	return nil
}

// GetUserIDFromContext extracts user ID from context
func GetUserIDFromContext(c *gin.Context) (primitive.ObjectID, error) {
	userIDStr, exists := c.Get("user_id")
	if !exists {
		return primitive.NilObjectID, fmt.Errorf("user ID not found in context")
	}

	switch v := userIDStr.(type) {
	case string:
		return primitive.ObjectIDFromHex(v)
	case primitive.ObjectID:
		return v, nil
	default:
		return primitive.NilObjectID, fmt.Errorf("invalid user ID type in context")
	}
}

// GetUserRoleFromContext extracts user role from context
func GetUserRoleFromContext(c *gin.Context) (string, error) {
	role, exists := c.Get("user_role")
	if !exists {
		return "", fmt.Errorf("user role not found in context")
	}

	roleStr, ok := role.(string)
	if !ok {
		return "", fmt.Errorf("invalid user role type in context")
	}

	return roleStr, nil
}

// Sanitize response data by removing sensitive fields
func SanitizeUserData(data interface{}) interface{} {
	// This is a simple implementation - in production you might want more sophisticated sanitization
	if jsonData, err := json.Marshal(data); err == nil {
		var result interface{}
		if err := json.Unmarshal(jsonData, &result); err == nil {
			sanitizeMap(result)
			return result
		}
	}
	return data
}

func sanitizeMap(data interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		// Remove sensitive fields
		sensitiveFields := []string{
			"password", "password_hash", "otp", "two_factor_secret",
			"encryption_key", "api_key", "secret", "token", "auth_token",
		}
		for _, field := range sensitiveFields {
			delete(v, field)
		}
		// Recursively sanitize nested objects
		for _, value := range v {
			sanitizeMap(value)
		}
	case []interface{}:
		for _, item := range v {
			sanitizeMap(item)
		}
	}
}

// Health Check Response
type HealthCheckResponse struct {
	Status    string                 `json:"status"`
	Timestamp int64                  `json:"timestamp"`
	Version   string                 `json:"version"`
	Services  map[string]ServiceInfo `json:"services"`
	Uptime    string                 `json:"uptime"`
}

type ServiceInfo struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// HealthCheck sends a health check response
func HealthCheck(c *gin.Context, version string, services map[string]ServiceInfo, uptime time.Duration) {
	status := "healthy"
	for _, service := range services {
		if service.Status != "healthy" {
			status = "unhealthy"
			break
		}
	}

	response := HealthCheckResponse{
		Status:    status,
		Timestamp: time.Now().Unix(),
		Version:   version,
		Services:  services,
		Uptime:    uptime.String(),
	}

	statusCode := http.StatusOK
	if status == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, response)
}

// WebSocket Response Helpers

// WSMessage represents a WebSocket message
type WSMessage struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	Timestamp int64       `json:"timestamp"`
	ID        string      `json:"id,omitempty"`
}

// CreateWSMessage creates a WebSocket message
func CreateWSMessage(msgType string, data interface{}) *WSMessage {
	return &WSMessage{
		Type:      msgType,
		Data:      data,
		Timestamp: time.Now().Unix(),
		ID:        primitive.NewObjectID().Hex(),
	}
}

// WSError represents a WebSocket error message
type WSError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// CreateWSError creates a WebSocket error message
func CreateWSError(code, message, details string) *WSMessage {
	return &WSMessage{
		Type: "error",
		Data: WSError{
			Code:    code,
			Message: message,
			Details: details,
		},
		Timestamp: time.Now().Unix(),
	}
}

// Rate Limiting Response

// RateLimitExceeded sends a rate limit exceeded response with retry after header
func RateLimitExceeded(c *gin.Context, retryAfter int) {
	c.Header("Retry-After", strconv.Itoa(retryAfter))
	TooManyRequests(c, fmt.Sprintf("Rate limit exceeded. Try again in %d seconds", retryAfter))
}

// Custom Response Types

// AuthResponse represents authentication response
type AuthResponse struct {
	User         interface{} `json:"user"`
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	ExpiresAt    int64       `json:"expires_at"`
	TokenType    string      `json:"token_type"`
}

// MessageResponse represents a standard message response
type MessageResponse struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// StatusResponse represents a simple status response
type StatusResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// Helper Functions

// GetClientIP gets the real client IP
func GetClientIP(c *gin.Context) string {
	// Check X-Forwarded-For header
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}

	// Check X-Real-IP header
	if xri := c.GetHeader("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	return c.ClientIP()
}

// GetUserAgent gets the user agent
func GetUserAgent(c *gin.Context) string {
	return c.GetHeader("User-Agent")
}

// IsAPIRequest checks if request is an API request
func IsAPIRequest(c *gin.Context) bool {
	return strings.HasPrefix(c.Request.URL.Path, "/api/")
}

// CORS Response

// SetCORSHeaders sets CORS headers
func SetCORSHeaders(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Requested-With")
	c.Header("Access-Control-Expose-Headers", "Content-Length")
	c.Header("Access-Control-Allow-Credentials", "true")
	c.Header("Access-Control-Max-Age", "43200")
}

// Security Headers

// SetSecurityHeaders sets security headers
func SetSecurityHeaders(c *gin.Context) {
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "DENY")
	c.Header("X-XSS-Protection", "1; mode=block")
	c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	c.Header("Content-Security-Policy", "default-src 'self'")
}
func ParseISO8601(timeStr string) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, timeStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse time string: %s", timeStr)
}

// GetCurrentTime returns current time
func GetCurrentTime() time.Time {
	return time.Now()
}

// FormatDuration formats duration in human readable format
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		if seconds > 0 {
			return fmt.Sprintf("%dm %ds", minutes, seconds)
		}
		return fmt.Sprintf("%dm", minutes)
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if minutes > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dh", hours)
}

// FormatTimeAgo formats time as "time ago" string
func FormatTimeAgo(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	if diff < time.Minute {
		return "just now"
	}
	if diff < time.Hour {
		minutes := int(diff.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	}
	if diff < 24*time.Hour {
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}
	if diff < 7*24*time.Hour {
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
	if diff < 30*24*time.Hour {
		weeks := int(diff.Hours() / (24 * 7))
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	}
	if diff < 365*24*time.Hour {
		months := int(diff.Hours() / (24 * 30))
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	}

	years := int(diff.Hours() / (24 * 365))
	if years == 1 {
		return "1 year ago"
	}
	return fmt.Sprintf("%d years ago", years)
}

// Size formatting utilities

// FormatFileSize formats file size in human readable format
func FormatFileSize(bytes int64) string {
	if bytes == 0 {
		return "0 B"
	}

	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	base := float64(1024)

	logBase := math.Log(float64(bytes)) / math.Log(base)
	unitIndex := int(logBase)

	if unitIndex >= len(units) {
		unitIndex = len(units) - 1
	}

	value := float64(bytes) / math.Pow(base, float64(unitIndex))

	if value >= 100 {
		return fmt.Sprintf("%.0f %s", value, units[unitIndex])
	} else if value >= 10 {
		return fmt.Sprintf("%.1f %s", value, units[unitIndex])
	} else {
		return fmt.Sprintf("%.2f %s", value, units[unitIndex])
	}
}

// FormatBytes formats bytes with specific unit
func FormatBytes(bytes int64, unit string) string {
	switch unit {
	case "KB":
		return fmt.Sprintf("%.2f KB", float64(bytes)/1024)
	case "MB":
		return fmt.Sprintf("%.2f MB", float64(bytes)/(1024*1024))
	case "GB":
		return fmt.Sprintf("%.2f GB", float64(bytes)/(1024*1024*1024))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// String utilities

// TruncateString truncates string to specified length with ellipsis
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// SanitizeFileName sanitizes filename for safe storage
func SanitizeFileName(filename string) string {
	// Remove or replace dangerous characters
	dangerous := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", "\x00"}
	sanitized := filename

	for _, char := range dangerous {
		sanitized = strings.ReplaceAll(sanitized, char, "_")
	}

	// Trim whitespace and dots
	sanitized = strings.Trim(sanitized, " .")

	// Ensure filename is not empty
	if sanitized == "" {
		sanitized = "unnamed_file"
	}

	// Limit length
	if len(sanitized) > 255 {
		sanitized = sanitized[:255]
	}

	return sanitized
}

// ExtractFileExtension extracts file extension from filename
func ExtractFileExtension(filename string) string {
	lastDot := strings.LastIndex(filename, ".")
	if lastDot == -1 || lastDot == len(filename)-1 {
		return ""
	}
	return strings.ToLower(filename[lastDot+1:])
}

// Numeric utilities

// MinInt returns minimum of two integers
func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// MaxInt returns maximum of two integers
func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// MinInt64 returns minimum of two int64 values
func MinInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// MaxInt64 returns maximum of two int64 values
func MaxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// ClampInt clamps integer value between min and max
func ClampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// ClampInt64 clamps int64 value between min and max
func ClampInt64(value, min, max int64) int64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// RoundToDecimalPlaces rounds float to specified decimal places
func RoundToDecimalPlaces(value float64, places int) float64 {
	multiplier := math.Pow(10, float64(places))
	return math.Round(value*multiplier) / multiplier
}

// Percentage calculates percentage
func Percentage(part, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}

// Collection utilities

// Contains checks if slice contains item
func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// ContainsInt checks if int slice contains item
func ContainsInt(slice []int, item int) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// RemoveDuplicates removes duplicate strings from slice
func RemoveDuplicates(slice []string) []string {
	seen := make(map[string]bool)
	result := []string{}

	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}

// ChunkSlice splits slice into chunks of specified size
func ChunkSlice(slice []string, chunkSize int) [][]string {
	if chunkSize <= 0 {
		return [][]string{slice}
	}

	var chunks [][]string
	for i := 0; i < len(slice); i += chunkSize {
		end := i + chunkSize
		if end > len(slice) {
			end = len(slice)
		}
		chunks = append(chunks, slice[i:end])
	}

	return chunks
}

// Map utilities

// MergeStringMaps merges multiple string maps
func MergeStringMaps(maps ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

// GetMapKeys returns keys from string map
func GetMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Network utilities

// IsValidPort checks if port number is valid
func IsValidPort(port int) bool {
	return port > 0 && port <= 65535
}

// IsPrivateIP checks if IP address is private
func IsPrivateIP(ip string) bool {
	// Simple check - in production you'd use proper IP parsing
	return strings.HasPrefix(ip, "10.") ||
		strings.HasPrefix(ip, "192.168.") ||
		strings.HasPrefix(ip, "172.") ||
		strings.HasPrefix(ip, "127.")
}

// Security utilities

// MaskPhoneNumber masks phone number for display
func MaskPhoneNumber(phone string) string {
	if len(phone) < 4 {
		return "***"
	}
	return phone[:3] + strings.Repeat("*", len(phone)-6) + phone[len(phone)-3:]
}

// MaskEmail masks email address for display
func MaskEmail(email string) string {
	atIndex := strings.Index(email, "@")
	if atIndex == -1 || atIndex < 2 {
		return "***@***"
	}

	username := email[:atIndex]
	domain := email[atIndex:]

	if len(username) <= 2 {
		return "**" + domain
	}

	masked := username[:1] + strings.Repeat("*", len(username)-2) + username[len(username)-1:] + domain
	return masked
}

// IsValidHTTPURL checks if string is valid HTTP/HTTPS URL
func IsValidHTTPURL(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

// Error handling utilities

// SafeStringAccess safely accesses string from interface
func SafeStringAccess(data interface{}, defaultValue string) string {
	if str, ok := data.(string); ok {
		return str
	}
	return defaultValue
}

// SafeIntAccess safely accesses int from interface
func SafeIntAccess(data interface{}, defaultValue int) int {
	switch v := data.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return defaultValue
	}
}

// SafeBoolAccess safely accesses bool from interface
func SafeBoolAccess(data interface{}, defaultValue bool) bool {
	if b, ok := data.(bool); ok {
		return b
	}
	return defaultValue
}

// Configuration utilities

// GetEnvWithDefault gets environment variable with default value
func GetEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// ParseBoolFromString parses boolean from string
func ParseBoolFromString(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	return lower == "true" || lower == "yes" || lower == "1" || lower == "on"
}

// Conversion utilities

// IntToString converts int to string
func IntToString(i int) string {
	return strconv.Itoa(i)
}

// StringToInt converts string to int with default
func StringToInt(s string, defaultValue int) int {
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	return defaultValue
}

// Int64ToString converts int64 to string
func Int64ToString(i int64) string {
	return strconv.FormatInt(i, 10)
}

// StringToInt64 converts string to int64 with default
func StringToInt64(s string, defaultValue int64) int64 {
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	return defaultValue
}
