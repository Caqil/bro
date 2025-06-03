package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"bro/pkg/logger"
	"bro/pkg/redis"
)

// Hub maintains the set of active clients and broadcasts messages to the clients
type Hub struct {
	// Registered clients
	clients map[*Client]bool

	// User ID to clients mapping for quick lookup
	userClients map[primitive.ObjectID]map[*Client]bool

	// Room to clients mapping for group messaging
	roomClients map[string]map[*Client]bool

	// Inbound messages from the clients
	broadcast chan *Message

	// Register requests from the clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Message handlers
	messageHandlers map[string]MessageHandler

	// Redis client for distributed messaging
	redisClient *redis.Client

	// Hub statistics
	stats *HubStatistics

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
	EnableCompression bool
	EnableMetrics     bool
}

// HubStatistics contains hub statistics
type HubStatistics struct {
	TotalClients      int
	TotalUsers        int
	TotalRooms        int
	MessagesProcessed int64
	LastUpdated       time.Time
	mutex             sync.RWMutex
}

// MessageHandler defines the interface for message handlers
type MessageHandler interface {
	HandleMessage(client *Client, message *Message) error
	GetType() string
}

// NewHub creates a new WebSocket hub
func NewHub(redisClient *redis.Client, config *HubConfig) *Hub {
	if config == nil {
		config = DefaultHubConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	hub := &Hub{
		clients:         make(map[*Client]bool),
		userClients:     make(map[primitive.ObjectID]map[*Client]bool),
		roomClients:     make(map[string]map[*Client]bool),
		broadcast:       make(chan *Message, 256),
		register:        make(chan *Client),
		unregister:      make(chan *Client),
		messageHandlers: make(map[string]MessageHandler),
		redisClient:     redisClient,
		stats:           &HubStatistics{},
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
		EnableCompression: true,
		EnableMetrics:     true,
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
func (h *Hub) BroadcastMessage(message *Message) {
	select {
	case h.broadcast <- message:
	case <-h.ctx.Done():
	}
}

// SendToUser sends a message to specific user
func (h *Hub) SendToUser(userID primitive.ObjectID, messageType string, data interface{}) {
	message := &Message{
		Type:      messageType,
		Data:      data,
		Timestamp: time.Now(),
		Recipients: &MessageRecipients{
			Users: []primitive.ObjectID{userID},
		},
	}

	h.BroadcastMessage(message)
}

// SendToUsers sends a message to multiple users
func (h *Hub) SendToUsers(userIDs []primitive.ObjectID, messageType string, data interface{}) {
	message := &Message{
		Type:      messageType,
		Data:      data,
		Timestamp: time.Now(),
		Recipients: &MessageRecipients{
			Users: userIDs,
		},
	}

	h.BroadcastMessage(message)
}

// SendToRoom sends a message to all clients in a room
func (h *Hub) SendToRoom(roomID string, messageType string, data interface{}) {
	message := &Message{
		Type:      messageType,
		Data:      data,
		Timestamp: time.Now(),
		Recipients: &MessageRecipients{
			Rooms: []string{roomID},
		},
	}

	h.BroadcastMessage(message)
}

// SendToAll sends a message to all connected clients
func (h *Hub) SendToAll(messageType string, data interface{}) {
	message := &Message{
		Type:      messageType,
		Data:      data,
		Timestamp: time.Now(),
		Recipients: &MessageRecipients{
			Broadcast: true,
		},
	}

	h.BroadcastMessage(message)
}

// JoinRoom adds a client to a room
func (h *Hub) JoinRoom(client *Client, roomID string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.roomClients[roomID] == nil {
		h.roomClients[roomID] = make(map[*Client]bool)
	}
	h.roomClients[roomID][client] = true

	client.mutex.Lock()
	client.rooms[roomID] = true
	client.mutex.Unlock()

	logger.Debugf("Client %s joined room %s", client.ID, roomID)

	// Notify room about new member
	h.SendToRoom(roomID, "user_joined_room", map[string]interface{}{
		"user_id": client.UserID.Hex(),
		"room_id": roomID,
	})
}

// LeaveRoom removes a client from a room
func (h *Hub) LeaveRoom(client *Client, roomID string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if clients, exists := h.roomClients[roomID]; exists {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.roomClients, roomID)
		}
	}

	client.mutex.Lock()
	delete(client.rooms, roomID)
	client.mutex.Unlock()

	logger.Debugf("Client %s left room %s", client.ID, roomID)

	// Notify room about member leaving
	h.SendToRoom(roomID, "user_left_room", map[string]interface{}{
		"user_id": client.UserID.Hex(),
		"room_id": roomID,
	})
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

// GetRoomClients returns all clients in a room
func (h *Hub) GetRoomClients(roomID string) []*Client {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	clients := make([]*Client, 0)
	if roomClients, exists := h.roomClients[roomID]; exists {
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
func (h *Hub) ProcessMessage(client *Client, rawMessage []byte) error {
	var message Message
	if err := json.Unmarshal(rawMessage, &message); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Set message metadata
	message.SenderID = client.UserID
	message.SenderClientID = client.ID
	message.Timestamp = time.Now()

	// Find and execute message handler
	handler, exists := h.messageHandlers[message.Type]
	if !exists {
		return fmt.Errorf("no handler found for message type: %s", message.Type)
	}

	// Execute handler
	if err := handler.HandleMessage(client, &message); err != nil {
		logger.Errorf("Message handler error for type %s: %v", message.Type, err)
		return err
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

	// Check if user has too many connections
	if userClients, exists := h.userClients[client.UserID]; exists {
		if len(userClients) >= h.config.MaxClientsPerUser {
			logger.Warnf("User %s has too many connections (%d), closing oldest",
				client.UserID.Hex(), len(userClients))

			// Close oldest client
			var oldestClient *Client
			var oldestTime time.Time
			for c := range userClients {
				if oldestClient == nil || c.ConnectedAt.Before(oldestTime) {
					oldestClient = c
					oldestTime = c.ConnectedAt
				}
			}
			if oldestClient != nil {
				oldestClient.Close()
			}
		}
	}

	// Register client
	h.clients[client] = true

	// Add to user clients mapping
	if h.userClients[client.UserID] == nil {
		h.userClients[client.UserID] = make(map[*Client]bool)
	}
	h.userClients[client.UserID][client] = true

	// Update user online status
	if h.redisClient != nil {
		h.redisClient.SetUserOnline(client.UserID.Hex(), client.Platform, 5*time.Minute)
	}

	// Auto-join user to their personal room
	personalRoom := fmt.Sprintf("user:%s", client.UserID.Hex())
	h.JoinRoom(client, personalRoom)

	logger.Infof("Client registered: %s (User: %s, Platform: %s)",
		client.ID, client.UserID.Hex(), client.Platform)

	// Notify user's contacts about online status
	h.notifyContactsAboutPresence(client.UserID, true)

	// Send welcome message
	client.Send("connection_established", map[string]interface{}{
		"client_id":    client.ID,
		"server_time":  time.Now(),
		"capabilities": []string{"messaging", "calls", "file_transfer", "presence"},
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

	// Remove from user clients mapping
	if userClients, exists := h.userClients[client.UserID]; exists {
		delete(userClients, client)
		if len(userClients) == 0 {
			delete(h.userClients, client.UserID)

			// Update user offline status
			if h.redisClient != nil {
				h.redisClient.SetUserOffline(client.UserID.Hex())
			}

			// Notify contacts about offline status
			h.notifyContactsAboutPresence(client.UserID, false)
		}
	}

	// Remove from all rooms
	for roomID := range client.rooms {
		if roomClients, exists := h.roomClients[roomID]; exists {
			delete(roomClients, client)
			if len(roomClients) == 0 {
				delete(h.roomClients, roomID)
			}
		}
	}

	// Close client connection
	client.Close()

	logger.Infof("Client unregistered: %s (User: %s)", client.ID, client.UserID.Hex())
}

// broadcastMessage handles message broadcasting
func (h *Hub) broadcastMessage(message *Message) {
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
				logger.Warnf("Client send channel full, dropping message for client %s", client.ID)
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
					logger.Warnf("Client send channel full, dropping message for client %s", client.ID)
				}
			}
		}
	}

	// Send to specific rooms
	for _, roomID := range recipients.Rooms {
		if roomClients, exists := h.roomClients[roomID]; exists {
			for client := range roomClients {
				select {
				case client.send <- messageData:
				default:
					logger.Warnf("Client send channel full, dropping message for client %s", client.ID)
				}
			}
		}
	}

	// Publish to Redis for distributed messaging
	if h.redisClient != nil && (len(recipients.Users) > 0 || len(recipients.Rooms) > 0) {
		h.publishToRedis(message)
	}
}

// notifyContactsAboutPresence notifies user's contacts about presence change
func (h *Hub) notifyContactsAboutPresence(userID primitive.ObjectID, isOnline bool) {
	// This would get user's contacts from database and notify them
	// For now, we'll just log the presence change
	status := "offline"
	if isOnline {
		status = "online"
	}

	logger.Debugf("User %s is now %s", userID.Hex(), status)

	// In a real implementation, you would:
	// 1. Get user's contacts from database
	// 2. Send presence notification to each contact
	// 3. Update presence cache in Redis
}

// publishToRedis publishes message to Redis for distributed messaging
func (h *Hub) publishToRedis(message *Message) {
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
			var message Message
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

// registerDefaultHandlers registers default message handlers
func (h *Hub) registerDefaultHandlers() {
	// Register default handlers
	h.RegisterMessageHandler(&PingHandler{})
	h.RegisterMessageHandler(&PresenceHandler{hub: h})
	h.RegisterMessageHandler(&TypingHandler{hub: h})
	h.RegisterMessageHandler(&JoinRoomHandler{hub: h})
	h.RegisterMessageHandler(&LeaveRoomHandler{hub: h})
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
	totalRooms := len(h.roomClients)
	h.mutex.RUnlock()

	h.stats.mutex.Lock()
	h.stats.TotalClients = totalClients
	h.stats.TotalUsers = totalUsers
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
		client.Send("heartbeat", map[string]interface{}{
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
		if client.LastSeen.Before(cutoff) {
			staleClients = append(staleClients, client)
		}
	}

	// Remove stale clients
	for _, client := range staleClients {
		logger.Warnf("Removing stale client: %s", client.ID)
		h.unregisterClient(client)
	}

	// Clean up empty rooms
	for roomID, clients := range h.roomClients {
		if len(clients) == 0 {
			delete(h.roomClients, roomID)
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
