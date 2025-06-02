package websocket

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Hub maintains the set of active clients and broadcasts messages to clients
type Hub struct {
	// Registered clients by user ID
	clients map[primitive.ObjectID]map[string]*Client

	// Chat rooms - maps chat ID to user IDs in that chat
	chatRooms map[primitive.ObjectID]map[primitive.ObjectID]bool

	// User presence - maps user ID to their online status
	userPresence map[primitive.ObjectID]*UserPresence

	// Typing indicators - maps chat ID to typing users
	typingUsers map[primitive.ObjectID]map[primitive.ObjectID]*TypingStatus

	// Register requests from clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Broadcast message to all clients in a chat
	broadcast chan *BroadcastMessage

	// Direct message to specific user
	directMessage chan *DirectMessage

	// Typing status updates
	typing chan *TypingMessage

	// User presence updates
	presence chan *PresenceUpdate

	// Mutex for thread safety
	mutex sync.RWMutex
}

type Client struct {
	// User ID
	UserID primitive.ObjectID

	// Connection ID (socket ID)
	ConnectionID string

	// Device information
	DeviceID string
	Platform string // "web", "ios", "android"

	// WebSocket connection (interface for different socket implementations)
	Send chan []byte

	// Disconnect callback
	Disconnect func()

	// Last activity
	LastActivity time.Time

	// Connected chats
	JoinedChats map[primitive.ObjectID]bool
}

type UserPresence struct {
	UserID      primitive.ObjectID `json:"user_id"`
	IsOnline    bool               `json:"is_online"`
	LastSeen    time.Time          `json:"last_seen"`
	Platform    string             `json:"platform"`
	DeviceCount int                `json:"device_count"`
}

type TypingStatus struct {
	UserID    primitive.ObjectID `json:"user_id"`
	IsTyping  bool               `json:"is_typing"`
	StartedAt time.Time          `json:"started_at"`
}

type BroadcastMessage struct {
	ChatID      primitive.ObjectID  `json:"chat_id"`
	Type        string              `json:"type"`
	Data        interface{}         `json:"data"`
	ExcludeUser *primitive.ObjectID `json:"exclude_user,omitempty"`
}

type DirectMessage struct {
	UserID primitive.ObjectID `json:"user_id"`
	Type   string             `json:"type"`
	Data   interface{}        `json:"data"`
}

type TypingMessage struct {
	ChatID   primitive.ObjectID `json:"chat_id"`
	UserID   primitive.ObjectID `json:"user_id"`
	IsTyping bool               `json:"is_typing"`
}

type PresenceUpdate struct {
	UserID   primitive.ObjectID `json:"user_id"`
	IsOnline bool               `json:"is_online"`
	Platform string             `json:"platform"`
}

// Event types
const (
	EventMessageReceived     = "message_received"
	EventMessageStatusUpdate = "message_status_update"
	EventUserTyping          = "user_typing"
	EventUserStoppedTyping   = "user_stopped_typing"
	EventUserOnline          = "user_online"
	EventUserOffline         = "user_offline"
	EventChatUpdated         = "chat_updated"
	EventCallIncoming        = "call_incoming"
	EventCallAccepted        = "call_accepted"
	EventCallRejected        = "call_rejected"
	EventCallEnded           = "call_ended"
	EventReactionAdded       = "reaction_added"
	EventReactionRemoved     = "reaction_removed"
	EventMessageEdited       = "message_edited"
	EventMessageDeleted      = "message_deleted"
	EventGroupMemberAdded    = "group_member_added"
	EventGroupMemberRemoved  = "group_member_removed"
	EventGroupInfoUpdated    = "group_info_updated"
)

// NewHub creates a new WebSocket hub
func NewHub() *Hub {
	return &Hub{
		clients:       make(map[primitive.ObjectID]map[string]*Client),
		chatRooms:     make(map[primitive.ObjectID]map[primitive.ObjectID]bool),
		userPresence:  make(map[primitive.ObjectID]*UserPresence),
		typingUsers:   make(map[primitive.ObjectID]map[primitive.ObjectID]*TypingStatus),
		register:      make(chan *Client),
		unregister:    make(chan *Client),
		broadcast:     make(chan *BroadcastMessage),
		directMessage: make(chan *DirectMessage),
		typing:        make(chan *TypingMessage),
		presence:      make(chan *PresenceUpdate),
	}
}

// Run starts the hub
func (h *Hub) Run() {
	// Clean up typing indicators every 30 seconds
	typingCleanupTicker := time.NewTicker(30 * time.Second)
	defer typingCleanupTicker.Stop()

	// Clean up inactive connections every 5 minutes
	inactiveCleanupTicker := time.NewTicker(5 * time.Minute)
	defer inactiveCleanupTicker.Stop()

	for {
		select {
		case client := <-h.register:
			h.registerClient(client)

		case client := <-h.unregister:
			h.unregisterClient(client)

		case message := <-h.broadcast:
			h.broadcastToChat(message)

		case message := <-h.directMessage:
			h.sendDirectMessage(message)

		case typingMsg := <-h.typing:
			h.handleTyping(typingMsg)

		case presence := <-h.presence:
			h.handlePresenceUpdate(presence)

		case <-typingCleanupTicker.C:
			h.cleanupTypingIndicators()

		case <-inactiveCleanupTicker.C:
			h.cleanupInactiveConnections()
		}
	}
}

// RegisterClient registers a new client
func (h *Hub) RegisterClient(userID primitive.ObjectID, connectionID, deviceID, platform string, sendChan chan []byte, disconnectFunc func()) {
	client := &Client{
		UserID:       userID,
		ConnectionID: connectionID,
		DeviceID:     deviceID,
		Platform:     platform,
		Send:         sendChan,
		Disconnect:   disconnectFunc,
		LastActivity: time.Now(),
		JoinedChats:  make(map[primitive.ObjectID]bool),
	}

	h.register <- client
}

// UnregisterClient unregisters a client
func (h *Hub) UnregisterClient(connectionID string) {
	h.mutex.RLock()
	var targetClient *Client
	for _, userClients := range h.clients {
		if client, exists := userClients[connectionID]; exists {
			targetClient = client
			break
		}
	}
	h.mutex.RUnlock()

	if targetClient != nil {
		h.unregister <- targetClient
	}
}

// BroadcastMessage broadcasts a message to all users in a chat
func (h *Hub) BroadcastMessage(senderConnectionID string, data map[string]interface{}) {
	chatIDStr, ok := data["chat_id"].(string)
	if !ok {
		log.Printf("Invalid chat_id in broadcast message")
		return
	}

	chatID, err := primitive.ObjectIDFromHex(chatIDStr)
	if err != nil {
		log.Printf("Invalid chat_id format: %v", err)
		return
	}

	h.mutex.RLock()
	var senderUserID *primitive.ObjectID
	for userID, userClients := range h.clients {
		if _, exists := userClients[senderConnectionID]; exists {
			senderUserID = &userID
			break
		}
	}
	h.mutex.RUnlock()

	message := &BroadcastMessage{
		ChatID:      chatID,
		Type:        EventMessageReceived,
		Data:        data,
		ExcludeUser: senderUserID,
	}

	h.broadcast <- message
}

// BroadcastTyping broadcasts typing status
func (h *Hub) BroadcastTyping(senderConnectionID string, data map[string]interface{}, isTyping bool) {
	chatIDStr, ok := data["chat_id"].(string)
	if !ok {
		return
	}

	chatID, err := primitive.ObjectIDFromHex(chatIDStr)
	if err != nil {
		return
	}

	h.mutex.RLock()
	var senderUserID primitive.ObjectID
	for userID, userClients := range h.clients {
		if _, exists := userClients[senderConnectionID]; exists {
			senderUserID = userID
			break
		}
	}
	h.mutex.RUnlock()

	typingMsg := &TypingMessage{
		ChatID:   chatID,
		UserID:   senderUserID,
		IsTyping: isTyping,
	}

	h.typing <- typingMsg
}

// JoinChat adds a user to a chat room
func (h *Hub) JoinChat(connectionID string, data map[string]interface{}) {
	chatIDStr, ok := data["chat_id"].(string)
	if !ok {
		return
	}

	chatID, err := primitive.ObjectIDFromHex(chatIDStr)
	if err != nil {
		return
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	// Find the client
	var targetClient *Client
	for _, userClients := range h.clients {
		if client, exists := userClients[connectionID]; exists {
			targetClient = client
			break
		}
	}

	if targetClient == nil {
		return
	}

	// Add client to chat room
	if h.chatRooms[chatID] == nil {
		h.chatRooms[chatID] = make(map[primitive.ObjectID]bool)
	}
	h.chatRooms[chatID][targetClient.UserID] = true

	// Add chat to client's joined chats
	targetClient.JoinedChats[chatID] = true
	targetClient.LastActivity = time.Now()

	log.Printf("User %s joined chat %s", targetClient.UserID.Hex(), chatID.Hex())
}

// LeaveChat removes a user from a chat room
func (h *Hub) LeaveChat(connectionID string, data map[string]interface{}) {
	chatIDStr, ok := data["chat_id"].(string)
	if !ok {
		return
	}

	chatID, err := primitive.ObjectIDFromHex(chatIDStr)
	if err != nil {
		return
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	// Find the client
	var targetClient *Client
	for _, userClients := range h.clients {
		if client, exists := userClients[connectionID]; exists {
			targetClient = client
			break
		}
	}

	if targetClient == nil {
		return
	}

	// Remove client from chat room
	if h.chatRooms[chatID] != nil {
		delete(h.chatRooms[chatID], targetClient.UserID)
		if len(h.chatRooms[chatID]) == 0 {
			delete(h.chatRooms, chatID)
		}
	}

	// Remove chat from client's joined chats
	delete(targetClient.JoinedChats, chatID)

	log.Printf("User %s left chat %s", targetClient.UserID.Hex(), chatID.Hex())
}

// SendToUser sends a message to a specific user
func (h *Hub) SendToUser(userID primitive.ObjectID, eventType string, data interface{}) {
	message := &DirectMessage{
		UserID: userID,
		Type:   eventType,
		Data:   data,
	}

	h.directMessage <- message
}

// SendToChat sends a message to all users in a chat
func (h *Hub) SendToChat(chatID primitive.ObjectID, eventType string, data interface{}, excludeUser *primitive.ObjectID) {
	message := &BroadcastMessage{
		ChatID:      chatID,
		Type:        eventType,
		Data:        data,
		ExcludeUser: excludeUser,
	}

	h.broadcast <- message
}

// GetUserPresence returns user presence information
func (h *Hub) GetUserPresence(userID primitive.ObjectID) *UserPresence {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if presence, exists := h.userPresence[userID]; exists {
		return presence
	}

	return &UserPresence{
		UserID:   userID,
		IsOnline: false,
		LastSeen: time.Now().Add(-24 * time.Hour), // Default to 24 hours ago
	}
}

// GetOnlineUsers returns list of online users in a chat
func (h *Hub) GetOnlineUsers(chatID primitive.ObjectID) []primitive.ObjectID {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	var onlineUsers []primitive.ObjectID

	if chatUsers, exists := h.chatRooms[chatID]; exists {
		for userID := range chatUsers {
			if presence, exists := h.userPresence[userID]; exists && presence.IsOnline {
				onlineUsers = append(onlineUsers, userID)
			}
		}
	}

	return onlineUsers
}

// IsUserOnline checks if a user is online
func (h *Hub) IsUserOnline(userID primitive.ObjectID) bool {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if presence, exists := h.userPresence[userID]; exists {
		return presence.IsOnline
	}

	return false
}

// Private methods

func (h *Hub) registerClient(client *Client) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.clients[client.UserID] == nil {
		h.clients[client.UserID] = make(map[string]*Client)
	}
	h.clients[client.UserID][client.ConnectionID] = client

	// Update user presence
	deviceCount := len(h.clients[client.UserID])
	h.userPresence[client.UserID] = &UserPresence{
		UserID:      client.UserID,
		IsOnline:    true,
		LastSeen:    time.Now(),
		Platform:    client.Platform,
		DeviceCount: deviceCount,
	}

	// Notify contacts about user coming online
	go h.notifyPresenceChange(client.UserID, true)

	log.Printf("Client registered: %s (User: %s, Platform: %s)",
		client.ConnectionID, client.UserID.Hex(), client.Platform)
}

func (h *Hub) unregisterClient(client *Client) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if userClients, exists := h.clients[client.UserID]; exists {
		if _, exists := userClients[client.ConnectionID]; exists {
			delete(userClients, client.ConnectionID)
			close(client.Send)

			// Remove user from all chat rooms for this connection
			for chatID := range client.JoinedChats {
				if h.chatRooms[chatID] != nil {
					// Only remove if this was the last connection for this user
					if len(userClients) == 0 {
						delete(h.chatRooms[chatID], client.UserID)
						if len(h.chatRooms[chatID]) == 0 {
							delete(h.chatRooms, chatID)
						}
					}
				}
			}

			// Update user presence
			if len(userClients) == 0 {
				delete(h.clients, client.UserID)
				// User is completely offline
				h.userPresence[client.UserID] = &UserPresence{
					UserID:      client.UserID,
					IsOnline:    false,
					LastSeen:    time.Now(),
					Platform:    client.Platform,
					DeviceCount: 0,
				}

				// Notify contacts about user going offline
				go h.notifyPresenceChange(client.UserID, false)
			} else {
				// User still has other connections
				h.userPresence[client.UserID].DeviceCount = len(userClients)
			}
		}
	}

	log.Printf("Client unregistered: %s (User: %s)",
		client.ConnectionID, client.UserID.Hex())
}

func (h *Hub) broadcastToChat(message *BroadcastMessage) {
	h.mutex.RLock()
	chatUsers := h.chatRooms[message.ChatID]
	h.mutex.RUnlock()

	if chatUsers == nil {
		return
	}

	messageBytes, err := json.Marshal(map[string]interface{}{
		"type": message.Type,
		"data": message.Data,
	})
	if err != nil {
		log.Printf("Error marshaling broadcast message: %v", err)
		return
	}

	h.mutex.RLock()
	for userID := range chatUsers {
		// Skip excluded user
		if message.ExcludeUser != nil && *message.ExcludeUser == userID {
			continue
		}

		if userClients, exists := h.clients[userID]; exists {
			for _, client := range userClients {
				select {
				case client.Send <- messageBytes:
				default:
					// Client's send channel is blocked, close it
					close(client.Send)
					delete(userClients, client.ConnectionID)
				}
			}
		}
	}
	h.mutex.RUnlock()
}

func (h *Hub) sendDirectMessage(message *DirectMessage) {
	messageBytes, err := json.Marshal(map[string]interface{}{
		"type": message.Type,
		"data": message.Data,
	})
	if err != nil {
		log.Printf("Error marshaling direct message: %v", err)
		return
	}

	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if userClients, exists := h.clients[message.UserID]; exists {
		for _, client := range userClients {
			select {
			case client.Send <- messageBytes:
			default:
				close(client.Send)
				delete(userClients, client.ConnectionID)
			}
		}
	}
}

func (h *Hub) handleTyping(typingMsg *TypingMessage) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.typingUsers[typingMsg.ChatID] == nil {
		h.typingUsers[typingMsg.ChatID] = make(map[primitive.ObjectID]*TypingStatus)
	}

	if typingMsg.IsTyping {
		h.typingUsers[typingMsg.ChatID][typingMsg.UserID] = &TypingStatus{
			UserID:    typingMsg.UserID,
			IsTyping:  true,
			StartedAt: time.Now(),
		}
	} else {
		delete(h.typingUsers[typingMsg.ChatID], typingMsg.UserID)
	}

	// Broadcast typing status to other users in the chat
	eventType := EventUserTyping
	if !typingMsg.IsTyping {
		eventType = EventUserStoppedTyping
	}

	go h.SendToChat(typingMsg.ChatID, eventType, map[string]interface{}{
		"user_id":   typingMsg.UserID,
		"is_typing": typingMsg.IsTyping,
	}, &typingMsg.UserID)
}

func (h *Hub) handlePresenceUpdate(update *PresenceUpdate) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.userPresence[update.UserID] == nil {
		h.userPresence[update.UserID] = &UserPresence{
			UserID: update.UserID,
		}
	}

	h.userPresence[update.UserID].IsOnline = update.IsOnline
	h.userPresence[update.UserID].LastSeen = time.Now()
	h.userPresence[update.UserID].Platform = update.Platform
}

func (h *Hub) cleanupTypingIndicators() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	now := time.Now()
	for chatID, typingUsers := range h.typingUsers {
		for userID, status := range typingUsers {
			// Remove typing indicators older than 10 seconds
			if now.Sub(status.StartedAt) > 10*time.Second {
				delete(typingUsers, userID)

				// Notify that user stopped typing
				go h.SendToChat(chatID, EventUserStoppedTyping, map[string]interface{}{
					"user_id":   userID,
					"is_typing": false,
				}, &userID)
			}
		}

		// Remove empty chat typing maps
		if len(typingUsers) == 0 {
			delete(h.typingUsers, chatID)
		}
	}
}

func (h *Hub) cleanupInactiveConnections() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	now := time.Now()
	for userID, userClients := range h.clients {
		for connectionID, client := range userClients {
			// Remove connections inactive for more than 30 minutes
			if now.Sub(client.LastActivity) > 30*time.Minute {
				close(client.Send)
				delete(userClients, connectionID)

				log.Printf("Cleaned up inactive connection: %s", connectionID)
			}
		}

		// Remove empty user client maps
		if len(userClients) == 0 {
			delete(h.clients, userID)

			// Update presence
			if presence, exists := h.userPresence[userID]; exists {
				presence.IsOnline = false
				presence.LastSeen = now
				presence.DeviceCount = 0
			}
		}
	}
}

func (h *Hub) notifyPresenceChange(userID primitive.ObjectID, isOnline bool) {
	// This would typically query the database to get user's contacts
	// and notify them about the presence change
	// Implementation depends on your user/contact service

	eventType := EventUserOnline
	if !isOnline {
		eventType = EventUserOffline
	}

	// For now, we'll just log it
	log.Printf("User %s is now %s", userID.Hex(), map[bool]string{true: "online", false: "offline"}[isOnline])
}
