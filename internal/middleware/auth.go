package middleware

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"bro/internal/models"
	"bro/internal/utils"
	"bro/pkg/database"
	"bro/pkg/logger"
	"bro/pkg/redis"
)

// AuthConfig represents authentication middleware configuration
type AuthConfig struct {
	JWTSecret           string
	SessionTimeout      time.Duration
	RequireVerification bool
	EnableSessionStore  bool
	SkipPaths           []string
	AdminPaths          []string
	ModeratorPaths      []string
}

// UserContext represents authenticated user context
type UserContext struct {
	User      *models.User
	UserID    primitive.ObjectID
	Role      models.UserRole
	SessionID string
	DeviceID  string
	Platform  string
}

// AuthMiddleware creates the main authentication middleware
func AuthMiddleware(jwtSecret string) gin.HandlerFunc {
	config := &AuthConfig{
		JWTSecret:           jwtSecret,
		SessionTimeout:      24 * time.Hour,
		RequireVerification: true,
		EnableSessionStore:  true,
		SkipPaths: []string{
			"/api/auth/register",
			"/api/auth/login",
			"/api/auth/verify",
			"/api/auth/refresh",
			"/api/auth/resend-otp",
			"/health",
			"/socket.io",
		},
	}

	return AuthMiddlewareWithConfig(config)
}

// AuthMiddlewareWithConfig creates authentication middleware with custom config
func AuthMiddlewareWithConfig(config *AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method
		userAgent := c.GetHeader("User-Agent")
		ip := utils.GetClientIP(c)

		// Skip authentication for certain paths
		if shouldSkipAuth(path, config.SkipPaths) {
			c.Next()
			return
		}

		// Get authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			logger.LogSecurityEvent("missing_auth_header", "", ip, map[string]interface{}{
				"path":       path,
				"method":     method,
				"user_agent": userAgent,
			})
			utils.Unauthorized(c, "Authorization header required")
			return
		}

		// Extract token
		token := extractToken(authHeader)
		if token == "" {
			logger.LogSecurityEvent("invalid_auth_format", "", ip, map[string]interface{}{
				"path":        path,
				"method":      method,
				"auth_header": authHeader,
			})
			utils.Unauthorized(c, "Invalid authorization format")
			return
		}

		// Validate token
		claims, err := utils.ValidateToken(token, config.JWTSecret)
		if err != nil {
			logger.LogSecurityEvent("invalid_token", "", ip, map[string]interface{}{
				"path":   path,
				"method": method,
				"error":  err.Error(),
			})

			if strings.Contains(err.Error(), "expired") {
				utils.ExpiredToken(c)
			} else {
				utils.InvalidToken(c)
			}
			return
		}

		// Parse user ID
		userID, err := primitive.ObjectIDFromHex(claims.UserID)
		if err != nil {
			logger.LogSecurityEvent("invalid_user_id", claims.UserID, ip, map[string]interface{}{
				"path":   path,
				"method": method,
				"error":  err.Error(),
			})
			utils.InvalidToken(c)
			return
		}

		// Get user from database
		user, err := getUserFromDatabase(userID)
		if err != nil {
			logger.LogSecurityEvent("user_not_found", claims.UserID, ip, map[string]interface{}{
				"path":   path,
				"method": method,
				"error":  err.Error(),
			})
			utils.Unauthorized(c, "User not found")
			return
		}

		// Check if user is active
		if !user.IsActive || user.IsDeleted {
			logger.LogSecurityEvent("inactive_user_access", claims.UserID, ip, map[string]interface{}{
				"path":       path,
				"method":     method,
				"is_active":  user.IsActive,
				"is_deleted": user.IsDeleted,
			})
			utils.Unauthorized(c, "Account is inactive")
			return
		}

		// Check phone verification if required
		if config.RequireVerification && !user.IsPhoneVerified {
			logger.LogSecurityEvent("unverified_user_access", claims.UserID, ip, map[string]interface{}{
				"path":   path,
				"method": method,
			})
			utils.Unauthorized(c, "Phone number not verified")
			return
		}

		// Check session if enabled
		if config.EnableSessionStore {
			if err := validateSession(claims, userID, ip); err != nil {
				logger.LogSecurityEvent("invalid_session", claims.UserID, ip, map[string]interface{}{
					"path":   path,
					"method": method,
					"error":  err.Error(),
				})
				utils.Unauthorized(c, "Invalid session")
				return
			}
		}

		// Update user last activity
		go updateUserActivity(user, ip, userAgent)

		// Set user context
		userContext := &UserContext{
			User:      user,
			UserID:    userID,
			Role:      user.Role,
			SessionID: claims.ID,
			DeviceID:  claims.DeviceID,
			Platform:  claims.PhoneNumber, // This might need adjustment based on your token structure
		}

		// Set context values
		c.Set("user", user)
		c.Set("user_id", userID)
		c.Set("user_role", string(user.Role))
		c.Set("session_id", claims.ID)
		c.Set("device_id", claims.DeviceID)
		c.Set("user_context", userContext)

		// Log successful authentication
		duration := time.Since(startTime)
		logger.LogUserAction(claims.UserID, "auth_success", "middleware", map[string]interface{}{
			"path":        path,
			"method":      method,
			"duration_ms": duration.Milliseconds(),
			"ip":          ip,
			"device_id":   claims.DeviceID,
		})

		c.Next()
	}
}

// AdminMiddleware ensures user has admin role
func AdminMiddleware(jwtSecret string) gin.HandlerFunc {
	return RoleMiddleware(jwtSecret, models.RoleAdmin, models.RoleSuper)
}

// ModeratorMiddleware ensures user has moderator role or higher
func ModeratorMiddleware(jwtSecret string) gin.HandlerFunc {
	return RoleMiddleware(jwtSecret, models.RoleModerator, models.RoleAdmin, models.RoleSuper)
}

// RoleMiddleware checks if user has one of the required roles
func RoleMiddleware(jwtSecret string, allowedRoles ...models.UserRole) gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		// First run authentication
		AuthMiddleware(jwtSecret)(c)

		// Check if authentication failed
		if c.IsAborted() {
			return
		}

		// Get user role from context
		userRole, exists := c.Get("user_role")
		if !exists {
			utils.Forbidden(c, "User role not found")
			return
		}

		role := models.UserRole(userRole.(string))

		// Check if user has required role
		hasPermission := false
		for _, allowedRole := range allowedRoles {
			if role == allowedRole {
				hasPermission = true
				break
			}
		}

		if !hasPermission {
			userID, _ := c.Get("user_id")
			logger.LogSecurityEvent("insufficient_permissions", userID.(primitive.ObjectID).Hex(), utils.GetClientIP(c), map[string]interface{}{
				"required_roles": allowedRoles,
				"user_role":      role,
				"path":           c.Request.URL.Path,
				"method":         c.Request.Method,
			})
			utils.Forbidden(c, "Insufficient permissions")
			return
		}

		c.Next()
	})
}

// SessionMiddleware validates session without requiring JWT
func SessionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.GetHeader("X-Session-ID")
		if sessionID == "" {
			// Try to get from cookie
			if cookie, err := c.Cookie("session_id"); err == nil {
				sessionID = cookie
			}
		}

		if sessionID == "" {
			utils.Unauthorized(c, "Session ID required")
			return
		}

		// Get session from Redis
		redisClient := redis.GetClient()
		if redisClient == nil {
			utils.InternalServerError(c, "Session store unavailable")
			return
		}

		session, err := redisClient.GetSession(sessionID)
		if err != nil {
			utils.Unauthorized(c, "Invalid session")
			return
		}

		// Parse user ID
		userID, err := primitive.ObjectIDFromHex(session.UserID)
		if err != nil {
			utils.Unauthorized(c, "Invalid session data")
			return
		}

		// Get user
		user, err := getUserFromDatabase(userID)
		if err != nil {
			utils.Unauthorized(c, "User not found")
			return
		}

		// Set context
		c.Set("user", user)
		c.Set("user_id", userID)
		c.Set("session_id", sessionID)
		c.Set("session_data", session)

		c.Next()
	}
}

// OptionalAuthMiddleware provides optional authentication
func OptionalAuthMiddleware(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

		token := extractToken(authHeader)
		if token == "" {
			c.Next()
			return
		}

		claims, err := utils.ValidateToken(token, jwtSecret)
		if err != nil {
			c.Next()
			return
		}

		userID, err := primitive.ObjectIDFromHex(claims.UserID)
		if err != nil {
			c.Next()
			return
		}

		user, err := getUserFromDatabase(userID)
		if err != nil {
			c.Next()
			return
		}

		// Set context if authentication succeeded
		c.Set("user", user)
		c.Set("user_id", userID)
		c.Set("user_role", string(user.Role))
		c.Set("authenticated", true)

		c.Next()
	}
}

// DeviceMiddleware validates device information
func DeviceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		deviceID := c.GetHeader("X-Device-ID")
		platform := c.GetHeader("X-Platform")
		appVersion := c.GetHeader("X-App-Version")

		if deviceID == "" {
			utils.BadRequest(c, "Device ID required")
			return
		}

		// Set device context
		c.Set("device_id", deviceID)
		c.Set("platform", platform)
		c.Set("app_version", appVersion)

		// Log device info for analytics
		userID, exists := c.Get("user_id")
		if exists {
			logger.LogUserAction(userID.(primitive.ObjectID).Hex(), "device_access", "middleware", map[string]interface{}{
				"device_id":   deviceID,
				"platform":    platform,
				"app_version": appVersion,
				"ip":          utils.GetClientIP(c),
			})
		}

		c.Next()
	}
}

// VerificationMiddleware ensures user phone is verified
func VerificationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := c.Get("user")
		if !exists {
			utils.Unauthorized(c, "Authentication required")
			return
		}

		userObj := user.(*models.User)
		if !userObj.IsPhoneVerified {
			utils.Forbidden(c, "Phone verification required")
			return
		}

		c.Next()
	}
}

// Helper functions

// shouldSkipAuth checks if path should skip authentication
func shouldSkipAuth(path string, skipPaths []string) bool {
	for _, skipPath := range skipPaths {
		if strings.HasPrefix(path, skipPath) {
			return true
		}
	}
	return false
}

// extractToken extracts JWT token from Authorization header
func extractToken(authHeader string) string {
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}
	return parts[1]
}

// getUserFromDatabase retrieves user from database
func getUserFromDatabase(userID primitive.ObjectID) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	collections := database.GetCollections()
	if collections == nil {
		return nil, fmt.Errorf("database not available")
	}

	var user models.User
	err := collections.Users.FindOne(ctx, bson.M{
		"_id":        userID,
		"is_active":  true,
		"is_deleted": false,
	}).Decode(&user)

	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	return &user, nil
}

// validateSession validates user session in Redis
func validateSession(claims *utils.JWTClaims, userID primitive.ObjectID, ip string) error {
	redisClient := redis.GetClient()
	if redisClient == nil {
		return nil // Skip session validation if Redis is not available
	}

	sessionKey := fmt.Sprintf("session:%s", claims.ID)
	exists, err := redisClient.Exists(sessionKey)
	if err != nil {
		logger.Error("Failed to check session in Redis:", err)
		return nil // Allow request if Redis check fails
	}

	if !exists {
		return fmt.Errorf("session not found")
	}

	// Get session data
	session, err := redisClient.GetSession(claims.ID)
	if err != nil {
		return fmt.Errorf("failed to get session data: %w", err)
	}

	// Validate session belongs to the same user
	if session.UserID != userID.Hex() {
		return fmt.Errorf("session user mismatch")
	}

	// Update session activity
	session.UpdatedAt = time.Now()
	if err := redisClient.SetSession(claims.ID, session, 24*time.Hour); err != nil {
		logger.Error("Failed to update session:", err)
	}

	return nil
}

// updateUserActivity updates user's last activity
func updateUserActivity(user *models.User, ip, userAgent string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	collections := database.GetCollections()
	if collections == nil {
		return
	}

	// Update user last seen and online status
	update := bson.M{
		"$set": bson.M{
			"last_seen":  time.Now(),
			"is_online":  true,
			"updated_at": time.Now(),
		},
	}

	_, err := collections.Users.UpdateOne(ctx, bson.M{"_id": user.ID}, update)
	if err != nil {
		logger.Error("Failed to update user activity:", err)
	}

	// Update Redis presence
	redisClient := redis.GetClient()
	if redisClient != nil {
		redisClient.SetUserOnline(user.ID.Hex(), "", 5*time.Minute)
	}

	// Log activity for analytics
	logger.LogUserAction(user.ID.Hex(), "activity_update", "system", map[string]interface{}{
		"ip":         ip,
		"user_agent": userAgent,
	})
}

// GetUserFromContext extracts user from gin context
func GetUserFromContext(c *gin.Context) (*models.User, error) {
	user, exists := c.Get("user")
	if !exists {
		return nil, fmt.Errorf("user not found in context")
	}

	userObj, ok := user.(*models.User)
	if !ok {
		return nil, fmt.Errorf("invalid user object in context")
	}

	return userObj, nil
}

// GetUserIDFromContext extracts user ID from gin context
func GetUserIDFromContext(c *gin.Context) (primitive.ObjectID, error) {
	userID, exists := c.Get("user_id")
	if !exists {
		return primitive.NilObjectID, fmt.Errorf("user ID not found in context")
	}

	userObjID, ok := userID.(primitive.ObjectID)
	if !ok {
		return primitive.NilObjectID, fmt.Errorf("invalid user ID type in context")
	}

	return userObjID, nil
}

// GetUserContextFromContext extracts user context from gin context
func GetUserContextFromContext(c *gin.Context) (*UserContext, error) {
	userContext, exists := c.Get("user_context")
	if !exists {
		return nil, fmt.Errorf("user context not found")
	}

	userCtx, ok := userContext.(*UserContext)
	if !ok {
		return nil, fmt.Errorf("invalid user context type")
	}

	return userCtx, nil
}

// RequireRole checks if user has required role
func RequireRole(c *gin.Context, requiredRoles ...models.UserRole) bool {
	userRole, exists := c.Get("user_role")
	if !exists {
		return false
	}

	role := models.UserRole(userRole.(string))
	for _, requiredRole := range requiredRoles {
		if role == requiredRole {
			return true
		}
	}

	return false
}

// IsAuthenticated checks if request is authenticated
func IsAuthenticated(c *gin.Context) bool {
	_, exists := c.Get("user")
	return exists
}

// IsAdmin checks if user is admin
func IsAdmin(c *gin.Context) bool {
	return RequireRole(c, models.RoleAdmin, models.RoleSuper)
}

// IsModerator checks if user is moderator or higher
func IsModerator(c *gin.Context) bool {
	return RequireRole(c, models.RoleModerator, models.RoleAdmin, models.RoleSuper)
}

// LogoutUser logs out user by invalidating session
func LogoutUser(c *gin.Context) error {
	sessionID, exists := c.Get("session_id")
	if !exists {
		return fmt.Errorf("session ID not found")
	}

	redisClient := redis.GetClient()
	if redisClient == nil {
		return fmt.Errorf("Redis client not available")
	}

	// Delete session from Redis
	if err := redisClient.DeleteSession(sessionID.(string)); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	// Update user online status
	userID, _ := GetUserIDFromContext(c)
	if !userID.IsZero() {
		redisClient.SetUserOffline(userID.Hex())

		// Log logout
		logger.LogUserAction(userID.Hex(), "logout", "auth", map[string]interface{}{
			"session_id": sessionID,
			"ip":         utils.GetClientIP(c),
		})
	}

	return nil
}

// RefreshUserData refreshes user data in context
func RefreshUserData(c *gin.Context) error {
	userID, err := GetUserIDFromContext(c)
	if err != nil {
		return err
	}

	user, err := getUserFromDatabase(userID)
	if err != nil {
		return err
	}

	// Update context
	c.Set("user", user)
	c.Set("user_role", string(user.Role))

	return nil
}
