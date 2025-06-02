package utils

import (
	"fmt"
	"mime"
	"net/mail"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code"`
	Value   string `json:"value,omitempty"`
}

// ValidationResult represents the result of validation
type ValidationResult struct {
	IsValid bool                       `json:"is_valid"`
	Errors  map[string]ValidationError `json:"errors,omitempty"`
}

// Validator interface for custom validators
type Validator interface {
	Validate(value interface{}) *ValidationError
}

// ValidationRules represents validation rules for a field
type ValidationRules struct {
	Required     bool
	MinLength    int
	MaxLength    int
	MinValue     float64
	MaxValue     float64
	Pattern      string
	CustomRules  []Validator
	AllowedTypes []string
	MaxSize      int64
}

// Common Validation Patterns
var (
	// Phone number patterns for different regions
	PhonePatterns = map[string]*regexp.Regexp{
		"US":     regexp.MustCompile(`^\+1[2-9]\d{2}[2-9]\d{2}\d{4}$`),
		"UK":     regexp.MustCompile(`^\+44[1-9]\d{8,9}$`),
		"IN":     regexp.MustCompile(`^\+91[6-9]\d{9}$`),
		"ID":     regexp.MustCompile(`^\+62[8]\d{8,11}$`),
		"GLOBAL": regexp.MustCompile(`^\+[1-9]\d{1,14}$`),
	}

	// Email pattern
	EmailPattern = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

	// Username pattern (alphanumeric + underscore, 3-30 chars)
	UsernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]{3,30}$`)

	// Strong password pattern
	StrongPasswordPattern = regexp.MustCompile(`^(?=.*[a-z])(?=.*[A-Z])(?=.*\d)(?=.*[@$!%*?&])[A-Za-z\d@$!%*?&]{8,}$`)

	// URL pattern
	URLPattern = regexp.MustCompile(`^https?://[^\s/$.?#].[^\s]*$`)

	// IPv4 pattern
	IPv4Pattern = regexp.MustCompile(`^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`)

	// IPv6 pattern (simplified)
	IPv6Pattern = regexp.MustCompile(`^(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$`)

	// MAC address pattern
	MACPattern = regexp.MustCompile(`^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`)

	// Hex color pattern
	HexColorPattern = regexp.MustCompile(`^#([A-Fa-f0-9]{6}|[A-Fa-f0-9]{3})$`)

	// Base64 pattern
	Base64Pattern = regexp.MustCompile(`^[A-Za-z0-9+/]*={0,2}$`)

	// Credit card pattern (basic)
	CreditCardPattern = regexp.MustCompile(`^[0-9]{13,19}$`)
)

// Validation Error Codes
const (
	ErrRequired           = "REQUIRED"
	ErrInvalidFormat      = "INVALID_FORMAT"
	ErrTooShort           = "TOO_SHORT"
	ErrTooLong            = "TOO_LONG"
	ErrTooSmall           = "TOO_SMALL"
	ErrTooLarge           = "TOO_LARGE"
	ErrInvalidEmail       = "INVALID_EMAIL"
	ErrInvalidPhone       = "INVALID_PHONE"
	ErrInvalidPassword    = "INVALID_PASSWORD"
	ErrInvalidURL         = "INVALID_URL"
	ErrInvalidDate        = "INVALID_DATE"
	ErrInvalidObjectID    = "INVALID_OBJECT_ID"
	ErrInvalidFileType    = "INVALID_FILE_TYPE"
	ErrFileTooLarge       = "FILE_TOO_LARGE"
	ErrInvalidCountryCode = "INVALID_COUNTRY_CODE"
	ErrInvalidLanguage    = "INVALID_LANGUAGE"
	ErrInvalidTimezone    = "INVALID_TIMEZONE"
	ErrPasswordTooWeak    = "PASSWORD_TOO_WEAK"
	ErrInvalidCharacters  = "INVALID_CHARACTERS"
	ErrDuplicateValue     = "DUPLICATE_VALUE"
	ErrOutOfRange         = "OUT_OF_RANGE"
)

// String Validation

// ValidateString validates a string field
func ValidateString(value, fieldName string, rules ValidationRules) *ValidationError {
	// Required check
	if rules.Required && strings.TrimSpace(value) == "" {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("%s is required", fieldName),
			Code:    ErrRequired,
		}
	}

	// Skip other validations if empty and not required
	if value == "" && !rules.Required {
		return nil
	}

	// Length checks
	if rules.MinLength > 0 && len(value) < rules.MinLength {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("%s must be at least %d characters long", fieldName, rules.MinLength),
			Code:    ErrTooShort,
			Value:   value,
		}
	}

	if rules.MaxLength > 0 && len(value) > rules.MaxLength {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("%s must be no more than %d characters long", fieldName, rules.MaxLength),
			Code:    ErrTooLong,
			Value:   value,
		}
	}

	// Pattern check
	if rules.Pattern != "" {
		pattern := regexp.MustCompile(rules.Pattern)
		if !pattern.MatchString(value) {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf("%s has invalid format", fieldName),
				Code:    ErrInvalidFormat,
				Value:   value,
			}
		}
	}

	// Custom validation rules
	for _, validator := range rules.CustomRules {
		if err := validator.Validate(value); err != nil {
			err.Field = fieldName
			return err
		}
	}

	return nil
}

// Email Validation

// ValidateEmail validates an email address
func ValidateEmail(email string) *ValidationError {
	if email == "" {
		return &ValidationError{
			Field:   "email",
			Message: "Email is required",
			Code:    ErrRequired,
		}
	}

	// Use Go's mail package for robust validation
	_, err := mail.ParseAddress(email)
	if err != nil {
		return &ValidationError{
			Field:   "email",
			Message: "Invalid email format",
			Code:    ErrInvalidEmail,
			Value:   email,
		}
	}

	// Additional pattern check
	if !EmailPattern.MatchString(email) {
		return &ValidationError{
			Field:   "email",
			Message: "Invalid email format",
			Code:    ErrInvalidEmail,
			Value:   email,
		}
	}

	// Check email length
	if len(email) > 254 {
		return &ValidationError{
			Field:   "email",
			Message: "Email is too long",
			Code:    ErrTooLong,
			Value:   email,
		}
	}

	return nil
}

// Phone Number Validation

// ValidatePhoneNumber validates a phone number
func ValidatePhoneNumber(phone, countryCode string) *ValidationError {
	if phone == "" {
		return &ValidationError{
			Field:   "phone_number",
			Message: "Phone number is required",
			Code:    ErrRequired,
		}
	}

	// Remove all non-digit characters except +
	cleanPhone := regexp.MustCompile(`[^\d+]`).ReplaceAllString(phone, "")

	// Ensure it starts with +
	if !strings.HasPrefix(cleanPhone, "+") {
		if countryCode != "" {
			cleanPhone = countryCode + cleanPhone
		} else {
			return &ValidationError{
				Field:   "phone_number",
				Message: "Phone number must start with country code",
				Code:    ErrInvalidPhone,
				Value:   phone,
			}
		}
	}

	// Check against global E.164 format
	if pattern, exists := PhonePatterns["GLOBAL"]; exists {
		if !pattern.MatchString(cleanPhone) {
			return &ValidationError{
				Field:   "phone_number",
				Message: "Invalid phone number format",
				Code:    ErrInvalidPhone,
				Value:   phone,
			}
		}
	}

	// Additional length check
	if len(cleanPhone) < 8 || len(cleanPhone) > 16 {
		return &ValidationError{
			Field:   "phone_number",
			Message: "Phone number length is invalid",
			Code:    ErrInvalidPhone,
			Value:   phone,
		}
	}

	return nil
}

// ValidateCountryCode validates a country code
func ValidateCountryCode(code string) *ValidationError {
	if code == "" {
		return &ValidationError{
			Field:   "country_code",
			Message: "Country code is required",
			Code:    ErrRequired,
		}
	}

	// Ensure it starts with +
	if !strings.HasPrefix(code, "+") {
		return &ValidationError{
			Field:   "country_code",
			Message: "Country code must start with +",
			Code:    ErrInvalidCountryCode,
			Value:   code,
		}
	}

	// Validate length (1-4 digits after +)
	digits := code[1:]
	if len(digits) < 1 || len(digits) > 4 {
		return &ValidationError{
			Field:   "country_code",
			Message: "Invalid country code length",
			Code:    ErrInvalidCountryCode,
			Value:   code,
		}
	}

	// Check if all characters after + are digits
	for _, char := range digits {
		if !unicode.IsDigit(char) {
			return &ValidationError{
				Field:   "country_code",
				Message: "Country code must contain only digits after +",
				Code:    ErrInvalidCountryCode,
				Value:   code,
			}
		}
	}

	return nil
}

// Password Validation

// ValidatePassword validates a password with strength requirements
func ValidatePassword(password string, requirements PasswordRequirements) *ValidationError {
	if password == "" {
		return &ValidationError{
			Field:   "password",
			Message: "Password is required",
			Code:    ErrRequired,
		}
	}

	// Length check
	if len(password) < requirements.MinLength {
		return &ValidationError{
			Field:   "password",
			Message: fmt.Sprintf("Password must be at least %d characters long", requirements.MinLength),
			Code:    ErrTooShort,
		}
	}

	if requirements.MaxLength > 0 && len(password) > requirements.MaxLength {
		return &ValidationError{
			Field:   "password",
			Message: fmt.Sprintf("Password must be no more than %d characters long", requirements.MaxLength),
			Code:    ErrTooLong,
		}
	}

	var missingRequirements []string

	// Check for uppercase
	if requirements.RequireUppercase && !regexp.MustCompile(`[A-Z]`).MatchString(password) {
		missingRequirements = append(missingRequirements, "uppercase letter")
	}

	// Check for lowercase
	if requirements.RequireLowercase && !regexp.MustCompile(`[a-z]`).MatchString(password) {
		missingRequirements = append(missingRequirements, "lowercase letter")
	}

	// Check for numbers
	if requirements.RequireNumbers && !regexp.MustCompile(`\d`).MatchString(password) {
		missingRequirements = append(missingRequirements, "number")
	}

	// Check for special characters
	if requirements.RequireSpecialChars && !regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]`).MatchString(password) {
		missingRequirements = append(missingRequirements, "special character")
	}

	if len(missingRequirements) > 0 {
		return &ValidationError{
			Field:   "password",
			Message: fmt.Sprintf("Password must contain at least one %s", strings.Join(missingRequirements, ", ")),
			Code:    ErrPasswordTooWeak,
		}
	}

	// Check for common weak passwords
	if isCommonPassword(password) {
		return &ValidationError{
			Field:   "password",
			Message: "Password is too common, please choose a stronger password",
			Code:    ErrPasswordTooWeak,
		}
	}

	return nil
}

// PasswordRequirements represents password validation requirements
type PasswordRequirements struct {
	MinLength           int
	MaxLength           int
	RequireUppercase    bool
	RequireLowercase    bool
	RequireNumbers      bool
	RequireSpecialChars bool
}

// DefaultPasswordRequirements returns default password requirements
func DefaultPasswordRequirements() PasswordRequirements {
	return PasswordRequirements{
		MinLength:           8,
		MaxLength:           128,
		RequireUppercase:    true,
		RequireLowercase:    true,
		RequireNumbers:      true,
		RequireSpecialChars: true,
	}
}

// Check if password is in common passwords list
func isCommonPassword(password string) bool {
	commonPasswords := []string{
		"password", "123456", "123456789", "12345678", "12345", "1234567",
		"admin", "qwerty", "abc123", "letmein", "monkey", "password123",
		"welcome", "login", "dragon", "passw0rd", "master", "hello",
		"freedom", "whatever", "qazwsx", "trustno1",
	}

	lowerPassword := strings.ToLower(password)
	for _, common := range commonPasswords {
		if lowerPassword == common {
			return true
		}
	}
	return false
}

// Object ID Validation

// ValidateObjectID validates a MongoDB ObjectID
func ValidateObjectID(id string, fieldName string) *ValidationError {
	if id == "" {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("%s is required", fieldName),
			Code:    ErrRequired,
		}
	}

	if !primitive.IsValidObjectID(id) {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("Invalid %s format", fieldName),
			Code:    ErrInvalidObjectID,
			Value:   id,
		}
	}

	return nil
}

// File Validation

// ValidateFile validates an uploaded file
func ValidateFile(filename string, size int64, allowedTypes []string, maxSize int64) *ValidationError {
	if filename == "" {
		return &ValidationError{
			Field:   "file",
			Message: "File is required",
			Code:    ErrRequired,
		}
	}

	// Check file size
	if maxSize > 0 && size > maxSize {
		return &ValidationError{
			Field:   "file",
			Message: fmt.Sprintf("File size exceeds maximum allowed size of %s", FormatFileSize(maxSize)),
			Code:    ErrFileTooLarge,
		}
	}

	// Check file type
	if len(allowedTypes) > 0 {
		ext := strings.ToLower(filepath.Ext(filename))
		mimeType := mime.TypeByExtension(ext)

		isAllowed := false
		for _, allowedType := range allowedTypes {
			if mimeType == allowedType || ext == allowedType {
				isAllowed = true
				break
			}
		}

		if !isAllowed {
			return &ValidationError{
				Field:   "file",
				Message: fmt.Sprintf("File type not allowed. Allowed types: %s", strings.Join(allowedTypes, ", ")),
				Code:    ErrInvalidFileType,
				Value:   ext,
			}
		}
	}

	return nil
}

// URL Validation

// ValidateURL validates a URL
func ValidateURL(url string) *ValidationError {
	if url == "" {
		return &ValidationError{
			Field:   "url",
			Message: "URL is required",
			Code:    ErrRequired,
		}
	}

	if !URLPattern.MatchString(url) {
		return &ValidationError{
			Field:   "url",
			Message: "Invalid URL format",
			Code:    ErrInvalidURL,
			Value:   url,
		}
	}

	return nil
}

// Date Validation

// ValidateDate validates a date string
func ValidateDate(dateStr, layout, fieldName string) *ValidationError {
	if dateStr == "" {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("%s is required", fieldName),
			Code:    ErrRequired,
		}
	}

	_, err := time.Parse(layout, dateStr)
	if err != nil {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("Invalid %s format", fieldName),
			Code:    ErrInvalidDate,
			Value:   dateStr,
		}
	}

	return nil
}

// ValidateDateRange validates that start date is before end date
func ValidateDateRange(startDate, endDate time.Time) *ValidationError {
	if startDate.After(endDate) {
		return &ValidationError{
			Field:   "date_range",
			Message: "Start date must be before end date",
			Code:    ErrOutOfRange,
		}
	}

	return nil
}

// Numeric Validation

// ValidateInteger validates an integer value
func ValidateInteger(value int, fieldName string, min, max int) *ValidationError {
	if min != 0 && value < min {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("%s must be at least %d", fieldName, min),
			Code:    ErrTooSmall,
			Value:   strconv.Itoa(value),
		}
	}

	if max != 0 && value > max {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("%s must be no more than %d", fieldName, max),
			Code:    ErrTooLarge,
			Value:   strconv.Itoa(value),
		}
	}

	return nil
}

// ValidateFloat validates a float value
func ValidateFloat(value float64, fieldName string, min, max float64) *ValidationError {
	if min != 0 && value < min {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("%s must be at least %.2f", fieldName, min),
			Code:    ErrTooSmall,
			Value:   fmt.Sprintf("%.2f", value),
		}
	}

	if max != 0 && value > max {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("%s must be no more than %.2f", fieldName, max),
			Code:    ErrTooLarge,
			Value:   fmt.Sprintf("%.2f", value),
		}
	}

	return nil
}

// Array Validation

// ValidateArray validates an array field
func ValidateArray(arr []interface{}, fieldName string, minItems, maxItems int) *ValidationError {
	if len(arr) < minItems {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("%s must have at least %d items", fieldName, minItems),
			Code:    ErrTooSmall,
		}
	}

	if maxItems > 0 && len(arr) > maxItems {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("%s must have no more than %d items", fieldName, maxItems),
			Code:    ErrTooLarge,
		}
	}

	return nil
}

// Chat Application Specific Validations

// ValidateUsername validates a username
func ValidateUsername(username string) *ValidationError {
	if username == "" {
		return &ValidationError{
			Field:   "username",
			Message: "Username is required",
			Code:    ErrRequired,
		}
	}

	if !UsernamePattern.MatchString(username) {
		return &ValidationError{
			Field:   "username",
			Message: "Username must be 3-30 characters long and contain only letters, numbers, and underscores",
			Code:    ErrInvalidFormat,
			Value:   username,
		}
	}

	// Check for reserved usernames
	reservedUsernames := []string{
		"admin", "administrator", "root", "system", "api", "support",
		"help", "info", "contact", "about", "www", "mail", "email",
		"chatapp", "app", "service", "bot", "null", "undefined",
	}

	lowerUsername := strings.ToLower(username)
	for _, reserved := range reservedUsernames {
		if lowerUsername == reserved {
			return &ValidationError{
				Field:   "username",
				Message: "Username is reserved and cannot be used",
				Code:    ErrInvalidFormat,
				Value:   username,
			}
		}
	}

	return nil
}

// ValidateGroupName validates a group name
func ValidateGroupName(name string) *ValidationError {
	if name == "" {
		return &ValidationError{
			Field:   "group_name",
			Message: "Group name is required",
			Code:    ErrRequired,
		}
	}

	// Trim and check length
	name = strings.TrimSpace(name)
	if len(name) < 1 {
		return &ValidationError{
			Field:   "group_name",
			Message: "Group name cannot be empty",
			Code:    ErrRequired,
		}
	}

	if len(name) > 100 {
		return &ValidationError{
			Field:   "group_name",
			Message: "Group name must be no more than 100 characters",
			Code:    ErrTooLong,
			Value:   name,
		}
	}

	// Check for inappropriate content (basic)
	inappropriateWords := []string{"admin", "system", "bot", "api"}
	lowerName := strings.ToLower(name)
	for _, word := range inappropriateWords {
		if strings.Contains(lowerName, word) {
			return &ValidationError{
				Field:   "group_name",
				Message: "Group name contains inappropriate content",
				Code:    ErrInvalidFormat,
				Value:   name,
			}
		}
	}

	return nil
}

// ValidateMessageContent validates message content
func ValidateMessageContent(content string, maxLength int) *ValidationError {
	if content == "" {
		return &ValidationError{
			Field:   "content",
			Message: "Message content is required",
			Code:    ErrRequired,
		}
	}

	if len(content) > maxLength {
		return &ValidationError{
			Field:   "content",
			Message: fmt.Sprintf("Message content must be no more than %d characters", maxLength),
			Code:    ErrTooLong,
		}
	}

	return nil
}

// ValidateOTP validates OTP code
func ValidateOTP(otp string) *ValidationError {
	if otp == "" {
		return &ValidationError{
			Field:   "otp",
			Message: "OTP is required",
			Code:    ErrRequired,
		}
	}

	// OTP should be 6 digits
	if len(otp) != 6 {
		return &ValidationError{
			Field:   "otp",
			Message: "OTP must be 6 digits",
			Code:    ErrInvalidFormat,
			Value:   otp,
		}
	}

	// Check if all characters are digits
	for _, char := range otp {
		if !unicode.IsDigit(char) {
			return &ValidationError{
				Field:   "otp",
				Message: "OTP must contain only digits",
				Code:    ErrInvalidFormat,
				Value:   otp,
			}
		}
	}

	return nil
}

// Batch Validation

// ValidateStruct validates a struct using reflection and validation tags
func ValidateStruct(s interface{}) map[string]ValidationError {
	errors := make(map[string]ValidationError)

	// This is a simplified implementation
	// In production, you might want to use a library like go-playground/validator
	// or implement full reflection-based validation

	return errors
}

// ValidateMultiple validates multiple fields and returns all errors
func ValidateMultiple(validations map[string]func() *ValidationError) map[string]ValidationError {
	errors := make(map[string]ValidationError)

	for field, validationFunc := range validations {
		if err := validationFunc(); err != nil {
			errors[field] = *err
		}
	}

	return errors
}

// Utility Functions

// SanitizeString removes potentially dangerous characters
func SanitizeString(input string) string {
	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")

	// Trim whitespace
	input = strings.TrimSpace(input)

	// Remove control characters except \n, \r, \t
	var result strings.Builder
	for _, r := range input {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			continue
		}
		result.WriteRune(r)
	}

	return result.String()
}

// CleanPhoneNumber removes all non-digit characters except +
func CleanPhoneNumber(phone string) string {
	return regexp.MustCompile(`[^\d+]`).ReplaceAllString(phone, "")
}

// IsValidLanguageCode validates ISO 639-1 language codes
func IsValidLanguageCode(code string) bool {
	validCodes := []string{
		"en", "es", "fr", "de", "it", "pt", "ru", "zh", "ja", "ko",
		"ar", "hi", "tr", "pl", "nl", "sv", "da", "no", "fi", "he",
		"th", "vi", "id", "ms", "tl", "sw", "am", "bn", "gu", "kn",
		"ml", "mr", "ta", "te", "ur", "fa", "ps", "ku", "ky", "uz",
	}

	for _, valid := range validCodes {
		if code == valid {
			return true
		}
	}
	return false
}

// IsValidTimezone validates timezone strings
func IsValidTimezone(tz string) bool {
	_, err := time.LoadLocation(tz)
	return err == nil
}

// Custom Validators

// ProfanityValidator checks for profanity in text
type ProfanityValidator struct {
	ProfanityWords []string
}

// Validate implements Validator interface
func (p *ProfanityValidator) Validate(value interface{}) *ValidationError {
	str, ok := value.(string)
	if !ok {
		return nil
	}

	lowerStr := strings.ToLower(str)
	for _, word := range p.ProfanityWords {
		if strings.Contains(lowerStr, strings.ToLower(word)) {
			return &ValidationError{
				Message: "Content contains inappropriate language",
				Code:    ErrInvalidCharacters,
			}
		}
	}

	return nil
}

// LengthValidator validates string length
type LengthValidator struct {
	Min int
	Max int
}

// Validate implements Validator interface
func (l *LengthValidator) Validate(value interface{}) *ValidationError {
	str, ok := value.(string)
	if !ok {
		return nil
	}

	if l.Min > 0 && len(str) < l.Min {
		return &ValidationError{
			Message: fmt.Sprintf("Must be at least %d characters long", l.Min),
			Code:    ErrTooShort,
		}
	}

	if l.Max > 0 && len(str) > l.Max {
		return &ValidationError{
			Message: fmt.Sprintf("Must be no more than %d characters long", l.Max),
			Code:    ErrTooLong,
		}
	}

	return nil
}
