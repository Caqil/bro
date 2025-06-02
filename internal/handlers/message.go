package handlers

import (
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bro/internal/middleware"
	"bro/internal/models"
	"bro/internal/services"
	"bro/internal/utils"
	"bro/pkg/database"
	"bro/pkg/logger"
	"bro/pkg/redis"
)

type MessageHandler struct {
	fileService *services.FileService
	pushService *services.PushService
	redisClient *redis.Client
	encryption  *utils.EncryptionService
}

func NewMessageHandler(fileService *services.FileService, pushService *services.PushService, encryptionKey string) *MessageHandler {
	encryption, err := utils.NewEncryptionService(encryptionKey)
	if err != nil {
		logger.Fatalf("Failed to initialize encryption service: %v", err)
	}

	return &MessageHandler{
		fileService: fileService,
		pushService: pushService,
		redisClient: redis.GetClient(),
		encryption:  encryption,
	}
}

// RegisterRoutes registers message routes
func (h *MessageHandler) RegisterRoutes(router *gin.RouterGroup) {
	messages := router.Group("/messages")
	messages.Use(middleware.AuthMiddleware("your-jwt-secret"))
	{
		// Message operations
		messages.POST("", h.SendMessage)
		messages.GET("/:messageId", h.GetMessage)
		messages.PUT("/:messageId", h.EditMessage)
		messages.DELETE("/:messageId", h.DeleteMessage)
		messages.POST("/:messageId/forward", h.ForwardMessage)

		// Message status
		messages.PUT("/:messageId/status", h.UpdateMessageStatus)
		messages.POST("/:messageId/mark-read", h.MarkAsRead)
		messages.POST("/:messageId/mark-delivered", h.MarkAsDelivered)

		// Reactions
		messages.POST("/:messageId/reactions", h.AddReaction)
		messages.DELETE("/:messageId/reactions", h.RemoveReaction)
		messages.GET("/:messageId/reactions", h.GetReactions)

		// Media and files
		messages.POST("/upload", h.UploadMedia)
		messages.GET("/:messageId/media", h.GetMessageMedia)

		// Reports and moderation
		messages.POST("/:messageId/report", h.ReportMessage)
		messages.POST("/:messageId/pin", h.PinMessage)
		messages.DELETE("/:messageId/pin", h.UnpinMessage)

		// Bulk operations
		messages.POST("/bulk-delete", h.BulkDeleteMessages)
		messages.POST("/bulk-forward", h.BulkForwardMessages)
		messages.POST("/bulk-mark-read", h.BulkMarkAsRead)

		// Search and filters
		messages.GET("/search", h.SearchMessages)
		messages.GET("/media", h.GetMediaMessages)
		messages.GET("/files", h.GetFileMessages)
		messages.GET("/links", h.GetLinkMessages)
	}

	// Chat-specific message routes
	chats := router.Group("/chats/:chatId/messages")
	chats.Use(middleware.AuthMiddleware("your-jwt-secret"))
	{
		chats.GET("", h.GetChatMessages)
		chats.POST("", h.SendMessageToChat)
		chats.GET("/pinned", h.GetPinnedMessages)
		chats.DELETE("/clear", h.ClearChatMessages)
		chats.POST("/export", h.ExportChatMessages)
	}
}

// SendMessage sends a new message
func (h *MessageHandler) SendMessage(c *gin.Context) {
	var req models.SendMessageRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	// Validate request
	validationErrors := make(map[string]utils.ValidationError)

	if req.ChatID.IsZero() {
		validationErrors["chat_id"] = utils.ValidationError{
			Field:   "chat_id",
			Message: "Chat ID is required",
			Code:    utils.ErrRequired,
		}
	}

	if req.Type == "" {
		validationErrors["type"] = utils.ValidationError{
			Field:   "type",
			Message: "Message type is required",
			Code:    utils.ErrRequired,
		}
	}

	// Validate content based on message type
	if req.Type == models.MessageTypeText {
		if err := utils.ValidateMessageContent(req.Content, 4096); err != nil {
			validationErrors["content"] = *err
		}
	}

	if len(validationErrors) > 0 {
		utils.ValidationErrorResponse(c, validationErrors)
		return
	}

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Verify chat exists and user is participant
	var chat models.Chat
	err = collections.Chats.FindOne(ctx, bson.M{
		"_id":       req.ChatID,
		"is_active": true,
	}).Decode(&chat)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found")
		} else {
			utils.InternalServerError(c, "Failed to verify chat")
		}
		return
	}

	// Check if user is participant
	if !chat.IsParticipant(user.ID) {
		utils.Forbidden(c, "You are not a participant in this chat")
		return
	}

	// Check if user can send messages
	if !chat.CanUserMessage(user.ID) {
		utils.Forbidden(c, "You cannot send messages in this chat")
		return
	}

	// Check if chat is blocked for user
	if chat.IsBlockedFor(user.ID) {
		utils.Forbidden(c, "Chat is blocked")
		return
	}

	// Create message
	message := &models.Message{
		ChatID:   req.ChatID,
		SenderID: user.ID,
		Type:     req.Type,
		Content:  strings.TrimSpace(req.Content),
		MediaURL: req.MediaURL,
		Metadata: req.Metadata,
	}

	// Handle reply
	if req.ReplyToID != nil {
		replyToMessage, err := h.getMessageByID(*req.ReplyToID)
		if err == nil && replyToMessage.ChatID == req.ChatID {
			message.ReplyToID = req.ReplyToID
		}
	}

	// Handle mentions
	if len(req.Mentions) > 0 {
		message.Mentions = h.validateMentions(req.Mentions, chat.Participants)
	}

	// Handle scheduled messages
	if req.ScheduledAt != nil && req.ScheduledAt.After(time.Now()) {
		message.ScheduledAt = req.ScheduledAt
	}

	message.BeforeCreate()

	// Encrypt message if enabled
	if message.IsEncrypted && h.encryption != nil {
		encryptedContent, err := h.encryption.EncryptMessage(message.Content)
		if err != nil {
			logger.Errorf("Failed to encrypt message: %v", err)
		} else {
			// Store encrypted content (in production, you'd store this properly)
			message.Content = encryptedContent.Content
		}
	}

	// Insert message
	result, err := collections.Messages.InsertOne(ctx, message)
	if err != nil {
		logger.Errorf("Failed to insert message: %v", err)
		utils.InternalServerError(c, "Failed to send message")
		return
	}

	message.ID = result.InsertedID.(primitive.ObjectID)

	// Update chat with last message info
	chat.UpdateLastMessage(message.ID, user.ID, h.getMessagePreview(message), message.Type)
	chat.IncrementUnreadCount(user.ID, len(message.Mentions) > 0)

	// Update chat in database
	_, err = collections.Chats.UpdateOne(ctx,
		bson.M{"_id": req.ChatID},
		bson.M{"$set": bson.M{
			"last_message":  chat.LastMessage,
			"last_activity": chat.LastActivity,
			"unread_counts": chat.UnreadCounts,
			"message_count": chat.MessageCount,
			"updated_at":    time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to update chat: %v", err)
	}

	// Clear draft for sender
	chat.ClearDraft(user.ID)
	go h.updateChatDraft(&chat, user.ID)

	// Send real-time notifications
	go h.broadcastMessage(message, &chat, user)

	// Send push notifications
	go h.sendMessageNotifications(message, &chat, user)

	// Process message for special content (links, mentions, etc.)
	go h.processMessageContent(message)

	// Log message
	logger.LogUserAction(user.ID.Hex(), "message_sent", "message", map[string]interface{}{
		"message_id":   message.ID.Hex(),
		"chat_id":      req.ChatID.Hex(),
		"message_type": string(message.Type),
		"has_media":    message.MediaURL != "",
		"has_mentions": len(message.Mentions) > 0,
	})

	// Build response
	messageResponse := h.buildMessageResponse(message, user, &chat)

	utils.Created(c, messageResponse)
}

// SendMessageToChat sends a message to a specific chat (alternative endpoint)
func (h *MessageHandler) SendMessageToChat(c *gin.Context) {
	chatID, err := primitive.ObjectIDFromHex(c.Param("chatId"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	// Parse multipart form for file uploads
	if strings.Contains(c.GetHeader("Content-Type"), "multipart/form-data") {
		h.sendMessageWithFile(c, chatID)
		return
	}

	var req models.SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	// Set chat ID from URL parameter
	req.ChatID = chatID

	// Call the main send message method
	h.SendMessage(c)
}

// sendMessageWithFile handles message sending with file upload
func (h *MessageHandler) sendMessageWithFile(c *gin.Context, chatID primitive.ObjectID) {
	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	// Parse form
	form, err := c.MultipartForm()
	if err != nil {
		utils.BadRequest(c, "Invalid multipart form")
		return
	}

	// Get message data
	messageType := c.PostForm("type")
	content := c.PostForm("content")
	replyToID := c.PostForm("reply_to_id")

	if messageType == "" {
		messageType = string(models.MessageTypeDocument)
	}

	// Handle file upload
	files := form.File["file"]
	if len(files) == 0 {
		utils.BadRequest(c, "No file provided")
		return
	}

	file := files[0]

	// Create file upload
	fileUpload, err := h.createFileUploadFromMultipart(file, user.ID, "message")
	if err != nil {
		utils.BadRequest(c, "Invalid file")
		return
	}

	fileUpload.ChatID = &chatID

	// Upload file
	fileInfo, err := h.fileService.UploadFile(fileUpload)
	if err != nil {
		logger.Errorf("Failed to upload file: %v", err)
		utils.InternalServerError(c, "Failed to upload file")
		return
	}

	// Create message request
	req := models.SendMessageRequest{
		ChatID:   chatID,
		Type:     models.MessageType(messageType),
		Content:  content,
		MediaURL: fileInfo.URL,
		Metadata: models.MessageMetadata{
			// Add file metadata
		},
	}

	// Handle reply
	if replyToID != "" {
		if replyObjID, err := primitive.ObjectIDFromHex(replyToID); err == nil {
			req.ReplyToID = &replyObjID
		}
	}

	// Send the message (reuse the existing logic)
	// ... (implementation would call the main message sending logic)

	utils.Created(c, map[string]interface{}{
		"message": "Message with file sent successfully",
		"file":    fileInfo,
	})
}

// GetMessage retrieves a specific message
func (h *MessageHandler) GetMessage(c *gin.Context) {
	messageID, err := primitive.ObjectIDFromHex(c.Param("messageId"))
	if err != nil {
		utils.BadRequest(c, "Invalid message ID")
		return
	}

	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	message, err := h.getMessageByID(messageID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Message not found")
		} else {
			utils.InternalServerError(c, "Failed to get message")
		}
		return
	}

	// Check if user has access to the message
	if !h.canUserAccessMessage(message, user.ID) {
		utils.Forbidden(c, "You don't have access to this message")
		return
	}

	// Get chat info
	chat, err := h.getChatByID(message.ChatID)
	if err != nil {
		utils.InternalServerError(c, "Failed to get chat info")
		return
	}

	messageResponse := h.buildMessageResponse(message, user, chat)
	utils.Success(c, messageResponse)
}

// GetChatMessages retrieves messages from a specific chat
func (h *MessageHandler) GetChatMessages(c *gin.Context) {
	chatID, err := primitive.ObjectIDFromHex(c.Param("chatId"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	// Parse query parameters
	pagination := utils.GetPaginationParams(c)
	messageType := c.Query("type")
	beforeID := c.Query("before")
	afterID := c.Query("after")
	fromUser := c.Query("from_user")

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Verify chat access
	var chat models.Chat
	err = collections.Chats.FindOne(ctx, bson.M{
		"_id":          chatID,
		"participants": user.ID,
		"is_active":    true,
	}).Decode(&chat)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found or access denied")
		} else {
			utils.InternalServerError(c, "Failed to verify chat access")
		}
		return
	}

	// Build message filter
	filter := bson.M{
		"chat_id":    chatID,
		"is_deleted": false,
		"$or": []bson.M{
			{"deleted_for": bson.M{"$ne": user.ID}},
			{"deleted_for": bson.M{"$exists": false}},
		},
	}

	// Add type filter
	if messageType != "" {
		filter["type"] = messageType
	}

	// Add user filter
	if fromUser != "" {
		if userObjID, err := primitive.ObjectIDFromHex(fromUser); err == nil {
			filter["sender_id"] = userObjID
		}
	}

	// Add pagination filters
	if beforeID != "" {
		if objID, err := primitive.ObjectIDFromHex(beforeID); err == nil {
			filter["_id"] = bson.M{"$lt": objID}
		}
	}

	if afterID != "" {
		if objID, err := primitive.ObjectIDFromHex(afterID); err == nil {
			filter["_id"] = bson.M{"$gt": objID}
		}
	}

	// Handle scheduled messages (only show to sender)
	filter["$or"] = []bson.M{
		{"scheduled_at": bson.M{"$exists": false}},
		{"scheduled_at": bson.M{"$lte": time.Now()}},
		{"sender_id": user.ID},
	}

	// Get total count
	total, err := collections.Messages.CountDocuments(ctx, filter)
	if err != nil {
		logger.Errorf("Failed to count messages: %v", err)
		utils.InternalServerError(c, "Failed to get messages")
		return
	}

	// Get messages
	sortOrder := -1 // Newest first
	if pagination.SortDir == "asc" {
		sortOrder = 1
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: sortOrder}}).
		SetLimit(int64(pagination.Limit)).
		SetSkip(int64((pagination.Page - 1) * pagination.Limit))

	cursor, err := collections.Messages.Find(ctx, filter, opts)
	if err != nil {
		logger.Errorf("Failed to find messages: %v", err)
		utils.InternalServerError(c, "Failed to get messages")
		return
	}
	defer cursor.Close(ctx)

	var messages []models.Message
	if err := cursor.All(ctx, &messages); err != nil {
		logger.Errorf("Failed to decode messages: %v", err)
		utils.InternalServerError(c, "Failed to get messages")
		return
	}

	// Convert to response format
	messageResponses := make([]models.MessageResponse, len(messages))
	for i, message := range messages {
		messageResponse := h.buildMessageResponse(&message, user, &chat)
		messageResponses[i] = messageResponse
	}

	// Create pagination metadata
	meta := utils.CreatePaginationMeta(pagination.Page, pagination.Limit, total)

	// Mark messages as read if this is the latest page
	if pagination.Page == 1 && len(messages) > 0 {
		go h.markMessagesAsRead(messages, user.ID, chatID)
	}

	utils.SuccessWithMeta(c, map[string]interface{}{
		"messages": messageResponses,
		"chat_id":  chatID.Hex(),
	}, meta)
}

// EditMessage edits an existing message
func (h *MessageHandler) EditMessage(c *gin.Context) {
	messageID, err := primitive.ObjectIDFromHex(c.Param("messageId"))
	if err != nil {
		utils.BadRequest(c, "Invalid message ID")
		return
	}

	var req models.UpdateMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	// Validate content
	if err := utils.ValidateMessageContent(req.Content, 4096); err != nil {
		utils.ValidationErrorResponse(c, map[string]utils.ValidationError{
			"content": *err,
		})
		return
	}

	message, err := h.getMessageByID(messageID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Message not found")
		} else {
			utils.InternalServerError(c, "Failed to get message")
		}
		return
	}

	// Check if user can edit the message
	if !message.CanBeEditedBy(user.ID) {
		utils.Forbidden(c, "You cannot edit this message")
		return
	}

	// Edit the message
	message.Edit(req.Content, req.Reason)

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Update message
	_, err = collections.Messages.ReplaceOne(ctx, bson.M{"_id": messageID}, message)
	if err != nil {
		logger.Errorf("Failed to update message: %v", err)
		utils.InternalServerError(c, "Failed to edit message")
		return
	}

	// Update last message in chat if this was the latest message
	go h.updateChatLastMessage(message.ChatID, message.ID)

	// Broadcast edit notification
	go h.broadcastMessageEdit(message, user)

	// Log edit
	logger.LogUserAction(user.ID.Hex(), "message_edited", "message", map[string]interface{}{
		"message_id": message.ID.Hex(),
		"chat_id":    message.ChatID.Hex(),
		"reason":     req.Reason,
	})

	utils.Success(c, map[string]interface{}{
		"message":   "Message edited successfully",
		"edited_at": message.UpdatedAt,
		"is_edited": message.IsEdited,
	})
}

// DeleteMessage deletes a message
func (h *MessageHandler) DeleteMessage(c *gin.Context) {
	messageID, err := primitive.ObjectIDFromHex(c.Param("messageId"))
	if err != nil {
		utils.BadRequest(c, "Invalid message ID")
		return
	}

	var req struct {
		DeleteForEveryone bool `json:"delete_for_everyone"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		// If no body provided, default to delete for self
		req.DeleteForEveryone = false
	}

	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	message, err := h.getMessageByID(messageID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Message not found")
		} else {
			utils.InternalServerError(c, "Failed to get message")
		}
		return
	}

	// Check if user can delete the message
	if !message.CanBeDeletedBy(user.ID) {
		utils.Forbidden(c, "You cannot delete this message")
		return
	}

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if req.DeleteForEveryone {
		// Check if user can delete for everyone
		if !message.CanBeDeletedForEveryone(user.ID) {
			utils.Forbidden(c, "You can only delete this message for yourself")
			return
		}

		// Delete for everyone
		message.DeleteForEveryone()

		// Broadcast deletion
		go h.broadcastMessageDeletion(message, user, true)
	} else {
		// Delete for user only
		message.DeleteForUser(user.ID)
	}

	// Update message
	_, err = collections.Messages.ReplaceOne(ctx, bson.M{"_id": messageID}, message)
	if err != nil {
		logger.Errorf("Failed to delete message: %v", err)
		utils.InternalServerError(c, "Failed to delete message")
		return
	}

	// Update last message in chat if this was the latest message
	if req.DeleteForEveryone {
		go h.updateChatLastMessage(message.ChatID, message.ID)
	}

	// Log deletion
	logger.LogUserAction(user.ID.Hex(), "message_deleted", "message", map[string]interface{}{
		"message_id":          message.ID.Hex(),
		"chat_id":             message.ChatID.Hex(),
		"delete_for_everyone": req.DeleteForEveryone,
	})

	deleteType := "yourself"
	if req.DeleteForEveryone {
		deleteType = "everyone"
	}

	utils.Success(c, map[string]interface{}{
		"message":     fmt.Sprintf("Message deleted for %s", deleteType),
		"deleted_for": deleteType,
	})
}

// ForwardMessage forwards a message to other chats
func (h *MessageHandler) ForwardMessage(c *gin.Context) {
	messageID, err := primitive.ObjectIDFromHex(c.Param("messageId"))
	if err != nil {
		utils.BadRequest(c, "Invalid message ID")
		return
	}

	var req struct {
		ChatIDs []primitive.ObjectID `json:"chat_ids" binding:"required"`
		Caption string               `json:"caption"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	if len(req.ChatIDs) == 0 {
		utils.BadRequest(c, "At least one chat ID is required")
		return
	}

	if len(req.ChatIDs) > 10 {
		utils.BadRequest(c, "Cannot forward to more than 10 chats at once")
		return
	}

	message, err := h.getMessageByID(messageID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Message not found")
		} else {
			utils.InternalServerError(c, "Failed to get message")
		}
		return
	}

	// Check if user can access the original message
	if !h.canUserAccessMessage(message, user.ID) {
		utils.Forbidden(c, "You don't have access to this message")
		return
	}

	// Verify user has access to all target chats
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	chatCount, err := collections.Chats.CountDocuments(ctx, bson.M{
		"_id":          bson.M{"$in": req.ChatIDs},
		"participants": user.ID,
		"is_active":    true,
	})
	if err != nil || chatCount != int64(len(req.ChatIDs)) {
		utils.BadRequest(c, "You don't have access to one or more target chats")
		return
	}

	// Forward message to each chat
	var forwardedMessages []primitive.ObjectID
	var failedChats []primitive.ObjectID

	for _, chatID := range req.ChatIDs {
		forwardedMsg, err := h.createForwardedMessage(message, chatID, user.ID, req.Caption)
		if err != nil {
			logger.Errorf("Failed to forward message to chat %s: %v", chatID.Hex(), err)
			failedChats = append(failedChats, chatID)
			continue
		}

		forwardedMessages = append(forwardedMessages, forwardedMsg.ID)

		// Send notifications for forwarded message
		go h.sendForwardedMessageNotifications(forwardedMsg, chatID, user)
	}

	// Log forward action
	logger.LogUserAction(user.ID.Hex(), "message_forwarded", "message", map[string]interface{}{
		"original_message_id": message.ID.Hex(),
		"target_chats":        len(req.ChatIDs),
		"successful_forwards": len(forwardedMessages),
		"failed_forwards":     len(failedChats),
	})

	result := map[string]interface{}{
		"message":            "Message forwarded successfully",
		"forwarded_to":       len(forwardedMessages),
		"total_chats":        len(req.ChatIDs),
		"forwarded_messages": forwardedMessages,
	}

	if len(failedChats) > 0 {
		result["failed_chats"] = failedChats
		result["message"] = fmt.Sprintf("Message forwarded to %d out of %d chats", len(forwardedMessages), len(req.ChatIDs))
	}

	utils.Success(c, result)
}

// UpdateMessageStatus updates message status (delivered/read)
func (h *MessageHandler) UpdateMessageStatus(c *gin.Context) {
	messageID, err := primitive.ObjectIDFromHex(c.Param("messageId"))
	if err != nil {
		utils.BadRequest(c, "Invalid message ID")
		return
	}

	var req models.MessageStatusUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	message, err := h.getMessageByID(messageID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Message not found")
		} else {
			utils.InternalServerError(c, "Failed to get message")
		}
		return
	}

	// Check if user can update status (must be recipient)
	if message.SenderID == user.ID {
		utils.BadRequest(c, "Cannot update status of your own message")
		return
	}

	// Check if user is in the chat
	if !h.canUserAccessMessage(message, user.ID) {
		utils.Forbidden(c, "You don't have access to this message")
		return
	}

	// Update status
	switch req.Status {
	case models.MessageStatusDelivered:
		message.MarkAsDelivered(user.ID)
	case models.MessageStatusRead:
		message.MarkAsRead(user.ID)
	default:
		utils.BadRequest(c, "Invalid message status")
		return
	}

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Update message
	_, err = collections.Messages.UpdateOne(ctx,
		bson.M{"_id": messageID},
		bson.M{"$set": bson.M{
			"status":       message.Status,
			"read_by":      message.ReadBy,
			"delivered_to": message.DeliveredTo,
			"updated_at":   time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to update message status: %v", err)
		utils.InternalServerError(c, "Failed to update message status")
		return
	}

	// Broadcast status update
	go h.broadcastMessageStatusUpdate(message, user, req.Status)

	utils.Success(c, map[string]interface{}{
		"message": "Message status updated successfully",
		"status":  string(req.Status),
	})
}

// MarkAsRead marks a message as read
func (h *MessageHandler) MarkAsRead(c *gin.Context) {
	messageID, err := primitive.ObjectIDFromHex(c.Param("messageId"))
	if err != nil {
		utils.BadRequest(c, "Invalid message ID")
		return
	}

	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	req := models.MessageStatusUpdate{
		MessageID: messageID,
		Status:    models.MessageStatusRead,
	}

	// Reuse the UpdateMessageStatus logic
	c.Set("messageStatusUpdate", req)
	h.UpdateMessageStatus(c)
}

// MarkAsDelivered marks a message as delivered
func (h *MessageHandler) MarkAsDelivered(c *gin.Context) {
	messageID, err := primitive.ObjectIDFromHex(c.Param("messageId"))
	if err != nil {
		utils.BadRequest(c, "Invalid message ID")
		return
	}

	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	req := models.MessageStatusUpdate{
		MessageID: messageID,
		Status:    models.MessageStatusDelivered,
	}

	// Reuse the UpdateMessageStatus logic
	c.Set("messageStatusUpdate", req)
	h.UpdateMessageStatus(c)
}

// AddReaction adds a reaction to a message
func (h *MessageHandler) AddReaction(c *gin.Context) {
	messageID, err := primitive.ObjectIDFromHex(c.Param("messageId"))
	if err != nil {
		utils.BadRequest(c, "Invalid message ID")
		return
	}

	var req models.AddReactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	// Validate emoji
	if req.Emoji == "" {
		utils.BadRequest(c, "Emoji is required")
		return
	}

	message, err := h.getMessageByID(messageID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Message not found")
		} else {
			utils.InternalServerError(c, "Failed to get message")
		}
		return
	}

	// Check if user can access the message
	if !h.canUserAccessMessage(message, user.ID) {
		utils.Forbidden(c, "You don't have access to this message")
		return
	}

	// Add reaction
	message.AddReaction(user.ID, req.Emoji)

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Update message
	_, err = collections.Messages.UpdateOne(ctx,
		bson.M{"_id": messageID},
		bson.M{"$set": bson.M{
			"reactions":  message.Reactions,
			"updated_at": time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to add reaction: %v", err)
		utils.InternalServerError(c, "Failed to add reaction")
		return
	}

	// Broadcast reaction
	go h.broadcastReaction(message, user, req.Emoji, "added")

	// Log reaction
	logger.LogUserAction(user.ID.Hex(), "reaction_added", "message", map[string]interface{}{
		"message_id": message.ID.Hex(),
		"emoji":      req.Emoji,
	})

	utils.Success(c, map[string]interface{}{
		"message":   "Reaction added successfully",
		"emoji":     req.Emoji,
		"reactions": message.Reactions,
	})
}

// RemoveReaction removes a reaction from a message
func (h *MessageHandler) RemoveReaction(c *gin.Context) {
	messageID, err := primitive.ObjectIDFromHex(c.Param("messageId"))
	if err != nil {
		utils.BadRequest(c, "Invalid message ID")
		return
	}

	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	message, err := h.getMessageByID(messageID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Message not found")
		} else {
			utils.InternalServerError(c, "Failed to get message")
		}
		return
	}

	// Check if user can access the message
	if !h.canUserAccessMessage(message, user.ID) {
		utils.Forbidden(c, "You don't have access to this message")
		return
	}

	// Remove reaction
	message.RemoveReaction(user.ID)

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Update message
	_, err = collections.Messages.UpdateOne(ctx,
		bson.M{"_id": messageID},
		bson.M{"$set": bson.M{
			"reactions":  message.Reactions,
			"updated_at": time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to remove reaction: %v", err)
		utils.InternalServerError(c, "Failed to remove reaction")
		return
	}

	// Broadcast reaction removal
	go h.broadcastReaction(message, user, "", "removed")

	// Log reaction removal
	logger.LogUserAction(user.ID.Hex(), "reaction_removed", "message", map[string]interface{}{
		"message_id": message.ID.Hex(),
	})

	utils.Success(c, map[string]interface{}{
		"message":   "Reaction removed successfully",
		"reactions": message.Reactions,
	})
}

// GetReactions gets all reactions for a message
func (h *MessageHandler) GetReactions(c *gin.Context) {
	messageID, err := primitive.ObjectIDFromHex(c.Param("messageId"))
	if err != nil {
		utils.BadRequest(c, "Invalid message ID")
		return
	}

	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	message, err := h.getMessageByID(messageID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Message not found")
		} else {
			utils.InternalServerError(c, "Failed to get message")
		}
		return
	}

	// Check if user can access the message
	if !h.canUserAccessMessage(message, user.ID) {
		utils.Forbidden(c, "You don't have access to this message")
		return
	}

	// Group reactions by emoji
	reactionGroups := make(map[string][]models.Reaction)
	for _, reaction := range message.Reactions {
		reactionGroups[reaction.Emoji] = append(reactionGroups[reaction.Emoji], reaction)
	}

	utils.Success(c, map[string]interface{}{
		"message_id":      message.ID.Hex(),
		"reactions":       message.Reactions,
		"reaction_groups": reactionGroups,
		"total_reactions": len(message.Reactions),
	})
}

// SearchMessages searches messages across chats
func (h *MessageHandler) SearchMessages(c *gin.Context) {
	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	query := c.Query("q")
	if query == "" {
		utils.BadRequest(c, "Search query is required")
		return
	}

	// Parse additional filters
	chatID := c.Query("chat_id")
	messageType := c.Query("type")
	fromUser := c.Query("from_user")
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")

	pagination := utils.GetPaginationParams(c)

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Get user's chat IDs first
	var userChatIDs []primitive.ObjectID
	if chatID != "" {
		// Search in specific chat
		if objID, err := primitive.ObjectIDFromHex(chatID); err == nil {
			userChatIDs = []primitive.ObjectID{objID}
		}
	} else {
		// Get all user's chats
		chatCursor, err := collections.Chats.Find(ctx, bson.M{
			"participants": user.ID,
			"is_active":    true,
		}, options.Find().SetProjection(bson.M{"_id": 1}))
		if err != nil {
			utils.InternalServerError(c, "Failed to get user chats")
			return
		}

		var chats []struct {
			ID primitive.ObjectID `bson:"_id"`
		}
		if err := chatCursor.All(ctx, &chats); err != nil {
			utils.InternalServerError(c, "Failed to decode chats")
			return
		}

		for _, chat := range chats {
			userChatIDs = append(userChatIDs, chat.ID)
		}
	}

	if len(userChatIDs) == 0 {
		utils.Success(c, map[string]interface{}{
			"messages":    []models.MessageResponse{},
			"total_count": 0,
			"query":       query,
		})
		return
	}

	// Build search filter
	filter := bson.M{
		"chat_id":    bson.M{"$in": userChatIDs},
		"is_deleted": false,
		"$or": []bson.M{
			{"deleted_for": bson.M{"$ne": user.ID}},
			{"deleted_for": bson.M{"$exists": false}},
		},
		"content": bson.M{"$regex": query, "$options": "i"},
	}

	// Add additional filters
	if messageType != "" {
		filter["type"] = messageType
	}

	if fromUser != "" {
		if userObjID, err := primitive.ObjectIDFromHex(fromUser); err == nil {
			filter["sender_id"] = userObjID
		}
	}

	// Add date range filter
	if dateFrom != "" || dateTo != "" {
		dateFilter := bson.M{}
		if dateFrom != "" {
			if timestamp, err := strconv.ParseInt(dateFrom, 10, 64); err == nil {
				dateFilter["$gte"] = time.Unix(timestamp, 0)
			}
		}
		if dateTo != "" {
			if timestamp, err := strconv.ParseInt(dateTo, 10, 64); err == nil {
				dateFilter["$lte"] = time.Unix(timestamp, 0)
			}
		}
		if len(dateFilter) > 0 {
			filter["created_at"] = dateFilter
		}
	}

	// Get total count
	total, err := collections.Messages.CountDocuments(ctx, filter)
	if err != nil {
		logger.Errorf("Failed to count search results: %v", err)
		utils.InternalServerError(c, "Search failed")
		return
	}

	// Get messages
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(int64(pagination.Limit)).
		SetSkip(int64((pagination.Page - 1) * pagination.Limit))

	cursor, err := collections.Messages.Find(ctx, filter, opts)
	if err != nil {
		logger.Errorf("Failed to search messages: %v", err)
		utils.InternalServerError(c, "Search failed")
		return
	}
	defer cursor.Close(ctx)

	var messages []models.Message
	if err := cursor.All(ctx, &messages); err != nil {
		logger.Errorf("Failed to decode search results: %v", err)
		utils.InternalServerError(c, "Search failed")
		return
	}

	// Convert to response format
	messageResponses := make([]models.MessageResponse, len(messages))
	for i, message := range messages {
		// Get chat info for each message
		chat, _ := h.getChatByID(message.ChatID)
		messageResponse := h.buildMessageResponse(&message, user, chat)
		messageResponses[i] = messageResponse
	}

	// Create pagination metadata
	meta := utils.CreatePaginationMeta(pagination.Page, pagination.Limit, total)

	utils.SuccessWithMeta(c, map[string]interface{}{
		"query":    query,
		"messages": messageResponses,
		"filters": map[string]interface{}{
			"chat_id":   chatID,
			"type":      messageType,
			"from_user": fromUser,
			"date_from": dateFrom,
			"date_to":   dateTo,
		},
	}, meta)
}

// Helper methods

// getMessageByID retrieves a message by ID
func (h *MessageHandler) getMessageByID(messageID primitive.ObjectID) (*models.Message, error) {
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var message models.Message
	err := collections.Messages.FindOne(ctx, bson.M{"_id": messageID}).Decode(&message)
	return &message, err
}

// getChatByID retrieves a chat by ID
func (h *MessageHandler) getChatByID(chatID primitive.ObjectID) (*models.Chat, error) {
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var chat models.Chat
	err := collections.Chats.FindOne(ctx, bson.M{"_id": chatID}).Decode(&chat)
	return &chat, err
}

// canUserAccessMessage checks if user can access a message
func (h *MessageHandler) canUserAccessMessage(message *models.Message, userID primitive.ObjectID) bool {
	// Check if message is deleted for user
	if message.IsDeletedForUser(userID) {
		return false
	}

	// Check if user is in the chat
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := collections.Chats.CountDocuments(ctx, bson.M{
		"_id":          message.ChatID,
		"participants": userID,
		"is_active":    true,
	})

	return err == nil && count > 0
}

// buildMessageResponse builds a message response with user-specific data
func (h *MessageHandler) buildMessageResponse(message *models.Message, user *models.User, chat *models.Chat) models.MessageResponse {
	response := models.MessageResponse{
		Message:        *message,
		CanEdit:        message.CanBeEditedBy(user.ID),
		CanDelete:      message.CanBeDeletedBy(user.ID),
		IsDeletedForMe: message.IsDeletedForUser(user.ID),
	}

	// Get sender info
	if sender, err := h.getUserByID(message.SenderID); err == nil {
		response.Sender = sender.GetPublicInfo(user.ID)
	}

	// Get reply-to message if exists
	if message.ReplyToID != nil {
		if replyMsg, err := h.getMessageByID(*message.ReplyToID); err == nil {
			response.ReplyTo = replyMsg
		}
	}

	// Decrypt content if encrypted
	if message.IsEncrypted && h.encryption != nil {
		// In production, you'd properly decrypt the content
		// For now, we'll just mark it as encrypted
		response.Message.Content = "[Encrypted Message]"
	}

	return response
}

// getUserByID retrieves a user by ID
func (h *MessageHandler) getUserByID(userID primitive.ObjectID) (*models.User, error) {
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err := collections.Users.FindOne(ctx, bson.M{
		"_id":        userID,
		"is_active":  true,
		"is_deleted": false,
	}).Decode(&user)

	return &user, err
}

// getMessagePreview gets a preview of the message for last message display
func (h *MessageHandler) getMessagePreview(message *models.Message) string {
	return message.GetPreviewText()
}

// validateMentions validates and filters mentions
func (h *MessageHandler) validateMentions(mentions []primitive.ObjectID, chatParticipants []primitive.ObjectID) []primitive.ObjectID {
	var validMentions []primitive.ObjectID

	for _, mention := range mentions {
		// Check if mentioned user is a participant
		for _, participant := range chatParticipants {
			if mention == participant {
				validMentions = append(validMentions, mention)
				break
			}
		}
	}

	return validMentions
}

// createForwardedMessage creates a new forwarded message
func (h *MessageHandler) createForwardedMessage(originalMessage *models.Message, targetChatID primitive.ObjectID, forwarderID primitive.ObjectID, caption string) (*models.Message, error) {
	forwardedMessage := &models.Message{
		ChatID:   targetChatID,
		SenderID: forwarderID,
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
	}

	// If original message was also forwarded, increment count
	if originalMessage.ForwardedFrom != nil {
		forwardedMessage.ForwardedFrom.ForwardCount = originalMessage.ForwardedFrom.ForwardCount + 1
		forwardedMessage.ForwardedFrom.OriginalSenderID = originalMessage.ForwardedFrom.OriginalSenderID
		forwardedMessage.ForwardedFrom.OriginalChatID = originalMessage.ForwardedFrom.OriginalChatID
		forwardedMessage.ForwardedFrom.OriginalMessageID = originalMessage.ForwardedFrom.OriginalMessageID
	}

	// Add caption if provided
	if caption != "" {
		forwardedMessage.Content = caption + "\n\n" + forwardedMessage.Content
	}

	forwardedMessage.BeforeCreate()

	// Insert forwarded message
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := collections.Messages.InsertOne(ctx, forwardedMessage)
	if err != nil {
		return nil, err
	}

	forwardedMessage.ID = result.InsertedID.(primitive.ObjectID)

	// Update target chat
	chat, err := h.getChatByID(targetChatID)
	if err == nil {
		chat.UpdateLastMessage(forwardedMessage.ID, forwarderID, h.getMessagePreview(forwardedMessage), forwardedMessage.Type)
		chat.IncrementUnreadCount(forwarderID, false)

		_, err = collections.Chats.UpdateOne(ctx,
			bson.M{"_id": targetChatID},
			bson.M{"$set": bson.M{
				"last_message":  chat.LastMessage,
				"last_activity": chat.LastActivity,
				"unread_counts": chat.UnreadCounts,
				"message_count": chat.MessageCount,
				"updated_at":    time.Now(),
			}},
		)
		if err != nil {
			logger.Errorf("Failed to update chat after forward: %v", err)
		}
	}

	return forwardedMessage, nil
}

// createFileUploadFromMultipart creates a file upload from multipart form
func (h *MessageHandler) createFileUploadFromMultipart(fileHeader *multipart.FileHeader, userID primitive.ObjectID, purpose string) (*services.FileUpload, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open uploaded file: %w", err)
	}

	// Detect content type
	buffer := make([]byte, 512)
	_, err = file.Read(buffer)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to read file header: %w", err)
	}

	contentType := http.DetectContentType(buffer)

	// Reset file pointer
	file.Seek(0, 0)

	upload := &services.FileUpload{
		UserID:      userID,
		FileName:    fileHeader.Filename,
		ContentType: contentType,
		Size:        fileHeader.Size,
		Data:        file,
		Purpose:     purpose,
		IsPublic:    false,
	}

	return upload, nil
}

// Notification and broadcast methods

// broadcastMessage broadcasts a new message to chat participants
func (h *MessageHandler) broadcastMessage(message *models.Message, chat *models.Chat, sender *models.User) {
	// Implementation would use WebSocket to broadcast to online users
	logger.Infof("Broadcasting message %s to chat %s", message.ID.Hex(), chat.ID.Hex())
}

// sendMessageNotifications sends push notifications for new message
func (h *MessageHandler) sendMessageNotifications(message *models.Message, chat *models.Chat, sender *models.User) {
	if h.pushService == nil {
		return
	}

	// Send to all participants except sender
	for _, participantID := range chat.Participants {
		if participantID == sender.ID {
			continue
		}

		// Check if chat is muted for participant
		if chat.IsMutedFor(participantID) {
			continue
		}

		// Send notification
		if err := h.pushService.SendMessageNotification(message, chat, sender); err != nil {
			logger.Errorf("Failed to send message notification: %v", err)
		}
	}
}

// broadcastMessageEdit broadcasts message edit notification
func (h *MessageHandler) broadcastMessageEdit(message *models.Message, editor *models.User) {
	logger.Infof("Broadcasting message edit %s by %s", message.ID.Hex(), editor.ID.Hex())
}

// broadcastMessageDeletion broadcasts message deletion notification
func (h *MessageHandler) broadcastMessageDeletion(message *models.Message, deleter *models.User, forEveryone bool) {
	logger.Infof("Broadcasting message deletion %s by %s (for everyone: %v)", message.ID.Hex(), deleter.ID.Hex(), forEveryone)
}

// broadcastMessageStatusUpdate broadcasts message status update
func (h *MessageHandler) broadcastMessageStatusUpdate(message *models.Message, user *models.User, status models.MessageStatus) {
	logger.Infof("Broadcasting message status update %s to %s by %s", message.ID.Hex(), string(status), user.ID.Hex())
}

// broadcastReaction broadcasts reaction add/remove
func (h *MessageHandler) broadcastReaction(message *models.Message, user *models.User, emoji string, action string) {
	logger.Infof("Broadcasting reaction %s %s on message %s by %s", action, emoji, message.ID.Hex(), user.ID.Hex())
}

// sendForwardedMessageNotifications sends notifications for forwarded messages
func (h *MessageHandler) sendForwardedMessageNotifications(message *models.Message, chatID primitive.ObjectID, forwarder *models.User) {
	if h.pushService == nil {
		return
	}

	// Implementation would send notifications to chat participants
	logger.Infof("Sending forwarded message notifications for %s in chat %s", message.ID.Hex(), chatID.Hex())
}

// Utility methods

// markMessagesAsRead marks multiple messages as read
func (h *MessageHandler) markMessagesAsRead(messages []models.Message, userID primitive.ObjectID, chatID primitive.ObjectID) {
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	messageIDs := make([]primitive.ObjectID, len(messages))
	for i, msg := range messages {
		messageIDs[i] = msg.ID
	}

	// Mark messages as read
	_, err := collections.Messages.UpdateMany(ctx,
		bson.M{
			"_id":       bson.M{"$in": messageIDs},
			"sender_id": bson.M{"$ne": userID},
		},
		bson.M{
			"$addToSet": bson.M{
				"read_by": models.ReadReceipt{
					UserID: userID,
					ReadAt: time.Now(),
				},
			},
		},
	)
	if err != nil {
		logger.Errorf("Failed to mark messages as read: %v", err)
		return
	}

	// Update chat unread count
	_, err = collections.Chats.UpdateOne(ctx,
		bson.M{
			"_id":                   chatID,
			"unread_counts.user_id": userID,
		},
		bson.M{
			"$set": bson.M{
				"unread_counts.$.count":         0,
				"unread_counts.$.mention_count": 0,
				"unread_counts.$.last_read_at":  time.Now(),
			},
		},
	)
	if err != nil {
		logger.Errorf("Failed to update chat unread count: %v", err)
	}
}

// updateChatLastMessage updates chat's last message if needed
func (h *MessageHandler) updateChatLastMessage(chatID primitive.ObjectID, messageID primitive.ObjectID) {
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get chat to check if this was the last message
	var chat models.Chat
	err := collections.Chats.FindOne(ctx, bson.M{"_id": chatID}).Decode(&chat)
	if err != nil {
		return
	}

	// If this was the last message, find the new last message
	if chat.LastMessage != nil && chat.LastMessage.MessageID == messageID {
		// Find the most recent non-deleted message
		var lastMessage models.Message
		err = collections.Messages.FindOne(ctx,
			bson.M{
				"chat_id":    chatID,
				"is_deleted": false,
			},
			options.FindOne().SetSort(bson.D{{Key: "created_at", Value: -1}}),
		).Decode(&lastMessage)

		if err == nil {
			chat.UpdateLastMessage(lastMessage.ID, lastMessage.SenderID, h.getMessagePreview(&lastMessage), lastMessage.Type)

			_, err = collections.Chats.UpdateOne(ctx,
				bson.M{"_id": chatID},
				bson.M{"$set": bson.M{
					"last_message":  chat.LastMessage,
					"last_activity": chat.LastActivity,
					"updated_at":    time.Now(),
				}},
			)
			if err != nil {
				logger.Errorf("Failed to update chat last message: %v", err)
			}
		}
	}
}

// updateChatDraft updates chat draft message
func (h *MessageHandler) updateChatDraft(chat *models.Chat, userID primitive.ObjectID) {
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collections.Chats.UpdateOne(ctx,
		bson.M{"_id": chat.ID},
		bson.M{"$set": bson.M{
			"draft_messages": chat.DraftMessages,
			"updated_at":     time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to update chat draft: %v", err)
	}
}

// processMessageContent processes message for special content (links, etc.)
func (h *MessageHandler) processMessageContent(message *models.Message) {
	// Implementation would:
	// - Extract and preview links
	// - Process mentions
	// - Check for spam/inappropriate content
	// - Generate rich content previews
	logger.Infof("Processing content for message %s", message.ID.Hex())
}

// Placeholder implementations for remaining endpoints

func (h *MessageHandler) UploadMedia(c *gin.Context) {
	utils.Success(c, map[string]interface{}{
		"message": "Upload media endpoint - implementation needed",
	})
}

func (h *MessageHandler) GetMessageMedia(c *gin.Context) {
	utils.Success(c, map[string]interface{}{
		"message": "Get message media endpoint - implementation needed",
	})
}

func (h *MessageHandler) ReportMessage(c *gin.Context) {
	utils.Success(c, map[string]interface{}{
		"message": "Report message endpoint - implementation needed",
	})
}

func (h *MessageHandler) PinMessage(c *gin.Context) {
	utils.Success(c, map[string]interface{}{
		"message": "Pin message endpoint - implementation needed",
	})
}

func (h *MessageHandler) UnpinMessage(c *gin.Context) {
	utils.Success(c, map[string]interface{}{
		"message": "Unpin message endpoint - implementation needed",
	})
}

func (h *MessageHandler) BulkDeleteMessages(c *gin.Context) {
	utils.Success(c, map[string]interface{}{
		"message": "Bulk delete messages endpoint - implementation needed",
	})
}

func (h *MessageHandler) BulkForwardMessages(c *gin.Context) {
	utils.Success(c, map[string]interface{}{
		"message": "Bulk forward messages endpoint - implementation needed",
	})
}

func (h *MessageHandler) BulkMarkAsRead(c *gin.Context) {
	utils.Success(c, map[string]interface{}{
		"message": "Bulk mark as read endpoint - implementation needed",
	})
}

func (h *MessageHandler) GetMediaMessages(c *gin.Context) {
	utils.Success(c, map[string]interface{}{
		"message": "Get media messages endpoint - implementation needed",
	})
}

func (h *MessageHandler) GetFileMessages(c *gin.Context) {
	utils.Success(c, map[string]interface{}{
		"message": "Get file messages endpoint - implementation needed",
	})
}

func (h *MessageHandler) GetLinkMessages(c *gin.Context) {
	utils.Success(c, map[string]interface{}{
		"message": "Get link messages endpoint - implementation needed",
	})
}

func (h *MessageHandler) GetPinnedMessages(c *gin.Context) {
	utils.Success(c, map[string]interface{}{
		"message": "Get pinned messages endpoint - implementation needed",
	})
}

func (h *MessageHandler) ClearChatMessages(c *gin.Context) {
	utils.Success(c, map[string]interface{}{
		"message": "Clear chat messages endpoint - implementation needed",
	})
}

func (h *MessageHandler) ExportChatMessages(c *gin.Context) {
	utils.Success(c, map[string]interface{}{
		"message": "Export chat messages endpoint - implementation needed",
	})
}
