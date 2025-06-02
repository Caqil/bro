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

// ChatHandler handles chat-related HTTP requests
type ChatHandler struct {
	chatService *services.ChatService
}

// NewChatHandler creates a new chat handler
func NewChatHandler(chatService *services.ChatService) *ChatHandler {
	return &ChatHandler{
		chatService: chatService,
	}
}

// CreateChatRequest represents the request body for creating a chat
type CreateChatRequest struct {
	Type         models.ChatType      `json:"type" validate:"required"`
	Participants []string             `json:"participants" validate:"required"`
	Name         string               `json:"name,omitempty"`
	Description  string               `json:"description,omitempty"`
	Avatar       string               `json:"avatar,omitempty"`
	Settings     *models.ChatSettings `json:"settings,omitempty"`
}

// UpdateChatRequest represents the request body for updating a chat
type UpdateChatRequest struct {
	Name        *string              `json:"name,omitempty"`
	Description *string              `json:"description,omitempty"`
	Avatar      *string              `json:"avatar,omitempty"`
	Settings    *models.ChatSettings `json:"settings,omitempty"`
}

// AddParticipantRequest represents the request body for adding a participant
type AddParticipantRequest struct {
	UserID string `json:"user_id" validate:"required"`
}

// MuteChatRequest represents the request body for muting a chat
type MuteChatRequest struct {
	Mute       bool       `json:"mute"`
	MutedUntil *time.Time `json:"muted_until,omitempty"`
}

// SetDraftRequest represents the request body for setting a draft
type SetDraftRequest struct {
	Content string             `json:"content"`
	Type    models.MessageType `json:"type,omitempty"`
}

// RegisterRoutes registers chat routes
func (h *ChatHandler) RegisterRoutes(router *gin.RouterGroup, authMiddleware gin.HandlerFunc) {
	chats := router.Group("/chats")
	chats.Use(authMiddleware)

	// Chat management
	chats.POST("", h.CreateChat)
	chats.GET("", h.GetUserChats)
	chats.GET("/:id", h.GetChat)
	chats.PUT("/:id", h.UpdateChat)
	chats.DELETE("/:id", h.DeleteChat)

	// Participant management
	chats.POST("/:id/participants", h.AddParticipant)
	chats.DELETE("/:id/participants/:user_id", h.RemoveParticipant)

	// Chat state management
	chats.PUT("/:id/archive", h.ArchiveChat)
	chats.PUT("/:id/mute", h.MuteChat)
	chats.PUT("/:id/pin", h.PinChat)
	chats.PUT("/:id/read", h.MarkAsRead)

	// Draft management
	chats.PUT("/:id/draft", h.SetDraft)
	chats.GET("/:id/draft", h.GetDraft)
	chats.DELETE("/:id/draft", h.ClearDraft)

	// Statistics (admin only)
	chats.GET("/stats", h.GetChatStatistics)
}

// CreateChat handles POST /api/chats
func (h *ChatHandler) CreateChat(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	var req CreateChatRequest
	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	// Validate request
	validationErrors := h.validateCreateChatRequest(&req)
	if len(validationErrors) > 0 {
		utils.ValidationErrorResponse(c, validationErrors)
		return
	}

	// Convert participant IDs
	participantIDs := make([]primitive.ObjectID, len(req.Participants))
	for i, participantStr := range req.Participants {
		participantID, err := primitive.ObjectIDFromHex(participantStr)
		if err != nil {
			utils.BadRequest(c, "Invalid participant ID: "+participantStr)
			return
		}
		participantIDs[i] = participantID
	}

	// Create service request
	serviceReq := &services.ChatRequest{
		Type:         req.Type,
		Participants: participantIDs,
		Name:         req.Name,
		Description:  req.Description,
		Avatar:       req.Avatar,
		Settings:     req.Settings,
	}

	// Create chat
	chat, err := h.chatService.CreateChat(userID, serviceReq)
	if err != nil {
		logger.Errorf("Failed to create chat: %v", err)
		utils.InternalServerError(c, "Failed to create chat")
		return
	}

	logger.LogUserAction(userID.Hex(), "chat_created", "chat_handler", map[string]interface{}{
		"chat_id":      chat.ID.Hex(),
		"chat_type":    chat.Type,
		"participants": len(participantIDs),
	})

	utils.Created(c, chat)
}

// GetUserChats handles GET /api/chats
func (h *ChatHandler) GetUserChats(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	// Parse query parameters
	var req services.ChatListRequest
	if err := utils.ParseQuery(c, &req); err != nil {
		utils.BadRequest(c, "Invalid query parameters")
		return
	}

	// Set default values
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 || req.Limit > 50 {
		req.Limit = 20
	}

	// Get chats
	chats, err := h.chatService.GetUserChats(userID, &req)
	if err != nil {
		logger.Errorf("Failed to get user chats: %v", err)
		utils.InternalServerError(c, "Failed to get chats")
		return
	}

	// Create pagination metadata
	meta := utils.CreatePaginationMeta(req.Page, req.Limit, chats.TotalCount)

	utils.SuccessWithMeta(c, chats.Chats, meta)
}

// GetChat handles GET /api/chats/:id
func (h *ChatHandler) GetChat(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	chatID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	chat, err := h.chatService.GetChat(chatID, userID)
	if err != nil {
		if err.Error() == "chat not found" {
			utils.NotFound(c, "Chat not found")
			return
		}
		if err.Error() == "user is not a participant of this chat" {
			utils.Forbidden(c, "Access denied")
			return
		}
		logger.Errorf("Failed to get chat: %v", err)
		utils.InternalServerError(c, "Failed to get chat")
		return
	}

	utils.Success(c, chat)
}

// UpdateChat handles PUT /api/chats/:id
func (h *ChatHandler) UpdateChat(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	chatID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	var req UpdateChatRequest
	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	// Validate request
	validationErrors := h.validateUpdateChatRequest(&req)
	if len(validationErrors) > 0 {
		utils.ValidationErrorResponse(c, validationErrors)
		return
	}

	// Create service request
	serviceReq := &services.ChatUpdateRequest{
		Name:        req.Name,
		Description: req.Description,
		Avatar:      req.Avatar,
		Settings:    req.Settings,
	}

	chat, err := h.chatService.UpdateChat(chatID, userID, serviceReq)
	if err != nil {
		if err.Error() == "chat not found" {
			utils.NotFound(c, "Chat not found")
			return
		}
		if err.Error() == "insufficient permissions to edit chat" {
			utils.Forbidden(c, "Insufficient permissions")
			return
		}
		logger.Errorf("Failed to update chat: %v", err)
		utils.InternalServerError(c, "Failed to update chat")
		return
	}

	logger.LogUserAction(userID.Hex(), "chat_updated", "chat_handler", map[string]interface{}{
		"chat_id": chatID.Hex(),
	})

	utils.Success(c, chat)
}

// AddParticipant handles POST /api/chats/:id/participants
func (h *ChatHandler) AddParticipant(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	chatID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	var req AddParticipantRequest
	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	// Validate participant ID
	participantID, err := primitive.ObjectIDFromHex(req.UserID)
	if err != nil {
		utils.BadRequest(c, "Invalid user ID")
		return
	}

	err = h.chatService.AddParticipant(chatID, userID, participantID)
	if err != nil {
		if err.Error() == "chat not found" {
			utils.NotFound(c, "Chat not found")
			return
		}
		if err.Error() == "insufficient permissions to add members" {
			utils.Forbidden(c, "Insufficient permissions")
			return
		}
		if err.Error() == "user is already a participant" {
			utils.Conflict(c, "User is already a participant")
			return
		}
		logger.Errorf("Failed to add participant: %v", err)
		utils.InternalServerError(c, "Failed to add participant")
		return
	}

	logger.LogUserAction(userID.Hex(), "participant_added", "chat_handler", map[string]interface{}{
		"chat_id":        chatID.Hex(),
		"participant_id": participantID.Hex(),
	})

	utils.SuccessWithMessage(c, "Participant added successfully", nil)
}

// RemoveParticipant handles DELETE /api/chats/:id/participants/:user_id
func (h *ChatHandler) RemoveParticipant(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	chatID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	participantID, err := primitive.ObjectIDFromHex(c.Param("user_id"))
	if err != nil {
		utils.BadRequest(c, "Invalid user ID")
		return
	}

	err = h.chatService.RemoveParticipant(chatID, userID, participantID)
	if err != nil {
		if err.Error() == "chat not found" {
			utils.NotFound(c, "Chat not found")
			return
		}
		if err.Error() == "insufficient permissions to remove members" {
			utils.Forbidden(c, "Insufficient permissions")
			return
		}
		if err.Error() == "user is not a participant" {
			utils.NotFound(c, "User is not a participant")
			return
		}
		if err.Error() == "cannot remove participants from private chat" {
			utils.BadRequest(c, "Cannot remove participants from private chat")
			return
		}
		logger.Errorf("Failed to remove participant: %v", err)
		utils.InternalServerError(c, "Failed to remove participant")
		return
	}

	logger.LogUserAction(userID.Hex(), "participant_removed", "chat_handler", map[string]interface{}{
		"chat_id":        chatID.Hex(),
		"participant_id": participantID.Hex(),
	})

	utils.SuccessWithMessage(c, "Participant removed successfully", nil)
}

// ArchiveChat handles PUT /api/chats/:id/archive
func (h *ChatHandler) ArchiveChat(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	chatID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	// Parse archive flag from query parameter
	archiveStr := c.DefaultQuery("archive", "true")
	archive, err := strconv.ParseBool(archiveStr)
	if err != nil {
		utils.BadRequest(c, "Invalid archive parameter")
		return
	}

	err = h.chatService.ArchiveChat(chatID, userID, archive)
	if err != nil {
		if err.Error() == "chat not found" {
			utils.NotFound(c, "Chat not found")
			return
		}
		if err.Error() == "user is not a participant of this chat" {
			utils.Forbidden(c, "Access denied")
			return
		}
		logger.Errorf("Failed to archive chat: %v", err)
		utils.InternalServerError(c, "Failed to update archive status")
		return
	}

	message := "Chat archived successfully"
	if !archive {
		message = "Chat unarchived successfully"
	}

	logger.LogUserAction(userID.Hex(), "chat_archive_toggled", "chat_handler", map[string]interface{}{
		"chat_id":  chatID.Hex(),
		"archived": archive,
	})

	utils.SuccessWithMessage(c, message, nil)
}

// MuteChat handles PUT /api/chats/:id/mute
func (h *ChatHandler) MuteChat(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	chatID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	var req MuteChatRequest
	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	err = h.chatService.MuteChat(chatID, userID, req.Mute, req.MutedUntil)
	if err != nil {
		if err.Error() == "chat not found" {
			utils.NotFound(c, "Chat not found")
			return
		}
		if err.Error() == "user is not a participant of this chat" {
			utils.Forbidden(c, "Access denied")
			return
		}
		logger.Errorf("Failed to mute chat: %v", err)
		utils.InternalServerError(c, "Failed to update mute status")
		return
	}

	message := "Chat muted successfully"
	if !req.Mute {
		message = "Chat unmuted successfully"
	}

	logger.LogUserAction(userID.Hex(), "chat_mute_toggled", "chat_handler", map[string]interface{}{
		"chat_id":     chatID.Hex(),
		"muted":       req.Mute,
		"muted_until": req.MutedUntil,
	})

	utils.SuccessWithMessage(c, message, nil)
}

// PinChat handles PUT /api/chats/:id/pin
func (h *ChatHandler) PinChat(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	chatID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	// Parse pin flag from query parameter
	pinStr := c.DefaultQuery("pin", "true")
	pin, err := strconv.ParseBool(pinStr)
	if err != nil {
		utils.BadRequest(c, "Invalid pin parameter")
		return
	}

	err = h.chatService.PinChat(chatID, userID, pin)
	if err != nil {
		if err.Error() == "chat not found" {
			utils.NotFound(c, "Chat not found")
			return
		}
		if err.Error() == "user is not a participant of this chat" {
			utils.Forbidden(c, "Access denied")
			return
		}
		logger.Errorf("Failed to pin chat: %v", err)
		utils.InternalServerError(c, "Failed to update pin status")
		return
	}

	message := "Chat pinned successfully"
	if !pin {
		message = "Chat unpinned successfully"
	}

	logger.LogUserAction(userID.Hex(), "chat_pin_toggled", "chat_handler", map[string]interface{}{
		"chat_id": chatID.Hex(),
		"pinned":  pin,
	})

	utils.SuccessWithMessage(c, message, nil)
}

// MarkAsRead handles PUT /api/chats/:id/read
func (h *ChatHandler) MarkAsRead(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	chatID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	err = h.chatService.MarkAsRead(chatID, userID)
	if err != nil {
		if err.Error() == "chat not found" {
			utils.NotFound(c, "Chat not found")
			return
		}
		if err.Error() == "user is not a participant of this chat" {
			utils.Forbidden(c, "Access denied")
			return
		}
		logger.Errorf("Failed to mark chat as read: %v", err)
		utils.InternalServerError(c, "Failed to mark as read")
		return
	}

	utils.SuccessWithMessage(c, "Chat marked as read", nil)
}

// SetDraft handles PUT /api/chats/:id/draft
func (h *ChatHandler) SetDraft(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	chatID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	var req SetDraftRequest
	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	// Default message type
	if req.Type == "" {
		req.Type = models.MessageTypeText
	}

	err = h.chatService.SetDraft(chatID, userID, req.Content, req.Type)
	if err != nil {
		if err.Error() == "chat not found" {
			utils.NotFound(c, "Chat not found")
			return
		}
		if err.Error() == "user is not a participant of this chat" {
			utils.Forbidden(c, "Access denied")
			return
		}
		logger.Errorf("Failed to set draft: %v", err)
		utils.InternalServerError(c, "Failed to set draft")
		return
	}

	utils.SuccessWithMessage(c, "Draft saved successfully", nil)
}

// GetDraft handles GET /api/chats/:id/draft
func (h *ChatHandler) GetDraft(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	chatID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	draft, err := h.chatService.GetDraft(chatID, userID)
	if err != nil {
		logger.Errorf("Failed to get draft: %v", err)
		utils.InternalServerError(c, "Failed to get draft")
		return
	}

	utils.Success(c, draft)
}

// ClearDraft handles DELETE /api/chats/:id/draft
func (h *ChatHandler) ClearDraft(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	chatID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	err = h.chatService.SetDraft(chatID, userID, "", models.MessageTypeText)
	if err != nil {
		logger.Errorf("Failed to clear draft: %v", err)
		utils.InternalServerError(c, "Failed to clear draft")
		return
	}

	utils.SuccessWithMessage(c, "Draft cleared successfully", nil)
}

// DeleteChat handles DELETE /api/chats/:id
func (h *ChatHandler) DeleteChat(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	chatID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	err = h.chatService.DeleteChat(chatID, userID)
	if err != nil {
		if err.Error() == "chat not found" {
			utils.NotFound(c, "Chat not found")
			return
		}
		if err.Error() == "insufficient permissions to delete chat" {
			utils.Forbidden(c, "Insufficient permissions")
			return
		}
		logger.Errorf("Failed to delete chat: %v", err)
		utils.InternalServerError(c, "Failed to delete chat")
		return
	}

	logger.LogUserAction(userID.Hex(), "chat_deleted", "chat_handler", map[string]interface{}{
		"chat_id": chatID.Hex(),
	})

	utils.SuccessWithMessage(c, "Chat deleted successfully", nil)
}

// GetChatStatistics handles GET /api/chats/stats (admin only)
func (h *ChatHandler) GetChatStatistics(c *gin.Context) {
	userRole, err := utils.GetUserRoleFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	if userRole != string(models.RoleAdmin) && userRole != string(models.RoleSuper) {
		utils.Forbidden(c, "Admin access required")
		return
	}

	stats := h.chatService.GetChatStatistics()
	utils.Success(c, stats)
}

// Validation helper methods

// validateCreateChatRequest validates create chat request
func (h *ChatHandler) validateCreateChatRequest(req *CreateChatRequest) map[string]utils.ValidationError {
	errors := make(map[string]utils.ValidationError)

	// Validate chat type
	if req.Type == "" {
		errors["type"] = utils.ValidationError{
			Field:   "type",
			Message: "Chat type is required",
			Code:    utils.ErrRequired,
		}
	} else {
		validTypes := []models.ChatType{
			models.ChatTypePrivate,
			models.ChatTypeGroup,
			models.ChatTypeBroadcast,
			models.ChatTypeBot,
			models.ChatTypeSupport,
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
				Message: "Invalid chat type",
				Code:    utils.ErrInvalidFormat,
			}
		}
	}

	// Validate participants
	if len(req.Participants) == 0 {
		errors["participants"] = utils.ValidationError{
			Field:   "participants",
			Message: "At least one participant is required",
			Code:    utils.ErrRequired,
		}
	} else {
		// Validate participant IDs
		for i, participantStr := range req.Participants {
			if !primitive.IsValidObjectID(participantStr) {
				errors[fmt.Sprintf("participants[%d]", i)] = utils.ValidationError{
					Field:   fmt.Sprintf("participants[%d]", i),
					Message: "Invalid participant ID",
					Code:    utils.ErrInvalidObjectID,
				}
			}
		}

		// Validate participant count based on type
		switch req.Type {
		case models.ChatTypePrivate:
			if len(req.Participants) != 1 {
				errors["participants"] = utils.ValidationError{
					Field:   "participants",
					Message: "Private chat must have exactly 1 other participant",
					Code:    utils.ErrInvalidFormat,
				}
			}
		case models.ChatTypeGroup:
			if len(req.Participants) < 2 {
				errors["participants"] = utils.ValidationError{
					Field:   "participants",
					Message: "Group chat must have at least 2 participants",
					Code:    utils.ErrTooSmall,
				}
			}
			if len(req.Participants) > 256 {
				errors["participants"] = utils.ValidationError{
					Field:   "participants",
					Message: "Group chat cannot have more than 256 participants",
					Code:    utils.ErrTooLarge,
				}
			}
		}
	}

	// Validate name for group chats
	if req.Type == models.ChatTypeGroup || req.Type == models.ChatTypeBroadcast {
		if err := utils.ValidateGroupName(req.Name); err != nil {
			errors["name"] = *err
		}
	}

	// Validate description length
	if len(req.Description) > 500 {
		errors["description"] = utils.ValidationError{
			Field:   "description",
			Message: "Description must be no more than 500 characters",
			Code:    utils.ErrTooLong,
		}
	}

	// Validate avatar URL if provided
	if req.Avatar != "" {
		if err := utils.ValidateURL(req.Avatar); err != nil {
			errors["avatar"] = *err
		}
	}

	return errors
}

// validateUpdateChatRequest validates update chat request
func (h *ChatHandler) validateUpdateChatRequest(req *UpdateChatRequest) map[string]utils.ValidationError {
	errors := make(map[string]utils.ValidationError)

	// Validate name if provided
	if req.Name != nil {
		if err := utils.ValidateGroupName(*req.Name); err != nil {
			errors["name"] = *err
		}
	}

	// Validate description length if provided
	if req.Description != nil && len(*req.Description) > 500 {
		errors["description"] = utils.ValidationError{
			Field:   "description",
			Message: "Description must be no more than 500 characters",
			Code:    utils.ErrTooLong,
		}
	}

	// Validate avatar URL if provided
	if req.Avatar != nil && *req.Avatar != "" {
		if err := utils.ValidateURL(*req.Avatar); err != nil {
			errors["avatar"] = *err
		}
	}

	return errors
}

// Helper methods

// getChatIDFromParam extracts and validates chat ID from URL parameter
func (h *ChatHandler) getChatIDFromParam(c *gin.Context) (primitive.ObjectID, error) {
	chatIDStr := c.Param("id")
	if chatIDStr == "" {
		return primitive.NilObjectID, fmt.Errorf("chat ID is required")
	}

	chatID, err := primitive.ObjectIDFromHex(chatIDStr)
	if err != nil {
		return primitive.NilObjectID, fmt.Errorf("invalid chat ID format")
	}

	return chatID, nil
}

// getUserIDFromParam extracts and validates user ID from URL parameter
func (h *ChatHandler) getUserIDFromParam(c *gin.Context, paramName string) (primitive.ObjectID, error) {
	userIDStr := c.Param(paramName)
	if userIDStr == "" {
		return primitive.NilObjectID, fmt.Errorf("user ID is required")
	}

	userID, err := primitive.ObjectIDFromHex(userIDStr)
	if err != nil {
		return primitive.NilObjectID, fmt.Errorf("invalid user ID format")
	}

	return userID, nil
}
