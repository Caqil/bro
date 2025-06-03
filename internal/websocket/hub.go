package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"bro/internal/models"
	"bro/internal/utils"
	"bro/pkg/logger"
	"bro/pkg/redis"
)

// WSMessage represents a websocket message
type WSMessage struct {
	Type       string             `json:"type"`
	Data       interface{}        `json:"data"`
	ID         string             `json:"id,omitempty"`
	SenderID   string             `json:"sender_id,omitempty"`
	ChatID     string             `json:"chat_id,omitempty"`
	Timestamp  int64              `json:"timestamp"`
	Recipients *MessageRecipients `json:"recipients,omitempty"`
}

// MessageRecipients defines who should receive the message
type MessageRecipients struct {
	Users     []primitive.ObjectID `json:"users,omitempty"`
	Chats     []primitive.ObjectID `json:"chats,omitempty"`
	Broadcast bool                 `json:"broadcast,omitempty"`
}

// WebRTCSignalingServer interface to avoid circular imports
type WebRTCSignalingServer interface {
	HandleSignalingMessage(client *Client, message *WSMessage) error
}

// Hub maintains the set of active clients and broadcasts messages to the clients
type Hub struct {
	// Registered clients
	clients map[*Client]bool

	// User ID to clients mapping for quick lookup
	userClients map[primitive.ObjectID]map[*Client]bool

	// Chat ID to clients mapping for chat rooms
	chatClients map[primitive.ObjectID]map[*Client]bool

	// WebSocket room management (for calls and other grouped activities)
	rooms map[string]map[*Client]bool

	// Inbound messages from the clients
	broadcast chan *WSMessage

	// Register requests from the clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Message handlers
	messageHandlers map[string]MessageHandler

	// WebRTC signaling server reference
	signalingServer WebRTCSignalingServer

	// Redis client for distributed messaging
	redisClient *redis.Client

	// Hub statistics
	stats *HubStatistics

	// Services dependencies
	jwtSecret string

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Mutex for thread-safe operations
	mutex sync.RWMutex

	// Configuration
	config *HubConfig
}

// HubConfig contains hub configuration
type HubConfig struct {
	MaxClientsPerUser int
	HeartbeatInterval time.Duration
	CleanupInterval   time.Duration
	MessageTimeout    time.Duration
	EnableMetrics     bool
}

// HubStatistics contains hub statistics
type HubStatistics struct {
	TotalClients      int
	TotalUsers        int
	TotalChats        int
	TotalRooms        int
	MessagesProcessed int64
	LastUpdated       time.Time
	mutex             sync.RWMutex
}

// NewHub creates a new WebSocket hub
func NewHub(redisClient *redis.Client, jwtSecret string, config *HubConfig) *Hub {
	if config == nil {
		config = DefaultHubConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	hub := &Hub{
		clients:         make(map[*Client]bool),
		userClients:     make(map[primitive.ObjectID]map[*Client]bool),
		chatClients:     make(map[primitive.ObjectID]map[*Client]bool),
		rooms:           make(map[string]map[*Client]bool),
		broadcast:       make(chan *WSMessage, 256),
		register:        make(chan *Client),
		unregister:      make(chan *Client),
		messageHandlers: make(map[string]MessageHandler),
		redisClient:     redisClient,
		stats:           &HubStatistics{},
		jwtSecret:       jwtSecret,
		ctx:             ctx,
		cancel:          cancel,
		config:          config,
	}

	// Register default message handlers
	hub.registerDefaultHandlers()

	return hub
}

// DefaultHubConfig returns default hub configuration
func DefaultHubConfig() *HubConfig {
	return &HubConfig{
		MaxClientsPerUser: 5,
		HeartbeatInterval: 30 * time.Second,
		CleanupInterval:   5 * time.Minute,
		MessageTimeout:    10 * time.Second,
		EnableMetrics:     true,
	}
}

// SetSignalingServer sets the WebRTC signaling server
func (h *Hub) SetSignalingServer(server WebRTCSignalingServer) {
	h.signalingServer = server

	// Update the CallSignalingHandler with the server reference
	if handler, exists := h.messageHandlers["call_signal"]; exists {
		if callHandler, ok := handler.(*CallSignalingHandler); ok {
			callHandler.SetSignalingServer(server)
		}
	}
}

// Run starts the hub and handles client registration/unregistration and message broadcasting
func (h *Hub) Run() {
	logger.Info("WebSocket Hub starting...")

	// Start background processes
	h.wg.Add(3)
	go h.statsCollector()
	go h.heartbeatManager()
	go h.cleanupManager()

	// Subscribe to Redis for distributed messaging
	if h.redisClient != nil {
		h.wg.Add(1)
		go h.subscribeToRedis()
	}

	// Main hub loop
	for {
		select {
		case client := <-h.register:
			h.registerClient(client)

		case client := <-h.unregister:
			h.unregisterClient(client)

		case message := <-h.broadcast:
			h.broadcastMessage(message)

		case <-h.ctx.Done():
			logger.Info("WebSocket Hub shutting down...")
			h.cleanup()
			return
		}
	}
}

// RegisterMessageHandler registers a message handler
func (h *Hub) RegisterMessageHandler(handler MessageHandler) {
	h.messageHandlers[handler.GetType()] = handler
	logger.Infof("Registered message handler for type: %s", handler.GetType())
}

// RegisterClient registers a new client
func (h *Hub) RegisterClient(client *Client) {
	select {
	case h.register <- client:
	case <-h.ctx.Done():
		client.Close()
	}
}

// UnregisterClient unregisters a client
func (h *Hub) UnregisterClient(client *Client) {
	select {
	case h.unregister <- client:
	case <-h.ctx.Done():
	}
}

// BroadcastMessage broadcasts a message
func (h *Hub) BroadcastMessage(message *WSMessage) {
	select {
	case h.broadcast <- message:
	case <-h.ctx.Done():
	}
}

// SendToUser sends a message to specific user
func (h *Hub) SendToUser(userID primitive.ObjectID, messageType string, data interface{}) {
	message := &WSMessage{
		Type:      messageType,
		Data:      data,
		Timestamp: time.Now().Unix(),
		ID:        primitive.NewObjectID().Hex(),
		Recipients: &MessageRecipients{
			Users: []primitive.ObjectID{userID},
		},
	}

	h.BroadcastMessage(message)
}

// SendToUsers sends a message to multiple users
func (h *Hub) SendToUsers(userIDs []primitive.ObjectID, messageType string, data interface{}) {
	message := &WSMessage{
		Type:      messageType,
		Data:      data,
		Timestamp: time.Now().Unix(),
		ID:        primitive.NewObjectID().Hex(),
		Recipients: &MessageRecipients{
			Users: userIDs,
		},
	}

	h.BroadcastMessage(message)
}

// SendToChat sends a message to all clients in a chat
func (h *Hub) SendToChat(chatID primitive.ObjectID, messageType string, data interface{}) {
	message := &WSMessage{
		Type:      messageType,
		Data:      data,
		ChatID:    chatID.Hex(),
		Timestamp: time.Now().Unix(),
		ID:        primitive.NewObjectID().Hex(),
		Recipients: &MessageRecipients{
			Chats: []primitive.ObjectID{chatID},
		},
	}

	h.BroadcastMessage(message)
}

// SendToRoom sends a message to all clients in a WebSocket room
func (h *Hub) SendToRoom(roomID string, messageType string, data interface{}) {
	h.mutex.RLock()
	clients, exists := h.rooms[roomID]
	h.mutex.RUnlock()

	if !exists {
		return
	}

	message := &WSMessage{
		Type:      messageType,
		Data:      data,
		Timestamp: time.Now().Unix(),
		ID:        primitive.NewObjectID().Hex(),
	}

	messageData, err := json.Marshal(message)
	if err != nil {
		logger.Errorf("Failed to marshal room message: %v", err)
		return
	}

	for client := range clients {
		select {
		case client.send <- messageData:
		default:
			logger.Warnf("Client send channel full, dropping message for client %s", client.GetConnID())
		}
	}
}

// SendToAll sends a message to all connected clients
func (h *Hub) SendToAll(messageType string, data interface{}) {
	message := &WSMessage{
		Type:      messageType,
		Data:      data,
		Timestamp: time.Now().Unix(),
		ID:        primitive.NewObjectID().Hex(),
		Recipients: &MessageRecipients{
			Broadcast: true,
		},
	}

	h.BroadcastMessage(message)
}

// JoinChat adds a client to a chat room
func (h *Hub) JoinChat(client *Client, chatID primitive.ObjectID) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.chatClients[chatID] == nil {
		h.chatClients[chatID] = make(map[*Client]bool)
	}
	h.chatClients[chatID][client] = true

	logger.Debugf("Client %s joined chat %s", client.GetConnID(), chatID.Hex())

	// Set active chat for the client
	client.SetActiveChat(chatID, true)
}

// LeaveChat removes a client from a chat room
func (h *Hub) LeaveChat(client *Client, chatID primitive.ObjectID) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if clients, exists := h.chatClients[chatID]; exists {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.chatClients, chatID)
		}
	}

	logger.Debugf("Client %s left chat %s", client.GetConnID(), chatID.Hex())

	// Remove active chat for the client
	client.SetActiveChat(chatID, false)
}

// JoinRoom adds a client to a WebSocket room (for calls, etc.)
func (h *Hub) JoinRoom(client *Client, roomID string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.rooms[roomID] == nil {
		h.rooms[roomID] = make(map[*Client]bool)
	}
	h.rooms[roomID][client] = true

	logger.Debugf("Client %s joined room %s", client.GetConnID(), roomID)
}

// LeaveRoom removes a client from a WebSocket room
func (h *Hub) LeaveRoom(client *Client, roomID string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if clients, exists := h.rooms[roomID]; exists {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.rooms, roomID)
		}
	}

	logger.Debugf("Client %s left room %s", client.GetConnID(), roomID)
}

// GetUserClients returns all clients for a user
func (h *Hub) GetUserClients(userID primitive.ObjectID) []*Client {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	clients := make([]*Client, 0)
	if userClients, exists := h.userClients[userID]; exists {
		for client := range userClients {
			clients = append(clients, client)
		}
	}
	return clients
}

// GetChatClients returns all clients in a chat
func (h *Hub) GetChatClients(chatID primitive.ObjectID) []*Client {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	clients := make([]*Client, 0)
	if chatClients, exists := h.chatClients[chatID]; exists {
		for client := range chatClients {
			clients = append(clients, client)
		}
	}
	return clients
}

// GetRoomClients returns all clients in a room
func (h *Hub) GetRoomClients(roomID string) []*Client {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	clients := make([]*Client, 0)
	if roomClients, exists := h.rooms[roomID]; exists {
		for client := range roomClients {
			clients = append(clients, client)
		}
	}
	return clients
}

// IsUserOnline checks if a user is online
func (h *Hub) IsUserOnline(userID primitive.ObjectID) bool {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	userClients, exists := h.userClients[userID]
	return exists && len(userClients) > 0
}

// GetOnlineUsers returns list of online user IDs
func (h *Hub) GetOnlineUsers() []primitive.ObjectID {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	users := make([]primitive.ObjectID, 0, len(h.userClients))
	for userID := range h.userClients {
		users = append(users, userID)
	}
	return users
}

// GetStatistics returns hub statistics
func (h *Hub) GetStatistics() *HubStatistics {
	h.stats.mutex.RLock()
	defer h.stats.mutex.RUnlock()

	stats := *h.stats
	return &stats
}

// ProcessMessage processes an incoming message from a client
func (h *Hub) ProcessMessage(client *Client, message *WSMessage) error {
	// Special handling for WebRTC signaling messages
	if message.Type == "call_signal" && h.signalingServer != nil {
		return h.signalingServer.HandleSignalingMessage(client, message)
	}

	// Find and execute message handler
	handler, exists := h.messageHandlers[message.Type]
	if !exists {
		logger.Warnf("No handler found for message type: %s", message.Type)
		return client.SendError("UNKNOWN_MESSAGE_TYPE", "Unknown message type", fmt.Sprintf("No handler for type: %s", message.Type))
	}

	// Execute handler
	if err := handler.HandleMessage(client, message); err != nil {
		logger.Errorf("Message handler error for type %s: %v", message.Type, err)
		return client.SendError("HANDLER_ERROR", "Message processing failed", err.Error())
	}

	// Update statistics
	h.updateMessageStats()

	return nil
}

// Internal methods

// registerClient handles client registration
func (h *Hub) registerClient(client *Client) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	// Add to main clients map
	h.clients[client] = true

	logger.Infof("Client registered: %s (IP: %s)", client.GetConnID(), client.remoteAddr)

	// Send welcome message
	client.SendJSON("connection_established", map[string]interface{}{
		"conn_id":      client.GetConnID(),
		"server_time":  time.Now(),
		"capabilities": []string{"messaging", "calls", "file_transfer", "presence", "typing_indicators"},
	})
}

// registerAuthenticatedClient handles authenticated client registration
func (h *Hub) registerAuthenticatedClient(client *Client) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	userID := client.GetUserID()

	// Check if user has too many connections
	if userClients, exists := h.userClients[userID]; exists {
		if len(userClients) >= h.config.MaxClientsPerUser {
			logger.Warnf("User %s has too many connections (%d), closing oldest",
				userID.Hex(), len(userClients))

			// Close oldest client
			var oldestClient *Client
			var oldestTime time.Time = time.Now()
			for c := range userClients {
				if c.connectedAt.Before(oldestTime) {
					oldestClient = c
					oldestTime = c.connectedAt
				}
			}
			if oldestClient != nil {
				oldestClient.Close()
			}
		}
	}

	// Add to user clients mapping
	if h.userClients[userID] == nil {
		h.userClients[userID] = make(map[*Client]bool)
	}
	h.userClients[userID][client] = true

	// Update user online status in Redis
	if h.redisClient != nil {
		h.redisClient.SetUserOnline(userID.Hex(), client.platform, 5*time.Minute)
	}

	logger.Infof("Client authenticated: %s (User: %s, Platform: %s)",
		client.GetConnID(), userID.Hex(), client.platform)

	// Notify user's contacts about online status
	h.notifyContactsAboutPresence(userID, true)

	// Send authentication success
	client.SendJSON("authentication_success", map[string]interface{}{
		"user_id":  userID.Hex(),
		"platform": client.platform,
		"features": []string{"messaging", "calls", "file_transfer", "presence", "typing_indicators"},
	})
}

// unregisterClient handles client unregistration
func (h *Hub) unregisterClient(client *Client) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	// Remove from main clients map
	if _, exists := h.clients[client]; exists {
		delete(h.clients, client)
	}

	// Remove from user clients mapping if authenticated
	if client.IsAuthenticated() {
		userID := client.GetUserID()
		if userClients, exists := h.userClients[userID]; exists {
			delete(userClients, client)
			if len(userClients) == 0 {
				delete(h.userClients, userID)

				// Update user offline status in Redis
				if h.redisClient != nil {
					h.redisClient.SetUserOffline(userID.Hex())
				}

				// Notify contacts about offline status
				h.notifyContactsAboutPresence(userID, false)
			}
		}

		// Remove from all chat rooms
		for chatID, chatClients := range h.chatClients {
			if _, exists := chatClients[client]; exists {
				delete(chatClients, client)
				if len(chatClients) == 0 {
					delete(h.chatClients, chatID)
				}
			}
		}

		// Remove from all WebSocket rooms
		for roomID, roomClients := range h.rooms {
			if _, exists := roomClients[client]; exists {
				delete(roomClients, client)
				if len(roomClients) == 0 {
					delete(h.rooms, roomID)
				}
			}
		}
	}

	logger.Infof("Client unregistered: %s", client.GetConnID())
}

// broadcastMessage handles message broadcasting
func (h *Hub) broadcastMessage(message *WSMessage) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	recipients := message.Recipients
	if recipients == nil {
		return
	}

	messageData, err := json.Marshal(message)
	if err != nil {
		logger.Errorf("Failed to marshal broadcast message: %v", err)
		return
	}

	// Broadcast to all clients
	if recipients.Broadcast {
		for client := range h.clients {
			select {
			case client.send <- messageData:
			default:
				logger.Warnf("Client send channel full, dropping message for client %s", client.GetConnID())
			}
		}
		return
	}

	// Send to specific users
	for _, userID := range recipients.Users {
		if userClients, exists := h.userClients[userID]; exists {
			for client := range userClients {
				select {
				case client.send <- messageData:
				default:
					logger.Warnf("Client send channel full, dropping message for client %s", client.GetConnID())
				}
			}
		}
	}

	// Send to specific chats
	for _, chatID := range recipients.Chats {
		if chatClients, exists := h.chatClients[chatID]; exists {
			for client := range chatClients {
				select {
				case client.send <- messageData:
				default:
					logger.Warnf("Client send channel full, dropping message for client %s", client.GetConnID())
				}
			}
		}
	}

	// Publish to Redis for distributed messaging
	if h.redisClient != nil && (len(recipients.Users) > 0 || len(recipients.Chats) > 0) {
		h.publishToRedis(message)
	}
}

// notifyContactsAboutPresence notifies user's contacts about presence change
func (h *Hub) notifyContactsAboutPresence(userID primitive.ObjectID, isOnline bool) {
	// Create presence notification
	status := "offline"
	if isOnline {
		status = "online"
	}

	// Broadcast presence change to all users (they can filter on client side)
	// In a production app, you'd get user's contacts from database and only notify them
	h.SendToAll("presence_change", map[string]interface{}{
		"user_id":   userID.Hex(),
		"is_online": isOnline,
		"status":    status,
		"timestamp": time.Now().Unix(),
	})

	logger.Debugf("User %s is now %s", userID.Hex(), status)
}

// broadcastPresenceChange broadcasts presence change to relevant users
func (h *Hub) broadcastPresenceChange(userID primitive.ObjectID, isOnline bool, deviceID, platform string) {
	h.SendToAll("presence_update", map[string]interface{}{
		"user_id":   userID.Hex(),
		"is_online": isOnline,
		"device_id": deviceID,
		"platform":  platform,
		"timestamp": time.Now().Unix(),
	})
}

// publishToRedis publishes message to Redis for distributed messaging
func (h *Hub) publishToRedis(message *WSMessage) {
	if h.redisClient == nil {
		return
	}

	messageData, err := json.Marshal(message)
	if err != nil {
		logger.Errorf("Failed to marshal message for Redis: %v", err)
		return
	}

	channel := "websocket:broadcast"
	if err := h.redisClient.Publish(channel, string(messageData)); err != nil {
		logger.Errorf("Failed to publish message to Redis: %v", err)
	}
}

// subscribeToRedis subscribes to Redis for distributed messaging
func (h *Hub) subscribeToRedis() {
	defer h.wg.Done()

	channel := "websocket:broadcast"
	subscription := h.redisClient.Subscribe(channel)
	defer subscription.Close()

	logger.Infof("Subscribed to Redis channel: %s", channel)

	for {
		select {
		case msg := <-subscription.Channel():
			var message WSMessage
			if err := json.Unmarshal([]byte(msg.Payload), &message); err != nil {
				logger.Errorf("Failed to unmarshal Redis message: %v", err)
				continue
			}

			// Re-broadcast the message locally
			h.broadcastMessage(&message)

		case <-h.ctx.Done():
			return
		}
	}
}

// authenticateClient authenticates a client with JWT token
func (h *Hub) authenticateClient(client *Client, token string) error {
	// Remove "Bearer " prefix if present
	if strings.HasPrefix(token, "Bearer ") {
		token = strings.TrimPrefix(token, "Bearer ")
	}

	// Validate JWT token
	claims, err := utils.ValidateToken(token, h.jwtSecret)
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	// Parse user ID
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return fmt.Errorf("invalid user ID: %w", err)
	}

	// Create a basic user object (in production, you might fetch from database)
	user := &models.User{
		ID:          userID,
		PhoneNumber: claims.PhoneNumber,
		Role:        models.UserRole(claims.Role),
	}

	// Set user information
	client.SetUser(user, claims.ID, claims.DeviceID, claims.PhoneNumber) // Using PhoneNumber as platform temporarily

	// Register authenticated client
	h.registerAuthenticatedClient(client)

	return nil
}

// registerDefaultHandlers registers default message handlers
func (h *Hub) registerDefaultHandlers() {
	// Register default handlers
	h.RegisterMessageHandler(&AuthHandler{hub: h})
	h.RegisterMessageHandler(&PingHandler{})
	h.RegisterMessageHandler(&PresenceHandler{hub: h})
	h.RegisterMessageHandler(&TypingHandler{hub: h})
	h.RegisterMessageHandler(&JoinChatHandler{hub: h})
	h.RegisterMessageHandler(&LeaveChatHandler{hub: h})
	h.RegisterMessageHandler(&CallSignalingHandler{hub: h}) // This will be updated with signaling server reference
	h.RegisterMessageHandler(&ChatMessageHandler{hub: h})
	h.RegisterMessageHandler(&StatusHandler{hub: h})
	h.RegisterMessageHandler(&ReactionHandler{hub: h})
	h.RegisterMessageHandler(&ReadReceiptHandler{hub: h})
}

// Background processes

// statsCollector collects and updates hub statistics
func (h *Hub) statsCollector() {
	defer h.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.updateStatistics()
		case <-h.ctx.Done():
			return
		}
	}
}

// updateStatistics updates hub statistics
func (h *Hub) updateStatistics() {
	h.mutex.RLock()
	totalClients := len(h.clients)
	totalUsers := len(h.userClients)
	totalChats := len(h.chatClients)
	totalRooms := len(h.rooms)
	h.mutex.RUnlock()

	h.stats.mutex.Lock()
	h.stats.TotalClients = totalClients
	h.stats.TotalUsers = totalUsers
	h.stats.TotalChats = totalChats
	h.stats.TotalRooms = totalRooms
	h.stats.LastUpdated = time.Now()
	h.stats.mutex.Unlock()
}

// updateMessageStats updates message statistics
func (h *Hub) updateMessageStats() {
	h.stats.mutex.Lock()
	h.stats.MessagesProcessed++
	h.stats.mutex.Unlock()
}

// heartbeatManager manages client heartbeats
func (h *Hub) heartbeatManager() {
	defer h.wg.Done()

	ticker := time.NewTicker(h.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.sendHeartbeats()
		case <-h.ctx.Done():
			return
		}
	}
}

// sendHeartbeats sends heartbeat messages to all clients
func (h *Hub) sendHeartbeats() {
	h.mutex.RLock()
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mutex.RUnlock()

	for _, client := range clients {
		client.SendJSON("heartbeat", map[string]interface{}{
			"timestamp": time.Now().Unix(),
		})
	}
}

// cleanupManager performs periodic cleanup
func (h *Hub) cleanupManager() {
	defer h.wg.Done()

	ticker := time.NewTicker(h.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.performCleanup()
		case <-h.ctx.Done():
			return
		}
	}
}

// performCleanup performs cleanup of stale connections and data
func (h *Hub) performCleanup() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	staleClients := make([]*Client, 0)
	cutoff := time.Now().Add(-5 * time.Minute)

	// Find stale clients
	for client := range h.clients {
		if client.lastPing.Before(cutoff) || !client.IsHealthy() {
			staleClients = append(staleClients, client)
		}
	}

	// Remove stale clients
	for _, client := range staleClients {
		logger.Warnf("Removing stale client: %s", client.GetConnID())
		h.unregisterClient(client)
		client.Close()
	}

	// Clean up empty chat rooms
	for chatID, clients := range h.chatClients {
		if len(clients) == 0 {
			delete(h.chatClients, chatID)
		}
	}

	// Clean up empty WebSocket rooms
	for roomID, clients := range h.rooms {
		if len(clients) == 0 {
			delete(h.rooms, roomID)
		}
	}
}

// cleanup performs final cleanup when shutting down
func (h *Hub) cleanup() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	// Close all client connections
	for client := range h.clients {
		client.Close()
	}

	// Wait for background processes to finish
	h.wg.Wait()

	logger.Info("WebSocket Hub cleanup complete")
}

// Stop gracefully shuts down the hub
func (h *Hub) Stop() {
	h.cancel()
}
