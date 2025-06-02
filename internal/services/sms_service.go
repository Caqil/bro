package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"bro/internal/config"
	"bro/pkg/logger"
	"bro/pkg/redis"
)

// SMSService handles SMS operations
type SMSService struct {
	config      *config.Config
	redisClient *redis.Client
	httpClient  *http.Client
	provider    SMSProvider
}

// SMSProvider interface for different SMS providers
type SMSProvider interface {
	SendSMS(to, message string) (*SMSResponse, error)
	GetDeliveryStatus(messageID string) (*DeliveryStatus, error)
	GetProviderName() string
}

// SMSResponse represents SMS sending response
type SMSResponse struct {
	MessageID    string                 `json:"message_id"`
	Status       string                 `json:"status"`
	Provider     string                 `json:"provider"`
	To           string                 `json:"to"`
	Message      string                 `json:"message"`
	Cost         float64                `json:"cost,omitempty"`
	Segments     int                    `json:"segments,omitempty"`
	Timestamp    time.Time              `json:"timestamp"`
	ProviderData map[string]interface{} `json:"provider_data,omitempty"`
}

// DeliveryStatus represents SMS delivery status
type DeliveryStatus struct {
	MessageID     string    `json:"message_id"`
	Status        string    `json:"status"` // "pending", "sent", "delivered", "failed", "unknown"
	DeliveredAt   time.Time `json:"delivered_at,omitempty"`
	ErrorCode     string    `json:"error_code,omitempty"`
	ErrorMessage  string    `json:"error_message,omitempty"`
	Cost          float64   `json:"cost,omitempty"`
	LastUpdatedAt time.Time `json:"last_updated_at"`
}

// SMSTemplate represents SMS template
type SMSTemplate struct {
	Name      string            `json:"name"`
	Content   string            `json:"content"`
	Variables []string          `json:"variables"`
	Language  string            `json:"language"`
	IsActive  bool              `json:"is_active"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// SMSMetrics represents SMS metrics
type SMSMetrics struct {
	TotalSent      int64     `json:"total_sent"`
	TotalDelivered int64     `json:"total_delivered"`
	TotalFailed    int64     `json:"total_failed"`
	DeliveryRate   float64   `json:"delivery_rate"`
	AverageCost    float64   `json:"average_cost"`
	TotalCost      float64   `json:"total_cost"`
	LastUpdated    time.Time `json:"last_updated"`
}

// TwilioProvider implements SMS provider for Twilio
type TwilioProvider struct {
	config     *config.TwilioConfig
	httpClient *http.Client
}

// FirebaseProvider implements SMS provider for Firebase
type FirebaseProvider struct {
	config     *config.FirebaseConfig
	httpClient *http.Client
}

// OneSignalProvider implements SMS provider for OneSignal
type OneSignalProvider struct {
	config     *config.OneSignalConfig
	httpClient *http.Client
}

// Common SMS templates
var DefaultSMSTemplates = map[string]SMSTemplate{
	"otp_registration": {
		Name:      "OTP Registration",
		Content:   "Your verification code for {app_name} is: {otp}. This code will expire in {expiry_minutes} minutes.",
		Variables: []string{"app_name", "otp", "expiry_minutes"},
		Language:  "en",
		IsActive:  true,
	},
	"otp_login": {
		Name:      "OTP Login",
		Content:   "Your login code for {app_name} is: {otp}. If you didn't request this, please ignore this message.",
		Variables: []string{"app_name", "otp"},
		Language:  "en",
		IsActive:  true,
	},
	"password_reset": {
		Name:      "Password Reset",
		Content:   "Your password reset code for {app_name} is: {otp}. This code will expire in {expiry_minutes} minutes.",
		Variables: []string{"app_name", "otp", "expiry_minutes"},
		Language:  "en",
		IsActive:  true,
	},
	"welcome": {
		Name:      "Welcome Message",
		Content:   "Welcome to {app_name}! Your account has been successfully created. Start connecting with friends now!",
		Variables: []string{"app_name"},
		Language:  "en",
		IsActive:  true,
	},
	"security_alert": {
		Name:      "Security Alert",
		Content:   "Security Alert: Your {app_name} account was accessed from a new device. If this wasn't you, please secure your account immediately.",
		Variables: []string{"app_name"},
		Language:  "en",
		IsActive:  true,
	},
}

// NewSMSService creates a new SMS service
func NewSMSService(config *config.Config) *SMSService {
	redisClient := redis.GetClient()

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	service := &SMSService{
		config:      config,
		redisClient: redisClient,
		httpClient:  httpClient,
	}

	// Initialize provider based on configuration
	switch strings.ToLower(config.SMSProvider) {
	case "twilio":
		service.provider = &TwilioProvider{
			config:     &config.TwilioConfig,
			httpClient: httpClient,
		}
	case "firebase":
		service.provider = &FirebaseProvider{
			config:     &config.FirebaseConfig,
			httpClient: httpClient,
		}
	case "onesignal":
		service.provider = &OneSignalProvider{
			config:     &config.OneSignalConfig,
			httpClient: httpClient,
		}
	default:
		logger.Warn("No SMS provider configured, using mock provider")
		service.provider = &MockSMSProvider{}
	}

	logger.Infof("SMS service initialized with provider: %s", service.provider.GetProviderName())
	return service
}

// Public SMS Service Methods

// SendOTP sends OTP SMS to user
func (s *SMSService) SendOTP(phoneNumber, otp, reason string) error {
	startTime := time.Now()

	// Check rate limiting
	if err := s.checkRateLimit(phoneNumber); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}

	// Get appropriate template
	var templateName string
	switch reason {
	case "registration":
		templateName = "otp_registration"
	case "login":
		templateName = "otp_login"
	case "password_reset":
		templateName = "password_reset"
	default:
		templateName = "otp_registration"
	}

	// Generate message from template
	message, err := s.generateMessage(templateName, map[string]string{
		"app_name":       "ChatApp",
		"otp":            otp,
		"expiry_minutes": "10",
	})
	if err != nil {
		return fmt.Errorf("failed to generate message: %w", err)
	}

	// Send SMS
	response, err := s.provider.SendSMS(phoneNumber, message)
	if err != nil {
		duration := time.Since(startTime)
		logger.Errorf("Failed to send OTP SMS to %s: %v (duration: %dms)", phoneNumber, err, duration.Milliseconds())

		// Log failed SMS
		s.logSMSEvent("otp_send_failed", phoneNumber, message, map[string]interface{}{
			"error":       err.Error(),
			"reason":      reason,
			"duration_ms": duration.Milliseconds(),
		})

		return fmt.Errorf("failed to send SMS: %w", err)
	}

	// Update rate limiting
	s.updateRateLimit(phoneNumber)

	// Cache SMS response
	if s.redisClient != nil {
		s.cacheSMSResponse(response)
	}

	// Log successful SMS
	duration := time.Since(startTime)
	s.logSMSEvent("otp_sent", phoneNumber, message, map[string]interface{}{
		"message_id":  response.MessageID,
		"provider":    response.Provider,
		"reason":      reason,
		"duration_ms": duration.Milliseconds(),
		"cost":        response.Cost,
		"segments":    response.Segments,
	})

	logger.Infof("OTP SMS sent successfully to %s (ID: %s, Provider: %s)",
		phoneNumber, response.MessageID, response.Provider)

	return nil
}

// SendSMS sends a custom SMS message
func (s *SMSService) SendSMS(phoneNumber, message string, metadata map[string]interface{}) (*SMSResponse, error) {
	startTime := time.Now()

	// Validate inputs
	if err := s.validatePhoneNumber(phoneNumber); err != nil {
		return nil, fmt.Errorf("invalid phone number: %w", err)
	}

	if err := s.validateMessage(message); err != nil {
		return nil, fmt.Errorf("invalid message: %w", err)
	}

	// Check rate limiting
	if err := s.checkRateLimit(phoneNumber); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	// Send SMS
	response, err := s.provider.SendSMS(phoneNumber, message)
	if err != nil {
		duration := time.Since(startTime)
		logger.Errorf("Failed to send SMS to %s: %v (duration: %dms)", phoneNumber, err, duration.Milliseconds())

		s.logSMSEvent("sms_send_failed", phoneNumber, message, map[string]interface{}{
			"error":       err.Error(),
			"duration_ms": duration.Milliseconds(),
			"metadata":    metadata,
		})

		return nil, fmt.Errorf("failed to send SMS: %w", err)
	}

	// Update rate limiting
	s.updateRateLimit(phoneNumber)

	// Cache SMS response
	if s.redisClient != nil {
		s.cacheSMSResponse(response)
	}

	// Log successful SMS
	duration := time.Since(startTime)
	s.logSMSEvent("sms_sent", phoneNumber, message, map[string]interface{}{
		"message_id":  response.MessageID,
		"provider":    response.Provider,
		"duration_ms": duration.Milliseconds(),
		"cost":        response.Cost,
		"segments":    response.Segments,
		"metadata":    metadata,
	})

	return response, nil
}

// SendWelcomeMessage sends welcome message to new user
func (s *SMSService) SendWelcomeMessage(phoneNumber string) error {
	message, err := s.generateMessage("welcome", map[string]string{
		"app_name": "ChatApp",
	})
	if err != nil {
		return fmt.Errorf("failed to generate welcome message: %w", err)
	}

	_, err = s.SendSMS(phoneNumber, message, map[string]interface{}{
		"type": "welcome",
	})
	return err
}

// SendSecurityAlert sends security alert SMS
func (s *SMSService) SendSecurityAlert(phoneNumber, alertType string, details map[string]interface{}) error {
	message, err := s.generateMessage("security_alert", map[string]string{
		"app_name": "ChatApp",
	})
	if err != nil {
		return fmt.Errorf("failed to generate security alert: %w", err)
	}

	metadata := map[string]interface{}{
		"type":       "security_alert",
		"alert_type": alertType,
		"details":    details,
	}

	_, err = s.SendSMS(phoneNumber, message, metadata)
	return err
}

// GetDeliveryStatus gets delivery status for a message
func (s *SMSService) GetDeliveryStatus(messageID string) (*DeliveryStatus, error) {
	// Try to get from cache first
	if s.redisClient != nil {
		if cached, err := s.getCachedDeliveryStatus(messageID); err == nil && cached != nil {
			return cached, nil
		}
	}

	// Get from provider
	status, err := s.provider.GetDeliveryStatus(messageID)
	if err != nil {
		return nil, fmt.Errorf("failed to get delivery status: %w", err)
	}

	// Cache the status
	if s.redisClient != nil {
		s.cacheDeliveryStatus(status)
	}

	return status, nil
}

// GetSMSMetrics gets SMS metrics
func (s *SMSService) GetSMSMetrics(days int) (*SMSMetrics, error) {
	if s.redisClient == nil {
		return &SMSMetrics{}, nil
	}

	// This would typically aggregate data from logs or database
	// For now, return basic metrics from Redis
	metrics := &SMSMetrics{
		LastUpdated: time.Now(),
	}

	return metrics, nil
}

// Template Management

// generateMessage generates message from template
func (s *SMSService) generateMessage(templateName string, variables map[string]string) (string, error) {
	template, exists := DefaultSMSTemplates[templateName]
	if !exists {
		return "", fmt.Errorf("template not found: %s", templateName)
	}

	if !template.IsActive {
		return "", fmt.Errorf("template is inactive: %s", templateName)
	}

	message := template.Content

	// Replace variables
	for key, value := range variables {
		placeholder := fmt.Sprintf("{%s}", key)
		message = strings.ReplaceAll(message, placeholder, value)
	}

	// Check if all variables were replaced
	if strings.Contains(message, "{") && strings.Contains(message, "}") {
		logger.Warnf("Message still contains unreplaced variables: %s", message)
	}

	return message, nil
}

// Rate Limiting

// checkRateLimit checks if SMS can be sent to phone number
func (s *SMSService) checkRateLimit(phoneNumber string) error {
	if s.redisClient == nil {
		return nil // Skip rate limiting if Redis is not available
	}

	key := fmt.Sprintf("sms_rate_limit:%s", phoneNumber)

	// Check rate limit (5 SMS per hour per phone number)
	allowed, err := s.redisClient.RateLimitCheck(key, 5, 1*time.Hour)
	if err != nil {
		logger.Error("Failed to check SMS rate limit:", err)
		return nil // Allow SMS if rate limit check fails
	}

	if !allowed {
		return fmt.Errorf("SMS rate limit exceeded for phone number: %s", phoneNumber)
	}

	return nil
}

// updateRateLimit updates rate limit counter
func (s *SMSService) updateRateLimit(phoneNumber string) {
	if s.redisClient == nil {
		return
	}

	key := fmt.Sprintf("sms_rate_limit:%s", phoneNumber)
	s.redisClient.IncrementBy(key, 1)
}

// Validation

// validatePhoneNumber validates phone number format
func (s *SMSService) validatePhoneNumber(phoneNumber string) error {
	if phoneNumber == "" {
		return fmt.Errorf("phone number is required")
	}

	// Basic validation - should start with + and contain only digits
	if !strings.HasPrefix(phoneNumber, "+") {
		return fmt.Errorf("phone number must start with country code (+)")
	}

	digits := strings.TrimPrefix(phoneNumber, "+")
	if len(digits) < 8 || len(digits) > 15 {
		return fmt.Errorf("invalid phone number length")
	}

	for _, char := range digits {
		if char < '0' || char > '9' {
			return fmt.Errorf("phone number must contain only digits")
		}
	}

	return nil
}

// validateMessage validates SMS message content
func (s *SMSService) validateMessage(message string) error {
	if message == "" {
		return fmt.Errorf("message is required")
	}

	if len(message) > 1600 { // Maximum for concatenated SMS
		return fmt.Errorf("message too long (max 1600 characters)")
	}

	return nil
}

// Caching

// cacheSMSResponse caches SMS response in Redis
func (s *SMSService) cacheSMSResponse(response *SMSResponse) {
	if s.redisClient == nil {
		return
	}

	key := fmt.Sprintf("sms_response:%s", response.MessageID)
	if err := s.redisClient.SetEX(key, response, 24*time.Hour); err != nil {
		logger.Error("Failed to cache SMS response:", err)
	}
}

// cacheDeliveryStatus caches delivery status in Redis
func (s *SMSService) cacheDeliveryStatus(status *DeliveryStatus) {
	if s.redisClient == nil {
		return
	}

	key := fmt.Sprintf("sms_delivery:%s", status.MessageID)
	if err := s.redisClient.SetEX(key, status, 1*time.Hour); err != nil {
		logger.Error("Failed to cache delivery status:", err)
	}
}

// getCachedDeliveryStatus gets cached delivery status
func (s *SMSService) getCachedDeliveryStatus(messageID string) (*DeliveryStatus, error) {
	key := fmt.Sprintf("sms_delivery:%s", messageID)
	var status DeliveryStatus
	if err := s.redisClient.GetJSON(key, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// Logging

// logSMSEvent logs SMS-related events
func (s *SMSService) logSMSEvent(event, phoneNumber, message string, metadata map[string]interface{}) {
	fields := map[string]interface{}{
		"event":          event,
		"phone_number":   phoneNumber,
		"message_length": len(message),
		"provider":       s.provider.GetProviderName(),
		"type":           "sms_event",
	}

	for k, v := range metadata {
		fields[k] = v
	}

	logger.WithFields(fields).Info("SMS event")
}

// Provider Implementations

// Twilio Provider

func (t *TwilioProvider) SendSMS(to, message string) (*SMSResponse, error) {
	if t.config.AccountSID == "" || t.config.AuthToken == "" {
		return nil, fmt.Errorf("Twilio credentials not configured")
	}

	// Prepare request
	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", t.config.AccountSID)

	data := url.Values{}
	data.Set("To", to)
	data.Set("From", t.config.PhoneNumber)
	data.Set("Body", message)

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(t.config.AccountSID, t.config.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Send request
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var twilioResp struct {
		SID          string  `json:"sid"`
		Status       string  `json:"status"`
		To           string  `json:"to"`
		From         string  `json:"from"`
		Body         string  `json:"body"`
		NumSegments  string  `json:"num_segments"`
		Price        string  `json:"price"`
		ErrorCode    *string `json:"error_code"`
		ErrorMessage *string `json:"error_message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&twilioResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if resp.StatusCode != 201 {
		errorMsg := "unknown error"
		if twilioResp.ErrorMessage != nil {
			errorMsg = *twilioResp.ErrorMessage
		}
		return nil, fmt.Errorf("Twilio API error (%d): %s", resp.StatusCode, errorMsg)
	}

	// Convert segments to int
	segments := 1
	if twilioResp.NumSegments != "" {
		if s, err := fmt.Sscanf(twilioResp.NumSegments, "%d", &segments); err != nil || s != 1 {
			segments = 1
		}
	}

	// Convert price to float
	var cost float64
	if twilioResp.Price != "" {
		fmt.Sscanf(twilioResp.Price, "%f", &cost)
		if cost < 0 {
			cost = -cost // Convert negative price to positive
		}
	}

	response := &SMSResponse{
		MessageID: twilioResp.SID,
		Status:    twilioResp.Status,
		Provider:  "twilio",
		To:        to,
		Message:   message,
		Cost:      cost,
		Segments:  segments,
		Timestamp: time.Now(),
		ProviderData: map[string]interface{}{
			"from":         twilioResp.From,
			"raw_response": twilioResp,
		},
	}

	return response, nil
}

func (t *TwilioProvider) GetDeliveryStatus(messageID string) (*DeliveryStatus, error) {
	if t.config.AccountSID == "" || t.config.AuthToken == "" {
		return nil, fmt.Errorf("Twilio credentials not configured")
	}

	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages/%s.json",
		t.config.AccountSID, messageID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(t.config.AccountSID, t.config.AuthToken)

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var twilioResp struct {
		SID          string  `json:"sid"`
		Status       string  `json:"status"`
		ErrorCode    *int    `json:"error_code"`
		ErrorMessage *string `json:"error_message"`
		DateSent     *string `json:"date_sent"`
		Price        string  `json:"price"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&twilioResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	status := &DeliveryStatus{
		MessageID:     messageID,
		Status:        mapTwilioStatus(twilioResp.Status),
		LastUpdatedAt: time.Now(),
	}

	if twilioResp.ErrorCode != nil {
		status.ErrorCode = fmt.Sprintf("%d", *twilioResp.ErrorCode)
	}

	if twilioResp.ErrorMessage != nil {
		status.ErrorMessage = *twilioResp.ErrorMessage
	}

	if twilioResp.DateSent != nil && status.Status == "delivered" {
		if deliveredAt, err := time.Parse(time.RFC3339, *twilioResp.DateSent); err == nil {
			status.DeliveredAt = deliveredAt
		}
	}

	if twilioResp.Price != "" {
		fmt.Sscanf(twilioResp.Price, "%f", &status.Cost)
		if status.Cost < 0 {
			status.Cost = -status.Cost
		}
	}

	return status, nil
}

func (t *TwilioProvider) GetProviderName() string {
	return "twilio"
}

// Firebase Provider (placeholder implementation)

func (f *FirebaseProvider) SendSMS(to, message string) (*SMSResponse, error) {
	// Firebase doesn't have a direct SMS service, this would integrate with Firebase Auth
	// or use Firebase Functions with a third-party SMS provider
	return nil, fmt.Errorf("Firebase SMS not implemented yet")
}

func (f *FirebaseProvider) GetDeliveryStatus(messageID string) (*DeliveryStatus, error) {
	return nil, fmt.Errorf("Firebase SMS delivery status not implemented yet")
}

func (f *FirebaseProvider) GetProviderName() string {
	return "firebase"
}

// OneSignal Provider (placeholder implementation)

func (o *OneSignalProvider) SendSMS(to, message string) (*SMSResponse, error) {
	// OneSignal primarily handles push notifications, not SMS
	return nil, fmt.Errorf("OneSignal SMS not implemented yet")
}

func (o *OneSignalProvider) GetDeliveryStatus(messageID string) (*DeliveryStatus, error) {
	return nil, fmt.Errorf("OneSignal SMS delivery status not implemented yet")
}

func (o *OneSignalProvider) GetProviderName() string {
	return "onesignal"
}

// Mock Provider for testing

type MockSMSProvider struct{}

func (m *MockSMSProvider) SendSMS(to, message string) (*SMSResponse, error) {
	logger.Infof("MOCK SMS to %s: %s", to, message)

	return &SMSResponse{
		MessageID: fmt.Sprintf("mock_%d", time.Now().Unix()),
		Status:    "sent",
		Provider:  "mock",
		To:        to,
		Message:   message,
		Cost:      0.0,
		Segments:  1,
		Timestamp: time.Now(),
	}, nil
}

func (m *MockSMSProvider) GetDeliveryStatus(messageID string) (*DeliveryStatus, error) {
	return &DeliveryStatus{
		MessageID:     messageID,
		Status:        "delivered",
		DeliveredAt:   time.Now(),
		LastUpdatedAt: time.Now(),
	}, nil
}

func (m *MockSMSProvider) GetProviderName() string {
	return "mock"
}

// Helper Functions

// mapTwilioStatus maps Twilio status to standard status
func mapTwilioStatus(twilioStatus string) string {
	switch twilioStatus {
	case "queued", "accepted":
		return "pending"
	case "sending", "sent":
		return "sent"
	case "delivered":
		return "delivered"
	case "failed", "undelivered":
		return "failed"
	default:
		return "unknown"
	}
}

// GetSMSService returns the global SMS service instance
var globalSMSService *SMSService

func GetSMSService() *SMSService {
	return globalSMSService
}

func SetSMSService(service *SMSService) {
	globalSMSService = service
}
