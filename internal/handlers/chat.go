package handlers

import (
	"context"
	"fmt"
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

type ChatHandler struct {
	pushService *services.PushService
	redisClient *redis.Client
}

func NewChatHandler(pushService *services.PushService) *ChatHandler {
	return &ChatHandler{
		pushService: pushService,
		redisClient: redis.GetClient(),
	}
}

// RegisterRoutes registers chat routes
func (h *ChatHandler) RegisterRoutes(router *gin.RouterGroup) {
	chats := router.Group("/chats")
	chats.Use(middleware.AuthMiddleware("your-jwt-secret"))
	{
		// Chat management
		chats.GET("", h.GetChats)
		chats.POST("", h.CreateChat)
		chats.GET("/:chatId", h.GetChat)
		chats.PUT("/:chatId", h.UpdateChat)
		chats.DELETE("/:chatId", h.DeleteChat)
		chats.POST("/:chatId/leave", h.LeaveChat)

		// Chat participants
		chats.GET("/:chatId/participants", h.GetChatParticipants)
		chats.POST("/:chatId/participants", h.AddParticipant)
		chats.DELETE("/:chatId/participants/:userId", h.RemoveParticipant)

		// Chat actions
		chats.POST("/:chatId/archive", h.ArchiveChat)
		chats.POST("/:chatId/unarchive", h.UnarchiveChat)
		chats.POST("/:chatId/mute", h.MuteChat)
		chats.POST("/:chatId/unmute", h.UnmuteChat)
		chats.POST("/:chatId/pin", h.PinChat)
		chats.POST("/:chatId/unpin", h.UnpinChat)
		chats.POST("/:chatId/block", h.BlockChat)
		chats.POST("/:chatId/unblock", h.UnblockChat)
		chats.POST("/:chatId/mark-read", h.MarkChatAsRead)

		// Chat settings and preferences
		chats.GET("/:chatId/settings", h.GetChatSettings)
		chats.PUT("/:chatId/settings", h.UpdateChatSettings)
		chats.POST("/:chatId/clear-history", h.ClearChatHistory)

		// Draft messages
		chats.GET("/:chatId/draft", h.GetDraft)
		chats.PUT("/:chatId/draft", h.SetDraft)
		chats.DELETE("/:chatId/draft", h.ClearDraft)

		// Search and discovery
		chats.GET("/search", h.SearchChats)
		chats.GET("/archived", h.GetArchivedChats)
		chats.GET("/muted", h.GetMutedChats)
		chats.GET("/pinned", h.GetPinnedChats)

		// Chat info and statistics
		chats.GET("/:chatId/info", h.GetChatInfo)
		chats.GET("/:chatId/media", h.GetChatMedia)
		chats.GET("/:chatId/files", h.GetChatFiles)
		chats.GET("/:chatId/links", h.GetChatLinks)

		// Export and backup
		chats.POST("/:chatId/export", h.ExportChat)
		chats.POST("/:chatId/backup", h.BackupChat)

		// Reports and moderation
		chats.POST("/:chatId/report", h.ReportChat)
	}
}

// GetChats retrieves user's chats with pagination and filters
func (h *ChatHandler) GetChats(c *gin.Context) {
	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	// Parse query parameters
	pagination := utils.GetPaginationParams(c)
	chatType := c.Query("type")      // "private", "group", "broadcast"
	archived := c.Query("archived")  // "true" or "false"
	lastActivity := c.Query("since") // timestamp

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build filter
	filter := bson.M{
		"participants": user.ID,
		"is_active":    true,
	}

	// Add type filter
	if chatType != "" {
		filter["type"] = chatType
	}

	// Add archived filter
	isArchived := archived == "true"
	if isArchived {
		filter["is_archived"] = bson.M{
			"$elemMatch": bson.M{"user_id": user.ID},
		}
	} else {
		filter["is_archived"] = bson.M{
			"$not": bson.M{
				"$elemMatch": bson.M{"user_id": user.ID},
			},
		}
	}

	// Add last activity filter
	if lastActivity != "" {
		if timestamp, err := strconv.ParseInt(lastActivity, 10, 64); err == nil {
			filter["last_activity"] = bson.M{
				"$gte": time.Unix(timestamp, 0),
			}
		}
	}

	// Get total count
	total, err := collections.Chats.CountDocuments(ctx, filter)
	if err != nil {
		logger.Errorf("Failed to count chats: %v", err)
		utils.InternalServerError(c, "Failed to get chats")
		return
	}

	// Build sort order
	sortField := "last_activity"
	if pagination.SortBy == "created" {
		sortField = "created_at"
	} else if pagination.SortBy == "name" {
		sortField = "name"
	}

	sortOrder := -1 // descending
	if pagination.SortDir == "asc" {
		sortOrder = 1
	}

	// Get chats
	opts := options.Find().
		SetSort(bson.D{{Key: sortField, Value: sortOrder}}).
		SetLimit(int64(pagination.Limit)).
		SetSkip(int64((pagination.Page - 1) * pagination.Limit))

	cursor, err := collections.Chats.Find(ctx, filter, opts)
	if err != nil {
		logger.Errorf("Failed to find chats: %v", err)
		utils.InternalServerError(c, "Failed to get chats")
		return
	}
	defer cursor.Close(ctx)

	var chats []models.Chat
	if err := cursor.All(ctx, &chats); err != nil {
		logger.Errorf("Failed to decode chats: %v", err)
		utils.InternalServerError(c, "Failed to get chats")
		return
	}

	// Convert to response format
	chatResponses := make([]models.ChatResponse, len(chats))
	for i, chat := range chats {
		chatResponse := h.buildChatResponse(&chat, user.ID)
		chatResponses[i] = chatResponse
	}

	// Create pagination metadata
	meta := utils.CreatePaginationMeta(pagination.Page, pagination.Limit, total)

	utils.SuccessWithMeta(c, chatResponses, meta)
}

// CreateChat creates a new chat
func (h *ChatHandler) CreateChat(c *gin.Context) {
	var req models.CreateChatRequest

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

	if len(req.Participants) == 0 {
		validationErrors["participants"] = utils.ValidationError{
			Field:   "participants",
			Message: "At least one participant is required",
			Code:    utils.ErrRequired,
		}
	}

	if req.Type == "" {
		validationErrors["type"] = utils.ValidationError{
			Field:   "type",
			Message: "Chat type is required",
			Code:    utils.ErrRequired,
		}
	}

	// Validate group chat requirements
	if req.Type == models.ChatTypeGroup {
		if req.Name == "" {
			validationErrors["name"] = utils.ValidationError{
				Field:   "name",
				Message: "Group name is required",
				Code:    utils.ErrRequired,
			}
		} else if err := utils.ValidateGroupName(req.Name); err != nil {
			validationErrors["name"] = *err
		}

		if len(req.Participants) < 2 {
			validationErrors["participants"] = utils.ValidationError{
				Field:   "participants",
				Message: "Group chat requires at least 2 participants",
				Code:    utils.ErrTooSmall,
			}
		}
	}

	// For private chats, ensure only one participant (excluding creator)
	if req.Type == models.ChatTypePrivate && len(req.Participants) != 1 {
		validationErrors["participants"] = utils.ValidationError{
			Field:   "participants",
			Message: "Private chat must have exactly one participant",
			Code:    utils.ErrInvalidFormat,
		}
	}

	if len(validationErrors) > 0 {
		utils.ValidationErrorResponse(c, validationErrors)
		return
	}

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Check if private chat already exists
	if req.Type == models.ChatTypePrivate {
		existingChatFilter := bson.M{
			"type": models.ChatTypePrivate,
			"participants": bson.M{
				"$all":  []primitive.ObjectID{user.ID, req.Participants[0]},
				"$size": 2,
			},
			"is_active": true,
		}

		var existingChat models.Chat
		err := collections.Chats.FindOne(ctx, existingChatFilter).Decode(&existingChat)
		if err == nil {
			// Chat already exists
			chatResponse := h.buildChatResponse(&existingChat, user.ID)
			utils.Success(c, chatResponse)
			return
		}
	}

	// Verify all participants exist and are active
	participantFilter := bson.M{
		"_id":        bson.M{"$in": req.Participants},
		"is_active":  true,
		"is_deleted": false,
	}

	participantCount, err := collections.Users.CountDocuments(ctx, participantFilter)
	if err != nil {
		logger.Errorf("Failed to verify participants: %v", err)
		utils.InternalServerError(c, "Failed to create chat")
		return
	}

	if participantCount != int64(len(req.Participants)) {
		utils.BadRequest(c, "One or more participants not found or inactive")
		return
	}

	// Create chat
	chat := &models.Chat{
		Type:         req.Type,
		Participants: append(req.Participants, user.ID), // Add creator to participants
		CreatedBy:    user.ID,
		Name:         strings.TrimSpace(req.Name),
		Description:  strings.TrimSpace(req.Description),
	}

	// Set default settings if provided
	if req.Settings != nil {
		chat.Settings = *req.Settings
	}

	chat.BeforeCreate()

	// Insert chat
	result, err := collections.Chats.InsertOne(ctx, chat)
	if err != nil {
		logger.Errorf("Failed to create chat: %v", err)
		utils.InternalServerError(c, "Failed to create chat")
		return
	}

	chat.ID = result.InsertedID.(primitive.ObjectID)

	// Create welcome system message for group chats
	if chat.Type == models.ChatTypeGroup {
		go h.createWelcomeMessage(chat, user)
	}

	// Send notifications to participants (except creator)
	go h.notifyParticipantsAboutNewChat(chat, user)

	// Log chat creation
	logger.LogUserAction(user.ID.Hex(), "chat_created", "chat", map[string]interface{}{
		"chat_id":           chat.ID.Hex(),
		"chat_type":         string(chat.Type),
		"participant_count": len(chat.Participants),
	})

	// Return chat response
	chatResponse := h.buildChatResponse(chat, user.ID)
	utils.Created(c, chatResponse)
}

// GetChat retrieves a specific chat
func (h *ChatHandler) GetChat(c *gin.Context) {
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

	chat, err := h.getChatByID(chatID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found")
		} else {
			utils.InternalServerError(c, "Failed to get chat")
		}
		return
	}

	// Check if user is participant
	if !chat.IsParticipant(user.ID) {
		utils.Forbidden(c, "You are not a participant in this chat")
		return
	}

	chatResponse := h.buildChatResponse(chat, user.ID)
	utils.Success(c, chatResponse)
}

// UpdateChat updates chat information
func (h *ChatHandler) UpdateChat(c *gin.Context) {
	chatID, err := primitive.ObjectIDFromHex(c.Param("chatId"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	var req models.UpdateChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	chat, err := h.getChatByID(chatID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found")
		} else {
			utils.InternalServerError(c, "Failed to get chat")
		}
		return
	}

	// Check permissions
	if !h.canEditChatInfo(chat, user.ID) {
		utils.Forbidden(c, "You don't have permission to edit this chat")
		return
	}

	// Validate input
	validationErrors := make(map[string]utils.ValidationError)

	if req.Name != nil && *req.Name != "" {
		if err := utils.ValidateGroupName(*req.Name); err != nil {
			validationErrors["name"] = *err
		}
	}

	if len(validationErrors) > 0 {
		utils.ValidationErrorResponse(c, validationErrors)
		return
	}

	// Build update document
	updateFields := bson.M{}

	if req.Name != nil {
		updateFields["name"] = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		updateFields["description"] = strings.TrimSpace(*req.Description)
	}
	if req.Avatar != nil {
		updateFields["avatar"] = strings.TrimSpace(*req.Avatar)
	}
	if req.Settings != nil {
		updateFields["settings"] = *req.Settings
	}

	updateFields["updated_at"] = time.Now()

	// Update chat
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = collections.Chats.UpdateOne(ctx,
		bson.M{"_id": chatID},
		bson.M{"$set": updateFields},
	)
	if err != nil {
		logger.Errorf("Failed to update chat: %v", err)
		utils.InternalServerError(c, "Failed to update chat")
		return
	}

	// Log chat update
	logger.LogUserAction(user.ID.Hex(), "chat_updated", "chat", map[string]interface{}{
		"chat_id":        chat.ID.Hex(),
		"updated_fields": updateFields,
	})

	// Create system message for group chats
	if chat.Type == models.ChatTypeGroup {
		go h.createChatUpdateMessage(chat, user, updateFields)
	}

	utils.Success(c, map[string]interface{}{
		"message": "Chat updated successfully",
	})
}

// DeleteChat deletes or leaves a chat
func (h *ChatHandler) DeleteChat(c *gin.Context) {
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

	chat, err := h.getChatByID(chatID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found")
		} else {
			utils.InternalServerError(c, "Failed to get chat")
		}
		return
	}

	// Check if user is participant
	if !chat.IsParticipant(user.ID) {
		utils.Forbidden(c, "You are not a participant in this chat")
		return
	}

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if chat.Type == models.ChatTypePrivate {
		// For private chats, mark as deleted for the user only
		_, err = collections.Chats.UpdateOne(ctx,
			bson.M{"_id": chatID},
			bson.M{
				"$addToSet": bson.M{
					"is_archived": models.ArchivedFor{
						UserID:     user.ID,
						ArchivedAt: time.Now(),
					},
				},
				"$set": bson.M{"updated_at": time.Now()},
			},
		)
	} else {
		// For group chats, remove user from participants
		chat.RemoveParticipant(user.ID)

		// If no participants left, mark chat as inactive
		if len(chat.Participants) == 0 {
			_, err = collections.Chats.UpdateOne(ctx,
				bson.M{"_id": chatID},
				bson.M{"$set": bson.M{
					"is_active":  false,
					"updated_at": time.Now(),
				}},
			)
		} else {
			_, err = collections.Chats.UpdateOne(ctx,
				bson.M{"_id": chatID},
				bson.M{"$set": bson.M{
					"participants": chat.Participants,
					"updated_at":   time.Now(),
				}},
			)
		}

		// Create leave message
		go h.createLeaveMessage(chat, user)
	}

	if err != nil {
		logger.Errorf("Failed to delete/leave chat: %v", err)
		utils.InternalServerError(c, "Failed to delete chat")
		return
	}

	// Log action
	action := "chat_deleted"
	if chat.Type == models.ChatTypeGroup {
		action = "chat_left"
	}

	logger.LogUserAction(user.ID.Hex(), action, "chat", map[string]interface{}{
		"chat_id":   chat.ID.Hex(),
		"chat_type": string(chat.Type),
	})

	utils.Success(c, map[string]interface{}{
		"message": "Chat deleted successfully",
	})
}

// LeaveChat allows user to leave a group chat
func (h *ChatHandler) LeaveChat(c *gin.Context) {
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

	chat, err := h.getChatByID(chatID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found")
		} else {
			utils.InternalServerError(c, "Failed to get chat")
		}
		return
	}

	// Only group chats can be left
	if chat.Type != models.ChatTypeGroup {
		utils.BadRequest(c, "Can only leave group chats")
		return
	}

	// Check if user is participant
	if !chat.IsParticipant(user.ID) {
		utils.Forbidden(c, "You are not a participant in this chat")
		return
	}

	// Remove user from participants
	chat.RemoveParticipant(user.ID)

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Update chat
	update := bson.M{
		"$set": bson.M{
			"participants": chat.Participants,
			"updated_at":   time.Now(),
		},
	}

	// If no participants left, mark as inactive
	if len(chat.Participants) == 0 {
		update["$set"].(bson.M)["is_active"] = false
	}

	_, err = collections.Chats.UpdateOne(ctx, bson.M{"_id": chatID}, update)
	if err != nil {
		logger.Errorf("Failed to leave chat: %v", err)
		utils.InternalServerError(c, "Failed to leave chat")
		return
	}

	// Create leave message
	go h.createLeaveMessage(chat, user)

	// Log action
	logger.LogUserAction(user.ID.Hex(), "chat_left", "chat", map[string]interface{}{
		"chat_id": chat.ID.Hex(),
	})

	utils.Success(c, map[string]interface{}{
		"message": "Left chat successfully",
	})
}

// GetChatParticipants retrieves chat participants
func (h *ChatHandler) GetChatParticipants(c *gin.Context) {
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

	chat, err := h.getChatByID(chatID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found")
		} else {
			utils.InternalServerError(c, "Failed to get chat")
		}
		return
	}

	// Check if user is participant
	if !chat.IsParticipant(user.ID) {
		utils.Forbidden(c, "You are not a participant in this chat")
		return
	}

	// Get participant details
	participants, err := h.getChatParticipantDetails(chat.Participants, user.ID)
	if err != nil {
		logger.Errorf("Failed to get participant details: %v", err)
		utils.InternalServerError(c, "Failed to get participants")
		return
	}

	utils.Success(c, map[string]interface{}{
		"participants": participants,
		"total_count":  len(participants),
	})
}

// AddParticipant adds a participant to the chat
func (h *ChatHandler) AddParticipant(c *gin.Context) {
	chatID, err := primitive.ObjectIDFromHex(c.Param("chatId"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	var req struct {
		UserID primitive.ObjectID `json:"user_id" binding:"required"`
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

	chat, err := h.getChatByID(chatID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found")
		} else {
			utils.InternalServerError(c, "Failed to get chat")
		}
		return
	}

	// Check permissions
	if !h.canAddMembers(chat, user.ID) {
		utils.Forbidden(c, "You don't have permission to add participants")
		return
	}

	// Check if user is already a participant
	if chat.IsParticipant(req.UserID) {
		utils.BadRequest(c, "User is already a participant")
		return
	}

	// Verify the user exists and is active
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var targetUser models.User
	err = collections.Users.FindOne(ctx, bson.M{
		"_id":        req.UserID,
		"is_active":  true,
		"is_deleted": false,
	}).Decode(&targetUser)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "User not found")
		} else {
			utils.InternalServerError(c, "Failed to verify user")
		}
		return
	}

	// Add participant
	chat.AddParticipant(req.UserID)

	// Update chat
	_, err = collections.Chats.UpdateOne(ctx,
		bson.M{"_id": chatID},
		bson.M{"$set": bson.M{
			"participants": chat.Participants,
			"updated_at":   time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to add participant: %v", err)
		utils.InternalServerError(c, "Failed to add participant")
		return
	}

	// Create join message
	go h.createJoinMessage(chat, &targetUser, user)

	// Send notification to new participant
	go h.notifyUserAboutChatInvite(chat, &targetUser, user)

	// Log action
	logger.LogUserAction(user.ID.Hex(), "participant_added", "chat", map[string]interface{}{
		"chat_id":    chat.ID.Hex(),
		"added_user": req.UserID.Hex(),
	})

	utils.Success(c, map[string]interface{}{
		"message": "Participant added successfully",
	})
}

// RemoveParticipant removes a participant from the chat
func (h *ChatHandler) RemoveParticipant(c *gin.Context) {
	chatID, err := primitive.ObjectIDFromHex(c.Param("chatId"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	participantID, err := primitive.ObjectIDFromHex(c.Param("userId"))
	if err != nil {
		utils.BadRequest(c, "Invalid user ID")
		return
	}

	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	chat, err := h.getChatByID(chatID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found")
		} else {
			utils.InternalServerError(c, "Failed to get chat")
		}
		return
	}

	// Check permissions
	if !h.canRemoveMembers(chat, user.ID) && user.ID != participantID {
		utils.Forbidden(c, "You don't have permission to remove participants")
		return
	}

	// Check if participant exists in chat
	if !chat.IsParticipant(participantID) {
		utils.BadRequest(c, "User is not a participant in this chat")
		return
	}

	// Remove participant
	chat.RemoveParticipant(participantID)

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Update chat
	update := bson.M{
		"$set": bson.M{
			"participants": chat.Participants,
			"updated_at":   time.Now(),
		},
	}

	// If no participants left, mark as inactive
	if len(chat.Participants) == 0 {
		update["$set"].(bson.M)["is_active"] = false
	}

	_, err = collections.Chats.UpdateOne(ctx, bson.M{"_id": chatID}, update)
	if err != nil {
		logger.Errorf("Failed to remove participant: %v", err)
		utils.InternalServerError(c, "Failed to remove participant")
		return
	}

	// Create removal message
	action := "removed"
	if user.ID == participantID {
		action = "left"
	}
	go h.createRemovalMessage(chat, participantID, user, action)

	// Log action
	logger.LogUserAction(user.ID.Hex(), "participant_removed", "chat", map[string]interface{}{
		"chat_id":      chat.ID.Hex(),
		"removed_user": participantID.Hex(),
		"action":       action,
	})

	utils.Success(c, map[string]interface{}{
		"message": "Participant removed successfully",
	})
}

// ArchiveChat archives a chat for the user
func (h *ChatHandler) ArchiveChat(c *gin.Context) {
	h.toggleChatArchive(c, true)
}

// UnarchiveChat unarchives a chat for the user
func (h *ChatHandler) UnarchiveChat(c *gin.Context) {
	h.toggleChatArchive(c, false)
}

// MuteChat mutes a chat for the user
func (h *ChatHandler) MuteChat(c *gin.Context) {
	chatID, err := primitive.ObjectIDFromHex(c.Param("chatId"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	var req struct {
		Duration string `json:"duration"` // "1h", "8h", "1d", "1w", "forever"
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

	chat, err := h.getChatByID(chatID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found")
		} else {
			utils.InternalServerError(c, "Failed to get chat")
		}
		return
	}

	// Check if user is participant
	if !chat.IsParticipant(user.ID) {
		utils.Forbidden(c, "You are not a participant in this chat")
		return
	}

	// Parse mute duration
	var mutedUntil *time.Time
	if req.Duration != "forever" && req.Duration != "" {
		duration, err := time.ParseDuration(req.Duration)
		if err != nil {
			utils.BadRequest(c, "Invalid duration format")
			return
		}
		until := time.Now().Add(duration)
		mutedUntil = &until
	}

	// Mute chat
	chat.Mute(user.ID, mutedUntil)

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = collections.Chats.UpdateOne(ctx,
		bson.M{"_id": chatID},
		bson.M{"$set": bson.M{
			"is_muted":   chat.IsMuted,
			"updated_at": time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to mute chat: %v", err)
		utils.InternalServerError(c, "Failed to mute chat")
		return
	}

	// Log action
	logger.LogUserAction(user.ID.Hex(), "chat_muted", "chat", map[string]interface{}{
		"chat_id":     chat.ID.Hex(),
		"duration":    req.Duration,
		"muted_until": mutedUntil,
	})

	message := "Chat muted successfully"
	if mutedUntil != nil {
		message = fmt.Sprintf("Chat muted until %s", mutedUntil.Format("2006-01-02 15:04"))
	} else if req.Duration == "forever" {
		message = "Chat muted forever"
	}

	utils.Success(c, map[string]interface{}{
		"message":     message,
		"muted_until": mutedUntil,
	})
}

// UnmuteChat unmutes a chat for the user
func (h *ChatHandler) UnmuteChat(c *gin.Context) {
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

	chat, err := h.getChatByID(chatID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found")
		} else {
			utils.InternalServerError(c, "Failed to get chat")
		}
		return
	}

	// Check if user is participant
	if !chat.IsParticipant(user.ID) {
		utils.Forbidden(c, "You are not a participant in this chat")
		return
	}

	// Unmute chat
	chat.Unmute(user.ID)

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = collections.Chats.UpdateOne(ctx,
		bson.M{"_id": chatID},
		bson.M{"$set": bson.M{
			"is_muted":   chat.IsMuted,
			"updated_at": time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to unmute chat: %v", err)
		utils.InternalServerError(c, "Failed to unmute chat")
		return
	}

	// Log action
	logger.LogUserAction(user.ID.Hex(), "chat_unmuted", "chat", map[string]interface{}{
		"chat_id": chat.ID.Hex(),
	})

	utils.Success(c, map[string]interface{}{
		"message": "Chat unmuted successfully",
	})
}

// PinChat pins a chat for the user
func (h *ChatHandler) PinChat(c *gin.Context) {
	h.toggleChatPin(c, true)
}

// UnpinChat unpins a chat for the user
func (h *ChatHandler) UnpinChat(c *gin.Context) {
	h.toggleChatPin(c, false)
}

// BlockChat blocks a chat for the user
func (h *ChatHandler) BlockChat(c *gin.Context) {
	h.toggleChatBlock(c, true)
}

// UnblockChat unblocks a chat for the user
func (h *ChatHandler) UnblockChat(c *gin.Context) {
	h.toggleChatBlock(c, false)
}

// MarkChatAsRead marks all messages in chat as read
func (h *ChatHandler) MarkChatAsRead(c *gin.Context) {
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

	chat, err := h.getChatByID(chatID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found")
		} else {
			utils.InternalServerError(c, "Failed to get chat")
		}
		return
	}

	// Check if user is participant
	if !chat.IsParticipant(user.ID) {
		utils.Forbidden(c, "You are not a participant in this chat")
		return
	}

	// Mark chat as read
	chat.MarkAsRead(user.ID)

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = collections.Chats.UpdateOne(ctx,
		bson.M{"_id": chatID},
		bson.M{"$set": bson.M{
			"unread_counts": chat.UnreadCounts,
			"updated_at":    time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to mark chat as read: %v", err)
		utils.InternalServerError(c, "Failed to mark chat as read")
		return
	}

	utils.Success(c, map[string]interface{}{
		"message": "Chat marked as read",
	})
}

// GetChatSettings returns chat settings
func (h *ChatHandler) GetChatSettings(c *gin.Context) {
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

	chat, err := h.getChatByID(chatID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found")
		} else {
			utils.InternalServerError(c, "Failed to get chat")
		}
		return
	}

	// Check if user is participant
	if !chat.IsParticipant(user.ID) {
		utils.Forbidden(c, "You are not a participant in this chat")
		return
	}

	utils.Success(c, map[string]interface{}{
		"settings": chat.Settings,
	})
}

// UpdateChatSettings updates chat settings
func (h *ChatHandler) UpdateChatSettings(c *gin.Context) {
	chatID, err := primitive.ObjectIDFromHex(c.Param("chatId"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	var req struct {
		Settings models.ChatSettings `json:"settings" binding:"required"`
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

	chat, err := h.getChatByID(chatID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found")
		} else {
			utils.InternalServerError(c, "Failed to get chat")
		}
		return
	}

	// Check permissions
	if !h.canEditChatInfo(chat, user.ID) {
		utils.Forbidden(c, "You don't have permission to edit chat settings")
		return
	}

	// Update settings
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = collections.Chats.UpdateOne(ctx,
		bson.M{"_id": chatID},
		bson.M{"$set": bson.M{
			"settings":   req.Settings,
			"updated_at": time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to update chat settings: %v", err)
		utils.InternalServerError(c, "Failed to update chat settings")
		return
	}

	// Log action
	logger.LogUserAction(user.ID.Hex(), "chat_settings_updated", "chat", map[string]interface{}{
		"chat_id": chat.ID.Hex(),
	})

	utils.Success(c, map[string]interface{}{
		"message": "Chat settings updated successfully",
	})
}

// SearchChats searches user's chats
func (h *ChatHandler) SearchChats(c *gin.Context) {
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

	pagination := utils.GetPaginationParams(c)

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build search filter
	filter := bson.M{
		"participants": user.ID,
		"is_active":    true,
		"$or": []bson.M{
			{"name": bson.M{"$regex": query, "$options": "i"}},
			{"description": bson.M{"$regex": query, "$options": "i"}},
		},
	}

	// Get total count
	total, err := collections.Chats.CountDocuments(ctx, filter)
	if err != nil {
		logger.Errorf("Failed to count search results: %v", err)
		utils.InternalServerError(c, "Search failed")
		return
	}

	// Get chats
	opts := options.Find().
		SetSort(bson.D{{Key: "last_activity", Value: -1}}).
		SetLimit(int64(pagination.Limit)).
		SetSkip(int64((pagination.Page - 1) * pagination.Limit))

	cursor, err := collections.Chats.Find(ctx, filter, opts)
	if err != nil {
		logger.Errorf("Failed to search chats: %v", err)
		utils.InternalServerError(c, "Search failed")
		return
	}
	defer cursor.Close(ctx)

	var chats []models.Chat
	if err := cursor.All(ctx, &chats); err != nil {
		logger.Errorf("Failed to decode search results: %v", err)
		utils.InternalServerError(c, "Search failed")
		return
	}

	// Convert to response format
	chatResponses := make([]models.ChatResponse, len(chats))
	for i, chat := range chats {
		chatResponse := h.buildChatResponse(&chat, user.ID)
		chatResponses[i] = chatResponse
	}

	// Create pagination metadata
	meta := utils.CreatePaginationMeta(pagination.Page, pagination.Limit, total)

	utils.SuccessWithMeta(c, map[string]interface{}{
		"query":   query,
		"results": chatResponses,
	}, meta)
}

// Helper methods

// getChatByID retrieves chat by ID
func (h *ChatHandler) getChatByID(chatID primitive.ObjectID) (*models.Chat, error) {
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var chat models.Chat
	err := collections.Chats.FindOne(ctx, bson.M{
		"_id":       chatID,
		"is_active": true,
	}).Decode(&chat)

	return &chat, err
}

// buildChatResponse builds chat response with user-specific data
func (h *ChatHandler) buildChatResponse(chat *models.Chat, userID primitive.ObjectID) models.ChatResponse {
	response := models.ChatResponse{
		Chat:           *chat,
		IsMutedByMe:    chat.IsMutedFor(userID),
		IsArchivedByMe: chat.IsArchivedFor(userID),
		IsPinnedByMe:   chat.IsPinnedFor(userID),
		IsBlockedByMe:  chat.IsBlockedFor(userID),
		MyDraft:        chat.GetDraft(userID),
		CanMessage:     chat.CanUserMessage(userID),
		CanCall:        h.canMakeCalls(chat, userID),
		CanAddMembers:  h.canAddMembers(chat, userID),
		CanEditInfo:    h.canEditChatInfo(chat, userID),
	}

	// Get unread count for user
	if unreadCount := chat.GetUnreadCount(userID); unreadCount != nil {
		response.UnreadCount = unreadCount.Count
		response.MentionCount = unreadCount.MentionCount
	}

	// Get participant info for small chats
	if len(chat.Participants) <= 10 {
		participants, _ := h.getChatParticipantDetails(chat.Participants, userID)
		response.ParticipantInfo = participants
	}

	return response
}

// getChatParticipantDetails gets detailed info for chat participants
func (h *ChatHandler) getChatParticipantDetails(participantIDs []primitive.ObjectID, requesterID primitive.ObjectID) ([]models.UserPublicInfo, error) {
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := collections.Users.Find(ctx, bson.M{
		"_id":        bson.M{"$in": participantIDs},
		"is_active":  true,
		"is_deleted": false,
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var users []models.User
	if err := cursor.All(ctx, &users); err != nil {
		return nil, err
	}

	participants := make([]models.UserPublicInfo, len(users))
	for i, user := range users {
		participants[i] = user.GetPublicInfo(requesterID)
	}

	return participants, nil
}

// Permission check methods

func (h *ChatHandler) canEditChatInfo(chat *models.Chat, userID primitive.ObjectID) bool {
	if chat.Type == models.ChatTypePrivate {
		return false
	}
	if chat.CreatedBy == userID {
		return true
	}
	if chat.Settings.OnlyAdminsCanEditInfo {
		// Check if user is admin (would need group model integration)
		return false
	}
	return true
}

func (h *ChatHandler) canAddMembers(chat *models.Chat, userID primitive.ObjectID) bool {
	if chat.Type != models.ChatTypeGroup {
		return false
	}
	if chat.CreatedBy == userID {
		return true
	}
	if chat.Settings.OnlyAdminsCanAddMembers {
		// Check if user is admin (would need group model integration)
		return false
	}
	return true
}

func (h *ChatHandler) canRemoveMembers(chat *models.Chat, userID primitive.ObjectID) bool {
	if chat.Type != models.ChatTypeGroup {
		return false
	}
	if chat.CreatedBy == userID {
		return true
	}
	// Check if user is admin (would need group model integration)
	return false
}

func (h *ChatHandler) canMakeCalls(chat *models.Chat, userID primitive.ObjectID) bool {
	if !chat.IsParticipant(userID) {
		return false
	}
	return chat.Settings.AllowVoiceCalls || chat.Settings.AllowVideoCalls
}

// Toggle methods for archive, pin, block

func (h *ChatHandler) toggleChatArchive(c *gin.Context, archive bool) {
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

	chat, err := h.getChatByID(chatID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found")
		} else {
			utils.InternalServerError(c, "Failed to get chat")
		}
		return
	}

	if !chat.IsParticipant(user.ID) {
		utils.Forbidden(c, "You are not a participant in this chat")
		return
	}

	if archive {
		chat.Archive(user.ID)
	} else {
		chat.Unarchive(user.ID)
	}

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = collections.Chats.UpdateOne(ctx,
		bson.M{"_id": chatID},
		bson.M{"$set": bson.M{
			"is_archived": chat.IsArchived,
			"updated_at":  time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to update chat archive status: %v", err)
		utils.InternalServerError(c, "Failed to update chat")
		return
	}

	action := "archived"
	if !archive {
		action = "unarchived"
	}

	logger.LogUserAction(user.ID.Hex(), "chat_"+action, "chat", map[string]interface{}{
		"chat_id": chat.ID.Hex(),
	})

	utils.Success(c, map[string]interface{}{
		"message": fmt.Sprintf("Chat %s successfully", action),
	})
}

func (h *ChatHandler) toggleChatPin(c *gin.Context, pin bool) {
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

	chat, err := h.getChatByID(chatID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found")
		} else {
			utils.InternalServerError(c, "Failed to get chat")
		}
		return
	}

	if !chat.IsParticipant(user.ID) {
		utils.Forbidden(c, "You are not a participant in this chat")
		return
	}

	if pin {
		chat.Pin(user.ID)
	} else {
		chat.Unpin(user.ID)
	}

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = collections.Chats.UpdateOne(ctx,
		bson.M{"_id": chatID},
		bson.M{"$set": bson.M{
			"is_pinned":  chat.IsPinned,
			"updated_at": time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to update chat pin status: %v", err)
		utils.InternalServerError(c, "Failed to update chat")
		return
	}

	action := "pinned"
	if !pin {
		action = "unpinned"
	}

	utils.Success(c, map[string]interface{}{
		"message": fmt.Sprintf("Chat %s successfully", action),
	})
}

func (h *ChatHandler) toggleChatBlock(c *gin.Context, block bool) {
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

	chat, err := h.getChatByID(chatID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found")
		} else {
			utils.InternalServerError(c, "Failed to get chat")
		}
		return
	}

	if !chat.IsParticipant(user.ID) {
		utils.Forbidden(c, "You are not a participant in this chat")
		return
	}

	if block {
		chat.Block(user.ID)
	} else {
		chat.Unblock(user.ID)
	}

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = collections.Chats.UpdateOne(ctx,
		bson.M{"_id": chatID},
		bson.M{"$set": bson.M{
			"is_blocked": chat.IsBlocked,
			"updated_at": time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to update chat block status: %v", err)
		utils.InternalServerError(c, "Failed to update chat")
		return
	}

	action := "blocked"
	if !block {
		action = "unblocked"
	}

	utils.Success(c, map[string]interface{}{
		"message": fmt.Sprintf("Chat %s successfully", action),
	})
}

// System message creation methods

func (h *ChatHandler) createWelcomeMessage(chat *models.Chat, creator *models.User) {
	// Implementation would create a system message for group creation
	logger.Infof("Creating welcome message for group %s created by %s", chat.ID.Hex(), creator.Name)
}

func (h *ChatHandler) createJoinMessage(chat *models.Chat, joinedUser *models.User, addedBy *models.User) {
	// Implementation would create a system message for user joining
	logger.Infof("User %s added to group %s by %s", joinedUser.Name, chat.ID.Hex(), addedBy.Name)
}

func (h *ChatHandler) createLeaveMessage(chat *models.Chat, leftUser *models.User) {
	// Implementation would create a system message for user leaving
	logger.Infof("User %s left group %s", leftUser.Name, chat.ID.Hex())
}

func (h *ChatHandler) createRemovalMessage(chat *models.Chat, removedUserID primitive.ObjectID, removedBy *models.User, action string) {
	// Implementation would create a system message for user removal
	logger.Infof("User %s %s from group %s by %s", removedUserID.Hex(), action, chat.ID.Hex(), removedBy.Name)
}

func (h *ChatHandler) createChatUpdateMessage(chat *models.Chat, updatedBy *models.User, fields bson.M) {
	// Implementation would create a system message for chat updates
	logger.Infof("Chat %s updated by %s: %v", chat.ID.Hex(), updatedBy.Name, fields)
}

// Notification methods

func (h *ChatHandler) notifyParticipantsAboutNewChat(chat *models.Chat, creator *models.User) {
	if h.pushService == nil {
		return
	}

	for _, participantID := range chat.Participants {
		if participantID == creator.ID {
			continue
		}

		title := "New Chat"
		body := fmt.Sprintf("%s started a chat with you", creator.Name)

		if chat.Type == models.ChatTypeGroup {
			title = "Added to Group"
			body = fmt.Sprintf("%s added you to \"%s\"", creator.Name, chat.Name)
		}

		data := map[string]string{
			"type":    "new_chat",
			"chat_id": chat.ID.Hex(),
		}

		if err := h.pushService.SendNotification(participantID, title, body, data); err != nil {
			logger.Errorf("Failed to send chat notification: %v", err)
		}
	}
}

func (h *ChatHandler) notifyUserAboutChatInvite(chat *models.Chat, invitedUser *models.User, invitedBy *models.User) {
	if h.pushService == nil {
		return
	}

	title := "Added to Group"
	body := fmt.Sprintf("%s added you to \"%s\"", invitedBy.Name, chat.Name)

	data := map[string]string{
		"type":       "group_invite",
		"chat_id":    chat.ID.Hex(),
		"invited_by": invitedBy.ID.Hex(),
	}

	if err := h.pushService.SendNotification(invitedUser.ID, title, body, data); err != nil {
		logger.Errorf("Failed to send invite notification: %v", err)
	}
}

// Draft message methods

func (h *ChatHandler) GetDraft(c *gin.Context) {
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

	chat, err := h.getChatByID(chatID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found")
		} else {
			utils.InternalServerError(c, "Failed to get chat")
		}
		return
	}

	if !chat.IsParticipant(user.ID) {
		utils.Forbidden(c, "You are not a participant in this chat")
		return
	}

	draft := chat.GetDraft(user.ID)
	utils.Success(c, map[string]interface{}{
		"draft": draft,
	})
}

func (h *ChatHandler) SetDraft(c *gin.Context) {
	chatID, err := primitive.ObjectIDFromHex(c.Param("chatId"))
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	var req struct {
		Content string             `json:"content" binding:"required"`
		Type    models.MessageType `json:"type"`
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

	chat, err := h.getChatByID(chatID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found")
		} else {
			utils.InternalServerError(c, "Failed to get chat")
		}
		return
	}

	if !chat.IsParticipant(user.ID) {
		utils.Forbidden(c, "You are not a participant in this chat")
		return
	}

	// Set draft
	msgType := req.Type
	if msgType == "" {
		msgType = models.MessageTypeText
	}

	chat.SetDraft(user.ID, req.Content, msgType)

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = collections.Chats.UpdateOne(ctx,
		bson.M{"_id": chatID},
		bson.M{"$set": bson.M{
			"draft_messages": chat.DraftMessages,
			"updated_at":     time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to save draft: %v", err)
		utils.InternalServerError(c, "Failed to save draft")
		return
	}

	utils.Success(c, map[string]interface{}{
		"message": "Draft saved successfully",
	})
}

func (h *ChatHandler) ClearDraft(c *gin.Context) {
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

	chat, err := h.getChatByID(chatID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "Chat not found")
		} else {
			utils.InternalServerError(c, "Failed to get chat")
		}
		return
	}

	if !chat.IsParticipant(user.ID) {
		utils.Forbidden(c, "You are not a participant in this chat")
		return
	}

	// Clear draft
	chat.ClearDraft(user.ID)

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = collections.Chats.UpdateOne(ctx,
		bson.M{"_id": chatID},
		bson.M{"$set": bson.M{
			"draft_messages": chat.DraftMessages,
			"updated_at":     time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to clear draft: %v", err)
		utils.InternalServerError(c, "Failed to clear draft")
		return
	}

	utils.Success(c, map[string]interface{}{
		"message": "Draft cleared successfully",
	})
}

// Additional endpoints that would be implemented...

func (h *ChatHandler) GetArchivedChats(c *gin.Context) {
	// Implementation for getting archived chats
	utils.Success(c, map[string]interface{}{
		"message": "Archived chats endpoint - implementation needed",
	})
}

func (h *ChatHandler) GetMutedChats(c *gin.Context) {
	// Implementation for getting muted chats
	utils.Success(c, map[string]interface{}{
		"message": "Muted chats endpoint - implementation needed",
	})
}

func (h *ChatHandler) GetPinnedChats(c *gin.Context) {
	// Implementation for getting pinned chats
	utils.Success(c, map[string]interface{}{
		"message": "Pinned chats endpoint - implementation needed",
	})
}

func (h *ChatHandler) GetChatInfo(c *gin.Context) {
	// Implementation for getting detailed chat info
	utils.Success(c, map[string]interface{}{
		"message": "Chat info endpoint - implementation needed",
	})
}

func (h *ChatHandler) GetChatMedia(c *gin.Context) {
	// Implementation for getting chat media files
	utils.Success(c, map[string]interface{}{
		"message": "Chat media endpoint - implementation needed",
	})
}

func (h *ChatHandler) GetChatFiles(c *gin.Context) {
	// Implementation for getting chat files
	utils.Success(c, map[string]interface{}{
		"message": "Chat files endpoint - implementation needed",
	})
}

func (h *ChatHandler) GetChatLinks(c *gin.Context) {
	// Implementation for getting chat links
	utils.Success(c, map[string]interface{}{
		"message": "Chat links endpoint - implementation needed",
	})
}

func (h *ChatHandler) ClearChatHistory(c *gin.Context) {
	// Implementation for clearing chat history
	utils.Success(c, map[string]interface{}{
		"message": "Clear chat history endpoint - implementation needed",
	})
}

func (h *ChatHandler) ExportChat(c *gin.Context) {
	// Implementation for exporting chat
	utils.Success(c, map[string]interface{}{
		"message": "Export chat endpoint - implementation needed",
	})
}

func (h *ChatHandler) BackupChat(c *gin.Context) {
	// Implementation for backing up chat
	utils.Success(c, map[string]interface{}{
		"message": "Backup chat endpoint - implementation needed",
	})
}

func (h *ChatHandler) ReportChat(c *gin.Context) {
	// Implementation for reporting chat
	utils.Success(c, map[string]interface{}{
		"message": "Report chat endpoint - implementation needed",
	})
}
