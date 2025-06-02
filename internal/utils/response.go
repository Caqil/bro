package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
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

// Response Formatting Helpers

// FormatFileSize formats file size in human readable format
func FormatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// FormatDuration formats duration in human readable format
func FormatDuration(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%dh %dm", seconds/3600, (seconds%3600)/60)
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
