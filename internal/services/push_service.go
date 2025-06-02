package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
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

// PushService handles push notification operations
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
	SendNotification(notification *PushNotification) (*PushResponse, error)
	SendBulkNotifications(notifications []*PushNotification) (*BulkPushResponse, error)
	GetProviderName() string
	ValidateToken(token string) error
}

// PushNotification represents a push notification
type PushNotification struct {
	ID          string                 `json:"id"`
	UserID      primitive.ObjectID     `json:"user_id"`
	DeviceToken string                 `json:"device_token"`
	Platform    string                 `json:"platform"` // "ios", "android", "web"
	Title       string                 `json:"title"`
	Body        string                 `json:"body"`
	Data        map[string]interface{} `json:"data,omitempty"`
	Sound       string                 `json:"sound,omitempty"`
	Badge       int                    `json:"badge,omitempty"`
	Icon        string                 `json:"icon,omitempty"`
	Image       string                 `json:"image,omitempty"`
	ClickAction string                 `json:"click_action,omitempty"`
	Priority    string                 `json:"priority,omitempty"` // "high", "normal"
	TTL         int                    `json:"ttl,omitempty"`      // Time to live in seconds
	CollapseKey string                 `json:"collapse_key,omitempty"`
	Category    string                 `json:"category,omitempty"`
	Type        string                 `json:"type,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
}

// PushResponse represents push notification response
type PushResponse struct {
	NotificationID string                 `json:"notification_id"`
	MessageID      string                 `json:"message_id"`
	Status         string                 `json:"status"` // "success", "failed", "invalid_token"
	Error          string                 `json:"error,omitempty"`
	Platform       string                 `json:"platform"`
	Provider       string                 `json:"provider"`
	Timestamp      time.Time              `json:"timestamp"`
	Cost           float64                `json:"cost,omitempty"`
	ProviderData   map[string]interface{} `json:"provider_data,omitempty"`
}

// BulkPushResponse represents bulk push notification response
type BulkPushResponse struct {
	TotalSent     int             `json:"total_sent"`
	TotalFailed   int             `json:"total_failed"`
	Responses     []*PushResponse `json:"responses"`
	InvalidTokens []string        `json:"invalid_tokens"`
	Timestamp     time.Time       `json:"timestamp"`
}

// NotificationTemplate represents notification template
type NotificationTemplate struct {
	Name      string            `json:"name"`
	Title     string            `json:"title"`
	Body      string            `json:"body"`
	Variables []string          `json:"variables"`
	Platform  string            `json:"platform"` // "all", "ios", "android", "web"
	Sound     string            `json:"sound,omitempty"`
	Icon      string            `json:"icon,omitempty"`
	Category  string            `json:"category,omitempty"`
	Priority  string            `json:"priority,omitempty"`
	IsActive  bool              `json:"is_active"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// PushMetrics represents push notification metrics
type PushMetrics struct {
	TotalSent      int64     `json:"total_sent"`
	TotalDelivered int64     `json:"total_delivered"`
	TotalFailed    int64     `json:"total_failed"`
	TotalClicked   int64     `json:"total_clicked"`
	DeliveryRate   float64   `json:"delivery_rate"`
	ClickRate      float64   `json:"click_rate"`
	InvalidTokens  int64     `json:"invalid_tokens"`
	AverageCost    float64   `json:"average_cost"`
	TotalCost      float64   `json:"total_cost"`
	LastUpdated    time.Time `json:"last_updated"`
}

// Firebase Provider
type FirebasePushProvider struct {
	config     *config.FirebaseConfig
	httpClient *http.Client
	projectID  string
}

// OneSignal Provider
type OneSignalPushProvider struct {
	config     *config.OneSignalConfig
	httpClient *http.Client
}

// Default notification templates
var DefaultPushTemplates = map[string]NotificationTemplate{
	"new_message": {
		Name:      "New Message",
		Title:     "{sender_name}",
		Body:      "{message_preview}",
		Variables: []string{"sender_name", "message_preview", "chat_name"},
		Platform:  "all",
		Sound:     "default",
		Category:  "message",
		Priority:  "high",
		IsActive:  true,
	},
	"group_message": {
		Name:      "Group Message",
		Title:     "{chat_name}",
		Body:      "{sender_name}: {message_preview}",
		Variables: []string{"sender_name", "message_preview", "chat_name"},
		Platform:  "all",
		Sound:     "default",
		Category:  "message",
		Priority:  "high",
		IsActive:  true,
	},
	"incoming_call": {
		Name:      "Incoming Call",
		Title:     "Incoming call",
		Body:      "{caller_name} is calling you",
		Variables: []string{"caller_name", "call_type"},
		Platform:  "all",
		Sound:     "ringtone",
		Category:  "call",
		Priority:  "high",
		IsActive:  true,
	},
	"missed_call": {
		Name:      "Missed Call",
		Title:     "Missed call",
		Body:      "You missed a call from {caller_name}",
		Variables: []string{"caller_name", "call_type"},
		Platform:  "all",
		Sound:     "default",
		Category:  "call",
		Priority:  "normal",
		IsActive:  true,
	},
	"group_invite": {
		Name:      "Group Invitation",
		Title:     "Group invitation",
		Body:      "{inviter_name} invited you to join {group_name}",
		Variables: []string{"inviter_name", "group_name"},
		Platform:  "all",
		Sound:     "default",
		Category:  "social",
		Priority:  "normal",
		IsActive:  true,
	},
	"contact_joined": {
		Name:      "Contact Joined",
		Title:     "{contact_name} joined ChatApp",
		Body:      "Say hello to your friend!",
		Variables: []string{"contact_name"},
		Platform:  "all",
		Sound:     "default",
		Category:  "social",
		Priority:  "normal",
		IsActive:  true,
	},
	"security_alert": {
		Name:      "Security Alert",
		Title:     "Security Alert",
		Body:      "New login detected from {device_info}",
		Variables: []string{"device_info", "location"},
		Platform:  "all",
		Sound:     "alert",
		Category:  "security",
		Priority:  "high",
		IsActive:  true,
	},
}

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

// Public Push Service Methods

// SendMessageNotification sends notification for new message
func (s *PushService) SendMessageNotification(senderUser *models.User, recipientUser *models.User, message *models.Message, chat *models.Chat) error {
	// Check if recipient allows notifications
	if !s.shouldSendNotification(recipientUser.ID, "message") {
		return nil
	}

	// Check if chat is muted
	if chat.IsMutedFor(recipientUser.ID) {
		return nil
	}

	// Get recipient's device tokens
	deviceTokens := s.getActiveDeviceTokens(recipientUser.ID)
	if len(deviceTokens) == 0 {
		logger.Debugf("No active device tokens for user %s", recipientUser.ID.Hex())
		return nil
	}

	// Determine template and variables
	var templateName string
	variables := make(map[string]string)

	if chat.Type == models.ChatTypeGroup {
		templateName = "group_message"
		variables["chat_name"] = chat.Name
		if chat.Name == "" {
			variables["chat_name"] = "Group"
		}
	} else {
		templateName = "new_message"
	}

	variables["sender_name"] = senderUser.Name
	variables["message_preview"] = s.getMessagePreview(message)

	// Create notifications for each device
	var notifications []*PushNotification
	for _, token := range deviceTokens {
		notification, err := s.createNotificationFromTemplate(templateName, variables, token)
		if err != nil {
			logger.Errorf("Failed to create notification from template: %v", err)
			continue
		}

		// Add message-specific data
		notification.Data = map[string]interface{}{
			"type":       "message",
			"message_id": message.ID.Hex(),
			"chat_id":    chat.ID.Hex(),
			"sender_id":  senderUser.ID.Hex(),
		}

		// Set badge count
		unreadCount := s.getUnreadCount(recipientUser.ID)
		notification.Badge = int(unreadCount)

		notifications = append(notifications, notification)
	}

	// Send notifications
	return s.sendBulkNotifications(notifications, map[string]interface{}{
		"type":         "message",
		"sender_id":    senderUser.ID.Hex(),
		"recipient_id": recipientUser.ID.Hex(),
		"chat_id":      chat.ID.Hex(),
		"message_id":   message.ID.Hex(),
	})
}

// SendCallNotification sends notification for incoming call
func (s *PushService) SendCallNotification(caller *models.User, recipient *models.User, call *models.Call) error {
	// Check if recipient allows call notifications
	if !s.shouldSendNotification(recipient.ID, "call") {
		return nil
	}

	// Get recipient's device tokens
	deviceTokens := s.getActiveDeviceTokens(recipient.ID)
	if len(deviceTokens) == 0 {
		return nil
	}

	variables := map[string]string{
		"caller_name": caller.Name,
		"call_type":   string(call.Type),
	}

	// Create notifications
	var notifications []*PushNotification
	for _, token := range deviceTokens {
		notification, err := s.createNotificationFromTemplate("incoming_call", variables, token)
		if err != nil {
			continue
		}

		notification.Data = map[string]interface{}{
			"type":      "call",
			"call_id":   call.ID.Hex(),
			"caller_id": caller.ID.Hex(),
			"call_type": string(call.Type),
		}

		// High priority for calls
		notification.Priority = "high"
		notification.TTL = 30 // 30 seconds TTL for calls

		notifications = append(notifications, notification)
	}

	return s.sendBulkNotifications(notifications, map[string]interface{}{
		"type":         "call",
		"caller_id":    caller.ID.Hex(),
		"recipient_id": recipient.ID.Hex(),
		"call_id":      call.ID.Hex(),
	})
}

// SendMissedCallNotification sends notification for missed call
func (s *PushService) SendMissedCallNotification(caller *models.User, recipient *models.User, call *models.Call) error {
	if !s.shouldSendNotification(recipient.ID, "call") {
		return nil
	}

	deviceTokens := s.getActiveDeviceTokens(recipient.ID)
	if len(deviceTokens) == 0 {
		return nil
	}

	variables := map[string]string{
		"caller_name": caller.Name,
		"call_type":   string(call.Type),
	}

	var notifications []*PushNotification
	for _, token := range deviceTokens {
		notification, err := s.createNotificationFromTemplate("missed_call", variables, token)
		if err != nil {
			continue
		}

		notification.Data = map[string]interface{}{
			"type":      "missed_call",
			"call_id":   call.ID.Hex(),
			"caller_id": caller.ID.Hex(),
			"call_type": string(call.Type),
		}

		notifications = append(notifications, notification)
	}

	return s.sendBulkNotifications(notifications, map[string]interface{}{
		"type":         "missed_call",
		"caller_id":    caller.ID.Hex(),
		"recipient_id": recipient.ID.Hex(),
		"call_id":      call.ID.Hex(),
	})
}

// SendGroupInviteNotification sends notification for group invitation
func (s *PushService) SendGroupInviteNotification(inviter *models.User, invitee *models.User, group *models.Group) error {
	if !s.shouldSendNotification(invitee.ID, "social") {
		return nil
	}

	deviceTokens := s.getActiveDeviceTokens(invitee.ID)
	if len(deviceTokens) == 0 {
		return nil
	}

	variables := map[string]string{
		"inviter_name": inviter.Name,
		"group_name":   group.Name,
	}

	var notifications []*PushNotification
	for _, token := range deviceTokens {
		notification, err := s.createNotificationFromTemplate("group_invite", variables, token)
		if err != nil {
			continue
		}

		notification.Data = map[string]interface{}{
			"type":       "group_invite",
			"group_id":   group.ID.Hex(),
			"inviter_id": inviter.ID.Hex(),
		}

		notifications = append(notifications, notification)
	}

	return s.sendBulkNotifications(notifications, map[string]interface{}{
		"type":       "group_invite",
		"inviter_id": inviter.ID.Hex(),
		"invitee_id": invitee.ID.Hex(),
		"group_id":   group.ID.Hex(),
	})
}

// SendSecurityAlert sends security alert notification
func (s *PushService) SendSecurityAlert(user *models.User, alertType string, details map[string]interface{}) error {
	deviceTokens := s.getActiveDeviceTokens(user.ID)
	if len(deviceTokens) == 0 {
		return nil
	}

	deviceInfo := "unknown device"
	if info, exists := details["device_info"]; exists {
		deviceInfo = fmt.Sprintf("%v", info)
	}

	variables := map[string]string{
		"device_info": deviceInfo,
		"location":    fmt.Sprintf("%v", details["location"]),
	}

	var notifications []*PushNotification
	for _, token := range deviceTokens {
		notification, err := s.createNotificationFromTemplate("security_alert", variables, token)
		if err != nil {
			continue
		}

		notification.Data = map[string]interface{}{
			"type":       "security_alert",
			"alert_type": alertType,
			"details":    details,
		}

		notifications = append(notifications, notification)
	}

	return s.sendBulkNotifications(notifications, map[string]interface{}{
		"type":       "security_alert",
		"user_id":    user.ID.Hex(),
		"alert_type": alertType,
	})
}

// SendCustomNotification sends a custom notification
func (s *PushService) SendCustomNotification(userID primitive.ObjectID, title, body string, data map[string]interface{}) error {
	deviceTokens := s.getActiveDeviceTokens(userID)
	if len(deviceTokens) == 0 {
		return nil
	}

	var notifications []*PushNotification
	for _, token := range deviceTokens {
		notification := &PushNotification{
			ID:          primitive.NewObjectID().Hex(),
			UserID:      userID,
			DeviceToken: token.Token,
			Platform:    token.Platform,
			Title:       title,
			Body:        body,
			Data:        data,
			Sound:       "default",
			Priority:    "normal",
			Timestamp:   time.Now(),
		}

		notifications = append(notifications, notification)
	}

	return s.sendBulkNotifications(notifications, map[string]interface{}{
		"type":    "custom",
		"user_id": userID.Hex(),
	})
}

// SendBroadcastNotification sends notification to multiple users
func (s *PushService) SendBroadcastNotification(userIDs []primitive.ObjectID, title, body string, data map[string]interface{}) error {
	var allNotifications []*PushNotification

	for _, userID := range userIDs {
		if !s.shouldSendNotification(userID, "broadcast") {
			continue
		}

		deviceTokens := s.getActiveDeviceTokens(userID)
		for _, token := range deviceTokens {
			notification := &PushNotification{
				ID:          primitive.NewObjectID().Hex(),
				UserID:      userID,
				DeviceToken: token.Token,
				Platform:    token.Platform,
				Title:       title,
				Body:        body,
				Data:        data,
				Sound:       "default",
				Priority:    "normal",
				Timestamp:   time.Now(),
			}

			allNotifications = append(allNotifications, notification)
		}
	}

	// Send in batches to avoid overwhelming the provider
	batchSize := 100
	for i := 0; i < len(allNotifications); i += batchSize {
		end := i + batchSize
		if end > len(allNotifications) {
			end = len(allNotifications)
		}

		batch := allNotifications[i:end]
		if err := s.sendBulkNotifications(batch, map[string]interface{}{
			"type":       "broadcast",
			"batch_size": len(batch),
			"batch_num":  i/batchSize + 1,
		}); err != nil {
			logger.Errorf("Failed to send notification batch: %v", err)
		}
	}

	return nil
}

// Device Token Management

// RegisterDeviceToken registers a device token for push notifications
func (s *PushService) RegisterDeviceToken(userID primitive.ObjectID, token, platform, deviceID, appVersion string) error {
	// Validate token with provider
	if err := s.provider.ValidateToken(token); err != nil {
		return fmt.Errorf("invalid device token: %w", err)
	}

	// Get user
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	user, err := s.getUserByID(userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	// Check if token already exists
	tokenExists := false
	for i, existingToken := range user.DeviceTokens {
		if existingToken.Token == token || existingToken.DeviceID == deviceID {
			// Update existing token
			user.DeviceTokens[i] = models.DeviceToken{
				Token:      token,
				Platform:   platform,
				DeviceID:   deviceID,
				AppVersion: appVersion,
				IsActive:   true,
				CreatedAt:  existingToken.CreatedAt,
				UpdatedAt:  time.Now(),
			}
			tokenExists = true
			break
		}
	}

	// Add new token if not exists
	if !tokenExists {
		user.DeviceTokens = append(user.DeviceTokens, models.DeviceToken{
			Token:      token,
			Platform:   platform,
			DeviceID:   deviceID,
			AppVersion: appVersion,
			IsActive:   true,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		})
	}

	user.BeforeUpdate()

	// Update user in database
	if err := s.updateUser(user); err != nil {
		return fmt.Errorf("failed to register device token: %w", err)
	}

	logger.LogUserAction(userID.Hex(), "register_device_token", "push_service", map[string]interface{}{
		"platform":    platform,
		"device_id":   deviceID,
		"app_version": appVersion,
	})

	return nil
}

// UnregisterDeviceToken removes a device token
func (s *PushService) UnregisterDeviceToken(userID primitive.ObjectID, token string) error {
	user, err := s.getUserByID(userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	// Remove token
	for i, deviceToken := range user.DeviceTokens {
		if deviceToken.Token == token {
			user.DeviceTokens = append(user.DeviceTokens[:i], user.DeviceTokens[i+1:]...)
			break
		}
	}

	user.BeforeUpdate()

	if err := s.updateUser(user); err != nil {
		return fmt.Errorf("failed to unregister device token: %w", err)
	}

	logger.LogUserAction(userID.Hex(), "unregister_device_token", "push_service", map[string]interface{}{
		"token": token[:10] + "...", // Log only first 10 chars for security
	})

	return nil
}

// Helper Methods

// createNotificationFromTemplate creates notification from template
func (s *PushService) createNotificationFromTemplate(templateName string, variables map[string]string, token models.DeviceToken) (*PushNotification, error) {
	template, exists := DefaultPushTemplates[templateName]
	if !exists {
		return nil, fmt.Errorf("template not found: %s", templateName)
	}

	if !template.IsActive {
		return nil, fmt.Errorf("template is inactive: %s", templateName)
	}

	// Check platform compatibility
	if template.Platform != "all" && template.Platform != token.Platform {
		return nil, fmt.Errorf("template not compatible with platform: %s", token.Platform)
	}

	// Replace variables in title and body
	title := s.replaceVariables(template.Title, variables)
	body := s.replaceVariables(template.Body, variables)

	notification := &PushNotification{
		ID:          primitive.NewObjectID().Hex(),
		DeviceToken: token.Token,
		Platform:    token.Platform,
		Title:       title,
		Body:        body,
		Sound:       template.Sound,
		Icon:        template.Icon,
		Category:    template.Category,
		Priority:    template.Priority,
		Timestamp:   time.Now(),
	}

	return notification, nil
}

// replaceVariables replaces variables in text
func (s *PushService) replaceVariables(text string, variables map[string]string) string {
	result := text
	for key, value := range variables {
		placeholder := fmt.Sprintf("{%s}", key)
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

// getActiveDeviceTokens gets active device tokens for user
func (s *PushService) getActiveDeviceTokens(userID primitive.ObjectID) []models.DeviceToken {
	user, err := s.getUserByID(userID)
	if err != nil {
		return nil
	}

	var activeTokens []models.DeviceToken
	for _, token := range user.DeviceTokens {
		if token.IsActive {
			activeTokens = append(activeTokens, token)
		}
	}

	return activeTokens
}

// shouldSendNotification checks if notification should be sent
func (s *PushService) shouldSendNotification(userID primitive.ObjectID, notificationType string) bool {
	// Check user notification preferences
	// This would typically check user settings in database
	// For now, return true by default
	return true
}

// getUnreadCount gets unread message count for user
func (s *PushService) getUnreadCount(userID primitive.ObjectID) int64 {
	if s.redisClient == nil {
		return 0
	}

	// Try to get from Redis cache first
	key := fmt.Sprintf("unread_count:%s", userID.Hex())
	countStr, err := s.redisClient.Get(key)
	if err == nil {
		if count, err := strconv.ParseInt(countStr, 10, 64); err == nil {
			return count
		}
	}

	// Calculate from database if not in cache
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		{
			"$match": bson.M{
				"participants": userID,
				"is_active":    true,
			},
		},
		{
			"$unwind": "$unread_counts",
		},
		{
			"$match": bson.M{
				"unread_counts.user_id": userID,
			},
		},
		{
			"$group": bson.M{
				"_id": nil,
				"total_unread": bson.M{
					"$sum": "$unread_counts.count",
				},
			},
		},
	}

	cursor, err := s.collections.Chats.Aggregate(ctx, pipeline)
	if err != nil {
		return 0
	}
	defer cursor.Close(ctx)

	var result struct {
		TotalUnread int64 `bson:"total_unread"`
	}

	if cursor.Next(ctx) {
		cursor.Decode(&result)
	}

	// Cache the result
	if s.redisClient != nil {
		s.redisClient.SetEX(key, result.TotalUnread, 5*time.Minute)
	}

	return result.TotalUnread
}

// getMessagePreview generates message preview for notification
func (s *PushService) getMessagePreview(message *models.Message) string {
	if message.IsDeleted {
		return "This message was deleted"
	}

	switch message.Type {
	case models.MessageTypeText:
		if len(message.Content) > 50 {
			return message.Content[:50] + "..."
		}
		return message.Content
	case models.MessageTypeImage:
		return "📷 Photo"
	case models.MessageTypeVideo:
		return "🎥 Video"
	case models.MessageTypeAudio:
		return "🎵 Audio"
	case models.MessageTypeVoiceNote:
		return "🎤 Voice message"
	case models.MessageTypeDocument:
		return "📄 " + message.FileName
	case models.MessageTypeLocation:
		return "📍 Location"
	case models.MessageTypeContact:
		return "👤 Contact"
	case models.MessageTypeSticker:
		return "😊 Sticker"
	case models.MessageTypeGIF:
		return "🎞️ GIF"
	default:
		return "Message"
	}
}

// sendBulkNotifications sends multiple notifications
func (s *PushService) sendBulkNotifications(notifications []*PushNotification, metadata map[string]interface{}) error {
	if len(notifications) == 0 {
		return nil
	}

	startTime := time.Now()

	// Send notifications via provider
	response, err := s.provider.SendBulkNotifications(notifications)
	if err != nil {
		duration := time.Since(startTime)
		logger.Errorf("Failed to send bulk notifications: %v (duration: %dms)", err, duration.Milliseconds())

		s.logPushEvent("bulk_notification_failed", len(notifications), map[string]interface{}{
			"error":       err.Error(),
			"duration_ms": duration.Milliseconds(),
			"metadata":    metadata,
		})

		return fmt.Errorf("failed to send notifications: %w", err)
	}

	// Handle invalid tokens
	if len(response.InvalidTokens) > 0 {
		go s.cleanupInvalidTokens(response.InvalidTokens)
	}

	// Log successful send
	duration := time.Since(startTime)
	s.logPushEvent("bulk_notification_sent", len(notifications), map[string]interface{}{
		"total_sent":     response.TotalSent,
		"total_failed":   response.TotalFailed,
		"invalid_tokens": len(response.InvalidTokens),
		"duration_ms":    duration.Milliseconds(),
		"metadata":       metadata,
	})

	logger.Infof("Sent %d notifications successfully (%d failed, %d invalid tokens)",
		response.TotalSent, response.TotalFailed, len(response.InvalidTokens))

	return nil
}

// cleanupInvalidTokens removes invalid device tokens
func (s *PushService) cleanupInvalidTokens(invalidTokens []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, token := range invalidTokens {
		// Remove invalid token from all users
		_, err := s.collections.Users.UpdateMany(ctx,
			bson.M{},
			bson.M{
				"$pull": bson.M{
					"device_tokens": bson.M{
						"token": token,
					},
				},
			},
		)
		if err != nil {
			logger.Errorf("Failed to remove invalid token %s: %v", token, err)
		}
	}

	logger.Infof("Cleaned up %d invalid device tokens", len(invalidTokens))
}

// getUserByID gets user by ID
func (s *PushService) getUserByID(userID primitive.ObjectID) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err := s.collections.Users.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// updateUser updates user in database
func (s *PushService) updateUser(user *models.User) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.collections.Users.ReplaceOne(ctx, bson.M{"_id": user.ID}, user)
	return err
}

// logPushEvent logs push notification events
func (s *PushService) logPushEvent(event string, count int, metadata map[string]interface{}) {
	fields := map[string]interface{}{
		"event":              event,
		"notification_count": count,
		"provider":           s.provider.GetProviderName(),
		"type":               "push_event",
	}

	for k, v := range metadata {
		fields[k] = v
	}

	logger.WithFields(fields).Info("Push notification event")
}

// Provider Implementations

// Firebase Provider Implementation

func (f *FirebasePushProvider) SendNotification(notification *PushNotification) (*PushResponse, error) {
	// This is a simplified implementation
	// In production, you would use the Firebase Admin SDK

	fcmURL := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", f.projectID)

	// Construct FCM message
	message := map[string]interface{}{
		"message": map[string]interface{}{
			"token": notification.DeviceToken,
			"notification": map[string]interface{}{
				"title": notification.Title,
				"body":  notification.Body,
			},
			"data": notification.Data,
		},
	}

	if notification.Sound != "" {
		message["message"].(map[string]interface{})["android"] = map[string]interface{}{
			"notification": map[string]interface{}{
				"sound": notification.Sound,
			},
		}
		message["message"].(map[string]interface{})["apns"] = map[string]interface{}{
			"payload": map[string]interface{}{
				"aps": map[string]interface{}{
					"sound": notification.Sound,
				},
			},
		}
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal FCM message: %w", err)
	}

	req, err := http.NewRequest("POST", fcmURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// You would need to implement OAuth2 authentication here
	// req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var fcmResp struct {
		Name  string `json:"name"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&fcmResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	response := &PushResponse{
		NotificationID: notification.ID,
		MessageID:      fcmResp.Name,
		Platform:       notification.Platform,
		Provider:       "firebase",
		Timestamp:      time.Now(),
	}

	if resp.StatusCode != 200 {
		response.Status = "failed"
		response.Error = fcmResp.Error.Message
	} else {
		response.Status = "success"
	}

	return response, nil
}

func (f *FirebasePushProvider) SendBulkNotifications(notifications []*PushNotification) (*BulkPushResponse, error) {
	response := &BulkPushResponse{
		Responses: make([]*PushResponse, 0, len(notifications)),
		Timestamp: time.Now(),
	}

	for _, notification := range notifications {
		resp, err := f.SendNotification(notification)
		if err != nil {
			response.TotalFailed++
			// Check if it's an invalid token error
			if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "token") {
				response.InvalidTokens = append(response.InvalidTokens, notification.DeviceToken)
			}
		} else {
			if resp.Status == "success" {
				response.TotalSent++
			} else {
				response.TotalFailed++
			}
		}
		response.Responses = append(response.Responses, resp)
	}

	return response, nil
}

func (f *FirebasePushProvider) GetProviderName() string {
	return "firebase"
}

func (f *FirebasePushProvider) ValidateToken(token string) error {
	// Basic validation - in production you might want to verify with FCM
	if token == "" {
		return fmt.Errorf("token is empty")
	}
	if len(token) < 10 {
		return fmt.Errorf("token too short")
	}
	return nil
}

// OneSignal Provider Implementation

func (o *OneSignalPushProvider) SendNotification(notification *PushNotification) (*PushResponse, error) {
	if o.config.AppID == "" || o.config.RestAPIKey == "" {
		return nil, fmt.Errorf("OneSignal credentials not configured")
	}

	// OneSignal API endpoint
	url := "https://onesignal.com/api/v1/notifications"

	// Construct OneSignal message
	message := map[string]interface{}{
		"app_id":             o.config.AppID,
		"headings":           map[string]string{"en": notification.Title},
		"contents":           map[string]string{"en": notification.Body},
		"include_player_ids": []string{notification.DeviceToken},
	}

	if notification.Data != nil {
		message["data"] = notification.Data
	}

	if notification.Sound != "" {
		message["ios_sound"] = notification.Sound
		message["android_sound"] = notification.Sound
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OneSignal message: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Basic "+o.config.RestAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var oneSignalResp struct {
		ID     string                 `json:"id"`
		Errors map[string]interface{} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&oneSignalResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	response := &PushResponse{
		NotificationID: notification.ID,
		MessageID:      oneSignalResp.ID,
		Platform:       notification.Platform,
		Provider:       "onesignal",
		Timestamp:      time.Now(),
	}

	if len(oneSignalResp.Errors) > 0 {
		response.Status = "failed"
		response.Error = fmt.Sprintf("%v", oneSignalResp.Errors)
	} else {
		response.Status = "success"
	}

	return response, nil
}

func (o *OneSignalPushProvider) SendBulkNotifications(notifications []*PushNotification) (*BulkPushResponse, error) {
	response := &BulkPushResponse{
		Responses: make([]*PushResponse, 0, len(notifications)),
		Timestamp: time.Now(),
	}

	// OneSignal supports bulk sending, but for simplicity, we'll send one by one
	for _, notification := range notifications {
		resp, err := o.SendNotification(notification)
		if err != nil {
			response.TotalFailed++
		} else {
			if resp.Status == "success" {
				response.TotalSent++
			} else {
				response.TotalFailed++
			}
		}
		response.Responses = append(response.Responses, resp)
	}

	return response, nil
}

func (o *OneSignalPushProvider) GetProviderName() string {
	return "onesignal"
}

func (o *OneSignalPushProvider) ValidateToken(token string) error {
	if token == "" {
		return fmt.Errorf("token is empty")
	}
	return nil
}

// Mock Provider for testing

type MockPushProvider struct{}

func (m *MockPushProvider) SendNotification(notification *PushNotification) (*PushResponse, error) {
	logger.Infof("MOCK PUSH to %s (%s): %s - %s",
		notification.DeviceToken[:10]+"...", notification.Platform, notification.Title, notification.Body)

	return &PushResponse{
		NotificationID: notification.ID,
		MessageID:      fmt.Sprintf("mock_%d", time.Now().Unix()),
		Status:         "success",
		Platform:       notification.Platform,
		Provider:       "mock",
		Timestamp:      time.Now(),
	}, nil
}

func (m *MockPushProvider) SendBulkNotifications(notifications []*PushNotification) (*BulkPushResponse, error) {
	response := &BulkPushResponse{
		TotalSent: len(notifications),
		Responses: make([]*PushResponse, len(notifications)),
		Timestamp: time.Now(),
	}

	for i, notification := range notifications {
		resp, _ := m.SendNotification(notification)
		response.Responses[i] = resp
	}

	return response, nil
}

func (m *MockPushProvider) GetProviderName() string {
	return "mock"
}

func (m *MockPushProvider) ValidateToken(token string) error {
	return nil
}

// Global service instance
var globalPushService *PushService

func GetPushService() *PushService {
	return globalPushService
}

func SetPushService(service *PushService) {
	globalPushService = service
}
