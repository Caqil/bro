package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

// Client wraps Redis client with additional functionality
type Client struct {
	client *redis.Client
	ctx    context.Context
}

// SessionData represents session information stored in Redis
type SessionData struct {
	UserID    string    `json:"user_id"`
	DeviceID  string    `json:"device_id"`
	Platform  string    `json:"platform"`
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	IsActive  bool      `json:"is_active"`
}

// UserPresence represents user online presence
type UserPresence struct {
	UserID     string    `json:"user_id"`
	IsOnline   bool      `json:"is_online"`
	LastSeen   time.Time `json:"last_seen"`
	Platform   string    `json:"platform"`
	DeviceInfo string    `json:"device_info"`
}

var globalClient *Client

// NewClient creates a new Redis client
func NewClient(redisURL string) *Client {
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Printf("Failed to parse Redis URL: %v", err)
		// Fallback to default configuration
		opt = &redis.Options{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
		}
	}

	// Configure connection pool
	opt.PoolSize = 20
	opt.MinIdleConns = 5
	opt.MaxRetries = 3
	//opt.RetryDelay = time.Millisecond * 100
	opt.DialTimeout = 5 * time.Second
	opt.ReadTimeout = 3 * time.Second
	opt.WriteTimeout = 3 * time.Second
	opt.IdleTimeout = 5 * time.Minute

	rdb := redis.NewClient(opt)

	ctx := context.Background()

	// Test connection
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("Failed to connect to Redis: %v", err)
		return nil
	}

	client := &Client{
		client: rdb,
		ctx:    ctx,
	}

	globalClient = client
	log.Printf("Successfully connected to Redis at %s", opt.Addr)
	return client
}

// GetClient returns the global Redis client
func GetClient() *Client {
	return globalClient
}

// Close closes the Redis connection
func (c *Client) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// Health checks Redis connection health
func (c *Client) Health() error {
	return c.client.Ping(c.ctx).Err()
}

// Basic Redis Operations

// Set sets a key-value pair with optional expiration
func (c *Client) Set(key string, value interface{}, expiration time.Duration) error {
	return c.client.Set(c.ctx, key, value, expiration).Err()
}

// Get gets a value by key
func (c *Client) Get(key string) (string, error) {
	return c.client.Get(c.ctx, key).Result()
}

// Delete deletes a key
func (c *Client) Delete(key string) error {
	return c.client.Del(c.ctx, key).Err()
}

// Exists checks if a key exists
func (c *Client) Exists(key string) (bool, error) {
	count, err := c.client.Exists(c.ctx, key).Result()
	return count > 0, err
}

// Expire sets expiration for a key
func (c *Client) Expire(key string, expiration time.Duration) error {
	return c.client.Expire(c.ctx, key, expiration).Err()
}

// TTL returns time to live for a key
func (c *Client) TTL(key string) (time.Duration, error) {
	return c.client.TTL(c.ctx, key).Result()
}

// JSON Operations

// SetJSON sets a JSON object
func (c *Client) SetJSON(key string, value interface{}, expiration time.Duration) error {
	jsonData, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return c.client.Set(c.ctx, key, jsonData, expiration).Err()
}

// GetJSON gets a JSON object
func (c *Client) GetJSON(key string, dest interface{}) error {
	data, err := c.client.Get(c.ctx, key).Result()
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(data), dest)
}

// SetEX sets a key with expiration
func (c *Client) SetEX(key string, value interface{}, expiration time.Duration) error {
	return c.client.SetEX(c.ctx, key, value, expiration).Err()
}

// List Operations

// LPush pushes elements to the left of a list
func (c *Client) LPush(key string, values ...interface{}) error {
	return c.client.LPush(c.ctx, key, values...).Err()
}

// RPush pushes elements to the right of a list
func (c *Client) RPush(key string, values ...interface{}) error {
	return c.client.RPush(c.ctx, key, values...).Err()
}

// LPop pops an element from the left of a list
func (c *Client) LPop(key string) (string, error) {
	return c.client.LPop(c.ctx, key).Result()
}

// RPop pops an element from the right of a list
func (c *Client) RPop(key string) (string, error) {
	return c.client.RPop(c.ctx, key).Result()
}

// LRange gets a range of elements from a list
func (c *Client) LRange(key string, start, stop int64) ([]string, error) {
	return c.client.LRange(c.ctx, key, start, stop).Result()
}

// LLen returns the length of a list
func (c *Client) LLen(key string) (int64, error) {
	return c.client.LLen(c.ctx, key).Result()
}

// Set Operations

// SAdd adds members to a set
func (c *Client) SAdd(key string, members ...interface{}) error {
	return c.client.SAdd(c.ctx, key, members...).Err()
}

// SRem removes members from a set
func (c *Client) SRem(key string, members ...interface{}) error {
	return c.client.SRem(c.ctx, key, members...).Err()
}

// SMembers returns all members of a set
func (c *Client) SMembers(key string) ([]string, error) {
	return c.client.SMembers(c.ctx, key).Result()
}

// SIsMember checks if a value is a member of a set
func (c *Client) SIsMember(key string, member interface{}) (bool, error) {
	return c.client.SIsMember(c.ctx, key, member).Result()
}

// Hash Operations

// HSet sets fields in a hash
func (c *Client) HSet(key string, values ...interface{}) error {
	return c.client.HSet(c.ctx, key, values...).Err()
}

// HGet gets a field from a hash
func (c *Client) HGet(key, field string) (string, error) {
	return c.client.HGet(c.ctx, key, field).Result()
}

// HGetAll gets all fields and values from a hash
func (c *Client) HGetAll(key string) (map[string]string, error) {
	return c.client.HGetAll(c.ctx, key).Result()
}

// HDel deletes fields from a hash
func (c *Client) HDel(key string, fields ...string) error {
	return c.client.HDel(c.ctx, key, fields...).Err()
}

// Counter Operations

// Increment increments a counter by 1
func (c *Client) Increment(key string) (int64, error) {
	return c.client.Incr(c.ctx, key).Result()
}

// IncrementBy increments a counter by a specific value
func (c *Client) IncrementBy(key string, value int64) (int64, error) {
	return c.client.IncrBy(c.ctx, key, value).Result()
}

// Decrement decrements a counter by 1
func (c *Client) Decrement(key string) (int64, error) {
	return c.client.Decr(c.ctx, key).Result()
}

// DecrementBy decrements a counter by a specific value
func (c *Client) DecrementBy(key string, value int64) (int64, error) {
	return c.client.DecrBy(c.ctx, key, value).Result()
}

// Session Management

// SetSession stores session data
func (c *Client) SetSession(sessionID string, session *SessionData, expiration time.Duration) error {
	key := fmt.Sprintf("session:%s", sessionID)
	return c.SetJSON(key, session, expiration)
}

// GetSession retrieves session data
func (c *Client) GetSession(sessionID string) (*SessionData, error) {
	key := fmt.Sprintf("session:%s", sessionID)
	var session SessionData
	err := c.GetJSON(key, &session)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// DeleteSession deletes session data
func (c *Client) DeleteSession(sessionID string) error {
	key := fmt.Sprintf("session:%s", sessionID)
	return c.Delete(key)
}

// UpdateSessionActivity updates session last activity
func (c *Client) UpdateSessionActivity(sessionID string) error {
	key := fmt.Sprintf("session:%s", sessionID)
	exists, err := c.Exists(key)
	if err != nil || !exists {
		return err
	}

	session, err := c.GetSession(sessionID)
	if err != nil {
		return err
	}

	session.UpdatedAt = time.Now()
	return c.SetSession(sessionID, session, 24*time.Hour)
}

// User Presence Management

// SetUserOnline sets user as online
func (c *Client) SetUserOnline(userID, platform string, expiration time.Duration) error {
	key := fmt.Sprintf("presence:%s", userID)
	presence := &UserPresence{
		UserID:   userID,
		IsOnline: true,
		LastSeen: time.Now(),
		Platform: platform,
	}
	return c.SetJSON(key, presence, expiration)
}

// SetUserOffline sets user as offline
func (c *Client) SetUserOffline(userID string) error {
	key := fmt.Sprintf("presence:%s", userID)
	presence := &UserPresence{
		UserID:   userID,
		IsOnline: false,
		LastSeen: time.Now(),
	}
	return c.SetJSON(key, presence, 24*time.Hour)
}

// GetUserPresence gets user presence
func (c *Client) GetUserPresence(userID string) (*UserPresence, error) {
	key := fmt.Sprintf("presence:%s", userID)
	var presence UserPresence
	err := c.GetJSON(key, &presence)
	if err != nil {
		return nil, err
	}
	return &presence, nil
}

// IsUserOnline checks if user is online
func (c *Client) IsUserOnline(userID string) (bool, error) {
	presence, err := c.GetUserPresence(userID)
	if err != nil {
		return false, err
	}
	return presence.IsOnline, nil
}

// Cache Management

// SetCache sets cache with expiration
func (c *Client) SetCache(key string, value interface{}, expiration time.Duration) error {
	cacheKey := fmt.Sprintf("cache:%s", key)
	return c.SetJSON(cacheKey, value, expiration)
}

// GetCache gets cache
func (c *Client) GetCache(key string, dest interface{}) error {
	cacheKey := fmt.Sprintf("cache:%s", key)
	return c.GetJSON(cacheKey, dest)
}

// DeleteCache deletes cache
func (c *Client) DeleteCache(key string) error {
	cacheKey := fmt.Sprintf("cache:%s", key)
	return c.Delete(cacheKey)
}

// Rate Limiting

// CheckRateLimit checks rate limit for a key
func (c *Client) CheckRateLimit(key string, limit int64, window time.Duration) (bool, int64, error) {
	current, err := c.IncrementBy(key, 1)
	if err != nil {
		return false, 0, err
	}

	if current == 1 {
		// First request, set expiration
		c.Expire(key, window)
	}

	return current <= limit, limit - current, nil
}

// ResetRateLimit resets rate limit for a key
func (c *Client) ResetRateLimit(key string) error {
	return c.Delete(key)
}

// Chat Room Management

// JoinChatRoom adds user to chat room
func (c *Client) JoinChatRoom(chatID, userID string) error {
	key := fmt.Sprintf("chat_room:%s", chatID)
	return c.SAdd(key, userID)
}

// LeaveChatRoom removes user from chat room
func (c *Client) LeaveChatRoom(chatID, userID string) error {
	key := fmt.Sprintf("chat_room:%s", chatID)
	return c.SRem(key, userID)
}

// GetChatRoomMembers gets all members in a chat room
func (c *Client) GetChatRoomMembers(chatID string) ([]string, error) {
	key := fmt.Sprintf("chat_room:%s", chatID)
	return c.SMembers(key)
}

// IsInChatRoom checks if user is in chat room
func (c *Client) IsInChatRoom(chatID, userID string) (bool, error) {
	key := fmt.Sprintf("chat_room:%s", chatID)
	return c.SIsMember(key, userID)
}

// Notification Queue

// PushNotification pushes notification to queue
func (c *Client) PushNotification(userID string, notification interface{}) error {
	key := fmt.Sprintf("notifications:%s", userID)
	jsonData, err := json.Marshal(notification)
	if err != nil {
		return err
	}
	return c.LPush(key, jsonData)
}

// PopNotification pops notification from queue
func (c *Client) PopNotification(userID string) (string, error) {
	key := fmt.Sprintf("notifications:%s", userID)
	return c.RPop(key)
}

// GetNotificationCount gets notification count
func (c *Client) GetNotificationCount(userID string) (int64, error) {
	key := fmt.Sprintf("notifications:%s", userID)
	return c.LLen(key)
}

// Typing Indicators

// SetTypingIndicator sets typing indicator
func (c *Client) SetTypingIndicator(chatID, userID string, isTyping bool) error {
	key := fmt.Sprintf("typing:%s", chatID)
	if isTyping {
		return c.SAdd(key, userID)
	} else {
		return c.SRem(key, userID)
	}
}

// GetTypingUsers gets users who are typing
func (c *Client) GetTypingUsers(chatID string) ([]string, error) {
	key := fmt.Sprintf("typing:%s", chatID)
	return c.SMembers(key)
}

// WebRTC Signaling

// SetWebRTCOffer sets WebRTC offer
func (c *Client) SetWebRTCOffer(callID string, offer interface{}) error {
	key := fmt.Sprintf("webrtc_offer:%s", callID)
	return c.SetJSON(key, offer, 5*time.Minute)
}

// GetWebRTCOffer gets WebRTC offer
func (c *Client) GetWebRTCOffer(callID string) (interface{}, error) {
	key := fmt.Sprintf("webrtc_offer:%s", callID)
	var offer interface{}
	err := c.GetJSON(key, &offer)
	return offer, err
}

// SetWebRTCAnswer sets WebRTC answer
func (c *Client) SetWebRTCAnswer(callID string, answer interface{}) error {
	key := fmt.Sprintf("webrtc_answer:%s", callID)
	return c.SetJSON(key, answer, 5*time.Minute)
}

// GetWebRTCAnswer gets WebRTC answer
func (c *Client) GetWebRTCAnswer(callID string) (interface{}, error) {
	key := fmt.Sprintf("webrtc_answer:%s", callID)
	var answer interface{}
	err := c.GetJSON(key, &answer)
	return answer, err
}

// ICE Candidate Management

// AddICECandidate adds ICE candidate
func (c *Client) AddICECandidate(callID string, candidate interface{}) error {
	key := fmt.Sprintf("ice_candidates:%s", callID)
	jsonData, err := json.Marshal(candidate)
	if err != nil {
		return err
	}
	return c.LPush(key, jsonData)
}

// GetICECandidates gets all ICE candidates
func (c *Client) GetICECandidates(callID string) ([]string, error) {
	key := fmt.Sprintf("ice_candidates:%s", callID)
	return c.LRange(key, 0, -1)
}

// ClearICECandidates clears ICE candidates
func (c *Client) ClearICECandidates(callID string) error {
	key := fmt.Sprintf("ice_candidates:%s", callID)
	return c.Delete(key)
}

// Statistics and Monitoring

// GetConnectionCount gets current connection count
func (c *Client) GetConnectionCount() (int64, error) {
	return c.client.DBSize(c.ctx).Result()
}

// GetInfo gets Redis info
func (c *Client) GetInfo() (string, error) {
	return c.client.Info(c.ctx).Result()
}

// FlushDB flushes current database (use with caution)
func (c *Client) FlushDB() error {
	return c.client.FlushDB(c.ctx).Err()
}

// Keys returns all keys matching pattern
func (c *Client) Keys(pattern string) ([]string, error) {
	return c.client.Keys(c.ctx, pattern).Result()
}

// Utility Functions

// GenerateKey generates a Redis key with prefix
func GenerateKey(prefix, id string) string {
	return fmt.Sprintf("%s:%s", prefix, id)
}

// ParseCounter parses counter value
func ParseCounter(value string) (int64, error) {
	return strconv.ParseInt(value, 10, 64)
}

// PubSub Operations

// Publish publishes a message to a channel
func (c *Client) Publish(channel string, message interface{}) error {
	return c.client.Publish(c.ctx, channel, message).Err()
}

// Subscribe subscribes to channels
func (c *Client) Subscribe(channels ...string) *redis.PubSub {
	return c.client.Subscribe(c.ctx, channels...)
}

// PSubscribe subscribes to pattern
func (c *Client) PSubscribe(patterns ...string) *redis.PubSub {
	return c.client.PSubscribe(c.ctx, patterns...)
}

// Health Check
func Health() error {
	if globalClient == nil {
		return fmt.Errorf("Redis client not initialized")
	}
	return globalClient.Health()
}
