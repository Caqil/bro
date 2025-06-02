package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"bro/internal/config"
	"bro/internal/models"
	"bro/pkg/database"
	"bro/pkg/logger"
	"bro/pkg/redis"
)

// PushService handles push notifications
type PushService struct {
	config      *config.Config
	db          *mongo.Database
	collections *database.Collections
	redisClient *redis.Client
	httpClient  *http.Client
	provider    PushProvider
}

// PushProvider interface for different push notification providers
type PushProvider interface {
	SendNotification(notification *PushNotification) error
	SendBulkNotification(notifications []*PushNotification) error
	GetProviderName() string
}

// PushNotification represents a push notification
type PushNotification struct {
	UserID       primitive.ObjectID `json:"user_id"`
	DeviceTokens []string           `json:"device_tokens"`
	Title        string             `json:"title"`
	Body         string             `json:"body"`
	Data         map[string]string  `json:"data,omitempty"`
	Sound        string             `json:"sound,omitempty"`
	Badge        int                `json:"badge,omitempty"`
	Icon         string             `json:"icon,omitempty"`
	ClickAction  string             `json:"click_action,omitempty"`
	Priority     string             `json:"priority,omitempty"`
	TTL          time.Duration      `json:"ttl,omitempty"`
	CollapseKey  string             `json:"collapse_key,omitempty"`
	Image        string             `json:"image,omitempty"`
}

// NotificationTemplate represents a notification template
type NotificationTemplate struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Title     string                 `json:"title"`
	Body      string                 `json:"body"`
	Data      map[string]string      `json:"data,omitempty"`
	Variables []string               `json:"variables,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// NotificationResult represents the result of sending a notification
type NotificationResult struct {
	Success     bool   `json:"success"`
	MessageID   string `json:"message_id,omitempty"`
	Error       string `json:"error,omitempty"`
	DeviceToken string `json:"device_token"`
	FailureCode string `json:"failure_code,omitempty"`
	RetryAfter  int    `json:"retry_after,omitempty"`
}

// BulkNotificationResult represents the result of sending bulk notifications
type BulkNotificationResult struct {
	TotalSent   int                  `json:"total_sent"`
	TotalFailed int                  `json:"total_failed"`
	Results     []NotificationResult `json:"results"`
	MulticastID string               `json:"multicast_id,omitempty"`
}

// Firebase Push Provider
type FirebasePushProvider struct {
	config     *config.FirebaseConfig
	httpClient *http.Client
	projectID  string
}

// OneSignal Push Provider
type OneSignalPushProvider struct {
	config     *config.OneSignalConfig
	httpClient *http.Client
}

// Mock Push Provider for testing
type MockPushProvider struct{}

// NewPushService creates a new push notification service
func NewPushService(config *config.Config) *PushService {
	db := database.GetDB()
	collections := database.GetCollections()
	redisClient := redis.GetClient()

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	service := &PushService{
		config:      config,
		db:          db,
		collections: collections,
		redisClient: redisClient,
		httpClient:  httpClient,
	}

	// Initialize provider based on configuration
	switch strings.ToLower(config.PushProvider) {
	case "firebase":
		service.provider = &FirebasePushProvider{
			config:     &config.FirebaseConfig,
			httpClient: httpClient,
			projectID:  config.FirebaseConfig.ProjectID,
		}
	case "onesignal":
		service.provider = &OneSignalPushProvider{
			config:     &config.OneSignalConfig,
			httpClient: httpClient,
		}
	default:
		logger.Warn("No push provider configured, using mock provider")
		service.provider = &MockPushProvider{}
	}

	logger.Infof("Push service initialized with provider: %s", service.provider.GetProviderName())
	return service
}

// SendNotification sends a push notification to a user
func (ps *PushService) SendNotification(userID primitive.ObjectID, title, body string, data map[string]string) error {
	// Get user's device tokens
	deviceTokens, err := ps.getUserDeviceTokens(userID)
	if err != nil {
		return fmt.Errorf("failed to get device tokens: %w", err)
	}

	if len(deviceTokens) == 0 {
		logger.Debugf("No device tokens found for user %s", userID.Hex())
		return nil
	}

	notification := &PushNotification{
		UserID:       userID,
		DeviceTokens: deviceTokens,
		Title:        title,
		Body:         body,
		Data:         data,
		Sound:        "default",
		Priority:     "high",
		TTL:          24 * time.Hour,
	}

	err = ps.provider.SendNotification(notification)
	if err != nil {
		logger.LogPushNotification(ps.provider.GetProviderName(), userID.Hex(), "", false, err)
		return err
	}

	logger.LogPushNotification(ps.provider.GetProviderName(), userID.Hex(), "", true, nil)
	return nil
}

// SendNotificationToDevice sends a push notification to a specific device
func (ps *PushService) SendNotificationToDevice(deviceToken, title, body string, data map[string]string) error {
	notification := &PushNotification{
		DeviceTokens: []string{deviceToken},
		Title:        title,
		Body:         body,
		Data:         data,
		Sound:        "default",
		Priority:     "high",
		TTL:          24 * time.Hour,
	}

	err := ps.provider.SendNotification(notification)
	if err != nil {
		logger.LogPushNotification(ps.provider.GetProviderName(), "", deviceToken, false, err)
		return err
	}

	logger.LogPushNotification(ps.provider.GetProviderName(), "", deviceToken, true, nil)
	return nil
}

// SendBulkNotification sends notifications to multiple users
func (ps *PushService) SendBulkNotification(userIDs []primitive.ObjectID, title, body string, data map[string]string) error {
	var notifications []*PushNotification

	for _, userID := range userIDs {
		deviceTokens, err := ps.getUserDeviceTokens(userID)
		if err != nil {
			logger.Errorf("Failed to get device tokens for user %s: %v", userID.Hex(), err)
			continue
		}

		if len(deviceTokens) > 0 {
			notification := &PushNotification{
				UserID:       userID,
				DeviceTokens: deviceTokens,
				Title:        title,
				Body:         body,
				Data:         data,
				Sound:        "default",
				Priority:     "high",
				TTL:          24 * time.Hour,
			}
			notifications = append(notifications, notification)
		}
	}

	if len(notifications) == 0 {
		logger.Debug("No notifications to send in bulk")
		return nil
	}

	return ps.provider.SendBulkNotification(notifications)
}

// SendMessageNotification sends a notification for a new message
func (ps *PushService) SendMessageNotification(message *models.Message, chat *models.Chat, sender *models.User) error {
	// Get chat participants excluding sender
	var recipients []primitive.ObjectID
	for _, participantID := range chat.Participants {
		if participantID != sender.ID {
			recipients = append(recipients, participantID)
		}
	}

	if len(recipients) == 0 {
		return nil
	}

	// Check if users have muted the chat
	var activeRecipients []primitive.ObjectID
	for _, recipientID := range recipients {
		if !chat.IsMutedFor(recipientID) {
			activeRecipients = append(activeRecipients, recipientID)
		}
	}

	if len(activeRecipients) == 0 {
		return nil
	}

	// Prepare notification content
	title := sender.Name
	body := message.GetPreviewText()

	// For group chats, include group name
	if chat.Type == models.ChatTypeGroup && chat.Name != "" {
		title = fmt.Sprintf("%s in %s", sender.Name, chat.Name)
	}

	data := map[string]string{
		"type":       "message",
		"chat_id":    chat.ID.Hex(),
		"message_id": message.ID.Hex(),
		"sender_id":  sender.ID.Hex(),
	}

	return ps.SendBulkNotification(activeRecipients, title, body, data)
}

// SendCallNotification sends a notification for an incoming call
func (ps *PushService) SendCallNotification(call *models.Call, caller *models.User, recipient primitive.ObjectID) error {
	title := "Incoming Call"
	body := fmt.Sprintf("%s is calling you", caller.Name)

	if call.Type == models.CallTypeVideo {
		title = "Incoming Video Call"
		body = fmt.Sprintf("%s is video calling you", caller.Name)
	}

	data := map[string]string{
		"type":      "call",
		"call_id":   call.ID.Hex(),
		"call_type": string(call.Type),
		"caller_id": caller.ID.Hex(),
	}

	return ps.SendNotification(recipient, title, body, data)
}

// SendTemplateNotification sends a notification using a template
func (ps *PushService) SendTemplateNotification(userID primitive.ObjectID, templateID string, variables map[string]string) error {
	template, err := ps.getNotificationTemplate(templateID)
	if err != nil {
		return fmt.Errorf("failed to get template: %w", err)
	}

	// Replace variables in template
	title := ps.replaceVariables(template.Title, variables)
	body := ps.replaceVariables(template.Body, variables)

	// Merge template data with additional data
	data := make(map[string]string)
	for k, v := range template.Data {
		data[k] = v
	}
	for k, v := range variables {
		data[k] = v
	}

	return ps.SendNotification(userID, title, body, data)
}

// UpdateDeviceToken updates device token for a user
func (ps *PushService) UpdateDeviceToken(userID primitive.ObjectID, deviceToken, platform, deviceID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Find existing device token
	filter := bson.M{"_id": userID}
	var user models.User
	err := ps.collections.Users.FindOne(ctx, filter).Decode(&user)
	if err != nil {
		return fmt.Errorf("failed to find user: %w", err)
	}

	// Update or add device token
	tokenExists := false
	for i, token := range user.DeviceTokens {
		if token.DeviceID == deviceID {
			user.DeviceTokens[i].Token = deviceToken
			user.DeviceTokens[i].Platform = platform
			user.DeviceTokens[i].IsActive = true
			user.DeviceTokens[i].UpdatedAt = time.Now()
			tokenExists = true
			break
		}
	}

	if !tokenExists {
		newToken := models.DeviceToken{
			Token:     deviceToken,
			Platform:  platform,
			DeviceID:  deviceID,
			IsActive:  true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		user.DeviceTokens = append(user.DeviceTokens, newToken)
	}

	// Update user in database
	update := bson.M{
		"$set": bson.M{
			"device_tokens": user.DeviceTokens,
			"updated_at":    time.Now(),
		},
	}

	_, err = ps.collections.Users.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update device token: %w", err)
	}

	logger.Infof("Device token updated for user %s on platform %s", userID.Hex(), platform)
	return nil
}

// RemoveDeviceToken removes a device token for a user
func (ps *PushService) RemoveDeviceToken(userID primitive.ObjectID, deviceID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"_id": userID}
	update := bson.M{
		"$pull": bson.M{
			"device_tokens": bson.M{
				"device_id": deviceID,
			},
		},
		"$set": bson.M{
			"updated_at": time.Now(),
		},
	}

	_, err := ps.collections.Users.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to remove device token: %w", err)
	}

	logger.Infof("Device token removed for user %s device %s", userID.Hex(), deviceID)
	return nil
}

// Private helper methods

// getUserDeviceTokens retrieves active device tokens for a user
func (ps *PushService) getUserDeviceTokens(userID primitive.ObjectID) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err := ps.collections.Users.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		return nil, err
	}

	var tokens []string
	for _, deviceToken := range user.DeviceTokens {
		if deviceToken.IsActive && deviceToken.Token != "" {
			tokens = append(tokens, deviceToken.Token)
		}
	}

	return tokens, nil
}

// getNotificationTemplate retrieves a notification template
func (ps *PushService) getNotificationTemplate(templateID string) (*NotificationTemplate, error) {
	// This would typically fetch from database
	// For now, return some default templates
	templates := map[string]*NotificationTemplate{
		"welcome": {
			ID:    "welcome",
			Name:  "Welcome",
			Title: "Welcome to ChatApp!",
			Body:  "Hello {{username}}, welcome to our chat app!",
			Data: map[string]string{
				"template_id": "welcome",
			},
			Variables: []string{"username"},
		},
		"friend_request": {
			ID:    "friend_request",
			Name:  "Friend Request",
			Title: "New Friend Request",
			Body:  "{{sender_name}} sent you a friend request",
			Data: map[string]string{
				"template_id": "friend_request",
				"type":        "friend_request",
			},
			Variables: []string{"sender_name"},
		},
	}

	template, exists := templates[templateID]
	if !exists {
		return nil, fmt.Errorf("template not found: %s", templateID)
	}

	return template, nil
}

// replaceVariables replaces variables in a string template
func (ps *PushService) replaceVariables(template string, variables map[string]string) string {
	result := template
	for key, value := range variables {
		placeholder := fmt.Sprintf("{{%s}}", key)
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

// Firebase Provider Implementation

func (fp *FirebasePushProvider) SendNotification(notification *PushNotification) error {
	url := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", fp.projectID)

	for _, token := range notification.DeviceTokens {
		message := map[string]interface{}{
			"message": map[string]interface{}{
				"token": token,
				"notification": map[string]interface{}{
					"title": notification.Title,
					"body":  notification.Body,
				},
				"data": notification.Data,
				"android": map[string]interface{}{
					"priority": "high",
					"notification": map[string]interface{}{
						"sound": notification.Sound,
					},
				},
				"apns": map[string]interface{}{
					"payload": map[string]interface{}{
						"aps": map[string]interface{}{
							"alert": map[string]interface{}{
								"title": notification.Title,
								"body":  notification.Body,
							},
							"sound": notification.Sound,
							"badge": notification.Badge,
						},
					},
				},
			},
		}

		jsonData, err := json.Marshal(message)
		if err != nil {
			return fmt.Errorf("failed to marshal FCM message: %w", err)
		}

		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return fmt.Errorf("failed to create FCM request: %w", err)
		}

		// Add authorization header (would need to implement OAuth2 token generation)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+fp.getAccessToken())

		resp, err := fp.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to send FCM notification: %w", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("FCM returned status code: %d", resp.StatusCode)
		}
	}

	return nil
}

func (fp *FirebasePushProvider) SendBulkNotification(notifications []*PushNotification) error {
	// Firebase FCM v1 doesn't support bulk sending directly
	// We need to send individual notifications
	for _, notification := range notifications {
		if err := fp.SendNotification(notification); err != nil {
			logger.Errorf("Failed to send notification to user %s: %v", notification.UserID.Hex(), err)
		}
	}
	return nil
}

func (fp *FirebasePushProvider) GetProviderName() string {
	return "firebase"
}

func (fp *FirebasePushProvider) getAccessToken() string {
	// This should implement OAuth2 token generation using Firebase service account
	// For now, return a placeholder
	return "placeholder-access-token"
}

// OneSignal Provider Implementation

func (op *OneSignalPushProvider) SendNotification(notification *PushNotification) error {
	url := "https://onesignal.com/api/v1/notifications"

	message := map[string]interface{}{
		"app_id":             op.config.AppID,
		"include_player_ids": notification.DeviceTokens,
		"headings": map[string]string{
			"en": notification.Title,
		},
		"contents": map[string]string{
			"en": notification.Body,
		},
		"data": notification.Data,
	}

	if notification.Sound != "" {
		message["ios_sound"] = notification.Sound
		message["android_sound"] = notification.Sound
	}

	if notification.Badge > 0 {
		message["ios_badgeType"] = "SetTo"
		message["ios_badgeCount"] = notification.Badge
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal OneSignal message: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create OneSignal request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+op.config.RestAPIKey)

	resp, err := op.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send OneSignal notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OneSignal returned status code: %d", resp.StatusCode)
	}

	return nil
}

func (op *OneSignalPushProvider) SendBulkNotification(notifications []*PushNotification) error {
	// OneSignal supports bulk sending
	// Combine all device tokens and send as one notification
	var allTokens []string
	var title, body string
	var data map[string]string

	for _, notification := range notifications {
		allTokens = append(allTokens, notification.DeviceTokens...)
		if title == "" {
			title = notification.Title
			body = notification.Body
			data = notification.Data
		}
	}

	if len(allTokens) == 0 {
		return nil
	}

	bulkNotification := &PushNotification{
		DeviceTokens: allTokens,
		Title:        title,
		Body:         body,
		Data:         data,
	}

	return op.SendNotification(bulkNotification)
}

func (op *OneSignalPushProvider) GetProviderName() string {
	return "onesignal"
}

// Mock Provider Implementation

func (mp *MockPushProvider) SendNotification(notification *PushNotification) error {
	logger.Infof("Mock: Sending notification to %d devices: %s - %s",
		len(notification.DeviceTokens), notification.Title, notification.Body)
	return nil
}

func (mp *MockPushProvider) SendBulkNotification(notifications []*PushNotification) error {
	total := 0
	for _, notification := range notifications {
		total += len(notification.DeviceTokens)
	}
	logger.Infof("Mock: Sending bulk notification to %d total devices", total)
	return nil
}

func (mp *MockPushProvider) GetProviderName() string {
	return "mock"
}
