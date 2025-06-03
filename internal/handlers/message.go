package handlers

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"bro/internal/models"
	"bro/internal/services"
	"bro/internal/utils"
	"bro/pkg/logger"
)

// MessageHandler handles message-related HTTP requests
type MessageHandler struct {
	messageService *services.MessageService
}

// NewMessageHandler creates a new message handler
func NewMessageHandler(messageService *services.MessageService) *MessageHandler {
	return &MessageHandler{
		messageService: messageService,
	}
}

// RegisterRoutes registers message routes
func (h *MessageHandler) RegisterRoutes(router *gin.RouterGroup, authMiddleware gin.HandlerFunc) {
	messages := router.Group("/messages")
	messages.Use(authMiddleware)

	// Message CRUD operations
	messages.POST("", h.SendMessage)
	messages.GET("", h.GetMessages)
	messages.GET("/:id", h.GetMessage)
	messages.PUT("/:id", h.UpdateMessage)
	messages.DELETE("/:id", h.DeleteMessage)

	// Message actions
	messages.POST("/:id/reactions", h.AddReaction)
	messages.DELETE("/:id/reactions", h.RemoveReaction)
	messages.PUT("/:id/read", h.MarkAsRead)
	messages.POST("/:id/forward", h.ForwardMessage)

	// Bulk operations
	messages.PUT("/read", h.MarkMultipleAsRead)
	messages.DELETE("/bulk", h.BulkDeleteMessages)

	// Search and filters
	messages.GET("/search", h.SearchMessages)

	// Statistics (admin only)
	messages.GET("/stats", h.GetMessageStatistics)
}

// SendMessage handles POST /api/messages
func (h *MessageHandler) SendMessage(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	var req services.SendMessageRequest
	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	// Validate request
	validationErrors := h.validateSendMessageRequest(&req)
	if len(validationErrors) > 0 {
		utils.ValidationErrorResponse(c, validationErrors)
		return
	}

	// Send message
	message, err := h.messageService.SendMessage(userID, &req)
	if err != nil {
		h.handleMessageServiceError(c, err, "Failed to send message")
		return
	}

	logger.LogUserAction(userID.Hex(), "message_sent", "message_handler", map[string]interface{}{
		"message_id": message.ID.Hex(),
		"chat_id":    req.ChatID.Hex(),
		"type":       req.Type,
	})

	utils.Created(c, message)
}

// GetMessages handles GET /api/messages
func (h *MessageHandler) GetMessages(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	var req services.MessageListRequest
	if err := utils.ParseQuery(c, &req); err != nil {
		utils.BadRequest(c, "Invalid query parameters")
		return
	}

	// Parse chat_id from query
	chatIDStr := c.Query("chat_id")
	if chatIDStr == "" {
		utils.BadRequest(c, "chat_id is required")
		return
	}

	chatID, err := primitive.ObjectIDFromHex(chatIDStr)
	if err != nil {
		utils.BadRequest(c, "Invalid chat_id format")
		return
	}
	req.ChatID = chatID

	// Parse optional parameters
	if beforeStr := c.Query("before"); beforeStr != "" {
		if beforeID, err := primitive.ObjectIDFromHex(beforeStr); err == nil {
			req.Before = &beforeID
		}
	}

	if afterStr := c.Query("after"); afterStr != "" {
		if afterID, err := primitive.ObjectIDFromHex(afterStr); err == nil {
			req.After = &afterID
		}
	}

	if senderIDStr := c.Query("sender_id"); senderIDStr != "" {
		if senderID, err := primitive.ObjectIDFromHex(senderIDStr); err == nil {
			req.SenderID = &senderID
		}
	}

	// Parse date filters
	if dateFromStr := c.Query("date_from"); dateFromStr != "" {
		if dateFrom, err := utils.ParseISO8601(dateFromStr); err == nil {
			req.DateFrom = &dateFrom
		}
	}

	if dateToStr := c.Query("date_to"); dateToStr != "" {
		if dateTo, err := utils.ParseISO8601(dateToStr); err == nil {
			req.DateTo = &dateTo
		}
	}

	// Set defaults
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 50
	}

	// Get messages
	messages, err := h.messageService.GetMessages(userID, &req)
	if err != nil {
		h.handleMessageServiceError(c, err, "Failed to get messages")
		return
	}

	// Create pagination metadata
	meta := utils.CreatePaginationMeta(req.Page, req.Limit, messages.TotalCount)

	utils.SuccessWithMeta(c, messages.Messages, meta)
}

// GetMessage handles GET /api/messages/:id
func (h *MessageHandler) GetMessage(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	messageID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid message ID")
		return
	}

	message, err := h.messageService.GetMessage(messageID, userID)
	if err != nil {
		h.handleMessageServiceError(c, err, "Failed to get message")
		return
	}

	utils.Success(c, message)
}

// UpdateMessage handles PUT /api/messages/:id
func (h *MessageHandler) UpdateMessage(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	messageID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid message ID")
		return
	}

	var req services.MessageUpdateRequest
	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	// Validate request
	validationErrors := h.validateUpdateMessageRequest(&req)
	if len(validationErrors) > 0 {
		utils.ValidationErrorResponse(c, validationErrors)
		return
	}

	message, err := h.messageService.UpdateMessage(messageID, userID, &req)
	if err != nil {
		h.handleMessageServiceError(c, err, "Failed to update message")
		return
	}

	logger.LogUserAction(userID.Hex(), "message_updated", "message_handler", map[string]interface{}{
		"message_id": messageID.Hex(),
	})

	utils.Success(c, message)
}

// DeleteMessage handles DELETE /api/messages/:id
func (h *MessageHandler) DeleteMessage(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	messageID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid message ID")
		return
	}

	// Parse delete for everyone flag
	deleteForEveryoneStr := c.DefaultQuery("for_everyone", "false")
	deleteForEveryone, err := strconv.ParseBool(deleteForEveryoneStr)
	if err != nil {
		utils.BadRequest(c, "Invalid for_everyone parameter")
		return
	}

	err = h.messageService.DeleteMessage(messageID, userID, deleteForEveryone)
	if err != nil {
		h.handleMessageServiceError(c, err, "Failed to delete message")
		return
	}

	logger.LogUserAction(userID.Hex(), "message_deleted", "message_handler", map[string]interface{}{
		"message_id":          messageID.Hex(),
		"delete_for_everyone": deleteForEveryone,
	})

	utils.SuccessWithMessage(c, "Message deleted successfully", nil)
}

// AddReaction handles POST /api/messages/:id/reactions
func (h *MessageHandler) AddReaction(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	messageID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid message ID")
		return
	}

	var req services.MessageReactionRequest
	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	// Validate emoji
	if req.Emoji == "" {
		utils.BadRequest(c, "Emoji is required")
		return
	}

	err = h.messageService.AddReaction(messageID, userID, &req)
	if err != nil {
		h.handleMessageServiceError(c, err, "Failed to add reaction")
		return
	}

	logger.LogUserAction(userID.Hex(), "reaction_added", "message_handler", map[string]interface{}{
		"message_id": messageID.Hex(),
		"emoji":      req.Emoji,
	})

	utils.SuccessWithMessage(c, "Reaction added successfully", nil)
}

// RemoveReaction handles DELETE /api/messages/:id/reactions
func (h *MessageHandler) RemoveReaction(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	messageID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid message ID")
		return
	}

	err = h.messageService.RemoveReaction(messageID, userID)
	if err != nil {
		h.handleMessageServiceError(c, err, "Failed to remove reaction")
		return
	}

	logger.LogUserAction(userID.Hex(), "reaction_removed", "message_handler", map[string]interface{}{
		"message_id": messageID.Hex(),
	})

	utils.SuccessWithMessage(c, "Reaction removed successfully", nil)
}

// MarkAsRead handles PUT /api/messages/:id/read
func (h *MessageHandler) MarkAsRead(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	messageID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid message ID")
		return
	}

	err = h.messageService.MarkAsRead(messageID, userID)
	if err != nil {
		h.handleMessageServiceError(c, err, "Failed to mark message as read")
		return
	}

	utils.SuccessWithMessage(c, "Message marked as read", nil)
}

// ForwardMessage handles POST /api/messages/:id/forward
func (h *MessageHandler) ForwardMessage(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	messageID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid message ID")
		return
	}

	var req struct {
		ToChatID string `json:"to_chat_id" validate:"required"`
	}
	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	toChatID, err := primitive.ObjectIDFromHex(req.ToChatID)
	if err != nil {
		utils.BadRequest(c, "Invalid to_chat_id")
		return
	}

	message, err := h.messageService.ForwardMessage(messageID, userID, toChatID)
	if err != nil {
		h.handleMessageServiceError(c, err, "Failed to forward message")
		return
	}

	logger.LogUserAction(userID.Hex(), "message_forwarded", "message_handler", map[string]interface{}{
		"original_message_id": messageID.Hex(),
		"new_message_id":      message.ID.Hex(),
		"to_chat_id":          toChatID.Hex(),
	})

	utils.Created(c, message)
}

// MarkMultipleAsRead handles PUT /api/messages/read
func (h *MessageHandler) MarkMultipleAsRead(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	var req struct {
		MessageIDs []string `json:"message_ids" validate:"required"`
	}
	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	if len(req.MessageIDs) == 0 {
		utils.BadRequest(c, "At least one message ID is required")
		return
	}

	if len(req.MessageIDs) > 100 {
		utils.BadRequest(c, "Cannot mark more than 100 messages at once")
		return
	}

	// Convert string IDs to ObjectIDs
	messageIDs := make([]primitive.ObjectID, len(req.MessageIDs))
	for i, idStr := range req.MessageIDs {
		messageID, err := primitive.ObjectIDFromHex(idStr)
		if err != nil {
			utils.BadRequest(c, "Invalid message ID: "+idStr)
			return
		}
		messageIDs[i] = messageID
	}

	// Mark each message as read
	successCount := 0
	for _, messageID := range messageIDs {
		if err := h.messageService.MarkAsRead(messageID, userID); err != nil {
			logger.Errorf("Failed to mark message %s as read: %v", messageID.Hex(), err)
		} else {
			successCount++
		}
	}

	logger.LogUserAction(userID.Hex(), "messages_bulk_read", "message_handler", map[string]interface{}{
		"total_messages":  len(messageIDs),
		"success_count":   successCount,
		"requested_count": len(req.MessageIDs),
	})

	utils.SuccessWithMessage(c, "Messages marked as read", map[string]interface{}{
		"total":   len(messageIDs),
		"success": successCount,
		"failed":  len(messageIDs) - successCount,
	})
}

// BulkDeleteMessages handles DELETE /api/messages/bulk
func (h *MessageHandler) BulkDeleteMessages(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	var req struct {
		MessageIDs  []string `json:"message_ids" validate:"required"`
		ForEveryone bool     `json:"for_everyone"`
	}
	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	if len(req.MessageIDs) == 0 {
		utils.BadRequest(c, "At least one message ID is required")
		return
	}

	if len(req.MessageIDs) > 50 {
		utils.BadRequest(c, "Cannot delete more than 50 messages at once")
		return
	}

	// Convert string IDs to ObjectIDs
	messageIDs := make([]primitive.ObjectID, len(req.MessageIDs))
	for i, idStr := range req.MessageIDs {
		messageID, err := primitive.ObjectIDFromHex(idStr)
		if err != nil {
			utils.BadRequest(c, "Invalid message ID: "+idStr)
			return
		}
		messageIDs[i] = messageID
	}

	// Delete each message
	successCount := 0
	for _, messageID := range messageIDs {
		if err := h.messageService.DeleteMessage(messageID, userID, req.ForEveryone); err != nil {
			logger.Errorf("Failed to delete message %s: %v", messageID.Hex(), err)
		} else {
			successCount++
		}
	}

	logger.LogUserAction(userID.Hex(), "messages_bulk_deleted", "message_handler", map[string]interface{}{
		"total_messages":      len(messageIDs),
		"success_count":       successCount,
		"delete_for_everyone": req.ForEveryone,
	})

	utils.SuccessWithMessage(c, "Messages deleted", map[string]interface{}{
		"total":   len(messageIDs),
		"success": successCount,
		"failed":  len(messageIDs) - successCount,
	})
}

// SearchMessages handles GET /api/messages/search
func (h *MessageHandler) SearchMessages(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	// Parse search parameters
	chatIDStr := c.Query("chat_id")
	if chatIDStr == "" {
		utils.BadRequest(c, "chat_id is required for search")
		return
	}

	chatID, err := primitive.ObjectIDFromHex(chatIDStr)
	if err != nil {
		utils.BadRequest(c, "Invalid chat_id format")
		return
	}

	search := c.Query("q")
	if search == "" {
		utils.BadRequest(c, "Search query (q) is required")
		return
	}

	// Build search request
	req := services.MessageListRequest{
		ChatID: chatID,
		Search: search,
		Page:   1,
		Limit:  50,
	}

	// Parse optional parameters
	if pageStr := c.Query("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil && page > 0 {
			req.Page = page
		}
	}

	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit <= 100 {
			req.Limit = limit
		}
	}

	if typeStr := c.Query("type"); typeStr != "" {
		req.Type = typeStr
	}

	if senderIDStr := c.Query("sender_id"); senderIDStr != "" {
		if senderID, err := primitive.ObjectIDFromHex(senderIDStr); err == nil {
			req.SenderID = &senderID
		}
	}

	// Get search results
	messages, err := h.messageService.GetMessages(userID, &req)
	if err != nil {
		h.handleMessageServiceError(c, err, "Failed to search messages")
		return
	}

	// Create pagination metadata
	meta := utils.CreatePaginationMeta(req.Page, req.Limit, messages.TotalCount)

	logger.LogUserAction(userID.Hex(), "messages_searched", "message_handler", map[string]interface{}{
		"chat_id":     chatID.Hex(),
		"search_term": search,
		"results":     len(messages.Messages),
	})

	utils.SuccessWithMeta(c, messages.Messages, meta)
}

// GetMessageStatistics handles GET /api/messages/stats (admin only)
func (h *MessageHandler) GetMessageStatistics(c *gin.Context) {
	userRole, err := utils.GetUserRoleFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	if userRole != string(models.RoleAdmin) && userRole != string(models.RoleSuper) {
		utils.Forbidden(c, "Admin access required")
		return
	}

	stats := h.messageService.GetMessageStatistics()
	utils.Success(c, stats)
}

// Validation helper methods

// validateSendMessageRequest validates send message request
func (h *MessageHandler) validateSendMessageRequest(req *services.SendMessageRequest) map[string]utils.ValidationError {
	errors := make(map[string]utils.ValidationError)

	// Validate chat ID
	if req.ChatID.IsZero() {
		errors["chat_id"] = utils.ValidationError{
			Field:   "chat_id",
			Message: "Chat ID is required",
			Code:    utils.ErrRequired,
		}
	}

	// Validate message type
	if req.Type == "" {
		errors["type"] = utils.ValidationError{
			Field:   "type",
			Message: "Message type is required",
			Code:    utils.ErrRequired,
		}
	} else {
		validTypes := []models.MessageType{
			models.MessageTypeText,
			models.MessageTypeImage,
			models.MessageTypeVideo,
			models.MessageTypeAudio,
			models.MessageTypeDocument,
			models.MessageTypeVoiceNote,
			models.MessageTypeLocation,
			models.MessageTypeContact,
			models.MessageTypeSticker,
			models.MessageTypeGIF,
		}

		isValid := false
		for _, validType := range validTypes {
			if req.Type == validType {
				isValid = true
				break
			}
		}

		if !isValid {
			errors["type"] = utils.ValidationError{
				Field:   "type",
				Message: "Invalid message type",
				Code:    utils.ErrInvalidFormat,
			}
		}
	}

	// Validate content based on type
	switch req.Type {
	case models.MessageTypeText:
		if req.Content == "" {
			errors["content"] = utils.ValidationError{
				Field:   "content",
				Message: "Content is required for text messages",
				Code:    utils.ErrRequired,
			}
		} else if err := utils.ValidateMessageContent(req.Content, 4096); err != nil {
			errors["content"] = *err
		}

	case models.MessageTypeImage, models.MessageTypeVideo, models.MessageTypeAudio, models.MessageTypeDocument:
		if req.MediaURL == "" {
			errors["media_url"] = utils.ValidationError{
				Field:   "media_url",
				Message: "Media URL is required for media messages",
				Code:    utils.ErrRequired,
			}
		} else if err := utils.ValidateURL(req.MediaURL); err != nil {
			errors["media_url"] = *err
		}

	case models.MessageTypeLocation:
		if req.Metadata.Location == nil {
			errors["metadata.location"] = utils.ValidationError{
				Field:   "metadata.location",
				Message: "Location data is required for location messages",
				Code:    utils.ErrRequired,
			}
		} else {
			// Validate latitude and longitude
			if req.Metadata.Location.Latitude < -90 || req.Metadata.Location.Latitude > 90 {
				errors["metadata.location.latitude"] = utils.ValidationError{
					Field:   "metadata.location.latitude",
					Message: "Latitude must be between -90 and 90",
					Code:    utils.ErrOutOfRange,
				}
			}
			if req.Metadata.Location.Longitude < -180 || req.Metadata.Location.Longitude > 180 {
				errors["metadata.location.longitude"] = utils.ValidationError{
					Field:   "metadata.location.longitude",
					Message: "Longitude must be between -180 and 180",
					Code:    utils.ErrOutOfRange,
				}
			}
		}

	case models.MessageTypeContact:
		if req.Metadata.Contact == nil {
			errors["metadata.contact"] = utils.ValidationError{
				Field:   "metadata.contact",
				Message: "Contact data is required for contact messages",
				Code:    utils.ErrRequired,
			}
		} else {
			if req.Metadata.Contact.Name == "" {
				errors["metadata.contact.name"] = utils.ValidationError{
					Field:   "metadata.contact.name",
					Message: "Contact name is required",
					Code:    utils.ErrRequired,
				}
			}
			if req.Metadata.Contact.PhoneNumber == "" {
				errors["metadata.contact.phone_number"] = utils.ValidationError{
					Field:   "metadata.contact.phone_number",
					Message: "Contact phone number is required",
					Code:    utils.ErrRequired,
				}
			}
		}
	}

	// Validate reply_to_id if provided
	if req.ReplyToID != nil {
		if req.ReplyToID.IsZero() {
			errors["reply_to_id"] = utils.ValidationError{
				Field:   "reply_to_id",
				Message: "Invalid reply_to_id",
				Code:    utils.ErrInvalidObjectID,
			}
		}
	}

	// Validate mentions if provided
	if len(req.Mentions) > 10 {
		errors["mentions"] = utils.ValidationError{
			Field:   "mentions",
			Message: "Cannot mention more than 10 users",
			Code:    utils.ErrTooLarge,
		}
	}

	// Validate scheduled_at if provided
	if req.ScheduledAt != nil {
		if req.ScheduledAt.Before(time.Now()) {
			errors["scheduled_at"] = utils.ValidationError{
				Field:   "scheduled_at",
				Message: "Scheduled time must be in the future",
				Code:    utils.ErrInvalidDate,
			}
		}
		// Don't allow scheduling too far in the future (1 year)
		if req.ScheduledAt.After(time.Now().Add(365 * 24 * time.Hour)) {
			errors["scheduled_at"] = utils.ValidationError{
				Field:   "scheduled_at",
				Message: "Cannot schedule messages more than 1 year in the future",
				Code:    utils.ErrInvalidDate,
			}
		}
	}

	return errors
}

// validateUpdateMessageRequest validates update message request
func (h *MessageHandler) validateUpdateMessageRequest(req *services.MessageUpdateRequest) map[string]utils.ValidationError {
	errors := make(map[string]utils.ValidationError)

	// Validate content
	if req.Content == "" {
		errors["content"] = utils.ValidationError{
			Field:   "content",
			Message: "Content is required",
			Code:    utils.ErrRequired,
		}
	} else if err := utils.ValidateMessageContent(req.Content, 4096); err != nil {
		errors["content"] = *err
	}

	// Validate reason length if provided
	if len(req.Reason) > 200 {
		errors["reason"] = utils.ValidationError{
			Field:   "reason",
			Message: "Reason must be no more than 200 characters",
			Code:    utils.ErrTooLong,
		}
	}

	return errors
}

// Error handling

// handleMessageServiceError handles errors from message service
func (h *MessageHandler) handleMessageServiceError(c *gin.Context, err error, defaultMessage string) {
	errStr := err.Error()

	switch {
	case errStr == "message not found":
		utils.NotFound(c, "Message not found")
	case errStr == "chat not found":
		utils.NotFound(c, "Chat not found")
	case errStr == "user is not a participant of this chat":
		utils.Forbidden(c, "Access denied")
	case errStr == "user cannot send messages in this chat":
		utils.Forbidden(c, "Cannot send messages in this chat")
	case errStr == "message cannot be edited":
		utils.Forbidden(c, "Message cannot be edited")
	case errStr == "message cannot be deleted":
		utils.Forbidden(c, "Message cannot be deleted")
	case errStr == "message cannot be deleted for everyone":
		utils.Forbidden(c, "Message cannot be deleted for everyone")
	case errStr == "mentioned user is not a participant":
		utils.BadRequest(c, "Cannot mention users who are not participants")
	case errStr == "invalid mentions":
		utils.BadRequest(c, "Invalid mentions")
	case errStr == "user is already a participant":
		utils.Conflict(c, "User is already a participant")
	default:
		logger.Errorf("Message service error: %v", err)
		utils.InternalServerError(c, defaultMessage)
	}
}

// Helper methods

// getMessageIDFromParam extracts and validates message ID from URL parameter
func (h *MessageHandler) getMessageIDFromParam(c *gin.Context) (primitive.ObjectID, error) {
	messageIDStr := c.Param("id")
	if messageIDStr == "" {
		return primitive.NilObjectID, fmt.Errorf("message ID is required")
	}

	messageID, err := primitive.ObjectIDFromHex(messageIDStr)
	if err != nil {
		return primitive.NilObjectID, fmt.Errorf("invalid message ID format")
	}

	return messageID, nil
}

// getChatIDFromQuery extracts and validates chat ID from query parameter
func (h *MessageHandler) getChatIDFromQuery(c *gin.Context) (primitive.ObjectID, error) {
	chatIDStr := c.Query("chat_id")
	if chatIDStr == "" {
		return primitive.NilObjectID, fmt.Errorf("chat_id is required")
	}

	chatID, err := primitive.ObjectIDFromHex(chatIDStr)
	if err != nil {
		return primitive.NilObjectID, fmt.Errorf("invalid chat_id format")
	}

	return chatID, nil
}
