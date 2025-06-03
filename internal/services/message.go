package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bro/internal/config"
	"bro/internal/models"
	"bro/internal/utils"
	"bro/internal/websocket"
	"bro/pkg/database"
	"bro/pkg/logger"
	"bro/pkg/redis"
)

// MessageService handles all message-related operations
type MessageService struct {
	// Configuration
	config *config.Config

	// Database collections
	messagesCollection *mongo.Collection
	chatsCollection    *mongo.Collection
	usersCollection    *mongo.Collection

	// External services
	hub           *websocket.Hub
	redisClient   *redis.Client
	pushService   *PushService
	chatService   *ChatService
	encryptionSvc *utils.EncryptionService

	// Message processing
	messageQueue chan *MessageProcessingTask
	workers      int

	// Statistics
	messageStats *MessageStatistics

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// MessageStatistics contains message service statistics
type MessageStatistics struct {
	TotalMessages     int64
	MessagesToday     int64
	MessagesThisHour  int64
	TextMessages      int64
	MediaMessages     int64
	VoiceMessages     int64
	FileMessages      int64
	DeletedMessages   int64
	EditedMessages    int64
	ForwardedMessages int64
	LastUpdated       time.Time
	mutex             sync.RWMutex
}

// MessageProcessingTask represents a message processing task
type MessageProcessingTask struct {
	Message   *models.Message
	ChatID    primitive.ObjectID
	SenderID  primitive.ObjectID
	Type      string
	Priority  int
	CreatedAt time.Time
}

// SendMessageRequest represents a message sending request
type SendMessageRequest struct {
	ChatID      primitive.ObjectID     `json:"chat_id" validate:"required"`
	Type        models.MessageType     `json:"type" validate:"required"`
	Content     string                 `json:"content,omitempty"`
	MediaURL    string                 `json:"media_url,omitempty"`
	ReplyToID   *primitive.ObjectID    `json:"reply_to_id,omitempty"`
	Mentions    []primitive.ObjectID   `json:"mentions,omitempty"`
	ScheduledAt *time.Time             `json:"scheduled_at,omitempty"`
	Metadata    models.MessageMetadata `json:"metadata,omitempty"`
}

// MessageListRequest represents parameters for listing messages
type MessageListRequest struct {
	ChatID   primitive.ObjectID  `form:"chat_id" json:"chat_id" validate:"required"`
	Page     int                 `form:"page" json:"page"`
	Limit    int                 `form:"limit" json:"limit"`
	Before   *primitive.ObjectID `form:"before" json:"before"`
	After    *primitive.ObjectID `form:"after" json:"after"`
	Type     string              `form:"type" json:"type"`
	Search   string              `form:"search" json:"search"`
	SenderID *primitive.ObjectID `form:"sender_id" json:"sender_id"`
	DateFrom *time.Time          `form:"date_from" json:"date_from"`
	DateTo   *time.Time          `form:"date_to" json:"date_to"`
}

// MessageUpdateRequest represents a message update request
type MessageUpdateRequest struct {
	Content string `json:"content" validate:"required"`
	Reason  string `json:"reason,omitempty"`
}

// MessageReactionRequest represents a reaction request
type MessageReactionRequest struct {
	Emoji string `json:"emoji" validate:"required"`
}

// NewMessageService creates a new message service
func NewMessageService(
	cfg *config.Config,
	hub *websocket.Hub,
	pushService *PushService,
	chatService *ChatService,
) (*MessageService, error) {

	collections := database.GetCollections()
	if collections == nil {
		return nil, fmt.Errorf("database collections not available")
	}

	ctx, cancel := context.WithCancel(context.Background())

	encryptionSvc, err := utils.NewEncryptionService(cfg.EncryptionKey)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create encryption service: %w", err)
	}

	service := &MessageService{
		config:             cfg,
		messagesCollection: collections.Messages,
		chatsCollection:    collections.Chats,
		usersCollection:    collections.Users,
		hub:                hub,
		redisClient:        redis.GetClient(),
		pushService:        pushService,
		chatService:        chatService,
		encryptionSvc:      encryptionSvc,
		messageQueue:       make(chan *MessageProcessingTask, 1000),
		workers:            5,
		messageStats:       &MessageStatistics{},
		ctx:                ctx,
		cancel:             cancel,
	}

	// Start background processes
	service.wg.Add(2)
	go service.statsCollector()
	go service.messageProcessor()

	// Start worker goroutines
	for i := 0; i < service.workers; i++ {
		service.wg.Add(1)
		go service.messageWorker(i)
	}

	logger.Info("Message Service initialized successfully")
	return service, nil
}

// SendMessage sends a new message
func (ms *MessageService) SendMessage(senderID primitive.ObjectID, req *SendMessageRequest) (*models.MessageResponse, error) {
	// Validate request
	if err := ms.validateSendMessageRequest(req); err != nil {
		return nil, fmt.Errorf("invalid message request: %w", err)
	}

	// Get chat and verify permissions
	chat, err := ms.getChatFromDatabase(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("chat not found: %w", err)
	}

	// Check if user is participant
	if !chat.IsParticipant(senderID) {
		return nil, fmt.Errorf("user is not a participant of this chat")
	}

	// Check if user can send messages
	if !chat.CanUserMessage(senderID) {
		return nil, fmt.Errorf("user cannot send messages in this chat")
	}

	// Validate mentions
	if err := ms.validateMentions(req.Mentions, chat); err != nil {
		return nil, fmt.Errorf("invalid mentions: %w", err)
	}

	// Create message
	message := &models.Message{
		ChatID:      req.ChatID,
		SenderID:    senderID,
		Type:        req.Type,
		Content:     req.Content,
		MediaURL:    req.MediaURL,
		ReplyToID:   req.ReplyToID,
		Mentions:    req.Mentions,
		ScheduledAt: req.ScheduledAt,
		Metadata:    req.Metadata,
		IsEncrypted: true,
	}

	// Set before create
	message.BeforeCreate()

	// Encrypt content if needed
	if message.Content != "" && message.IsEncrypted {
		encryptedContent, err := ms.encryptMessageContent(message.Content, chat.EncryptionKey)
		if err != nil {
			logger.Errorf("Failed to encrypt message content: %v", err)
			// Continue without encryption for now
			message.IsEncrypted = false
		} else {
			message.Content = encryptedContent
		}
	}

	// Handle reply message
	if req.ReplyToID != nil {
		replyToMessage, err := ms.getMessageFromDatabase(*req.ReplyToID)
		if err == nil {
			message.QuotedMessage = replyToMessage
		}
	}

	// Handle scheduled messages
	if req.ScheduledAt != nil && req.ScheduledAt.After(time.Now()) {
		return ms.scheduleMessage(message)
	}

	// Insert message into database
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := ms.messagesCollection.InsertOne(ctx, message)
	if err != nil {
		return nil, fmt.Errorf("failed to insert message: %w", err)
	}

	message.ID = result.InsertedID.(primitive.ObjectID)

	// Update chat with last message
	go ms.updateChatLastMessage(chat, message)

	// Process message asynchronously
	ms.queueMessageProcessing(message, "new_message", 1)

	// Build response
	response, err := ms.buildMessageResponse(message, senderID)
	if err != nil {
		return nil, fmt.Errorf("failed to build response: %w", err)
	}

	logger.LogUserAction(senderID.Hex(), "message_sent", "message_service", map[string]interface{}{
		"message_id": message.ID.Hex(),
		"chat_id":    req.ChatID.Hex(),
		"type":       req.Type,
	})

	return response, nil
}

// GetMessages retrieves messages for a chat
func (ms *MessageService) GetMessages(userID primitive.ObjectID, req *MessageListRequest) (*models.MessagesResponse, error) {
	// Validate request
	if err := ms.validateMessageListRequest(req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Get chat and verify permissions
	chat, err := ms.getChatFromDatabase(req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("chat not found: %w", err)
	}

	if !chat.IsParticipant(userID) {
		return nil, fmt.Errorf("user is not a participant of this chat")
	}

	// Build query filter
	filter := bson.M{
		"chat_id":    req.ChatID,
		"is_deleted": false,
	}

	// Apply additional filters
	if req.Type != "" {
		filter["type"] = req.Type
	}

	if req.SenderID != nil {
		filter["sender_id"] = *req.SenderID
	}

	if req.Search != "" {
		filter["content"] = bson.M{"$regex": req.Search, "$options": "i"}
	}

	if req.DateFrom != nil || req.DateTo != nil {
		dateFilter := bson.M{}
		if req.DateFrom != nil {
			dateFilter["$gte"] = *req.DateFrom
		}
		if req.DateTo != nil {
			dateFilter["$lte"] = *req.DateTo
		}
		filter["created_at"] = dateFilter
	}

	// Handle cursor-based pagination
	if req.Before != nil {
		beforeMessage, err := ms.getMessageFromDatabase(*req.Before)
		if err == nil {
			filter["created_at"] = bson.M{"$lt": beforeMessage.CreatedAt}
		}
	}

	if req.After != nil {
		afterMessage, err := ms.getMessageFromDatabase(*req.After)
		if err == nil {
			filter["created_at"] = bson.M{"$gt": afterMessage.CreatedAt}
		}
	}

	// Set pagination defaults
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 50
	}

	// Calculate skip for page-based pagination
	skip := 0
	if req.Before == nil && req.After == nil {
		skip = (req.Page - 1) * req.Limit
	}

	// Execute query
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Count total messages
	total, err := ms.messagesCollection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to count messages: %w", err)
	}

	// Find messages
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(int64(req.Limit))

	if skip > 0 {
		opts.SetSkip(int64(skip))
	}

	cursor, err := ms.messagesCollection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find messages: %w", err)
	}
	defer cursor.Close(ctx)

	var messages []models.Message
	if err := cursor.All(ctx, &messages); err != nil {
		return nil, fmt.Errorf("failed to decode messages: %w", err)
	}

	// Decrypt messages if needed
	for i := range messages {
		if err := ms.decryptMessage(&messages[i], chat.EncryptionKey); err != nil {
			logger.Errorf("Failed to decrypt message %s: %v", messages[i].ID.Hex(), err)
		}

		// Remove from deleted for user list if present
		if messages[i].IsDeletedForUser(userID) {
			messages[i].Content = "This message was deleted"
			messages[i].MediaURL = ""
			messages[i].Type = models.MessageTypeDeleted
		}
	}

	// Build message responses
	messageResponses := make([]models.MessageResponse, len(messages))
	for i, message := range messages {
		response, err := ms.buildMessageResponse(&message, userID)
		if err != nil {
			logger.Errorf("Failed to build message response for message %s: %v", message.ID.Hex(), err)
			continue
		}
		messageResponses[i] = *response
	}

	// Reverse order for chronological display
	for i, j := 0, len(messageResponses)-1; i < j; i, j = i+1, j-1 {
		messageResponses[i], messageResponses[j] = messageResponses[j], messageResponses[i]
	}

	// Determine last message ID for pagination
	var lastMessageID *primitive.ObjectID
	if len(messages) > 0 {
		lastMessageID = &messages[0].ID
	}

	return &models.MessagesResponse{
		Messages:      messageResponses,
		HasMore:       int64(skip+req.Limit) < total,
		TotalCount:    total,
		LastMessageID: lastMessageID,
	}, nil
}

// GetMessage retrieves a specific message
func (ms *MessageService) GetMessage(messageID primitive.ObjectID, userID primitive.ObjectID) (*models.MessageResponse, error) {
	message, err := ms.getMessageFromDatabase(messageID)
	if err != nil {
		return nil, fmt.Errorf("message not found: %w", err)
	}

	// Verify user has access to this message
	chat, err := ms.getChatFromDatabase(message.ChatID)
	if err != nil {
		return nil, fmt.Errorf("chat not found: %w", err)
	}

	if !chat.IsParticipant(userID) {
		return nil, fmt.Errorf("user is not a participant of this chat")
	}

	// Decrypt message if needed
	if err := ms.decryptMessage(message, chat.EncryptionKey); err != nil {
		logger.Errorf("Failed to decrypt message: %v", err)
	}

	// Check if deleted for user
	if message.IsDeletedForUser(userID) {
		message.Content = "This message was deleted"
		message.MediaURL = ""
		message.Type = models.MessageTypeDeleted
	}

	return ms.buildMessageResponse(message, userID)
}

// UpdateMessage updates a message
func (ms *MessageService) UpdateMessage(messageID primitive.ObjectID, userID primitive.ObjectID, req *MessageUpdateRequest) (*models.MessageResponse, error) {
	message, err := ms.getMessageFromDatabase(messageID)
	if err != nil {
		return nil, fmt.Errorf("message not found: %w", err)
	}

	// Check if user can edit this message
	if !message.CanBeEditedBy(userID) {
		return nil, fmt.Errorf("message cannot be edited")
	}

	// Validate content
	if err := utils.ValidateMessageContent(req.Content, 4096); err != nil {
		return nil, fmt.Errorf("invalid content: %w", err)
	}

	// Get chat for encryption
	chat, err := ms.getChatFromDatabase(message.ChatID)
	if err != nil {
		return nil, fmt.Errorf("chat not found: %w", err)
	}

	// Edit message
	oldContent := message.Content
	message.Edit(req.Content, req.Reason)

	// Encrypt new content
	if message.IsEncrypted {
		encryptedContent, err := ms.encryptMessageContent(req.Content, chat.EncryptionKey)
		if err != nil {
			logger.Errorf("Failed to encrypt message content: %v", err)
		} else {
			message.Content = encryptedContent
		}
	}

	// Update in database
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"content":      message.Content,
			"is_edited":    message.IsEdited,
			"edit_history": message.EditHistory,
			"updated_at":   message.UpdatedAt,
		},
	}

	_, err = ms.messagesCollection.UpdateOne(ctx, bson.M{"_id": messageID}, update)
	if err != nil {
		return nil, fmt.Errorf("failed to update message: %w", err)
	}

	// Notify participants about message edit
	go ms.notifyMessageUpdate(message, userID, "message_edited")

	logger.LogUserAction(userID.Hex(), "message_edited", "message_service", map[string]interface{}{
		"message_id":  messageID.Hex(),
		"chat_id":     message.ChatID.Hex(),
		"old_content": oldContent,
		"new_content": req.Content,
	})

	// Decrypt for response
	if err := ms.decryptMessage(message, chat.EncryptionKey); err != nil {
		logger.Errorf("Failed to decrypt message for response: %v", err)
	}

	return ms.buildMessageResponse(message, userID)
}

// DeleteMessage deletes a message
func (ms *MessageService) DeleteMessage(messageID primitive.ObjectID, userID primitive.ObjectID, deleteForEveryone bool) error {
	message, err := ms.getMessageFromDatabase(messageID)
	if err != nil {
		return fmt.Errorf("message not found: %w", err)
	}

	// Check permissions
	if !message.CanBeDeletedBy(userID) {
		return fmt.Errorf("message cannot be deleted")
	}

	if deleteForEveryone && !message.CanBeDeletedForEveryone(userID) {
		return fmt.Errorf("message cannot be deleted for everyone")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var update bson.M
	var notificationType string

	if deleteForEveryone {
		// Delete for everyone
		message.DeleteForEveryone()
		update = bson.M{
			"$set": bson.M{
				"is_deleted": true,
				"deleted_at": message.DeletedAt,
				"type":       models.MessageTypeDeleted,
				"content":    "This message was deleted",
				"media_url":  "",
				"updated_at": time.Now(),
			},
		}
		notificationType = "message_deleted_for_everyone"
	} else {
		// Delete for user only
		message.DeleteForUser(userID)
		update = bson.M{
			"$addToSet": bson.M{"deleted_for": userID},
			"$set":      bson.M{"updated_at": time.Now()},
		}
		notificationType = "message_deleted_for_user"
	}

	_, err = ms.messagesCollection.UpdateOne(ctx, bson.M{"_id": messageID}, update)
	if err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}

	// Notify participants
	go ms.notifyMessageUpdate(message, userID, notificationType)

	// Update statistics
	ms.updateMessageStats("deleted", message.Type)

	logger.LogUserAction(userID.Hex(), "message_deleted", "message_service", map[string]interface{}{
		"message_id":          messageID.Hex(),
		"chat_id":             message.ChatID.Hex(),
		"delete_for_everyone": deleteForEveryone,
	})

	return nil
}

// AddReaction adds a reaction to a message
func (ms *MessageService) AddReaction(messageID primitive.ObjectID, userID primitive.ObjectID, req *MessageReactionRequest) error {
	message, err := ms.getMessageFromDatabase(messageID)
	if err != nil {
		return fmt.Errorf("message not found: %w", err)
	}

	// Verify user has access
	chat, err := ms.getChatFromDatabase(message.ChatID)
	if err != nil {
		return fmt.Errorf("chat not found: %w", err)
	}

	if !chat.IsParticipant(userID) {
		return fmt.Errorf("user is not a participant of this chat")
	}

	// Add reaction
	message.AddReaction(userID, req.Emoji)

	// Update in database
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"reactions":  message.Reactions,
			"updated_at": time.Now(),
		},
	}

	_, err = ms.messagesCollection.UpdateOne(ctx, bson.M{"_id": messageID}, update)
	if err != nil {
		return fmt.Errorf("failed to add reaction: %w", err)
	}

	// Notify participants
	go ms.notifyReaction(message, userID, req.Emoji, "reaction_added")

	logger.LogUserAction(userID.Hex(), "reaction_added", "message_service", map[string]interface{}{
		"message_id": messageID.Hex(),
		"emoji":      req.Emoji,
	})

	return nil
}

// RemoveReaction removes a reaction from a message
func (ms *MessageService) RemoveReaction(messageID primitive.ObjectID, userID primitive.ObjectID) error {
	message, err := ms.getMessageFromDatabase(messageID)
	if err != nil {
		return fmt.Errorf("message not found: %w", err)
	}

	// Verify user has access
	chat, err := ms.getChatFromDatabase(message.ChatID)
	if err != nil {
		return fmt.Errorf("chat not found: %w", err)
	}

	if !chat.IsParticipant(userID) {
		return fmt.Errorf("user is not a participant of this chat")
	}

	// Remove reaction
	message.RemoveReaction(userID)

	// Update in database
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"reactions":  message.Reactions,
			"updated_at": time.Now(),
		},
	}

	_, err = ms.messagesCollection.UpdateOne(ctx, bson.M{"_id": messageID}, update)
	if err != nil {
		return fmt.Errorf("failed to remove reaction: %w", err)
	}

	// Notify participants
	go ms.notifyReaction(message, userID, "", "reaction_removed")

	logger.LogUserAction(userID.Hex(), "reaction_removed", "message_service", map[string]interface{}{
		"message_id": messageID.Hex(),
	})

	return nil
}

// MarkAsRead marks message as read by user
func (ms *MessageService) MarkAsRead(messageID primitive.ObjectID, userID primitive.ObjectID) error {
	message, err := ms.getMessageFromDatabase(messageID)
	if err != nil {
		return fmt.Errorf("message not found: %w", err)
	}

	// Verify user has access
	chat, err := ms.getChatFromDatabase(message.ChatID)
	if err != nil {
		return fmt.Errorf("chat not found: %w", err)
	}

	if !chat.IsParticipant(userID) {
		return fmt.Errorf("user is not a participant of this chat")
	}

	// Mark as read
	message.MarkAsRead(userID)

	// Update in database
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"read_by":    message.ReadBy,
			"status":     message.Status,
			"updated_at": time.Now(),
		},
	}

	_, err = ms.messagesCollection.UpdateOne(ctx, bson.M{"_id": messageID}, update)
	if err != nil {
		return fmt.Errorf("failed to mark as read: %w", err)
	}

	// Send read receipt
	go ms.sendReadReceipt(message, userID)

	return nil
}

// ForwardMessage forwards a message to another chat
func (ms *MessageService) ForwardMessage(messageID primitive.ObjectID, userID primitive.ObjectID, toChatID primitive.ObjectID) (*models.MessageResponse, error) {
	// Get original message
	originalMessage, err := ms.getMessageFromDatabase(messageID)
	if err != nil {
		return nil, fmt.Errorf("message not found: %w", err)
	}

	// Verify user has access to original message
	originalChat, err := ms.getChatFromDatabase(originalMessage.ChatID)
	if err != nil {
		return nil, fmt.Errorf("original chat not found: %w", err)
	}

	if !originalChat.IsParticipant(userID) {
		return nil, fmt.Errorf("user is not a participant of original chat")
	}

	// Verify user can send to target chat
	targetChat, err := ms.getChatFromDatabase(toChatID)
	if err != nil {
		return nil, fmt.Errorf("target chat not found: %w", err)
	}

	if !targetChat.IsParticipant(userID) {
		return nil, fmt.Errorf("user is not a participant of target chat")
	}

	if !targetChat.CanUserMessage(userID) {
		return nil, fmt.Errorf("user cannot send messages in target chat")
	}

	// Decrypt original message content
	if err := ms.decryptMessage(originalMessage, originalChat.EncryptionKey); err != nil {
		logger.Errorf("Failed to decrypt message for forwarding: %v", err)
	}

	// Create forwarded message
	forwardedMessage := &models.Message{
		ChatID:   toChatID,
		SenderID: userID,
		Type:     originalMessage.Type,
		Content:  originalMessage.Content,
		MediaURL: originalMessage.MediaURL,
		Metadata: originalMessage.Metadata,
		ForwardedFrom: &models.ForwardInfo{
			OriginalSenderID:  originalMessage.SenderID,
			OriginalChatID:    originalMessage.ChatID,
			OriginalMessageID: originalMessage.ID,
			ForwardedAt:       time.Now(),
			ForwardCount:      1,
		},
		IsEncrypted: true,
	}

	// Handle nested forwards
	if originalMessage.ForwardedFrom != nil {
		forwardedMessage.ForwardedFrom.OriginalSenderID = originalMessage.ForwardedFrom.OriginalSenderID
		forwardedMessage.ForwardedFrom.OriginalChatID = originalMessage.ForwardedFrom.OriginalChatID
		forwardedMessage.ForwardedFrom.OriginalMessageID = originalMessage.ForwardedFrom.OriginalMessageID
		forwardedMessage.ForwardedFrom.ForwardCount = originalMessage.ForwardedFrom.ForwardCount + 1
	}

	forwardedMessage.BeforeCreate()

	// Encrypt content for target chat
	if forwardedMessage.Content != "" && forwardedMessage.IsEncrypted {
		encryptedContent, err := ms.encryptMessageContent(forwardedMessage.Content, targetChat.EncryptionKey)
		if err != nil {
			logger.Errorf("Failed to encrypt forwarded message content: %v", err)
			forwardedMessage.IsEncrypted = false
		} else {
			forwardedMessage.Content = encryptedContent
		}
	}

	// Insert into database
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := ms.messagesCollection.InsertOne(ctx, forwardedMessage)
	if err != nil {
		return nil, fmt.Errorf("failed to insert forwarded message: %w", err)
	}

	forwardedMessage.ID = result.InsertedID.(primitive.ObjectID)

	// Update target chat
	go ms.updateChatLastMessage(targetChat, forwardedMessage)

	// Process message
	ms.queueMessageProcessing(forwardedMessage, "forwarded_message", 1)

	// Update statistics
	ms.updateMessageStats("forwarded", forwardedMessage.Type)

	logger.LogUserAction(userID.Hex(), "message_forwarded", "message_service", map[string]interface{}{
		"original_message_id": messageID.Hex(),
		"new_message_id":      forwardedMessage.ID.Hex(),
		"from_chat_id":        originalMessage.ChatID.Hex(),
		"to_chat_id":          toChatID.Hex(),
	})

	// Decrypt for response
	if err := ms.decryptMessage(forwardedMessage, targetChat.EncryptionKey); err != nil {
		logger.Errorf("Failed to decrypt message for response: %v", err)
	}

	return ms.buildMessageResponse(forwardedMessage, userID)
}

// GetMessageStatistics returns message service statistics
func (ms *MessageService) GetMessageStatistics() *MessageStatistics {
	ms.messageStats.mutex.RLock()
	defer ms.messageStats.mutex.RUnlock()

	stats := *ms.messageStats
	return &stats
}

// validateSendMessageRequest validates send message request
func (ms *MessageService) validateSendMessageRequest(req *SendMessageRequest) error {
	if req.Type == "" {
		return fmt.Errorf("message type is required")
	}

	// Validate content based on type
	switch req.Type {
	case models.MessageTypeText:
		if req.Content == "" {
			return fmt.Errorf("text message content is required")
		}
		if err := utils.ValidateMessageContent(req.Content, 4096); err != nil {
			return err
		}
	case models.MessageTypeImage, models.MessageTypeVideo, models.MessageTypeAudio, models.MessageTypeDocument:
		if req.MediaURL == "" {
			return fmt.Errorf("media URL is required for media messages")
		}
		if err := utils.ValidateURL(req.MediaURL); err != nil {
			return fmt.Errorf("invalid media URL: %w", err)
		}
	}

	return nil
}

// validateMessageListRequest validates message list request
func (ms *MessageService) validateMessageListRequest(req *MessageListRequest) error {
	if req.ChatID.IsZero() {
		return fmt.Errorf("chat ID is required")
	}

	if req.Limit > 100 {
		return fmt.Errorf("limit cannot exceed 100")
	}

	return nil
}

// validateMentions validates mentioned users
func (ms *MessageService) validateMentions(mentions []primitive.ObjectID, chat *models.Chat) error {
	if len(mentions) == 0 {
		return nil
	}

	// Check if mentioned users are participants
	for _, mentionID := range mentions {
		if !chat.IsParticipant(mentionID) {
			return fmt.Errorf("mentioned user is not a participant")
		}
	}

	return nil
}

// Database helper methods

// getMessageFromDatabase retrieves message from database
func (ms *MessageService) getMessageFromDatabase(messageID primitive.ObjectID) (*models.Message, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var message models.Message
	err := ms.messagesCollection.FindOne(ctx, bson.M{
		"_id":        messageID,
		"is_deleted": false,
	}).Decode(&message)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("message not found")
		}
		return nil, err
	}

	return &message, nil
}

// getChatFromDatabase retrieves chat from database
func (ms *MessageService) getChatFromDatabase(chatID primitive.ObjectID) (*models.Chat, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var chat models.Chat
	err := ms.chatsCollection.FindOne(ctx, bson.M{
		"_id":       chatID,
		"is_active": true,
	}).Decode(&chat)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("chat not found")
		}
		return nil, err
	}

	return &chat, nil
}

// getUserFromDatabase retrieves user from database
func (ms *MessageService) getUserFromDatabase(userID primitive.ObjectID) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var user models.User
	err := ms.usersCollection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	return &user, nil
}

// buildMessageResponse builds message response with user-specific data
func (ms *MessageService) buildMessageResponse(message *models.Message, userID primitive.ObjectID) (*models.MessageResponse, error) {
	// Get sender info
	sender, err := ms.getUserFromDatabase(message.SenderID)
	if err != nil {
		logger.Errorf("Failed to get sender info: %v", err)
		sender = &models.User{
			ID:   message.SenderID,
			Name: "Unknown User",
		}
	}

	// Get reply message if exists
	var replyTo *models.Message
	if message.ReplyToID != nil {
		replyToMsg, err := ms.getMessageFromDatabase(*message.ReplyToID)
		if err == nil {
			// Decrypt reply message
			chat, err := ms.getChatFromDatabase(message.ChatID)
			if err == nil {
				ms.decryptMessage(replyToMsg, chat.EncryptionKey)
			}
			replyTo = replyToMsg
		}
	}

	response := &models.MessageResponse{
		Message:        *message,
		Sender:         sender.GetPublicInfo(userID),
		ReplyTo:        replyTo,
		CanEdit:        message.CanBeEditedBy(userID),
		CanDelete:      message.CanBeDeletedBy(userID),
		IsDeletedForMe: message.IsDeletedForUser(userID),
	}

	return response, nil
}

// Encryption methods

// encryptMessageContent encrypts message content
func (ms *MessageService) encryptMessageContent(content, chatKey string) (string, error) {
	if ms.encryptionSvc == nil {
		return content, fmt.Errorf("encryption service not available")
	}

	encrypted, err := ms.encryptionSvc.EncryptMessage(content)
	if err != nil {
		return content, err
	}

	encryptedJSON, err := json.Marshal(encrypted)
	if err != nil {
		return content, err
	}

	return string(encryptedJSON), nil
}

// decryptMessage decrypts message content
func (ms *MessageService) decryptMessage(message *models.Message, chatKey string) error {
	if !message.IsEncrypted || message.Content == "" || ms.encryptionSvc == nil {
		return nil
	}

	var encrypted utils.EncryptedMessage
	if err := json.Unmarshal([]byte(message.Content), &encrypted); err != nil {
		// Content might not be encrypted JSON, skip decryption
		return nil
	}

	decrypted, err := ms.encryptionSvc.DecryptMessage(&encrypted)
	if err != nil {
		return err
	}

	message.Content = decrypted
	return nil
}

// Notification methods

// notifyMessageUpdate notifies participants about message updates
func (ms *MessageService) notifyMessageUpdate(message *models.Message, userID primitive.ObjectID, eventType string) {
	if ms.hub == nil {
		return
	}

	// Get chat to find participants
	chat, err := ms.getChatFromDatabase(message.ChatID)
	if err != nil {
		logger.Errorf("Failed to get chat for notification: %v", err)
		return
	}

	// Notify all participants except the user who made the change
	for _, participantID := range chat.Participants {
		if participantID == userID {
			continue
		}

		data := map[string]interface{}{
			"message_id": message.ID.Hex(),
			"chat_id":    message.ChatID.Hex(),
			"user_id":    userID.Hex(),
			"type":       eventType,
		}

		ms.hub.SendToUser(participantID, eventType, data)
	}
}

// notifyReaction notifies participants about reactions
func (ms *MessageService) notifyReaction(message *models.Message, userID primitive.ObjectID, emoji, eventType string) {
	if ms.hub == nil {
		return
	}

	// Get chat to find participants
	chat, err := ms.getChatFromDatabase(message.ChatID)
	if err != nil {
		logger.Errorf("Failed to get chat for reaction notification: %v", err)
		return
	}

	// Notify all participants
	for _, participantID := range chat.Participants {
		data := map[string]interface{}{
			"message_id": message.ID.Hex(),
			"chat_id":    message.ChatID.Hex(),
			"user_id":    userID.Hex(),
			"emoji":      emoji,
		}

		ms.hub.SendToUser(participantID, eventType, data)
	}
}

// sendReadReceipt sends read receipt to message sender
func (ms *MessageService) sendReadReceipt(message *models.Message, readerID primitive.ObjectID) {
	if ms.hub == nil || message.SenderID == readerID {
		return
	}

	data := map[string]interface{}{
		"message_id": message.ID.Hex(),
		"chat_id":    message.ChatID.Hex(),
		"reader_id":  readerID.Hex(),
		"read_at":    time.Now(),
	}

	ms.hub.SendToUser(message.SenderID, "message_read", data)
}

// updateChatLastMessage updates chat's last message info
func (ms *MessageService) updateChatLastMessage(chat *models.Chat, message *models.Message) {
	chat.UpdateLastMessage(message.ID, message.SenderID, message.GetPreviewText(), message.Type)
	chat.IncrementUnreadCount(message.SenderID, len(message.Mentions) > 0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"last_message":  chat.LastMessage,
			"last_activity": chat.LastActivity,
			"message_count": chat.MessageCount,
			"unread_counts": chat.UnreadCounts,
			"updated_at":    time.Now(),
		},
	}

	_, err := ms.chatsCollection.UpdateOne(ctx, bson.M{"_id": chat.ID}, update)
	if err != nil {
		logger.Errorf("Failed to update chat last message: %v", err)
	}

	// Notify participants about new message
	if ms.hub != nil {
		for _, participantID := range chat.Participants {
			if participantID == message.SenderID {
				continue
			}

			// Decrypt message content for notification
			content := message.Content
			if message.IsEncrypted {
				if err := ms.decryptMessage(message, chat.EncryptionKey); err == nil {
					content = message.Content
				}
			}

			data := map[string]interface{}{
				"message_id":   message.ID.Hex(),
				"chat_id":      message.ChatID.Hex(),
				"sender_id":    message.SenderID.Hex(),
				"type":         message.Type,
				"content":      content,
				"preview_text": message.GetPreviewText(),
				"timestamp":    message.CreatedAt,
			}

			ms.hub.SendToUser(participantID, "new_message", data)
		}
	}
}

// scheduleMessage schedules a message for future delivery
func (ms *MessageService) scheduleMessage(message *models.Message) (*models.MessageResponse, error) {
	// Insert scheduled message
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := ms.messagesCollection.InsertOne(ctx, message)
	if err != nil {
		return nil, fmt.Errorf("failed to schedule message: %w", err)
	}

	message.ID = result.InsertedID.(primitive.ObjectID)

	// Store in Redis for scheduled delivery
	if ms.redisClient != nil {
		scheduleKey := fmt.Sprintf("scheduled_message:%s", message.ID.Hex())
		messageData, _ := json.Marshal(message)
		ms.redisClient.SetEX(scheduleKey, messageData, time.Until(*message.ScheduledAt))
	}

	logger.LogUserAction(message.SenderID.Hex(), "message_scheduled", "message_service", map[string]interface{}{
		"message_id":   message.ID.Hex(),
		"chat_id":      message.ChatID.Hex(),
		"scheduled_at": message.ScheduledAt,
	})

	return ms.buildMessageResponse(message, message.SenderID)
}

// Message processing

// queueMessageProcessing queues a message for processing
func (ms *MessageService) queueMessageProcessing(message *models.Message, taskType string, priority int) {
	task := &MessageProcessingTask{
		Message:   message,
		ChatID:    message.ChatID,
		SenderID:  message.SenderID,
		Type:      taskType,
		Priority:  priority,
		CreatedAt: time.Now(),
	}

	select {
	case ms.messageQueue <- task:
	default:
		logger.Warnf("Message processing queue is full, dropping task for message %s", message.ID.Hex())
	}
}

// messageProcessor processes the message queue
func (ms *MessageService) messageProcessor() {
	defer ms.wg.Done()

	for {
		select {
		case <-ms.ctx.Done():
			return
		case task := <-ms.messageQueue:
			// Process high priority tasks immediately
			if task.Priority > 5 {
				ms.processMessageTask(task)
			} else {
				// Queue for worker processing
				select {
				case ms.messageQueue <- task:
				default:
					logger.Warnf("Failed to requeue message task")
				}
			}
		}
	}
}

// messageWorker processes messages from the queue
func (ms *MessageService) messageWorker(workerID int) {
	defer ms.wg.Done()

	logger.Infof("Message worker %d started", workerID)

	for {
		select {
		case <-ms.ctx.Done():
			logger.Infof("Message worker %d stopping", workerID)
			return
		case task := <-ms.messageQueue:
			ms.processMessageTask(task)
		}
	}
}

// processMessageTask processes a single message task
func (ms *MessageService) processMessageTask(task *MessageProcessingTask) {
	switch task.Type {
	case "new_message":
		ms.processNewMessage(task.Message)
	case "forwarded_message":
		ms.processForwardedMessage(task.Message)
	case "scheduled_message":
		ms.processScheduledMessage(task.Message)
	default:
		logger.Warnf("Unknown message task type: %s", task.Type)
	}
}

// processNewMessage processes a new message
func (ms *MessageService) processNewMessage(message *models.Message) {
	// Send push notifications
	if ms.pushService != nil {
		go ms.sendPushNotifications(message)
	}

	// Update message statistics
	ms.updateMessageStats("sent", message.Type)

	// Process mentions
	if len(message.Mentions) > 0 {
		ms.processMentions(message)
	}

	// Update user last activity
	ms.updateUserActivity(message.SenderID)
}

// processForwardedMessage processes a forwarded message
func (ms *MessageService) processForwardedMessage(message *models.Message) {
	// Similar to new message but with forwarding-specific logic
	ms.processNewMessage(message)

	// Additional forwarding metrics
	ms.updateMessageStats("forwarded", message.Type)
}

// processScheduledMessage processes a scheduled message
func (ms *MessageService) processScheduledMessage(message *models.Message) {
	// Check if it's time to send
	if message.ScheduledAt != nil && time.Now().Before(*message.ScheduledAt) {
		// Not yet time, reschedule
		return
	}

	// Send the message
	message.ScheduledAt = nil
	ms.processNewMessage(message)

	// Update in database
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$unset": bson.M{"scheduled_at": ""},
		"$set":   bson.M{"updated_at": time.Now()},
	}

	ms.messagesCollection.UpdateOne(ctx, bson.M{"_id": message.ID}, update)
}

// processMentions processes mentioned users
func (ms *MessageService) processMentions(message *models.Message) {
	if ms.hub == nil {
		return
	}

	for _, mentionedUserID := range message.Mentions {
		// Send mention notification
		data := map[string]interface{}{
			"message_id":   message.ID.Hex(),
			"chat_id":      message.ChatID.Hex(),
			"sender_id":    message.SenderID.Hex(),
			"content":      message.GetPreviewText(),
			"mentioned_by": message.SenderID.Hex(),
		}

		ms.hub.SendToUser(mentionedUserID, "mention", data)

		// Send push notification for mention
		if ms.pushService != nil {
			go func(userID primitive.ObjectID) {
				user, err := ms.getUserFromDatabase(userID)
				if err != nil {
					return
				}

				sender, err := ms.getUserFromDatabase(message.SenderID)
				if err != nil {
					return
				}

				title := "You were mentioned"
				fmt.Sprintf("%s mentioned you in a message", sender.Name)

				// TODO: Implement based on your PushService interface
				// ms.pushService.SendNotification(user, title, body, data)
				logger.Infof("Push notification for mention: %s to %s", title, user.Name)
			}(mentionedUserID)
		}
	}
}

// sendPushNotifications sends push notifications for new messages
// sendPushNotifications sends push notifications for new messages
func (ms *MessageService) sendPushNotifications(message *models.Message) {
	if ms.pushService == nil {
		return
	}

	// Get chat to find participants
	chat, err := ms.getChatFromDatabase(message.ChatID)
	if err != nil {
		logger.Errorf("Failed to get chat for push notifications: %v", err)
		return
	}

	// Get sender info
	sender, err := ms.getUserFromDatabase(message.SenderID)
	if err != nil {
		logger.Errorf("Failed to get sender for push notification: %v", err)
		return
	}

	// Send to all participants except sender
	for _, participantID := range chat.Participants {
		if participantID == message.SenderID {
			continue
		}

		// Check if user is online (might skip push notification)
		if ms.hub != nil && ms.hub.IsUserOnline(participantID) {
			// User is online, they'll get real-time notification
			continue
		}

		// Get participant info
		_, err := ms.getUserFromDatabase(participantID)
		if err != nil {
			logger.Errorf("Failed to get participant for push notification: %v", err)
			continue
		}

		// Check if chat is muted for this user
		if chat.IsMutedFor(participantID) {
			continue
		}

		// Prepare notification
		title := sender.Name
		if chat.Type == models.ChatTypeGroup {
			title = fmt.Sprintf("%s • %s", chat.Name, sender.Name)
		}

		body := message.GetPreviewText()

		// Create notification data (must be map[string]string for your PushService)
		notificationData := map[string]string{
			"type":       "new_message",
			"message_id": message.ID.Hex(),
			"chat_id":    message.ChatID.Hex(),
			"sender_id":  message.SenderID.Hex(),
			"chat_type":  string(chat.Type),
		}

		// Add additional data based on message type
		if message.Type != models.MessageTypeText {
			notificationData["message_type"] = string(message.Type)
		}

		// Send push notification
		if err := ms.pushService.SendNotification(participantID, title, body, notificationData); err != nil {
			logger.Errorf("Failed to send push notification to user %s: %v", participantID.Hex(), err)
		} else {
			logger.Debugf("Push notification sent to user %s: %s - %s",
				participantID.Hex(), title, body)
		}
	}
}

// updateUserActivity updates user's last activity
func (ms *MessageService) updateUserActivity(userID primitive.ObjectID) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"last_seen":  time.Now(),
			"is_online":  true,
			"updated_at": time.Now(),
		},
	}

	ms.usersCollection.UpdateOne(ctx, bson.M{"_id": userID}, update)
}

// Background processes

// statsCollector collects message statistics
func (ms *MessageService) statsCollector() {
	defer ms.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ms.collectStatistics()
		case <-ms.ctx.Done():
			return
		}
	}
}

// collectStatistics collects and updates statistics
func (ms *MessageService) collectStatistics() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ms.messageStats.mutex.Lock()
	defer ms.messageStats.mutex.Unlock()

	// Count total messages
	total, _ := ms.messagesCollection.CountDocuments(ctx, bson.M{"is_deleted": false})
	ms.messageStats.TotalMessages = total

	// Count messages by type
	textMessages, _ := ms.messagesCollection.CountDocuments(ctx, bson.M{
		"type":       models.MessageTypeText,
		"is_deleted": false,
	})
	ms.messageStats.TextMessages = textMessages

	mediaMessages, _ := ms.messagesCollection.CountDocuments(ctx, bson.M{
		"type": bson.M{"$in": []models.MessageType{
			models.MessageTypeImage,
			models.MessageTypeVideo,
			models.MessageTypeAudio,
		}},
		"is_deleted": false,
	})
	ms.messageStats.MediaMessages = mediaMessages

	voiceMessages, _ := ms.messagesCollection.CountDocuments(ctx, bson.M{
		"type":       models.MessageTypeVoiceNote,
		"is_deleted": false,
	})
	ms.messageStats.VoiceMessages = voiceMessages

	fileMessages, _ := ms.messagesCollection.CountDocuments(ctx, bson.M{
		"type":       models.MessageTypeDocument,
		"is_deleted": false,
	})
	ms.messageStats.FileMessages = fileMessages

	// Count deleted and edited messages
	deletedMessages, _ := ms.messagesCollection.CountDocuments(ctx, bson.M{"is_deleted": true})
	ms.messageStats.DeletedMessages = deletedMessages

	editedMessages, _ := ms.messagesCollection.CountDocuments(ctx, bson.M{"is_edited": true})
	ms.messageStats.EditedMessages = editedMessages

	// Count messages today
	today := time.Now().Truncate(24 * time.Hour)
	messagesToday, _ := ms.messagesCollection.CountDocuments(ctx, bson.M{
		"created_at": bson.M{"$gte": today},
		"is_deleted": false,
	})
	ms.messageStats.MessagesToday = messagesToday

	// Count messages this hour
	thisHour := time.Now().Truncate(time.Hour)
	messagesThisHour, _ := ms.messagesCollection.CountDocuments(ctx, bson.M{
		"created_at": bson.M{"$gte": thisHour},
		"is_deleted": false,
	})
	ms.messageStats.MessagesThisHour = messagesThisHour

	ms.messageStats.LastUpdated = time.Now()
}

// updateMessageStats updates message statistics
func (ms *MessageService) updateMessageStats(eventType string, messageType models.MessageType) {
	ms.messageStats.mutex.Lock()
	defer ms.messageStats.mutex.Unlock()

	switch eventType {
	case "sent":
		ms.messageStats.TotalMessages++
		ms.messageStats.MessagesToday++
		ms.messageStats.MessagesThisHour++

		switch messageType {
		case models.MessageTypeText:
			ms.messageStats.TextMessages++
		case models.MessageTypeImage, models.MessageTypeVideo, models.MessageTypeAudio:
			ms.messageStats.MediaMessages++
		case models.MessageTypeVoiceNote:
			ms.messageStats.VoiceMessages++
		case models.MessageTypeDocument:
			ms.messageStats.FileMessages++
		}

	case "deleted":
		ms.messageStats.DeletedMessages++

	case "edited":
		ms.messageStats.EditedMessages++

	case "forwarded":
		ms.messageStats.ForwardedMessages++
	}

	ms.messageStats.LastUpdated = time.Now()
}

// Close gracefully shuts down the message service
func (ms *MessageService) Close() error {
	logger.Info("Shutting down Message Service...")

	// Cancel context and wait for goroutines
	ms.cancel()
	ms.wg.Wait()

	// Close message queue
	close(ms.messageQueue)

	logger.Info("Message Service shutdown complete")
	return nil
}
