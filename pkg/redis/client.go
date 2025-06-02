package redis

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gomodule/redigo/redis"
)

// Client represents a Redis client with connection pooling
type Client struct {
	pool    *redis.Pool
	config  *Config
	mutex   sync.RWMutex
	hooks   []Hook
	metrics *Metrics
}

// Config represents Redis configuration
type Config struct {
	URL            string
	Host           string
	Port           int
	Password       string
	Database       int
	MaxIdle        int
	MaxActive      int
	IdleTimeout    time.Duration
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	MaxRetries     int
	RetryDelay     time.Duration
	EnableMetrics  bool
}

// Hook represents a function that gets called on Redis operations
type Hook func(operation string, key string, duration time.Duration, err error)

// Metrics represents Redis operation metrics
type Metrics struct {
	TotalOperations   int64
	SuccessfulOps     int64
	FailedOps         int64
	TotalDuration     time.Duration
	AverageDuration   time.Duration
	ConnectionErrors  int64
	TimeoutErrors     int64
	LastOperation     time.Time
	ActiveConnections int32
	mutex             sync.RWMutex
}

// SessionData represents session information stored in Redis
type SessionData struct {
	UserID    string                 `json:"user_id"`
	DeviceID  string                 `json:"device_id"`
	Platform  string                 `json:"platform"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	Data      map[string]interface{} `json:"data"`
}

// CacheItem represents a cached item with metadata
type CacheItem struct {
	Key       string      `json:"key"`
	Value     interface{} `json:"value"`
	TTL       int64       `json:"ttl"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// PubSubMessage represents a published message
type PubSubMessage struct {
	Channel   string                 `json:"channel"`
	Message   string                 `json:"message"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// Global client instance
var globalClient *Client
var globalMutex sync.RWMutex

// DefaultConfig returns default Redis configuration
func DefaultConfig() *Config {
	return &Config{
		Host:           "localhost",
		Port:           6379,
		Database:       0,
		MaxIdle:        10,
		MaxActive:      100,
		IdleTimeout:    240 * time.Second,
		ConnectTimeout: 10 * time.Second,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxRetries:     3,
		RetryDelay:     100 * time.Millisecond,
		EnableMetrics:  true,
	}
}

// NewClient creates a new Redis client
func NewClient(redisURL string) *Client {
	config := DefaultConfig()

	if redisURL != "" {
		parseRedisURL(redisURL, config)
	}

	client := &Client{
		config:  config,
		hooks:   make([]Hook, 0),
		metrics: &Metrics{},
	}

	client.pool = &redis.Pool{
		MaxIdle:     config.MaxIdle,
		MaxActive:   config.MaxActive,
		IdleTimeout: config.IdleTimeout,
		Wait:        true,
		Dial: func() (redis.Conn, error) {
			return client.dial()
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}

	// Set global client
	globalMutex.Lock()
	globalClient = client
	globalMutex.Unlock()

	// Test connection
	if err := client.Ping(); err != nil {
		log.Printf("Redis connection test failed: %v", err)
	} else {
		log.Printf("Successfully connected to Redis at %s:%d", config.Host, config.Port)
	}

	return client
}

// parseRedisURL parses Redis URL and updates config
func parseRedisURL(redisURL string, config *Config) {
	config.URL = redisURL

	u, err := url.Parse(redisURL)
	if err != nil {
		log.Printf("Invalid Redis URL: %v", err)
		return
	}

	if u.Hostname() != "" {
		config.Host = u.Hostname()
	}

	if u.Port() != "" {
		if port, err := strconv.Atoi(u.Port()); err == nil {
			config.Port = port
		}
	}

	if u.User != nil {
		if password, set := u.User.Password(); set {
			config.Password = password
		}
	}

	if len(u.Path) > 1 {
		if db, err := strconv.Atoi(u.Path[1:]); err == nil {
			config.Database = db
		}
	}
}

// dial creates a new Redis connection
func (c *Client) dial() (redis.Conn, error) {
	address := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)

	conn, err := redis.DialTimeout(
		"tcp",
		address,
		c.config.ConnectTimeout,
		c.config.ReadTimeout,
		c.config.WriteTimeout,
	)

	if err != nil {
		c.updateMetrics("dial", "", 0, err)
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	// Authenticate if password is provided
	if c.config.Password != "" {
		if _, err := conn.Do("AUTH", c.config.Password); err != nil {
			conn.Close()
			return nil, fmt.Errorf("Redis authentication failed: %w", err)
		}
	}

	// Select database
	if c.config.Database != 0 {
		if _, err := conn.Do("SELECT", c.config.Database); err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to select Redis database: %w", err)
		}
	}

	return conn, nil
}

// getConnection gets a connection from the pool
func (c *Client) getConnection() redis.Conn {
	return c.pool.Get()
}

// executeWithRetry executes a Redis command with retry logic
func (c *Client) executeWithRetry(operation string, key string, fn func(redis.Conn) (interface{}, error)) (interface{}, error) {
	var lastErr error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		start := time.Now()
		conn := c.getConnection()

		result, err := fn(conn)
		duration := time.Since(start)

		conn.Close()

		if err == nil {
			c.updateMetrics(operation, key, duration, nil)
			return result, nil
		}

		lastErr = err
		c.updateMetrics(operation, key, duration, err)

		if attempt < c.config.MaxRetries {
			time.Sleep(c.config.RetryDelay * time.Duration(attempt+1))
		}
	}

	return nil, fmt.Errorf("Redis operation failed after %d retries: %w", c.config.MaxRetries, lastErr)
}

// updateMetrics updates operation metrics
func (c *Client) updateMetrics(operation string, key string, duration time.Duration, err error) {
	if !c.config.EnableMetrics {
		return
	}

	c.metrics.mutex.Lock()
	defer c.metrics.mutex.Unlock()

	c.metrics.TotalOperations++
	c.metrics.TotalDuration += duration
	c.metrics.AverageDuration = c.metrics.TotalDuration / time.Duration(c.metrics.TotalOperations)
	c.metrics.LastOperation = time.Now()

	if err != nil {
		c.metrics.FailedOps++
		if strings.Contains(err.Error(), "connection") {
			c.metrics.ConnectionErrors++
		}
		if strings.Contains(err.Error(), "timeout") {
			c.metrics.TimeoutErrors++
		}
	} else {
		c.metrics.SuccessfulOps++
	}

	// Execute hooks
	for _, hook := range c.hooks {
		go hook(operation, key, duration, err)
	}
}

// Basic Redis Operations

// Ping tests the Redis connection
func (c *Client) Ping() error {
	_, err := c.executeWithRetry("PING", "", func(conn redis.Conn) (interface{}, error) {
		return conn.Do("PING")
	})
	return err
}

// Set sets a key-value pair
func (c *Client) Set(key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	_, err = c.executeWithRetry("SET", key, func(conn redis.Conn) (interface{}, error) {
		return conn.Do("SET", key, data)
	})
	return err
}

// SetEX sets a key-value pair with expiration
func (c *Client) SetEX(key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	_, err = c.executeWithRetry("SETEX", key, func(conn redis.Conn) (interface{}, error) {
		return conn.Do("SETEX", key, int(expiration.Seconds()), data)
	})
	return err
}

// Get gets a value by key
func (c *Client) Get(key string) ([]byte, error) {
	result, err := c.executeWithRetry("GET", key, func(conn redis.Conn) (interface{}, error) {
		return conn.Do("GET", key)
	})

	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, fmt.Errorf("key not found: %s", key)
	}

	data, ok := result.([]byte)
	if !ok {
		return nil, fmt.Errorf("unexpected data type for key: %s", key)
	}

	return data, nil
}

// GetJSON gets a value and unmarshals it from JSON
func (c *Client) GetJSON(key string, dest interface{}) error {
	data, err := c.Get(key)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return nil
}

// Exists checks if a key exists
func (c *Client) Exists(key string) (bool, error) {
	result, err := c.executeWithRetry("EXISTS", key, func(conn redis.Conn) (interface{}, error) {
		return conn.Do("EXISTS", key)
	})

	if err != nil {
		return false, err
	}

	count, ok := result.(int64)
	if !ok {
		return false, fmt.Errorf("unexpected result type for EXISTS")
	}

	return count > 0, nil
}

// Delete deletes a key
func (c *Client) Delete(key string) error {
	_, err := c.executeWithRetry("DEL", key, func(conn redis.Conn) (interface{}, error) {
		return conn.Do("DEL", key)
	})
	return err
}

// Expire sets expiration for a key
func (c *Client) Expire(key string, expiration time.Duration) error {
	_, err := c.executeWithRetry("EXPIRE", key, func(conn redis.Conn) (interface{}, error) {
		return conn.Do("EXPIRE", key, int(expiration.Seconds()))
	})
	return err
}

// TTL returns the time to live for a key
func (c *Client) TTL(key string) (time.Duration, error) {
	result, err := c.executeWithRetry("TTL", key, func(conn redis.Conn) (interface{}, error) {
		return conn.Do("TTL", key)
	})

	if err != nil {
		return 0, err
	}

	seconds, ok := result.(int64)
	if !ok {
		return 0, fmt.Errorf("unexpected result type for TTL")
	}

	if seconds < 0 {
		return 0, nil // Key doesn't exist or has no expiration
	}

	return time.Duration(seconds) * time.Second, nil
}

// Increment increments a key by 1
func (c *Client) Increment(key string) (int64, error) {
	result, err := c.executeWithRetry("INCR", key, func(conn redis.Conn) (interface{}, error) {
		return conn.Do("INCR", key)
	})

	if err != nil {
		return 0, err
	}

	value, ok := result.(int64)
	if !ok {
		return 0, fmt.Errorf("unexpected result type for INCR")
	}

	return value, nil
}

// IncrementBy increments a key by a specific amount
func (c *Client) IncrementBy(key string, amount int64) (int64, error) {
	result, err := c.executeWithRetry("INCRBY", key, func(conn redis.Conn) (interface{}, error) {
		return conn.Do("INCRBY", key, amount)
	})

	if err != nil {
		return 0, err
	}

	value, ok := result.(int64)
	if !ok {
		return 0, fmt.Errorf("unexpected result type for INCRBY")
	}

	return value, nil
}

// List Operations

// ListPush pushes an item to the left of a list
func (c *Client) ListPush(key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	_, err = c.executeWithRetry("LPUSH", key, func(conn redis.Conn) (interface{}, error) {
		return conn.Do("LPUSH", key, data)
	})
	return err
}

// ListPop pops an item from the left of a list
func (c *Client) ListPop(key string) ([]byte, error) {
	result, err := c.executeWithRetry("LPOP", key, func(conn redis.Conn) (interface{}, error) {
		return conn.Do("LPOP", key)
	})

	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, fmt.Errorf("list is empty: %s", key)
	}

	data, ok := result.([]byte)
	if !ok {
		return nil, fmt.Errorf("unexpected data type")
	}

	return data, nil
}

// ListLength returns the length of a list
func (c *Client) ListLength(key string) (int64, error) {
	result, err := c.executeWithRetry("LLEN", key, func(conn redis.Conn) (interface{}, error) {
		return conn.Do("LLEN", key)
	})

	if err != nil {
		return 0, err
	}

	length, ok := result.(int64)
	if !ok {
		return 0, fmt.Errorf("unexpected result type for LLEN")
	}

	return length, nil
}

// Set Operations

// SetAdd adds members to a set
func (c *Client) SetAdd(key string, members ...interface{}) error {
	args := make([]interface{}, len(members)+1)
	args[0] = key

	for i, member := range members {
		data, err := json.Marshal(member)
		if err != nil {
			return fmt.Errorf("failed to marshal member: %w", err)
		}
		args[i+1] = data
	}

	_, err := c.executeWithRetry("SADD", key, func(conn redis.Conn) (interface{}, error) {
		return conn.Do("SADD", args...)
	})
	return err
}

// SetMembers returns all members of a set
func (c *Client) SetMembers(key string) ([][]byte, error) {
	result, err := c.executeWithRetry("SMEMBERS", key, func(conn redis.Conn) (interface{}, error) {
		return conn.Do("SMEMBERS", key)
	})

	if err != nil {
		return nil, err
	}

	members, err := redis.ByteSlices(result, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to convert result: %w", err)
	}

	return members, nil
}

// SetIsMember checks if a value is a member of a set
func (c *Client) SetIsMember(key string, member interface{}) (bool, error) {
	data, err := json.Marshal(member)
	if err != nil {
		return false, fmt.Errorf("failed to marshal member: %w", err)
	}

	result, err := c.executeWithRetry("SISMEMBER", key, func(conn redis.Conn) (interface{}, error) {
		return conn.Do("SISMEMBER", key, data)
	})

	if err != nil {
		return false, err
	}

	isMember, ok := result.(int64)
	if !ok {
		return false, fmt.Errorf("unexpected result type for SISMEMBER")
	}

	return isMember == 1, nil
}

// Hash Operations

// HashSet sets a field in a hash
func (c *Client) HashSet(key, field string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	_, err = c.executeWithRetry("HSET", key, func(conn redis.Conn) (interface{}, error) {
		return conn.Do("HSET", key, field, data)
	})
	return err
}

// HashGet gets a field from a hash
func (c *Client) HashGet(key, field string) ([]byte, error) {
	result, err := c.executeWithRetry("HGET", key, func(conn redis.Conn) (interface{}, error) {
		return conn.Do("HGET", key, field)
	})

	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, fmt.Errorf("field not found: %s", field)
	}

	data, ok := result.([]byte)
	if !ok {
		return nil, fmt.Errorf("unexpected data type")
	}

	return data, nil
}

// HashGetAll gets all fields and values from a hash
func (c *Client) HashGetAll(key string) (map[string][]byte, error) {
	result, err := c.executeWithRetry("HGETALL", key, func(conn redis.Conn) (interface{}, error) {
		return conn.Do("HGETALL", key)
	})

	if err != nil {
		return nil, err
	}

	values, err := redis.ByteSlices(result, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to convert result: %w", err)
	}

	hash := make(map[string][]byte)
	for i := 0; i < len(values); i += 2 {
		if i+1 < len(values) {
			hash[string(values[i])] = values[i+1]
		}
	}

	return hash, nil
}

// Pub/Sub Operations

// Publish publishes a message to a channel
func (c *Client) Publish(channel string, message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	_, err = c.executeWithRetry("PUBLISH", channel, func(conn redis.Conn) (interface{}, error) {
		return conn.Do("PUBLISH", channel, data)
	})
	return err
}

// Subscribe subscribes to channels and returns a PubSubConn
func (c *Client) Subscribe(channels ...string) (*redis.PubSubConn, error) {
	conn := c.getConnection()
	psc := &redis.PubSubConn{Conn: conn}

	if err := psc.Subscribe(redis.Args{}.AddFlat(channels)...); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	return psc, nil
}

// Chat Application Specific Methods

// SetSession stores session data
func (c *Client) SetSession(sessionID string, data *SessionData, expiration time.Duration) error {
	key := fmt.Sprintf("session:%s", sessionID)
	return c.SetEX(key, data, expiration)
}

// GetSession retrieves session data
func (c *Client) GetSession(sessionID string) (*SessionData, error) {
	key := fmt.Sprintf("session:%s", sessionID)
	var session SessionData
	if err := c.GetJSON(key, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

// DeleteSession deletes session data
func (c *Client) DeleteSession(sessionID string) error {
	key := fmt.Sprintf("session:%s", sessionID)
	return c.Delete(key)
}

// SetUserOnline sets user online status
func (c *Client) SetUserOnline(userID string, deviceID string, expiration time.Duration) error {
	key := fmt.Sprintf("user:online:%s", userID)
	data := map[string]interface{}{
		"device_id":   deviceID,
		"last_active": time.Now(),
	}
	return c.SetEX(key, data, expiration)
}

// IsUserOnline checks if user is online
func (c *Client) IsUserOnline(userID string) (bool, error) {
	key := fmt.Sprintf("user:online:%s", userID)
	return c.Exists(key)
}

// SetUserOffline removes user online status
func (c *Client) SetUserOffline(userID string) error {
	key := fmt.Sprintf("user:online:%s", userID)
	return c.Delete(key)
}

// CacheMessage caches a message
func (c *Client) CacheMessage(messageID string, message interface{}, expiration time.Duration) error {
	key := fmt.Sprintf("message:%s", messageID)
	return c.SetEX(key, message, expiration)
}

// GetCachedMessage retrieves a cached message
func (c *Client) GetCachedMessage(messageID string, dest interface{}) error {
	key := fmt.Sprintf("message:%s", messageID)
	return c.GetJSON(key, dest)
}

// RateLimitCheck checks and updates rate limit
func (c *Client) RateLimitCheck(identifier string, limit int64, window time.Duration) (bool, error) {
	key := fmt.Sprintf("rate_limit:%s", identifier)

	count, err := c.IncrementBy(key, 1)
	if err != nil {
		return false, err
	}

	if count == 1 {
		// First request, set expiration
		if err := c.Expire(key, window); err != nil {
			return false, err
		}
	}

	return count <= limit, nil
}

// AddToQueue adds an item to a processing queue
func (c *Client) AddToQueue(queueName string, item interface{}) error {
	key := fmt.Sprintf("queue:%s", queueName)
	return c.ListPush(key, item)
}

// GetFromQueue gets an item from a processing queue
func (c *Client) GetFromQueue(queueName string, dest interface{}) error {
	key := fmt.Sprintf("queue:%s", queueName)
	data, err := c.ListPop(key)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, dest)
}

// Management Methods

// AddHook adds a hook function
func (c *Client) AddHook(hook Hook) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.hooks = append(c.hooks, hook)
}

// GetMetrics returns current metrics
func (c *Client) GetMetrics() *Metrics {
	c.metrics.mutex.RLock()
	defer c.metrics.mutex.RUnlock()

	// Return a copy
	return &Metrics{
		TotalOperations:   c.metrics.TotalOperations,
		SuccessfulOps:     c.metrics.SuccessfulOps,
		FailedOps:         c.metrics.FailedOps,
		TotalDuration:     c.metrics.TotalDuration,
		AverageDuration:   c.metrics.AverageDuration,
		ConnectionErrors:  c.metrics.ConnectionErrors,
		TimeoutErrors:     c.metrics.TimeoutErrors,
		LastOperation:     c.metrics.LastOperation,
		ActiveConnections: c.metrics.ActiveConnections,
	}
}

// ResetMetrics resets all metrics
func (c *Client) ResetMetrics() {
	c.metrics.mutex.Lock()
	defer c.metrics.mutex.Unlock()

	c.metrics.TotalOperations = 0
	c.metrics.SuccessfulOps = 0
	c.metrics.FailedOps = 0
	c.metrics.TotalDuration = 0
	c.metrics.AverageDuration = 0
	c.metrics.ConnectionErrors = 0
	c.metrics.TimeoutErrors = 0
}

// HealthCheck performs a health check
func (c *Client) HealthCheck() map[string]interface{} {
	health := map[string]interface{}{
		"status": "unhealthy",
	}

	start := time.Now()
	if err := c.Ping(); err != nil {
		health["error"] = err.Error()
		health["latency_ms"] = time.Since(start).Milliseconds()
		return health
	}

	latency := time.Since(start)
	health["status"] = "healthy"
	health["latency_ms"] = latency.Milliseconds()
	health["metrics"] = c.GetMetrics()
	health["pool_stats"] = map[string]interface{}{
		"active_count": c.pool.ActiveCount(),
		"idle_count":   c.pool.IdleCount(),
	}

	return health
}

// FlushAll flushes all data (use with caution)
func (c *Client) FlushAll() error {
	_, err := c.executeWithRetry("FLUSHALL", "", func(conn redis.Conn) (interface{}, error) {
		return conn.Do("FLUSHALL")
	})
	return err
}

// Close closes the Redis client
func (c *Client) Close() error {
	return c.pool.Close()
}

// Global functions

// GetClient returns the global Redis client
func GetClient() *Client {
	globalMutex.RLock()
	defer globalMutex.RUnlock()
	return globalClient
}

// Global convenience functions

// Set sets a key-value pair using the global client
func Set(key string, value interface{}) error {
	if globalClient == nil {
		return fmt.Errorf("Redis client not initialized")
	}
	return globalClient.Set(key, value)
}

// Get gets a value by key using the global client
func Get(key string) ([]byte, error) {
	if globalClient == nil {
		return nil, fmt.Errorf("Redis client not initialized")
	}
	return globalClient.Get(key)
}

// Delete deletes a key using the global client
func Delete(key string) error {
	if globalClient == nil {
		return fmt.Errorf("Redis client not initialized")
	}
	return globalClient.Delete(key)
}

// Exists checks if a key exists using the global client
func Exists(key string) (bool, error) {
	if globalClient == nil {
		return false, fmt.Errorf("Redis client not initialized")
	}
	return globalClient.Exists(key)
}

// Publish publishes a message using the global client
func Publish(channel string, message interface{}) error {
	if globalClient == nil {
		return fmt.Errorf("Redis client not initialized")
	}
	return globalClient.Publish(channel, message)
}
