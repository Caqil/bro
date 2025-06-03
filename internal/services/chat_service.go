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
	"bro/internal/websocket"
	"bro/pkg/database"
	"bro/pkg/logger"
	"bro/pkg/redis"
)

// ChatService handles all chat-related operations
type ChatService struct {
	// Configuration
	config *config.Config

	// Database collections
	chatsCollection    *mongo.Collection
	usersCollection    *mongo.Collection
	messagesCollection *mongo.Collection
	groupsCollection   *mongo.Collection

	// External services
	hub           *websocket.Hub
	redisClient   *redis.Client
	pushService   *PushService
	encryptionKey string

	// Chat management
	activeDrafts map[string]*models.DraftMessage
	draftsMutex  sync.RWMutex

	// Statistics
	chatStats *ChatStatistics

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ChatStatistics contains chat service statistics
type ChatStatistics struct {
	TotalChats    int64
	PrivateChats  int64
	GroupChats    int64
	ActiveChats   int64
	TotalMessages int64
	MessagesToday int64
	LastUpdated   time.Time
	mutex         sync.RWMutex
}

// ChatRequest represents a chat creation request
type ChatRequest struct {
	Type         models.ChatType      `json:"type" validate:"required"`
	Participants []primitive.ObjectID `json:"participants" validate:"required"`
	Name         string               `json:"name,omitempty"`
	Description  string               `json:"description,omitempty"`
	Avatar       string               `json:"avatar,omitempty"`
	Settings     *models.ChatSettings `json:"settings,omitempty"`
}

// ChatUpdateRequest represents a chat update request
type ChatUpdateRequest struct {
	Name        *string              `json:"name,omitempty"`
	Description *string              `json:"description,omitempty"`
	Avatar      *string              `json:"avatar,omitempty"`
	Settings    *models.ChatSettings `json:"settings,omitempty"`
}

// ChatListRequest represents parameters for listing chats
type ChatListRequest struct {
	Page     int    `form:"page" json:"page"`
	Limit    int    `form:"limit" json:"limit"`
	Type     string `form:"type" json:"type"`
	Search   string `form:"search" json:"search"`
	Archived *bool  `form:"archived" json:"archived"`
	Pinned   *bool  `form:"pinned" json:"pinned"`
	Muted    *bool  `form:"muted" json:"muted"`
}

// NewChatService creates a new chat service
func NewChatService(
	cfg *config.Config,
	hub *websocket.Hub,
	pushService *PushService,
) (*ChatService, error) {

	collections := database.GetCollections()
	if collections == nil {
		return nil, fmt.Errorf("database collections not available")
	}

	ctx, cancel := context.WithCancel(context.Background())

	service := &ChatService{
		config:             cfg,
		chatsCollection:    collections.Chats,
		usersCollection:    collections.Users,
		messagesCollection: collections.Messages,
		groupsCollection:   collections.Groups,
		hub:                hub,
		redisClient:        redis.GetClient(),
		pushService:        pushService,
		encryptionKey:      cfg.EncryptionKey,
		activeDrafts:       make(map[string]*models.DraftMessage),
		chatStats:          &ChatStatistics{},
		ctx:                ctx,
		cancel:             cancel,
	}

	// Start background processes
	service.wg.Add(2)
	go service.statsCollector()
	go service.draftManager()

	logger.Info("Chat Service initialized successfully")
	return service, nil
}

// CreateChat creates a new chat
func (cs *ChatService) CreateChat(creatorID primitive.ObjectID, req *ChatRequest) (*models.ChatResponse, error) {
	// Validate request
	if err := cs.validateChatRequest(req); err != nil {
		return nil, fmt.Errorf("invalid chat request: %w", err)
	}

	// Check if users exist and are accessible
	if err := cs.validateParticipants(creatorID, req.Participants); err != nil {
		return nil, fmt.Errorf("invalid participants: %w", err)
	}

	// For private chats, check if chat already exists
	if req.Type == models.ChatTypePrivate {
		if existingChat, err := cs.findExistingPrivateChat(creatorID, req.Participants); err == nil && existingChat != nil {
			return cs.buildChatResponse(existingChat, creatorID)
		}
	}

	// Create chat in database
	chat, err := cs.createChatInDatabase(creatorID, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat: %w", err)
	}

	// Create group if it's a group chat
	if req.Type == models.ChatTypeGroup {
		if err := cs.createGroupForChat(chat, creatorID); err != nil {
			// Cleanup chat if group creation fails
			cs.deleteChatFromDatabase(chat.ID)
			return nil, fmt.Errorf("failed to create group: %w", err)
		}
	}

	// Notify participants about new chat
	go cs.notifyParticipantsAboutNewChat(chat, creatorID)

	// Update statistics
	cs.updateChatStats("created", req.Type)

	// Build response
	response, err := cs.buildChatResponse(chat, creatorID)
	if err != nil {
		return nil, fmt.Errorf("failed to build response: %w", err)
	}

	logger.LogUserAction(creatorID.Hex(), "chat_created", "chat_service", map[string]interface{}{
		"chat_id":      chat.ID.Hex(),
		"chat_type":    chat.Type,
		"participants": len(chat.Participants),
	})

	return response, nil
}

// GetUserChats retrieves user's chats with filtering and pagination
func (cs *ChatService) GetUserChats(userID primitive.ObjectID, req *ChatListRequest) (*models.ChatsListResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build query filter
	filter := bson.M{
		"participants": userID,
		"is_active":    true,
	}

	// Apply filters
	if req.Type != "" {
		filter["type"] = req.Type
	}

	if req.Search != "" {
		filter["$or"] = []bson.M{
			{"name": bson.M{"$regex": req.Search, "$options": "i"}},
			{"description": bson.M{"$regex": req.Search, "$options": "i"}},
		}
	}

	// Handle archived filter
	if req.Archived != nil {
		if *req.Archived {
			filter["is_archived.user_id"] = userID
		} else {
			filter["is_archived.user_id"] = bson.M{"$ne": userID}
		}
	}

	// Handle pinned filter
	if req.Pinned != nil {
		if *req.Pinned {
			filter["is_pinned.user_id"] = userID
		} else {
			filter["is_pinned.user_id"] = bson.M{"$ne": userID}
		}
	}

	// Handle muted filter
	if req.Muted != nil {
		if *req.Muted {
			filter["is_muted.user_id"] = userID
		} else {
			filter["is_muted.user_id"] = bson.M{"$ne": userID}
		}
	}

	// Calculate pagination
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 || req.Limit > 50 {
		req.Limit = 20
	}
	skip := (req.Page - 1) * req.Limit

	// Count total chats
	total, err := cs.chatsCollection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to count chats: %w", err)
	}

	// Build sort criteria (pinned first, then by last activity)
	sortStage := bson.D{
		{Key: "last_activity", Value: -1},
	}

	// Find chats with pagination
	opts := options.Find().
		SetSort(sortStage).
		SetSkip(int64(skip)).
		SetLimit(int64(req.Limit))

	cursor, err := cs.chatsCollection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find chats: %w", err)
	}
	defer cursor.Close(ctx)

	var chats []models.Chat
	if err := cursor.All(ctx, &chats); err != nil {
		return nil, fmt.Errorf("failed to decode chats: %w", err)
	}

	// Build chat responses
	chatResponses := make([]models.ChatResponse, len(chats))
	for i, chat := range chats {
		response, err := cs.buildChatResponse(&chat, userID)
		if err != nil {
			logger.Errorf("Failed to build chat response for chat %s: %v", chat.ID.Hex(), err)
			continue
		}
		chatResponses[i] = *response
	}

	return &models.ChatsListResponse{
		Chats:      chatResponses,
		TotalCount: total,
		HasMore:    int64(skip+req.Limit) < total,
	}, nil
}

// GetChat retrieves a specific chat
func (cs *ChatService) GetChat(chatID primitive.ObjectID, userID primitive.ObjectID) (*models.ChatResponse, error) {
	chat, err := cs.getChatFromDatabase(chatID)
	if err != nil {
		return nil, fmt.Errorf("chat not found: %w", err)
	}

	// Check if user is participant
	if !chat.IsParticipant(userID) {
		return nil, fmt.Errorf("user is not a participant of this chat")
	}

	return cs.buildChatResponse(chat, userID)
}

// UpdateChat updates chat information
func (cs *ChatService) UpdateChat(chatID primitive.ObjectID, userID primitive.ObjectID, req *ChatUpdateRequest) (*models.ChatResponse, error) {
	chat, err := cs.getChatFromDatabase(chatID)
	if err != nil {
		return nil, fmt.Errorf("chat not found: %w", err)
	}

	// Check if user can edit chat
	if !cs.canUserEditChat(chat, userID) {
		return nil, fmt.Errorf("insufficient permissions to edit chat")
	}

	// Update chat fields
	updateDoc := bson.M{}
	if req.Name != nil {
		chat.Name = *req.Name
		updateDoc["name"] = *req.Name
	}
	if req.Description != nil {
		chat.Description = *req.Description
		updateDoc["description"] = *req.Description
	}
	if req.Avatar != nil {
		chat.Avatar = *req.Avatar
		updateDoc["avatar"] = *req.Avatar
	}
	if req.Settings != nil {
		chat.Settings = *req.Settings
		updateDoc["settings"] = *req.Settings
	}

	if len(updateDoc) > 0 {
		updateDoc["updated_at"] = time.Now()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := cs.chatsCollection.UpdateOne(ctx, bson.M{"_id": chatID}, bson.M{"$set": updateDoc})
		if err != nil {
			return nil, fmt.Errorf("failed to update chat: %w", err)
		}
	}

	// Notify participants about chat update
	go cs.notifyParticipantsAboutChatUpdate(chat, userID, updateDoc)

	logger.LogUserAction(userID.Hex(), "chat_updated", "chat_service", map[string]interface{}{
		"chat_id": chatID.Hex(),
		"fields":  updateDoc,
	})

	return cs.buildChatResponse(chat, userID)
}

// AddParticipant adds a user to the chat
func (cs *ChatService) AddParticipant(chatID primitive.ObjectID, adderID primitive.ObjectID, userID primitive.ObjectID) error {
	chat, err := cs.getChatFromDatabase(chatID)
	if err != nil {
		return fmt.Errorf("chat not found: %w", err)
	}

	// Check permissions
	if !cs.canUserAddMembers(chat, adderID) {
		return fmt.Errorf("insufficient permissions to add members")
	}

	// Check if user is already a participant
	if chat.IsParticipant(userID) {
		return fmt.Errorf("user is already a participant")
	}

	// Validate the user exists and is active
	if err := cs.validateUser(userID); err != nil {
		return fmt.Errorf("invalid user: %w", err)
	}

	// Add participant to chat
	chat.AddParticipant(userID)

	// Update in database
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$addToSet": bson.M{"participants": userID},
		"$set":      bson.M{"updated_at": time.Now()},
	}

	_, err = cs.chatsCollection.UpdateOne(ctx, bson.M{"_id": chatID}, update)
	if err != nil {
		return fmt.Errorf("failed to add participant: %w", err)
	}

	// Notify participants
	go cs.notifyParticipantsAboutMemberChange(chat, "member_added", userID, adderID)

	logger.LogUserAction(adderID.Hex(), "participant_added", "chat_service", map[string]interface{}{
		"chat_id": chatID.Hex(),
		"user_id": userID.Hex(),
	})

	return nil
}

// RemoveParticipant removes a user from the chat
func (cs *ChatService) RemoveParticipant(chatID primitive.ObjectID, removerID primitive.ObjectID, userID primitive.ObjectID) error {
	chat, err := cs.getChatFromDatabase(chatID)
	if err != nil {
		return fmt.Errorf("chat not found: %w", err)
	}

	// Check permissions (user can remove themselves or admin can remove others)
	if removerID != userID && !cs.canUserRemoveMembers(chat, removerID) {
		return fmt.Errorf("insufficient permissions to remove members")
	}

	// Check if user is a participant
	if !chat.IsParticipant(userID) {
		return fmt.Errorf("user is not a participant")
	}

	// For private chats, don't allow removing participants
	if chat.Type == models.ChatTypePrivate {
		return fmt.Errorf("cannot remove participants from private chat")
	}

	// Remove participant from chat
	chat.RemoveParticipant(userID)

	// Update in database
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$pull": bson.M{"participants": userID},
		"$set":  bson.M{"updated_at": time.Now()},
	}

	_, err = cs.chatsCollection.UpdateOne(ctx, bson.M{"_id": chatID}, update)
	if err != nil {
		return fmt.Errorf("failed to remove participant: %w", err)
	}

	// Notify participants
	go cs.notifyParticipantsAboutMemberChange(chat, "member_removed", userID, removerID)

	logger.LogUserAction(removerID.Hex(), "participant_removed", "chat_service", map[string]interface{}{
		"chat_id": chatID.Hex(),
		"user_id": userID.Hex(),
	})

	return nil
}

// ArchiveChat archives/unarchives a chat for a user
func (cs *ChatService) ArchiveChat(chatID primitive.ObjectID, userID primitive.ObjectID, archive bool) error {
	chat, err := cs.getChatFromDatabase(chatID)
	if err != nil {
		return fmt.Errorf("chat not found: %w", err)
	}

	if !chat.IsParticipant(userID) {
		return fmt.Errorf("user is not a participant of this chat")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var update bson.M
	if archive {
		if !chat.IsArchivedFor(userID) {
			chat.Archive(userID)
			update = bson.M{
				"$addToSet": bson.M{
					"is_archived": bson.M{
						"user_id":     userID,
						"archived_at": time.Now(),
					},
				},
			}
		}
	} else {
		if chat.IsArchivedFor(userID) {
			chat.Unarchive(userID)
			update = bson.M{
				"$pull": bson.M{
					"is_archived": bson.M{"user_id": userID},
				},
			}
		}
	}

	if update != nil {
		_, err = cs.chatsCollection.UpdateOne(ctx, bson.M{"_id": chatID}, update)
		if err != nil {
			return fmt.Errorf("failed to update archive status: %w", err)
		}
	}

	action := "chat_unarchived"
	if archive {
		action = "chat_archived"
	}

	logger.LogUserAction(userID.Hex(), action, "chat_service", map[string]interface{}{
		"chat_id": chatID.Hex(),
	})

	return nil
}

// MuteChat mutes/unmutes a chat for a user
func (cs *ChatService) MuteChat(chatID primitive.ObjectID, userID primitive.ObjectID, mute bool, mutedUntil *time.Time) error {
	chat, err := cs.getChatFromDatabase(chatID)
	if err != nil {
		return fmt.Errorf("chat not found: %w", err)
	}

	if !chat.IsParticipant(userID) {
		return fmt.Errorf("user is not a participant of this chat")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var update bson.M
	if mute {
		chat.Mute(userID, mutedUntil)
		muteDoc := bson.M{
			"user_id":  userID,
			"muted_at": time.Now(),
		}
		if mutedUntil != nil {
			muteDoc["muted_until"] = *mutedUntil
		}
		update = bson.M{
			"$pull": bson.M{"is_muted": bson.M{"user_id": userID}},
		}
		if err := cs.updateChatField(chatID, update); err == nil {
			update = bson.M{
				"$addToSet": bson.M{"is_muted": muteDoc},
			}
		}
	} else {
		chat.Unmute(userID)
		update = bson.M{
			"$pull": bson.M{"is_muted": bson.M{"user_id": userID}},
		}
	}

	_, err = cs.chatsCollection.UpdateOne(ctx, bson.M{"_id": chatID}, update)
	if err != nil {
		return fmt.Errorf("failed to update mute status: %w", err)
	}

	action := "chat_unmuted"
	if mute {
		action = "chat_muted"
	}

	logger.LogUserAction(userID.Hex(), action, "chat_service", map[string]interface{}{
		"chat_id":     chatID.Hex(),
		"muted_until": mutedUntil,
	})

	return nil
}

// PinChat pins/unpins a chat for a user
func (cs *ChatService) PinChat(chatID primitive.ObjectID, userID primitive.ObjectID, pin bool) error {
	chat, err := cs.getChatFromDatabase(chatID)
	if err != nil {
		return fmt.Errorf("chat not found: %w", err)
	}

	if !chat.IsParticipant(userID) {
		return fmt.Errorf("user is not a participant of this chat")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var update bson.M
	if pin {
		if !chat.IsPinnedFor(userID) {
			chat.Pin(userID)
			update = bson.M{
				"$addToSet": bson.M{
					"is_pinned": bson.M{
						"user_id":   userID,
						"pinned_at": time.Now(),
					},
				},
			}
		}
	} else {
		if chat.IsPinnedFor(userID) {
			chat.Unpin(userID)
			update = bson.M{
				"$pull": bson.M{
					"is_pinned": bson.M{"user_id": userID},
				},
			}
		}
	}

	if update != nil {
		_, err = cs.chatsCollection.UpdateOne(ctx, bson.M{"_id": chatID}, update)
		if err != nil {
			return fmt.Errorf("failed to update pin status: %w", err)
		}
	}

	action := "chat_unpinned"
	if pin {
		action = "chat_pinned"
	}

	logger.LogUserAction(userID.Hex(), action, "chat_service", map[string]interface{}{
		"chat_id": chatID.Hex(),
	})

	return nil
}

// SetDraft sets draft message for a chat
func (cs *ChatService) SetDraft(chatID primitive.ObjectID, userID primitive.ObjectID, content string, msgType models.MessageType) error {
	chat, err := cs.getChatFromDatabase(chatID)
	if err != nil {
		return fmt.Errorf("chat not found: %w", err)
	}

	if !chat.IsParticipant(userID) {
		return fmt.Errorf("user is not a participant of this chat")
	}

	// Store draft in memory
	draftKey := fmt.Sprintf("%s:%s", chatID.Hex(), userID.Hex())
	cs.draftsMutex.Lock()
	if content == "" {
		delete(cs.activeDrafts, draftKey)
	} else {
		cs.activeDrafts[draftKey] = &models.DraftMessage{
			UserID:    userID,
			Content:   content,
			Type:      msgType,
			UpdatedAt: time.Now(),
		}
	}
	cs.draftsMutex.Unlock()

	// Update in database
	chat.SetDraft(userID, content, msgType)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if content == "" {
		// Remove draft
		update := bson.M{
			"$pull": bson.M{"draft_messages": bson.M{"user_id": userID}},
		}
		_, err = cs.chatsCollection.UpdateOne(ctx, bson.M{"_id": chatID}, update)
	} else {
		// Upsert draft
		filter := bson.M{
			"_id":                    chatID,
			"draft_messages.user_id": userID,
		}
		update := bson.M{
			"$set": bson.M{
				"draft_messages.$.content":    content,
				"draft_messages.$.type":       msgType,
				"draft_messages.$.updated_at": time.Now(),
			},
		}
		result, err := cs.chatsCollection.UpdateOne(ctx, filter, update)
		if err != nil {
			return err
		}
		if result.MatchedCount == 0 {
			// Draft doesn't exist, add it
			update = bson.M{
				"$addToSet": bson.M{
					"draft_messages": bson.M{
						"user_id":    userID,
						"content":    content,
						"type":       msgType,
						"updated_at": time.Now(),
					},
				},
			}
			_, err = cs.chatsCollection.UpdateOne(ctx, bson.M{"_id": chatID}, update)
		}
	}

	return err
}

// GetDraft gets draft message for a chat
func (cs *ChatService) GetDraft(chatID primitive.ObjectID, userID primitive.ObjectID) (*models.DraftMessage, error) {
	draftKey := fmt.Sprintf("%s:%s", chatID.Hex(), userID.Hex())
	cs.draftsMutex.RLock()
	defer cs.draftsMutex.RUnlock()

	if draft, exists := cs.activeDrafts[draftKey]; exists {
		return draft, nil
	}

	return nil, nil
}

// MarkAsRead marks chat as read for a user
func (cs *ChatService) MarkAsRead(chatID primitive.ObjectID, userID primitive.ObjectID) error {
	chat, err := cs.getChatFromDatabase(chatID)
	if err != nil {
		return fmt.Errorf("chat not found: %w", err)
	}

	if !chat.IsParticipant(userID) {
		return fmt.Errorf("user is not a participant of this chat")
	}

	// Update in memory
	chat.MarkAsRead(userID)

	// Update in database
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{
		"_id":                   chatID,
		"unread_counts.user_id": userID,
	}
	update := bson.M{
		"$set": bson.M{
			"unread_counts.$.count":         0,
			"unread_counts.$.mention_count": 0,
			"unread_counts.$.last_read_at":  time.Now(),
		},
	}

	_, err = cs.chatsCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to mark as read: %w", err)
	}

	logger.LogUserAction(userID.Hex(), "chat_read", "chat_service", map[string]interface{}{
		"chat_id": chatID.Hex(),
	})

	return nil
}

// DeleteChat deletes a chat (only for group chats and by admins)
func (cs *ChatService) DeleteChat(chatID primitive.ObjectID, userID primitive.ObjectID) error {
	chat, err := cs.getChatFromDatabase(chatID)
	if err != nil {
		return fmt.Errorf("chat not found: %w", err)
	}

	// Check permissions
	if !cs.canUserDeleteChat(chat, userID) {
		return fmt.Errorf("insufficient permissions to delete chat")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Mark chat as inactive instead of deleting
	update := bson.M{
		"$set": bson.M{
			"is_active":  false,
			"updated_at": time.Now(),
		},
	}

	_, err = cs.chatsCollection.UpdateOne(ctx, bson.M{"_id": chatID}, update)
	if err != nil {
		return fmt.Errorf("failed to delete chat: %w", err)
	}

	// Notify participants
	go cs.notifyParticipantsAboutChatDeletion(chat, userID)

	logger.LogUserAction(userID.Hex(), "chat_deleted", "chat_service", map[string]interface{}{
		"chat_id": chatID.Hex(),
	})

	return nil
}

// GetChatStatistics returns chat service statistics
func (cs *ChatService) GetChatStatistics() *ChatStatistics {
	cs.chatStats.mutex.RLock()
	defer cs.chatStats.mutex.RUnlock()

	// Create a copy
	stats := *cs.chatStats
	return &stats
}

// Helper methods

// validateChatRequest validates chat creation request
func (cs *ChatService) validateChatRequest(req *ChatRequest) error {
	if req.Type == "" {
		return fmt.Errorf("chat type is required")
	}

	if len(req.Participants) == 0 {
		return fmt.Errorf("at least one participant is required")
	}

	// Validate participant count based on chat type
	switch req.Type {
	case models.ChatTypePrivate:
		if len(req.Participants) != 1 {
			return fmt.Errorf("private chat must have exactly 1 other participant")
		}
	case models.ChatTypeGroup:
		if len(req.Participants) < 2 {
			return fmt.Errorf("group chat must have at least 2 participants")
		}
		if len(req.Participants) > 256 {
			return fmt.Errorf("group chat cannot have more than 256 participants")
		}
		if req.Name == "" {
			return fmt.Errorf("group chat name is required")
		}
	case models.ChatTypeBroadcast:
		if req.Name == "" {
			return fmt.Errorf("broadcast channel name is required")
		}
	}

	return nil
}

// validateParticipants validates that all participants exist and are accessible
func (cs *ChatService) validateParticipants(creatorID primitive.ObjectID, participantIDs []primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	allParticipants := append(participantIDs, creatorID)

	// Check if all users exist and are active
	count, err := cs.usersCollection.CountDocuments(ctx, bson.M{
		"_id":        bson.M{"$in": allParticipants},
		"is_active":  true,
		"is_deleted": false,
	})
	if err != nil {
		return fmt.Errorf("failed to validate participants: %w", err)
	}

	if int(count) != len(allParticipants) {
		return fmt.Errorf("one or more participants not found or inactive")
	}

	return nil
}

// validateUser validates a single user
func (cs *ChatService) validateUser(userID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	count, err := cs.usersCollection.CountDocuments(ctx, bson.M{
		"_id":        userID,
		"is_active":  true,
		"is_deleted": false,
	})
	if err != nil {
		return fmt.Errorf("failed to validate user: %w", err)
	}

	if count == 0 {
		return fmt.Errorf("user not found or inactive")
	}

	return nil
}

// findExistingPrivateChat finds existing private chat between users
func (cs *ChatService) findExistingPrivateChat(creatorID primitive.ObjectID, participants []primitive.ObjectID) (*models.Chat, error) {
	if len(participants) != 1 {
		return nil, fmt.Errorf("invalid participants for private chat")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	allParticipants := []primitive.ObjectID{creatorID, participants[0]}

	filter := bson.M{
		"type":         models.ChatTypePrivate,
		"participants": bson.M{"$all": allParticipants, "$size": 2},
		"is_active":    true,
	}

	var chat models.Chat
	err := cs.chatsCollection.FindOne(ctx, filter).Decode(&chat)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return &chat, nil
}

// createChatInDatabase creates chat in database
func (cs *ChatService) createChatInDatabase(creatorID primitive.ObjectID, req *ChatRequest) (*models.Chat, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create chat model
	chat := &models.Chat{
		Type:        req.Type,
		CreatedBy:   creatorID,
		Name:        req.Name,
		Description: req.Description,
		Avatar:      req.Avatar,
	}

	// Add creator to participants
	allParticipants := append([]primitive.ObjectID{creatorID}, req.Participants...)
	chat.Participants = allParticipants

	// Set default settings if not provided
	if req.Settings == nil {
		chat.Settings = models.ChatSettings{
			AllowMessages:           true,
			AllowMediaSharing:       true,
			AllowVoiceMessages:      true,
			AllowDocuments:          true,
			AllowVoiceCalls:         true,
			AllowVideoCalls:         true,
			OnlyAdminsCanMessage:    false,
			OnlyAdminsCanAddMembers: req.Type == models.ChatTypeGroup,
			OnlyAdminsCanEditInfo:   req.Type == models.ChatTypeGroup,
			ApprovalRequired:        false,
			MessageAutoDelete: models.AutoDeleteSetting{
				Enabled:  false,
				Duration: 0,
			},
			MentionSettings: models.MentionSettings{
				AllowMentions:    true,
				OnlyFromContacts: false,
				MentionEveryone:  req.Type == models.ChatTypeGroup,
			},
			ReadReceipts:     true,
			TypingIndicators: true,
			OnlineStatus:     true,
			AllowBackup:      true,
			AllowExport:      true,
		}
	} else {
		chat.Settings = *req.Settings
	}

	// Set before create
	chat.BeforeCreate()

	// Insert into database
	result, err := cs.chatsCollection.InsertOne(ctx, chat)
	if err != nil {
		return nil, fmt.Errorf("failed to insert chat: %w", err)
	}

	chat.ID = result.InsertedID.(primitive.ObjectID)
	return chat, nil
}

// createGroupForChat creates associated group for group chat
func (cs *ChatService) createGroupForChat(chat *models.Chat, creatorID primitive.ObjectID) error {
	if chat.Type != models.ChatTypeGroup {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create group model
	group := &models.Group{
		ChatID:      chat.ID,
		Name:        chat.Name,
		Description: chat.Description,
		Avatar:      chat.Avatar,
		CreatedBy:   creatorID,
		Owner:       creatorID,
	}

	group.BeforeCreate()

	// Add creator as admin
	group.AddMember(creatorID, creatorID, models.GroupRoleOwner)

	// Add other participants as members
	for _, participantID := range chat.Participants {
		if participantID != creatorID {
			group.AddMember(participantID, creatorID, models.GroupRoleMember)
		}
	}

	_, err := cs.groupsCollection.InsertOne(ctx, group)
	return err
}

// Database helper methods

// getChatFromDatabase retrieves chat from database
func (cs *ChatService) getChatFromDatabase(chatID primitive.ObjectID) (*models.Chat, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var chat models.Chat
	err := cs.chatsCollection.FindOne(ctx, bson.M{
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

// updateChatField updates a specific field in chat
func (cs *ChatService) updateChatField(chatID primitive.ObjectID, update bson.M) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := cs.chatsCollection.UpdateOne(ctx, bson.M{"_id": chatID}, update)
	return err
}

// deleteChatFromDatabase deletes chat from database
func (cs *ChatService) deleteChatFromDatabase(chatID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := cs.chatsCollection.DeleteOne(ctx, bson.M{"_id": chatID})
	return err
}

// Permission helper methods

// canUserEditChat checks if user can edit chat
func (cs *ChatService) canUserEditChat(chat *models.Chat, userID primitive.ObjectID) bool {
	if chat.Type == models.ChatTypePrivate {
		return chat.IsParticipant(userID)
	}

	if chat.Type == models.ChatTypeGroup {
		// Check if user is group admin
		return cs.isGroupAdmin(chat.ID, userID)
	}

	return chat.CreatedBy == userID
}

// canUserAddMembers checks if user can add members
func (cs *ChatService) canUserAddMembers(chat *models.Chat, userID primitive.ObjectID) bool {
	if chat.Type == models.ChatTypePrivate {
		return false
	}

	if chat.Settings.OnlyAdminsCanAddMembers {
		return cs.isGroupAdmin(chat.ID, userID)
	}

	return chat.IsParticipant(userID)
}

// canUserRemoveMembers checks if user can remove members
func (cs *ChatService) canUserRemoveMembers(chat *models.Chat, userID primitive.ObjectID) bool {
	if chat.Type == models.ChatTypePrivate {
		return false
	}

	// Use the same permission as adding members
	if chat.Settings.OnlyAdminsCanAddMembers {
		return cs.isGroupAdmin(chat.ID, userID)
	}

	return cs.isGroupAdmin(chat.ID, userID)
}

// canUserDeleteChat checks if user can delete chat
func (cs *ChatService) canUserDeleteChat(chat *models.Chat, userID primitive.ObjectID) bool {
	if chat.Type == models.ChatTypePrivate {
		return false // Private chats cannot be deleted
	}

	return cs.isGroupOwner(chat.ID, userID)
}

// isGroupAdmin checks if user is group admin
func (cs *ChatService) isGroupAdmin(chatID primitive.ObjectID, userID primitive.ObjectID) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	count, err := cs.groupsCollection.CountDocuments(ctx, bson.M{
		"chat_id": chatID,
		"$or": []bson.M{
			{"owner": userID},
			{"admins": userID},
		},
	})
	if err != nil {
		return false
	}

	return count > 0
}

// isGroupOwner checks if user is group owner
func (cs *ChatService) isGroupOwner(chatID primitive.ObjectID, userID primitive.ObjectID) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	count, err := cs.groupsCollection.CountDocuments(ctx, bson.M{
		"chat_id": chatID,
		"owner":   userID,
	})
	if err != nil {
		return false
	}

	return count > 0
}

// buildChatResponse builds chat response with user-specific data
func (cs *ChatService) buildChatResponse(chat *models.Chat, userID primitive.ObjectID) (*models.ChatResponse, error) {
	// Get unread count for user
	unreadCount := chat.GetUnreadCount(userID)
	var unreadCountValue, mentionCount int64
	if unreadCount != nil {
		unreadCountValue = unreadCount.Count
		mentionCount = unreadCount.MentionCount
	}

	// Get participant info
	participantInfo := make([]models.UserPublicInfo, 0, len(chat.Participants))
	for _, participantID := range chat.Participants {
		userInfo := cs.getUserInfo(participantID, userID)
		participantInfo = append(participantInfo, userInfo)
	}

	// Get draft for user
	draft := chat.GetDraft(userID)

	response := &models.ChatResponse{
		Chat:            *chat,
		UnreadCount:     unreadCountValue,
		MentionCount:    mentionCount,
		IsMutedByMe:     chat.IsMutedFor(userID),
		IsArchivedByMe:  chat.IsArchivedFor(userID),
		IsPinnedByMe:    chat.IsPinnedFor(userID),
		IsBlockedByMe:   chat.IsBlockedFor(userID),
		MyDraft:         draft,
		ParticipantInfo: participantInfo,
		CanMessage:      chat.CanUserMessage(userID),
		CanCall:         chat.Settings.AllowVoiceCalls || chat.Settings.AllowVideoCalls,
		CanAddMembers:   cs.canUserAddMembers(chat, userID),
		CanEditInfo:     cs.canUserEditChat(chat, userID),
	}

	return response, nil
}

// getUserInfo gets user public info with privacy considerations
func (cs *ChatService) getUserInfo(userID primitive.ObjectID, requesterID primitive.ObjectID) models.UserPublicInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var user models.User
	err := cs.usersCollection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		return models.UserPublicInfo{
			ID:   userID,
			Name: "Unknown User",
		}
	}

	return user.GetPublicInfo(requesterID)
}

// getUserFromDatabase gets user from database
func (cs *ChatService) getUserFromDatabase(userID primitive.ObjectID) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var user models.User
	err := cs.usersCollection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	return &user, nil
}
func (cs *ChatService) sendPushNotification(user *models.User, title, message string, data map[string]interface{}) error {
	if cs.pushService == nil {
		return fmt.Errorf("push service not available")
	}

	// Convert data from map[string]interface{} to map[string]string
	// since your PushService expects map[string]string
	stringData := make(map[string]string)
	for key, value := range data {
		// Convert interface{} values to strings
		switch v := value.(type) {
		case string:
			stringData[key] = v
		case int:
			stringData[key] = fmt.Sprintf("%d", v)
		case int64:
			stringData[key] = fmt.Sprintf("%d", v)
		case float64:
			stringData[key] = fmt.Sprintf("%.2f", v)
		case bool:
			stringData[key] = fmt.Sprintf("%t", v)
		default:
			// For complex types, convert to JSON string
			if jsonBytes, err := json.Marshal(v); err == nil {
				stringData[key] = string(jsonBytes)
			} else {
				stringData[key] = fmt.Sprintf("%v", v)
			}
		}
	}

	return cs.pushService.SendNotification(user.ID, title, message, stringData)
}

// notifyParticipantsAboutNewChat notifies participants about new chat
func (cs *ChatService) notifyParticipantsAboutNewChat(chat *models.Chat, creatorID primitive.ObjectID) {
	for _, participantID := range chat.Participants {
		if participantID == creatorID {
			continue
		}

		// Send WebSocket notification
		if cs.hub != nil {
			data := map[string]interface{}{
				"chat_id":    chat.ID.Hex(),
				"chat_type":  chat.Type,
				"creator_id": creatorID.Hex(),
				"chat_name":  chat.GetChatName(participantID),
			}
			cs.hub.SendToUser(participantID, "new_chat", data)
		}

		// Send push notification
		if cs.pushService != nil {
			go func(userID primitive.ObjectID) {
				// Get user from database for push notification
				user, err := cs.getUserFromDatabase(userID)
				if err != nil {
					logger.Errorf("Failed to get user for push notification: %v", err)
					return
				}

				creatorName := cs.getUserInfo(creatorID, userID).Name
				title := "New Chat"
				message := fmt.Sprintf("%s added you to a chat", creatorName)

				if chat.Type == models.ChatTypeGroup {
					message = fmt.Sprintf("%s added you to %s", creatorName, chat.Name)
				}

				// Prepare push notification data
				pushData := map[string]string{
					"type":       "new_chat",
					"chat_id":    chat.ID.Hex(),
					"chat_type":  string(chat.Type),
					"creator_id": creatorID.Hex(),
				}

				if chat.Name != "" {
					pushData["chat_name"] = chat.Name
				}

				if err := cs.pushService.SendNotification(user.ID, title, message, pushData); err != nil {
					logger.Errorf("Failed to send push notification: %v", err)
				}
			}(participantID)
		}
	}
}

// notifyParticipantsAboutChatUpdate notifies participants about chat updates
func (cs *ChatService) notifyParticipantsAboutChatUpdate(chat *models.Chat, updaterID primitive.ObjectID, updates bson.M) {
	updaterName := cs.getUserInfo(updaterID, primitive.NilObjectID).Name

	for _, participantID := range chat.Participants {
		// Send WebSocket notification
		if cs.hub != nil {
			data := map[string]interface{}{
				"chat_id":    chat.ID.Hex(),
				"updater_id": updaterID.Hex(),
				"updates":    updates,
			}
			cs.hub.SendToUser(participantID, "chat_updated", data)
		}

		// Send push notification for significant updates
		if cs.pushService != nil && cs.shouldSendPushForUpdate(updates) {
			go func(userID primitive.ObjectID) {
				user, err := cs.getUserFromDatabase(userID)
				if err != nil {
					return
				}

				title := "Chat Updated"
				message := cs.createUpdateMessage(chat, updaterName, updates)

				pushData := map[string]string{
					"type":       "chat_updated",
					"chat_id":    chat.ID.Hex(),
					"updater_id": updaterID.Hex(),
				}

				cs.pushService.SendNotification(user.ID, title, message, pushData)
			}(participantID)
		}
	}
}

// notifyParticipantsAboutMemberChange notifies about member addition/removal
func (cs *ChatService) notifyParticipantsAboutMemberChange(chat *models.Chat, action string, affectedUserID, actionBy primitive.ObjectID) {
	actionByName := cs.getUserInfo(actionBy, primitive.NilObjectID).Name
	affectedUserName := cs.getUserInfo(affectedUserID, primitive.NilObjectID).Name

	for _, participantID := range chat.Participants {
		// Send WebSocket notification
		if cs.hub != nil {
			data := map[string]interface{}{
				"chat_id":          chat.ID.Hex(),
				"action":           action,
				"affected_user_id": affectedUserID.Hex(),
				"action_by":        actionBy.Hex(),
			}
			cs.hub.SendToUser(participantID, "member_change", data)
		}

		// Send push notification
		if cs.pushService != nil && participantID != actionBy {
			go func(userID primitive.ObjectID) {
				user, err := cs.getUserFromDatabase(userID)
				if err != nil {
					return
				}

				title := "Group Updated"
				var message string

				switch action {
				case "member_added":
					message = fmt.Sprintf("%s added %s to %s", actionByName, affectedUserName, chat.Name)
				case "member_removed":
					message = fmt.Sprintf("%s removed %s from %s", actionByName, affectedUserName, chat.Name)
				default:
					message = fmt.Sprintf("Group %s was updated", chat.Name)
				}

				pushData := map[string]string{
					"type":             "member_change",
					"chat_id":          chat.ID.Hex(),
					"action":           action,
					"affected_user_id": affectedUserID.Hex(),
					"action_by":        actionBy.Hex(),
				}

				cs.pushService.SendNotification(user.ID, title, message, pushData)
			}(participantID)
		}
	}
}

// notifyParticipantsAboutChatDeletion notifies about chat deletion
func (cs *ChatService) notifyParticipantsAboutChatDeletion(chat *models.Chat, deletedBy primitive.ObjectID) {
	for _, participantID := range chat.Participants {
		if cs.hub != nil {
			data := map[string]interface{}{
				"chat_id":    chat.ID.Hex(),
				"deleted_by": deletedBy.Hex(),
			}
			cs.hub.SendToUser(participantID, "chat_deleted", data)
		}
	}
}

// statsCollector collects chat statistics
func (cs *ChatService) statsCollector() {
	defer cs.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cs.collectStatistics()
		case <-cs.ctx.Done():
			return
		}
	}
}

// collectStatistics collects and updates statistics
func (cs *ChatService) collectStatistics() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cs.chatStats.mutex.Lock()
	defer cs.chatStats.mutex.Unlock()

	// Count total chats
	total, _ := cs.chatsCollection.CountDocuments(ctx, bson.M{"is_active": true})
	cs.chatStats.TotalChats = total

	// Count by type
	privateChats, _ := cs.chatsCollection.CountDocuments(ctx, bson.M{
		"type":      models.ChatTypePrivate,
		"is_active": true,
	})
	cs.chatStats.PrivateChats = privateChats

	groupChats, _ := cs.chatsCollection.CountDocuments(ctx, bson.M{
		"type":      models.ChatTypeGroup,
		"is_active": true,
	})
	cs.chatStats.GroupChats = groupChats

	// Count active chats (with recent activity)
	activeThreshold := time.Now().Add(-24 * time.Hour)
	activeChats, _ := cs.chatsCollection.CountDocuments(ctx, bson.M{
		"is_active":     true,
		"last_activity": bson.M{"$gte": activeThreshold},
	})
	cs.chatStats.ActiveChats = activeChats

	// Count total messages
	totalMessages, _ := cs.messagesCollection.CountDocuments(ctx, bson.M{"is_deleted": false})
	cs.chatStats.TotalMessages = totalMessages

	// Count messages today
	today := time.Now().Truncate(24 * time.Hour)
	messagesToday, _ := cs.messagesCollection.CountDocuments(ctx, bson.M{
		"created_at": bson.M{"$gte": today},
		"is_deleted": false,
	})
	cs.chatStats.MessagesToday = messagesToday

	cs.chatStats.LastUpdated = time.Now()
}

// draftManager manages draft messages
func (cs *ChatService) draftManager() {
	defer cs.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cs.cleanupOldDrafts()
		case <-cs.ctx.Done():
			return
		}
	}
}

// cleanupOldDrafts removes old draft messages
func (cs *ChatService) cleanupOldDrafts() {
	cs.draftsMutex.Lock()
	defer cs.draftsMutex.Unlock()

	cutoff := time.Now().Add(-1 * time.Hour) // Remove drafts older than 1 hour
	for key, draft := range cs.activeDrafts {
		if draft.UpdatedAt.Before(cutoff) {
			delete(cs.activeDrafts, key)
		}
	}
}

// updateChatStats updates chat statistics
func (cs *ChatService) updateChatStats(eventType string, chatType models.ChatType) {
	cs.chatStats.mutex.Lock()
	defer cs.chatStats.mutex.Unlock()

	switch eventType {
	case "created":
		cs.chatStats.TotalChats++
		if chatType == models.ChatTypePrivate {
			cs.chatStats.PrivateChats++
		} else if chatType == models.ChatTypeGroup {
			cs.chatStats.GroupChats++
		}
	}

	cs.chatStats.LastUpdated = time.Now()
}

// Close gracefully shuts down the chat service
func (cs *ChatService) Close() error {
	logger.Info("Shutting down Chat Service...")

	// Cancel context and wait for goroutines
	cs.cancel()
	cs.wg.Wait()

	// Clear drafts
	cs.draftsMutex.Lock()
	cs.activeDrafts = make(map[string]*models.DraftMessage)
	cs.draftsMutex.Unlock()

	logger.Info("Chat Service shutdown complete")
	return nil
}

// shouldSendPushForUpdate determines if a push notification should be sent for an update
func (cs *ChatService) shouldSendPushForUpdate(updates bson.M) bool {
	// Send push notifications for name, description, or avatar changes
	significantFields := []string{"name", "description", "avatar"}
	for _, field := range significantFields {
		if _, exists := updates[field]; exists {
			return true
		}
	}
	return false
}

// createUpdateMessage creates a human-readable message for chat updates
func (cs *ChatService) createUpdateMessage(chat *models.Chat, updaterName string, updates bson.M) string {
	if _, exists := updates["name"]; exists {
		return fmt.Sprintf("%s changed the group name", updaterName)
	}
	if _, exists := updates["description"]; exists {
		return fmt.Sprintf("%s updated the group description", updaterName)
	}
	if _, exists := updates["avatar"]; exists {
		return fmt.Sprintf("%s changed the group photo", updaterName)
	}

	return fmt.Sprintf("%s updated the group", updaterName)
}
