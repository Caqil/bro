package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"bro/internal/models"
	"bro/pkg/logger"
	"bro/pkg/redis"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 8192 // 8KB

	// Buffer size for channels
	channelBufferSize = 256
)

// Client represents a websocket client connection
type Client struct {
	// Connection ID - unique for each websocket connection
	connID string

	// The websocket connection
	conn *websocket.Conn

	// Hub reference
	hub *Hub

	// User information (set after authentication)
	userID   primitive.ObjectID
	user     *models.User
	deviceID string
	platform string

	// Session information
	sessionID   string
	connectedAt time.Time
	lastPing    time.Time

	// Communication channels
	send chan []byte

	// Client state
	isAuthenticated bool
	isOnline        bool
	activeChats     map[primitive.ObjectID]bool // Track which chats user is actively viewing
	typingInChat    primitive.ObjectID          // Chat where user is currently typing
	callID          primitive.ObjectID          // Active call ID if in a call

	// Concurrency control
	mutex        sync.RWMutex
	sendMutex    sync.Mutex
	writeMutex   sync.Mutex
	closed       bool
	closeChannel chan struct{}

	// Rate limiting
	lastMessageTime time.Time
	messageCount    int
	rateLimitReset  time.Time

	// Connection metadata
	remoteAddr string
	userAgent  string
	headers    http.Header

	// Redis client for presence
	redisClient *redis.Client
}

// ClientInfo represents basic client information
type ClientInfo struct {
	ConnID      string             `json:"conn_id"`
	UserID      primitive.ObjectID `json:"user_id"`
	DeviceID    string             `json:"device_id"`
	Platform    string             `json:"platform"`
	ConnectedAt time.Time          `json:"connected_at"`
	LastPing    time.Time          `json:"last_ping"`
	IsOnline    bool               `json:"is_online"`
	RemoteAddr  string             `json:"remote_addr"`
}

// NewClient creates a new websocket client
func NewClient(conn *websocket.Conn, hub *Hub, req *http.Request) *Client {
	connID := primitive.NewObjectID().Hex()

	client := &Client{
		connID:          connID,
		conn:            conn,
		hub:             hub,
		send:            make(chan []byte, channelBufferSize),
		connectedAt:     time.Now(),
		lastPing:        time.Now(),
		isAuthenticated: false,
		isOnline:        false,
		activeChats:     make(map[primitive.ObjectID]bool),
		closeChannel:    make(chan struct{}),
		remoteAddr:      getClientIP(req),
		userAgent:       req.Header.Get("User-Agent"),
		headers:         req.Header,
		redisClient:     redis.GetClient(),
	}

	// Set connection parameters
	conn.SetReadLimit(maxMessageSize)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		client.lastPing = time.Now()
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	return client
}

// GetInfo returns client information
func (c *Client) GetInfo() ClientInfo {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return ClientInfo{
		ConnID:      c.connID,
		UserID:      c.userID,
		DeviceID:    c.deviceID,
		Platform:    c.platform,
		ConnectedAt: c.connectedAt,
		LastPing:    c.lastPing,
		IsOnline:    c.isOnline,
		RemoteAddr:  c.remoteAddr,
	}
}

// GetConnID returns the connection ID
func (c *Client) GetConnID() string {
	return c.connID
}

// SetUser sets the authenticated user for this client
func (c *Client) SetUser(user *models.User, sessionID, deviceID, platform string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.user = user
	c.userID = user.ID
	c.sessionID = sessionID
	c.deviceID = deviceID
	c.platform = platform
	c.isAuthenticated = true
	c.isOnline = true

	// Update user presence in Redis
	if c.redisClient != nil {
		go c.updatePresence(true)
	}

	logger.LogUserAction(c.userID.Hex(), "websocket_connected", "websocket", map[string]interface{}{
		"conn_id":     c.connID,
		"device_id":   deviceID,
		"platform":    platform,
		"remote_addr": c.remoteAddr,
		"user_agent":  c.userAgent,
	})
}

// IsAuthenticated returns whether the client is authenticated
func (c *Client) IsAuthenticated() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.isAuthenticated
}

// GetUserID returns the user ID
func (c *Client) GetUserID() primitive.ObjectID {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.userID
}

// GetUser returns the user
func (c *Client) GetUser() *models.User {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.user
}

// SetActiveChat sets which chat the user is actively viewing
func (c *Client) SetActiveChat(chatID primitive.ObjectID, active bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if active {
		c.activeChats[chatID] = true
	} else {
		delete(c.activeChats, chatID)
	}
}

// IsActiveChatUser checks if user is actively viewing a specific chat
func (c *Client) IsActiveChatUser(chatID primitive.ObjectID) bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.activeChats[chatID]
}

// SetTypingInChat sets which chat user is typing in
func (c *Client) SetTypingInChat(chatID primitive.ObjectID) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.typingInChat = chatID
}

// GetTypingInChat returns which chat user is typing in
func (c *Client) GetTypingInChat() primitive.ObjectID {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.typingInChat
}

// SetActiveCall sets the active call ID
func (c *Client) SetActiveCall(callID primitive.ObjectID) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.callID = callID
}

// GetActiveCall returns the active call ID
func (c *Client) GetActiveCall() primitive.ObjectID {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.callID
}

// SendMessage sends a message to the client
func (c *Client) SendMessage(message *WSMessage) error {
	if c.isClosed() {
		return fmt.Errorf("client connection is closed")
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	select {
	case c.send <- data:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout sending message to client")
	case <-c.closeChannel:
		return fmt.Errorf("client connection is closed")
	}
}

// SendJSON sends a JSON message to the client
func (c *Client) SendJSON(msgType string, data interface{}) error {
	message := &WSMessage{
		Type:      msgType,
		Data:      data,
		Timestamp: time.Now().Unix(),
		ID:        primitive.NewObjectID().Hex(),
	}

	if c.isAuthenticated {
		message.SenderID = c.userID.Hex()
	}

	return c.SendMessage(message)
}

// SendError sends an error message to the client
func (c *Client) SendError(code, message, details string) error {
	return c.SendJSON("error", map[string]interface{}{
		"code":    code,
		"message": message,
		"details": details,
	})
}

// checkRateLimit checks if client is within rate limits
func (c *Client) checkRateLimit() bool {
	now := time.Now()

	// Reset counter every minute
	if now.After(c.rateLimitReset) {
		c.messageCount = 0
		c.rateLimitReset = now.Add(time.Minute)
	}

	c.messageCount++

	// Allow 60 messages per minute for authenticated users, 10 for unauthenticated
	limit := 10
	if c.isAuthenticated {
		limit = 60
		if c.user != nil && (c.user.Role == models.RoleAdmin || c.user.Role == models.RoleSuper) {
			limit = 200 // Higher limit for admins
		}
	}

	if c.messageCount > limit {
		logger.Warnf("Rate limit exceeded for client %s (user: %s)", c.connID, c.userID.Hex())
		return false
	}

	return true
}

// readPump pumps messages from the websocket connection to the hub
func (c *Client) readPump() {
	defer func() {
		c.close()
	}()

	for {
		select {
		case <-c.closeChannel:
			return
		default:
			_, messageData, err := c.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					logger.Errorf("WebSocket error for user %s: %v", c.userID.Hex(), err)
				}
				return
			}

			// Rate limiting
			if !c.checkRateLimit() {
				c.SendError("RATE_LIMIT_EXCEEDED", "Too many messages", "Please slow down your message rate")
				continue
			}

			// Parse message
			var message WSMessage
			if err := json.Unmarshal(messageData, &message); err != nil {
				logger.Errorf("Failed to unmarshal message: %v", err)
				c.SendError("INVALID_MESSAGE", "Invalid message format", err.Error())
				continue
			}

			// Set sender information
			if c.isAuthenticated {
				message.SenderID = c.userID.Hex()
			}
			message.Timestamp = time.Now().Unix()

			// Process message
			c.hub.ProcessMessage(c, &message)
		}
	}
}

// writePump pumps messages from the hub to the websocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.writeMutex.Lock()
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				c.writeMutex.Unlock()
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				c.writeMutex.Unlock()
				return
			}

			w.Write(message)

			// Add queued messages to the current websocket message
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				c.writeMutex.Unlock()
				return
			}
			c.writeMutex.Unlock()

		case <-ticker.C:
			c.writeMutex.Lock()
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.writeMutex.Unlock()
				return
			}
			c.writeMutex.Unlock()

		case <-c.closeChannel:
			return
		}
	}
}

// Start starts the client's read and write pumps
func (c *Client) Start() {
	go c.writePump()
	go c.readPump()

	// Send welcome message
	c.SendJSON("welcome", map[string]interface{}{
		"message":       "Connected to ChatApp WebSocket",
		"server_time":   time.Now(),
		"connection_id": c.connID,
	})
}

// updatePresence updates user presence in Redis
func (c *Client) updatePresence(online bool) {
	if c.redisClient == nil || !c.isAuthenticated {
		return
	}

	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	presenceData := map[string]interface{}{
		"user_id":    c.userID.Hex(),
		"device_id":  c.deviceID,
		"platform":   c.platform,
		"last_seen":  time.Now(),
		"is_online":  online,
		"connection": c.remoteAddr,
		"session_id": c.sessionID,
		"conn_id":    c.connID,
	}

	if online {
		// Set online presence with expiration
		c.redisClient.SetUserOnline(c.userID.Hex(), c.deviceID, 2*time.Minute)
		c.redisClient.SetEX(
			fmt.Sprintf("presence:%s", c.userID.Hex()),
			presenceData,
			2*time.Minute,
		)
	} else {
		// Set offline presence
		c.redisClient.SetUserOffline(c.userID.Hex())
		presenceData["is_online"] = false
		c.redisClient.Set(
			fmt.Sprintf("presence:%s", c.userID.Hex()),
			presenceData, 2*time.Minute,
		)
	}

	// Publish presence change to other clients
	c.hub.broadcastPresenceChange(c.userID, online, c.deviceID, c.platform)
}

// isClosed checks if the client connection is closed
func (c *Client) isClosed() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.closed
}

// close closes the client connection
func (c *Client) close() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.closed {
		return
	}

	c.closed = true
	close(c.closeChannel)

	// Update presence to offline
	if c.isAuthenticated {
		go c.updatePresence(false)
	}

	// Close websocket connection
	c.conn.Close()

	// Close send channel
	close(c.send)

	// Notify hub about disconnection
	if c.isAuthenticated {
		c.hub.UnregisterClient(c)

		logger.LogUserAction(c.userID.Hex(), "websocket_disconnected", "websocket", map[string]interface{}{
			"conn_id":         c.connID,
			"device_id":       c.deviceID,
			"platform":        c.platform,
			"connection_time": time.Since(c.connectedAt).Seconds(),
			"remote_addr":     c.remoteAddr,
		})
	}
}

// Close gracefully closes the client connection
func (c *Client) Close() {
	c.close()
}

// Ping sends a ping message to check connection health
func (c *Client) Ping() error {
	if c.isClosed() {
		return fmt.Errorf("connection is closed")
	}

	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()

	c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	return c.conn.WriteMessage(websocket.PingMessage, []byte{})
}

// GetConnectionDuration returns how long the client has been connected
func (c *Client) GetConnectionDuration() time.Duration {
	return time.Since(c.connectedAt)
}

// GetLastPingDuration returns time since last ping
func (c *Client) GetLastPingDuration() time.Duration {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return time.Since(c.lastPing)
}

// IsHealthy checks if the connection is healthy
func (c *Client) IsHealthy() bool {
	if c.isClosed() {
		return false
	}

	// Consider unhealthy if no ping received in 2x pong wait time
	return c.GetLastPingDuration() < (2 * pongWait)
}

// GetStats returns client statistics
func (c *Client) GetStats() map[string]interface{} {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return map[string]interface{}{
		"conn_id":             c.connID,
		"user_id":             c.userID.Hex(),
		"device_id":           c.deviceID,
		"platform":            c.platform,
		"connected_at":        c.connectedAt,
		"connection_duration": time.Since(c.connectedAt).Seconds(),
		"last_ping":           c.lastPing,
		"last_ping_duration":  time.Since(c.lastPing).Seconds(),
		"is_authenticated":    c.isAuthenticated,
		"is_online":           c.isOnline,
		"is_healthy":          c.IsHealthy(),
		"message_count":       c.messageCount,
		"active_chats":        len(c.activeChats),
		"typing_in_chat":      c.typingInChat.Hex(),
		"active_call":         c.callID.Hex(),
		"remote_addr":         c.remoteAddr,
		"user_agent":          c.userAgent,
		"send_queue_size":     len(c.send),
	}
}

// Helper function to extract client IP from request
func getClientIP(req *http.Request) string {
	// Check X-Forwarded-For header
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}

	// Check X-Real-IP header
	if xri := req.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return req.RemoteAddr
}
