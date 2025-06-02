package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"bro/internal/config"
	"bro/pkg/logger"
)

// SMSService handles SMS operations
type SMSService struct {
	config   *config.Config
	provider SMSProvider
}

// SMSProvider interface for different SMS providers
type SMSProvider interface {
	SendSMS(phoneNumber, message string) error
	GetProviderName() string
}

// SMSResult represents the result of sending an SMS
type SMSResult struct {
	Success   bool   `json:"success"`
	MessageID string `json:"message_id,omitempty"`
	Error     string `json:"error,omitempty"`
	Cost      string `json:"cost,omitempty"`
}

// Twilio SMS Provider
type TwilioSMSProvider struct {
	config     *config.TwilioConfig
	httpClient *http.Client
}

// Firebase SMS Provider (using Firebase Auth)
type FirebaseSMSProvider struct {
	config     *config.FirebaseConfig
	httpClient *http.Client
}

// Mock SMS Provider for testing
type MockSMSProvider struct{}

// NewSMSService creates a new SMS service
func NewSMSService(config *config.Config) *SMSService {
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	service := &SMSService{
		config: config,
	}

	// Initialize provider based on configuration
	switch strings.ToLower(config.SMSProvider) {
	case "twilio":
		if config.TwilioConfig.AccountSID == "" || config.TwilioConfig.AuthToken == "" {
			logger.Warn("Twilio credentials not configured, using mock SMS provider")
			service.provider = &MockSMSProvider{}
		} else {
			service.provider = &TwilioSMSProvider{
				config:     &config.TwilioConfig,
				httpClient: httpClient,
			}
		}
	case "firebase":
		if config.FirebaseConfig.ProjectID == "" {
			logger.Warn("Firebase credentials not configured, using mock SMS provider")
			service.provider = &MockSMSProvider{}
		} else {
			service.provider = &FirebaseSMSProvider{
				config:     &config.FirebaseConfig,
				httpClient: httpClient,
			}
		}
	default:
		logger.Warn("No SMS provider configured, using mock provider")
		service.provider = &MockSMSProvider{}
	}

	logger.Infof("SMS service initialized with provider: %s", service.provider.GetProviderName())
	return service
}

// SendOTP sends OTP SMS to phone number
func (sms *SMSService) SendOTP(phoneNumber, otp string) error {
	message := fmt.Sprintf("Your ChatApp verification code is: %s. Do not share this code with anyone.", otp)

	err := sms.provider.SendSMS(phoneNumber, message)
	if err != nil {
		logger.LogSMSOperation(sms.provider.GetProviderName(), phoneNumber, false, err)
		return fmt.Errorf("failed to send OTP SMS: %w", err)
	}

	logger.LogSMSOperation(sms.provider.GetProviderName(), phoneNumber, true, nil)
	return nil
}

// SendWelcomeMessage sends welcome SMS to new users
func (sms *SMSService) SendWelcomeMessage(phoneNumber, userName string) error {
	message := fmt.Sprintf("Welcome to ChatApp, %s! Start chatting with friends and family now.", userName)

	err := sms.provider.SendSMS(phoneNumber, message)
	if err != nil {
		logger.LogSMSOperation(sms.provider.GetProviderName(), phoneNumber, false, err)
		return fmt.Errorf("failed to send welcome SMS: %w", err)
	}

	logger.LogSMSOperation(sms.provider.GetProviderName(), phoneNumber, true, nil)
	return nil
}

// SendCustomMessage sends a custom SMS message
func (sms *SMSService) SendCustomMessage(phoneNumber, message string) error {
	err := sms.provider.SendSMS(phoneNumber, message)
	if err != nil {
		logger.LogSMSOperation(sms.provider.GetProviderName(), phoneNumber, false, err)
		return fmt.Errorf("failed to send SMS: %w", err)
	}

	logger.LogSMSOperation(sms.provider.GetProviderName(), phoneNumber, true, nil)
	return nil
}

// SendBulkSMS sends SMS to multiple phone numbers
func (sms *SMSService) SendBulkSMS(phoneNumbers []string, message string) error {
	var errors []string
	successCount := 0

	for _, phoneNumber := range phoneNumbers {
		err := sms.provider.SendSMS(phoneNumber, message)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", phoneNumber, err))
			logger.LogSMSOperation(sms.provider.GetProviderName(), phoneNumber, false, err)
		} else {
			successCount++
			logger.LogSMSOperation(sms.provider.GetProviderName(), phoneNumber, true, nil)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to send %d/%d SMS messages: %s", len(errors), len(phoneNumbers), strings.Join(errors, "; "))
	}

	logger.Infof("Successfully sent bulk SMS to %d recipients", successCount)
	return nil
}

// SendPasswordResetCode sends password reset code
func (sms *SMSService) SendPasswordResetCode(phoneNumber, code string) error {
	message := fmt.Sprintf("Your ChatApp password reset code is: %s. This code will expire in 10 minutes.", code)

	err := sms.provider.SendSMS(phoneNumber, message)
	if err != nil {
		logger.LogSMSOperation(sms.provider.GetProviderName(), phoneNumber, false, err)
		return fmt.Errorf("failed to send password reset SMS: %w", err)
	}

	logger.LogSMSOperation(sms.provider.GetProviderName(), phoneNumber, true, nil)
	return nil
}

// SendAccountLockedNotification sends account locked notification
func (sms *SMSService) SendAccountLockedNotification(phoneNumber string) error {
	message := "Your ChatApp account has been temporarily locked due to suspicious activity. Please contact support if you need assistance."

	err := sms.provider.SendSMS(phoneNumber, message)
	if err != nil {
		logger.LogSMSOperation(sms.provider.GetProviderName(), phoneNumber, false, err)
		return fmt.Errorf("failed to send account locked SMS: %w", err)
	}

	logger.LogSMSOperation(sms.provider.GetProviderName(), phoneNumber, true, nil)
	return nil
}

// Twilio Provider Implementation

func (tp *TwilioSMSProvider) SendSMS(phoneNumber, message string) error {
	// Twilio API endpoint
	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", tp.config.AccountSID)

	// Prepare form data
	data := url.Values{}
	data.Set("To", phoneNumber)
	data.Set("From", tp.config.PhoneNumber)
	data.Set("Body", message)

	// Create request
	req, err := http.NewRequest("POST", apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create Twilio request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(tp.config.AccountSID, tp.config.AuthToken)

	// Send request
	resp, err := tp.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Twilio SMS: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("Twilio API returned status code: %d", resp.StatusCode)
	}

	// Parse response for message ID
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		logger.Errorf("Failed to parse Twilio response: %v", err)
	} else {
		if sid, ok := result["sid"].(string); ok {
			logger.Debugf("Twilio SMS sent with SID: %s", sid)
		}
	}

	return nil
}

func (tp *TwilioSMSProvider) GetProviderName() string {
	return "twilio"
}

// Firebase Provider Implementation

func (fp *FirebaseSMSProvider) SendSMS(phoneNumber, message string) error {
	// Firebase doesn't have direct SMS API like Twilio
	// This is a placeholder implementation
	// In reality, you might use Firebase Functions with a third-party SMS service

	apiURL := fmt.Sprintf("https://%s.cloudfunctions.net/sendSMS", fp.config.ProjectID)

	// Prepare JSON payload
	payload := map[string]string{
		"phoneNumber": phoneNumber,
		"message":     message,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Firebase SMS payload: %w", err)
	}

	// Create request
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create Firebase request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	// Add Firebase authentication if needed
	// req.Header.Set("Authorization", "Bearer " + accessToken)

	// Send request
	resp, err := fp.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Firebase SMS: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Firebase SMS API returned status code: %d", resp.StatusCode)
	}

	return nil
}

func (fp *FirebaseSMSProvider) GetProviderName() string {
	return "firebase"
}

// Mock Provider Implementation

func (mp *MockSMSProvider) SendSMS(phoneNumber, message string) error {
	// Log the SMS that would be sent
	logger.Infof("Mock SMS to %s: %s", phoneNumber, message)

	// Simulate some processing time
	time.Sleep(100 * time.Millisecond)

	return nil
}

func (mp *MockSMSProvider) GetProviderName() string {
	return "mock"
}

// Utility Functions

// FormatPhoneNumber formats phone number for SMS providers
func FormatPhoneNumber(phoneNumber string) string {
	// Remove any non-digit characters except +
	cleaned := ""
	for _, char := range phoneNumber {
		if char >= '0' && char <= '9' || char == '+' {
			cleaned += string(char)
		}
	}

	// Ensure it starts with +
	if !strings.HasPrefix(cleaned, "+") {
		cleaned = "+" + cleaned
	}

	return cleaned
}

// ValidatePhoneNumberForSMS validates phone number for SMS sending
func ValidatePhoneNumberForSMS(phoneNumber string) error {
	formatted := FormatPhoneNumber(phoneNumber)

	// Basic validation
	if len(formatted) < 8 || len(formatted) > 16 {
		return fmt.Errorf("invalid phone number length")
	}

	if !strings.HasPrefix(formatted, "+") {
		return fmt.Errorf("phone number must include country code")
	}

	return nil
}

// IsPhoneNumberSuppressed checks if phone number is in suppression list
func (sms *SMSService) IsPhoneNumberSuppressed(phoneNumber string) bool {
	// This would typically check against a database of suppressed numbers
	// For now, return false
	suppressedNumbers := []string{
		"+1234567890", // Example suppressed number
	}

	formatted := FormatPhoneNumber(phoneNumber)
	for _, suppressed := range suppressedNumbers {
		if formatted == suppressed {
			return true
		}
	}

	return false
}

// GetSMSCost estimates SMS cost (for reporting/analytics)
func (sms *SMSService) GetSMSCost(phoneNumber string) float64 {
	// This is a simplified cost calculation
	// In reality, costs vary by provider and destination country

	switch sms.provider.GetProviderName() {
	case "twilio":
		// Basic Twilio pricing (varies by country)
		if strings.HasPrefix(phoneNumber, "+1") { // US/Canada
			return 0.0075 // $0.0075 per SMS
		}
		return 0.05 // International rate
	case "firebase":
		return 0.01 // Estimated cost
	default:
		return 0.0 // Mock provider is free
	}
}

// SMSTemplate represents an SMS template
type SMSTemplate struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Content   string            `json:"content"`
	Variables []string          `json:"variables"`
	Category  string            `json:"category"`
	Language  string            `json:"language"`
	IsActive  bool              `json:"is_active"`
	Metadata  map[string]string `json:"metadata"`
}

// GetSMSTemplates returns available SMS templates
func (sms *SMSService) GetSMSTemplates() map[string]*SMSTemplate {
	return map[string]*SMSTemplate{
		"otp_verification": {
			ID:        "otp_verification",
			Name:      "OTP Verification",
			Content:   "Your {{app_name}} verification code is: {{otp_code}}. Do not share this code with anyone.",
			Variables: []string{"app_name", "otp_code"},
			Category:  "authentication",
			Language:  "en",
			IsActive:  true,
		},
		"welcome": {
			ID:        "welcome",
			Name:      "Welcome Message",
			Content:   "Welcome to {{app_name}}, {{user_name}}! Start chatting with friends and family now.",
			Variables: []string{"app_name", "user_name"},
			Category:  "onboarding",
			Language:  "en",
			IsActive:  true,
		},
		"password_reset": {
			ID:        "password_reset",
			Name:      "Password Reset",
			Content:   "Your {{app_name}} password reset code is: {{reset_code}}. This code will expire in {{expiry_minutes}} minutes.",
			Variables: []string{"app_name", "reset_code", "expiry_minutes"},
			Category:  "security",
			Language:  "en",
			IsActive:  true,
		},
		"account_locked": {
			ID:        "account_locked",
			Name:      "Account Locked",
			Content:   "Your {{app_name}} account has been temporarily locked due to suspicious activity. Please contact support if you need assistance.",
			Variables: []string{"app_name"},
			Category:  "security",
			Language:  "en",
			IsActive:  true,
		},
	}
}

// SendTemplatedSMS sends SMS using a template
func (sms *SMSService) SendTemplatedSMS(phoneNumber, templateID string, variables map[string]string) error {
	templates := sms.GetSMSTemplates()
	template, exists := templates[templateID]
	if !exists {
		return fmt.Errorf("SMS template not found: %s", templateID)
	}

	if !template.IsActive {
		return fmt.Errorf("SMS template is inactive: %s", templateID)
	}

	// Replace variables in template content
	message := template.Content
	for _, variable := range template.Variables {
		if value, exists := variables[variable]; exists {
			placeholder := fmt.Sprintf("{{%s}}", variable)
			message = strings.ReplaceAll(message, placeholder, value)
		}
	}

	// Send the SMS
	err := sms.provider.SendSMS(phoneNumber, message)
	if err != nil {
		logger.LogSMSOperation(sms.provider.GetProviderName(), phoneNumber, false, err)
		return fmt.Errorf("failed to send templated SMS: %w", err)
	}

	logger.LogSMSOperation(sms.provider.GetProviderName(), phoneNumber, true, nil)
	logger.Infof("Sent templated SMS using template: %s", templateID)
	return nil
}

// GetProviderName returns the current SMS provider name
func (sms *SMSService) GetProviderName() string {
	return sms.provider.GetProviderName()
}

// HealthCheck checks if SMS service is healthy
func (sms *SMSService) HealthCheck() error {
	// For mock provider, always return healthy
	if sms.provider.GetProviderName() == "mock" {
		return nil
	}

	// For real providers, you might want to test with a specific test number
	// For now, just check if provider is configured
	switch sms.provider.GetProviderName() {
	case "twilio":
		if tp, ok := sms.provider.(*TwilioSMSProvider); ok {
			if tp.config.AccountSID == "" || tp.config.AuthToken == "" {
				return fmt.Errorf("Twilio not properly configured")
			}
		}
	case "firebase":
		if fp, ok := sms.provider.(*FirebaseSMSProvider); ok {
			if fp.config.ProjectID == "" {
				return fmt.Errorf("Firebase not properly configured")
			}
		}
	}

	return nil
}
