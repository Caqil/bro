package middleware

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"bro/internal/utils"
	"bro/pkg/logger"
	"bro/pkg/redis"
)

// RateLimitConfig represents rate limiting configuration
type RateLimitConfig struct {
	// Rate limiting algorithm
	Algorithm RateLimitAlgorithm

	// Rate limiting strategy
	Strategy RateLimitStrategy

	// Requests per window
	Requests int64

	// Time window duration
	Window time.Duration

	// Burst size for token bucket algorithm
	BurstSize int64

	// Key generator function
	KeyGenerator func(*gin.Context) string

	// Skip function to bypass rate limiting for certain requests
	SkipFunc func(*gin.Context) bool

	// Error handler for rate limit exceeded
	ErrorHandler func(*gin.Context, RateLimitInfo)

	// Headers to include in response
	IncludeHeaders bool

	// Store for rate limit data (Redis)
	Store RateLimitStore

	// Different limits for different user types
	UserLimits map[string]RateLimitRule

	// Path-specific limits
	PathLimits map[string]RateLimitRule

	// Method-specific limits
	MethodLimits map[string]RateLimitRule

	// Enable distributed rate limiting
	Distributed bool
}

// RateLimitAlgorithm represents different rate limiting algorithms
type RateLimitAlgorithm int

const (
	FixedWindow RateLimitAlgorithm = iota
	SlidingWindow
	TokenBucket
	SlidingLog
)

// RateLimitStrategy represents different rate limiting strategies
type RateLimitStrategy int

const (
	IPBased RateLimitStrategy = iota
	UserBased
	IPAndUserBased
	HeaderBased
	CustomKey
)

// RateLimitRule represents a specific rate limiting rule
type RateLimitRule struct {
	Requests  int64
	Window    time.Duration
	BurstSize int64
}

// RateLimitInfo contains information about current rate limit status
type RateLimitInfo struct {
	Limit       int64
	Remaining   int64
	ResetTime   time.Time
	RetryAfter  int64
	TotalHits   int64
	WindowStart time.Time
}

// RateLimitStore interface for storing rate limit data
type RateLimitStore interface {
	Get(key string) (*RateLimitInfo, error)
	Set(key string, info *RateLimitInfo) error
	Increment(key string, window time.Duration) (*RateLimitInfo, error)
	Delete(key string) error
}

// RedisRateLimitStore implements RateLimitStore using Redis
type RedisRateLimitStore struct {
	client *redis.Client
	prefix string
}

// InMemoryRateLimitStore implements RateLimitStore using in-memory storage
type InMemoryRateLimitStore struct {
	data  map[string]*RateLimitInfo
	mutex sync.RWMutex
}

// DefaultRateLimitConfig returns default rate limiting configuration
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Algorithm:      FixedWindow,
		Strategy:       IPBased,
		Requests:       100,
		Window:         time.Minute,
		BurstSize:      10,
		IncludeHeaders: true,
		Distributed:    true,
		UserLimits: map[string]RateLimitRule{
			"free":    {Requests: 60, Window: time.Minute, BurstSize: 5},
			"premium": {Requests: 300, Window: time.Minute, BurstSize: 20},
			"admin":   {Requests: 1000, Window: time.Minute, BurstSize: 50},
		},
		PathLimits: map[string]RateLimitRule{
			"/api/auth/login":     {Requests: 5, Window: time.Minute, BurstSize: 2},
			"/api/auth/register":  {Requests: 3, Window: time.Minute, BurstSize: 1},
			"/api/auth/verify":    {Requests: 10, Window: time.Minute, BurstSize: 3},
			"/api/files/upload":   {Requests: 20, Window: time.Minute, BurstSize: 5},
			"/api/calls/initiate": {Requests: 30, Window: time.Minute, BurstSize: 10},
		},
		MethodLimits: map[string]RateLimitRule{
			"POST":   {Requests: 50, Window: time.Minute, BurstSize: 10},
			"PUT":    {Requests: 30, Window: time.Minute, BurstSize: 5},
			"DELETE": {Requests: 20, Window: time.Minute, BurstSize: 3},
		},
	}
}

// RateLimit creates a rate limiting middleware with default configuration
func RateLimit() gin.HandlerFunc {
	config := DefaultRateLimitConfig()
	return RateLimitWithConfig(config)
}

// RateLimitWithConfig creates a rate limiting middleware with custom configuration
func RateLimitWithConfig(config RateLimitConfig) gin.HandlerFunc {
	// Initialize store if not provided
	if config.Store == nil {
		redisClient := redis.GetClient()
		if redisClient != nil && config.Distributed {
			config.Store = &RedisRateLimitStore{
				client: redisClient,
				prefix: "rate_limit:",
			}
		} else {
			config.Store = &InMemoryRateLimitStore{
				data: make(map[string]*RateLimitInfo),
			}
		}
	}

	// Set default key generator if not provided
	if config.KeyGenerator == nil {
		config.KeyGenerator = defaultKeyGenerator(config.Strategy)
	}

	// Set default error handler if not provided
	if config.ErrorHandler == nil {
		config.ErrorHandler = defaultErrorHandler
	}

	return func(c *gin.Context) {
		startTime := time.Now()

		// Skip rate limiting if skip function returns true
		if config.SkipFunc != nil && config.SkipFunc(c) {
			c.Next()
			return
		}

		// Get the appropriate rate limit rule
		rule := getRateLimitRule(c, config)

		// Generate rate limit key
		key := config.KeyGenerator(c)

		// Check rate limit
		info, allowed := checkRateLimit(key, rule, config)

		// Add headers if enabled
		if config.IncludeHeaders {
			addRateLimitHeaders(c, info)
		}

		// Log rate limit check
		duration := time.Since(startTime)
		logRateLimitCheck(c, key, info, allowed, duration)

		if !allowed {
			config.ErrorHandler(c, *info)
			return
		}

		c.Next()
	}
}

// ChatAppRateLimit creates rate limiting specifically for chat application endpoints
func ChatAppRateLimit() gin.HandlerFunc {
	config := RateLimitConfig{
		Algorithm:      SlidingWindow,
		Strategy:       IPAndUserBased,
		Requests:       100,
		Window:         time.Minute,
		IncludeHeaders: true,
		Distributed:    true,
		UserLimits: map[string]RateLimitRule{
			"free":    {Requests: 60, Window: time.Minute, BurstSize: 5},
			"premium": {Requests: 300, Window: time.Minute, BurstSize: 20},
			"admin":   {Requests: 1000, Window: time.Minute, BurstSize: 50},
		},
		PathLimits: map[string]RateLimitRule{
			// Authentication endpoints - very strict
			"/api/auth/login":      {Requests: 5, Window: time.Minute, BurstSize: 2},
			"/api/auth/register":   {Requests: 3, Window: time.Minute, BurstSize: 1},
			"/api/auth/verify":     {Requests: 10, Window: time.Minute, BurstSize: 3},
			"/api/auth/resend-otp": {Requests: 3, Window: 5 * time.Minute, BurstSize: 1},

			// Messaging endpoints - moderate limits
			"/api/chats":            {Requests: 50, Window: time.Minute, BurstSize: 10},
			"/api/chats/*/messages": {Requests: 100, Window: time.Minute, BurstSize: 20},
			"/api/messages":         {Requests: 200, Window: time.Minute, BurstSize: 30},

			// File upload endpoints - limited
			"/api/files/upload": {Requests: 20, Window: time.Minute, BurstSize: 5},
			"/api/files/*":      {Requests: 50, Window: time.Minute, BurstSize: 10},

			// Call endpoints - moderate limits
			"/api/calls/initiate": {Requests: 30, Window: time.Minute, BurstSize: 10},
			"/api/calls/*/answer": {Requests: 50, Window: time.Minute, BurstSize: 15},
			"/api/calls/*/end":    {Requests: 50, Window: time.Minute, BurstSize: 15},

			// Group management - limited
			"/api/groups":           {Requests: 20, Window: time.Minute, BurstSize: 5},
			"/api/groups/*/members": {Requests: 30, Window: time.Minute, BurstSize: 8},

			// Admin endpoints - higher limits for admins
			"/api/admin/*": {Requests: 200, Window: time.Minute, BurstSize: 50},
		},
		MethodLimits: map[string]RateLimitRule{
			"POST":   {Requests: 80, Window: time.Minute, BurstSize: 15},
			"PUT":    {Requests: 60, Window: time.Minute, BurstSize: 10},
			"DELETE": {Requests: 40, Window: time.Minute, BurstSize: 8},
			"GET":    {Requests: 200, Window: time.Minute, BurstSize: 40},
		},
		SkipFunc: func(c *gin.Context) bool {
			// Skip rate limiting for health checks and static files
			path := c.Request.URL.Path
			return strings.HasPrefix(path, "/health") ||
				strings.HasPrefix(path, "/static") ||
				strings.HasPrefix(path, "/socket.io")
		},
	}

	return RateLimitWithConfig(config)
}

// StrictRateLimit creates very strict rate limiting for sensitive endpoints
func StrictRateLimit(requests int64, window time.Duration) gin.HandlerFunc {
	config := RateLimitConfig{
		Algorithm:      FixedWindow,
		Strategy:       IPBased,
		Requests:       requests,
		Window:         window,
		BurstSize:      1,
		IncludeHeaders: true,
		Distributed:    true,
	}

	return RateLimitWithConfig(config)
}

// UserBasedRateLimit creates user-based rate limiting
func UserBasedRateLimit() gin.HandlerFunc {
	config := DefaultRateLimitConfig()
	config.Strategy = UserBased
	config.KeyGenerator = func(c *gin.Context) string {
		userID, exists := c.Get("user_id")
		if exists {
			return fmt.Sprintf("user_rate_limit:%v", userID)
		}
		// Fallback to IP if no user
		return fmt.Sprintf("ip_rate_limit:%s", utils.GetClientIP(c))
	}

	return RateLimitWithConfig(config)
}

// Helper functions

// defaultKeyGenerator creates a default key generator based on strategy
func defaultKeyGenerator(strategy RateLimitStrategy) func(*gin.Context) string {
	return func(c *gin.Context) string {
		switch strategy {
		case IPBased:
			return fmt.Sprintf("ip_rate_limit:%s", utils.GetClientIP(c))
		case UserBased:
			userID, exists := c.Get("user_id")
			if exists {
				return fmt.Sprintf("user_rate_limit:%v", userID)
			}
			return fmt.Sprintf("ip_rate_limit:%s", utils.GetClientIP(c))
		case IPAndUserBased:
			userID, exists := c.Get("user_id")
			if exists {
				return fmt.Sprintf("user_ip_rate_limit:%v:%s", userID, utils.GetClientIP(c))
			}
			return fmt.Sprintf("ip_rate_limit:%s", utils.GetClientIP(c))
		case HeaderBased:
			apiKey := c.GetHeader("X-API-Key")
			if apiKey != "" {
				return fmt.Sprintf("api_key_rate_limit:%s", apiKey)
			}
			return fmt.Sprintf("ip_rate_limit:%s", utils.GetClientIP(c))
		default:
			return fmt.Sprintf("ip_rate_limit:%s", utils.GetClientIP(c))
		}
	}
}

// defaultErrorHandler handles rate limit exceeded errors
func defaultErrorHandler(c *gin.Context, info RateLimitInfo) {
	retryAfter := int(info.RetryAfter)
	if retryAfter <= 0 {
		retryAfter = int(info.ResetTime.Sub(time.Now()).Seconds())
		if retryAfter < 0 {
			retryAfter = 60 // Default to 60 seconds
		}
	}

	c.Header("Retry-After", strconv.Itoa(retryAfter))

	utils.RateLimitExceeded(c, retryAfter)
	c.Abort()
}

// getRateLimitRule determines the appropriate rate limit rule for the request
func getRateLimitRule(c *gin.Context, config RateLimitConfig) RateLimitRule {
	path := c.Request.URL.Path
	method := c.Request.Method

	// Check path-specific limits first
	if rule, exists := config.PathLimits[path]; exists {
		return rule
	}

	// Check for wildcard path matches
	for pattern, rule := range config.PathLimits {
		if strings.Contains(pattern, "*") {
			if matchPattern(path, pattern) {
				return rule
			}
		}
	}

	// Check user-specific limits
	userRole, exists := c.Get("user_role")
	if exists {
		if rule, exists := config.UserLimits[userRole.(string)]; exists {
			return rule
		}
	}

	// Check method-specific limits
	if rule, exists := config.MethodLimits[method]; exists {
		return rule
	}

	// Return default rule
	return RateLimitRule{
		Requests:  config.Requests,
		Window:    config.Window,
		BurstSize: config.BurstSize,
	}
}

// matchPattern matches a path against a pattern with wildcards
func matchPattern(path, pattern string) bool {
	if !strings.Contains(pattern, "*") {
		return path == pattern
	}

	parts := strings.Split(pattern, "*")
	if len(parts) != 2 {
		return false
	}

	prefix, suffix := parts[0], parts[1]
	return strings.HasPrefix(path, prefix) && strings.HasSuffix(path, suffix)
}

// checkRateLimit checks if the request is within rate limits
func checkRateLimit(key string, rule RateLimitRule, config RateLimitConfig) (*RateLimitInfo, bool) {
	store := config.Store

	switch config.Algorithm {
	case FixedWindow:
		return checkFixedWindow(store, key, rule)
	case SlidingWindow:
		return checkSlidingWindow(store, key, rule)
	case TokenBucket:
		return checkTokenBucket(store, key, rule)
	case SlidingLog:
		return checkSlidingLog(store, key, rule)
	default:
		return checkFixedWindow(store, key, rule)
	}
}

// checkFixedWindow implements fixed window rate limiting
func checkFixedWindow(store RateLimitStore, key string, rule RateLimitRule) (*RateLimitInfo, bool) {
	now := time.Now()
	windowStart := now.Truncate(rule.Window)
	windowKey := fmt.Sprintf("%s:fw:%d", key, windowStart.Unix())

	info, err := store.Increment(windowKey, rule.Window)
	if err != nil {
		// If we can't check the limit, allow the request but log the error
		logger.Errorf("Rate limit store error: %v", err)
		return &RateLimitInfo{
			Limit:     rule.Requests,
			Remaining: rule.Requests - 1,
			ResetTime: windowStart.Add(rule.Window),
		}, true
	}

	if info == nil {
		info = &RateLimitInfo{
			Limit:       rule.Requests,
			Remaining:   rule.Requests - 1,
			ResetTime:   windowStart.Add(rule.Window),
			TotalHits:   1,
			WindowStart: windowStart,
		}
	}

	info.Remaining = rule.Requests - info.TotalHits
	if info.Remaining < 0 {
		info.Remaining = 0
	}

	allowed := info.TotalHits <= rule.Requests
	if !allowed {
		info.RetryAfter = int64(info.ResetTime.Sub(now).Seconds())
	}

	return info, allowed
}

// checkSlidingWindow implements sliding window rate limiting
func checkSlidingWindow(store RateLimitStore, key string, rule RateLimitRule) (*RateLimitInfo, bool) {
	now := time.Now()
	currentWindow := now.Truncate(rule.Window)
	previousWindow := currentWindow.Add(-rule.Window)

	currentKey := fmt.Sprintf("%s:sw:current:%d", key, currentWindow.Unix())
	previousKey := fmt.Sprintf("%s:sw:previous:%d", key, previousWindow.Unix())

	// Get current window count
	currentInfo, _ := store.Increment(currentKey, rule.Window)
	currentCount := int64(0)
	if currentInfo != nil {
		currentCount = currentInfo.TotalHits
	}

	// Get previous window count
	previousInfo, _ := store.Get(previousKey)
	previousCount := int64(0)
	if previousInfo != nil {
		previousCount = previousInfo.TotalHits
	}

	// Calculate sliding window count
	timePassedInCurrentWindow := now.Sub(currentWindow)
	weightedPreviousCount := previousCount * int64(rule.Window-timePassedInCurrentWindow) / int64(rule.Window)
	totalCount := currentCount + weightedPreviousCount

	info := &RateLimitInfo{
		Limit:       rule.Requests,
		Remaining:   rule.Requests - totalCount,
		ResetTime:   currentWindow.Add(rule.Window),
		TotalHits:   totalCount,
		WindowStart: currentWindow,
	}

	if info.Remaining < 0 {
		info.Remaining = 0
	}

	allowed := totalCount <= rule.Requests
	if !allowed {
		info.RetryAfter = int64(info.ResetTime.Sub(now).Seconds())
	}

	return info, allowed
}

// checkTokenBucket implements token bucket rate limiting
func checkTokenBucket(store RateLimitStore, key string, rule RateLimitRule) (*RateLimitInfo, bool) {
	now := time.Now()
	bucketKey := fmt.Sprintf("%s:tb", key)

	info, err := store.Get(bucketKey)
	if err != nil || info == nil {
		// Initialize new bucket
		info = &RateLimitInfo{
			Limit:       rule.BurstSize,
			Remaining:   rule.BurstSize - 1,
			ResetTime:   now.Add(rule.Window),
			TotalHits:   1,
			WindowStart: now,
		}
	} else {
		// Calculate tokens to add based on time passed
		timePassed := now.Sub(info.WindowStart)
		tokensToAdd := int64(timePassed.Nanoseconds()) * rule.Requests / int64(rule.Window.Nanoseconds())

		info.Remaining = min(rule.BurstSize, info.Remaining+tokensToAdd)
		info.WindowStart = now

		if info.Remaining > 0 {
			info.Remaining--
			info.TotalHits++
		}
	}

	allowed := info.Remaining >= 0
	if !allowed {
		info.RetryAfter = int64(rule.Window.Seconds() / float64(rule.Requests))
	}

	// Update store
	store.Set(bucketKey, info)

	return info, allowed
}

// checkSlidingLog implements sliding log rate limiting
func checkSlidingLog(store RateLimitStore, key string, rule RateLimitRule) (*RateLimitInfo, bool) {
	now := time.Now()
	logKey := fmt.Sprintf("%s:sl", key)

	// This is a simplified implementation
	// In a real system, you'd maintain a log of timestamps
	info, err := store.Get(logKey)
	if err != nil || info == nil {
		info = &RateLimitInfo{
			Limit:       rule.Requests,
			Remaining:   rule.Requests - 1,
			ResetTime:   now.Add(rule.Window),
			TotalHits:   1,
			WindowStart: now,
		}
	} else {
		// Check if we're still in the window
		if now.Sub(info.WindowStart) > rule.Window {
			// Reset the window
			info.WindowStart = now
			info.TotalHits = 1
			info.Remaining = rule.Requests - 1
		} else {
			info.TotalHits++
			info.Remaining = rule.Requests - info.TotalHits
		}
	}

	if info.Remaining < 0 {
		info.Remaining = 0
	}

	allowed := info.TotalHits <= rule.Requests
	if !allowed {
		info.RetryAfter = int64(info.ResetTime.Sub(now).Seconds())
	}

	store.Set(logKey, info)
	return info, allowed
}

// addRateLimitHeaders adds rate limit headers to the response
func addRateLimitHeaders(c *gin.Context, info *RateLimitInfo) {
	c.Header("X-Rate-Limit-Limit", strconv.FormatInt(info.Limit, 10))
	c.Header("X-Rate-Limit-Remaining", strconv.FormatInt(info.Remaining, 10))
	c.Header("X-Rate-Limit-Reset", strconv.FormatInt(info.ResetTime.Unix(), 10))

	if info.RetryAfter > 0 {
		c.Header("Retry-After", strconv.FormatInt(info.RetryAfter, 10))
	}
}

// logRateLimitCheck logs rate limit check for monitoring
func logRateLimitCheck(c *gin.Context, key string, info *RateLimitInfo, allowed bool, duration time.Duration) {
	fields := map[string]interface{}{
		"key":         key,
		"limit":       info.Limit,
		"remaining":   info.Remaining,
		"total_hits":  info.TotalHits,
		"allowed":     allowed,
		"duration_ms": duration.Milliseconds(),
		"path":        c.Request.URL.Path,
		"method":      c.Request.Method,
		"type":        "rate_limit_check",
	}

	if !allowed {
		fields["retry_after"] = info.RetryAfter
		logger.WithFields(fields).Warn("Rate limit exceeded")
	} else {
		logger.WithFields(fields).Debug("Rate limit check passed")
	}
}

// Redis store implementation

func (r *RedisRateLimitStore) Get(key string) (*RateLimitInfo, error) {
	fullKey := r.prefix + key
	var info RateLimitInfo
	err := r.client.GetJSON(fullKey, &info)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func (r *RedisRateLimitStore) Set(key string, info *RateLimitInfo) error {
	fullKey := r.prefix + key
	return r.client.SetEX(fullKey, info, time.Hour) // Default TTL
}

func (r *RedisRateLimitStore) Increment(key string, window time.Duration) (*RateLimitInfo, error) {
	fullKey := r.prefix + key

	// Use Redis INCR for atomic increment
	count, err := r.client.IncrementBy(fullKey, 1)
	if err != nil {
		return nil, err
	}

	if count == 1 {
		// First increment, set expiration
		r.client.Expire(fullKey, window)
	}

	info := &RateLimitInfo{
		TotalHits:   count,
		WindowStart: time.Now(),
		ResetTime:   time.Now().Add(window),
	}

	return info, nil
}

func (r *RedisRateLimitStore) Delete(key string) error {
	fullKey := r.prefix + key
	return r.client.Delete(fullKey)
}

// In-memory store implementation

func (m *InMemoryRateLimitStore) Get(key string) (*RateLimitInfo, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	info, exists := m.data[key]
	if !exists {
		return nil, fmt.Errorf("key not found")
	}

	// Return a copy
	infoCopy := *info
	return &infoCopy, nil
}

func (m *InMemoryRateLimitStore) Set(key string, info *RateLimitInfo) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Store a copy
	infoCopy := *info
	m.data[key] = &infoCopy
	return nil
}

func (m *InMemoryRateLimitStore) Increment(key string, window time.Duration) (*RateLimitInfo, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	info, exists := m.data[key]

	if !exists {
		info = &RateLimitInfo{
			TotalHits:   1,
			WindowStart: now,
			ResetTime:   now.Add(window),
		}
	} else {
		// Check if window has expired
		if now.After(info.ResetTime) {
			info.TotalHits = 1
			info.WindowStart = now
			info.ResetTime = now.Add(window)
		} else {
			info.TotalHits++
		}
	}

	m.data[key] = info

	// Return a copy
	infoCopy := *info
	return &infoCopy, nil
}

func (m *InMemoryRateLimitStore) Delete(key string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.data, key)
	return nil
}

// Utility functions

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// GetRateLimitInfo returns current rate limit info for a key
func GetRateLimitInfo(c *gin.Context, strategy RateLimitStrategy) (*RateLimitInfo, error) {
	keyGen := defaultKeyGenerator(strategy)
	key := keyGen(c)

	redisClient := redis.GetClient()
	if redisClient == nil {
		return nil, fmt.Errorf("rate limit store not available")
	}

	store := &RedisRateLimitStore{
		client: redisClient,
		prefix: "rate_limit:",
	}

	return store.Get(key)
}

// ClearRateLimit clears rate limit for a specific key
func ClearRateLimit(c *gin.Context, strategy RateLimitStrategy) error {
	keyGen := defaultKeyGenerator(strategy)
	key := keyGen(c)

	redisClient := redis.GetClient()
	if redisClient == nil {
		return fmt.Errorf("rate limit store not available")
	}

	store := &RedisRateLimitStore{
		client: redisClient,
		prefix: "rate_limit:",
	}

	return store.Delete(key)
}
