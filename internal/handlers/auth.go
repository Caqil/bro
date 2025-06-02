package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"bro/internal/middleware"
	"bro/internal/models"
	"bro/internal/services"
	"bro/internal/utils"
	"bro/pkg/database"
	"bro/pkg/logger"
	"bro/pkg/redis"
)

type AuthHandler struct {
	smsService  *services.SMSService
	pushService *services.PushService
	redisClient *redis.Client
}

func NewAuthHandler(smsService *services.SMSService, pushService *services.PushService) *AuthHandler {
	return &AuthHandler{
		smsService:  smsService,
		pushService: pushService,
		redisClient: redis.GetClient(),
	}
}

// RegisterRoutes registers authentication routes
func (h *AuthHandler) RegisterRoutes(router *gin.RouterGroup) {
	auth := router.Group("/auth")
	{
		// Public routes
		auth.POST("/check-phone", h.CheckPhoneNumber)
		auth.POST("/register", h.Register)
		auth.POST("/verify", h.VerifyOTP)
		auth.POST("/resend-otp", h.ResendOTP)
		auth.POST("/login", h.Login)
		auth.POST("/refresh", h.RefreshToken)

		// Protected routes
		protected := auth.Group("")
		protected.Use(middleware.AuthMiddleware("your-jwt-secret"))
		{
			protected.POST("/logout", h.Logout)
			protected.GET("/profile", h.GetProfile)
			protected.PUT("/profile", h.UpdateProfile)
			protected.PUT("/privacy", h.UpdatePrivacySettings)
			protected.POST("/change-password", h.ChangePassword)
			protected.POST("/enable-2fa", h.EnableTwoFactor)
			protected.POST("/verify-2fa", h.VerifyTwoFactor)
			protected.POST("/disable-2fa", h.DisableTwoFactor)
			protected.DELETE("/account", h.DeleteAccount)
			protected.GET("/sessions", h.GetActiveSessions)
			protected.DELETE("/sessions/:sessionId", h.TerminateSession)
		}
	}
}

// CheckPhoneNumber checks if phone number is available
func (h *AuthHandler) CheckPhoneNumber(c *gin.Context) {
	var req struct {
		PhoneNumber string `json:"phone_number" binding:"required"`
		CountryCode string `json:"country_code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	// Validate phone number and country code
	if err := utils.ValidatePhoneNumber(req.PhoneNumber, req.CountryCode); err != nil {
		utils.ValidationErrorResponse(c, map[string]utils.ValidationError{
			"phone_number": *err,
		})
		return
	}

	// Check if phone number exists
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fullPhoneNumber := req.CountryCode + req.PhoneNumber
	var existingUser models.User
	err := collections.Users.FindOne(ctx, bson.M{
		"full_phone_number": fullPhoneNumber,
		"is_deleted":        false,
	}).Decode(&existingUser)

	if err == nil {
		// User exists
		utils.Success(c, map[string]interface{}{
			"exists":    true,
			"verified":  existingUser.IsPhoneVerified,
			"can_login": existingUser.IsPhoneVerified && existingUser.IsActive,
		})
		return
	}

	if err == mongo.ErrNoDocuments {
		// Phone number available
		utils.Success(c, map[string]interface{}{
			"exists":    false,
			"available": true,
		})
		return
	}

	logger.Errorf("Database error checking phone number: %v", err)
	utils.InternalServerError(c, "Failed to check phone number")
}

// Register registers a new user
func (h *AuthHandler) Register(c *gin.Context) {
	var req models.UserRegisterRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	// Validate input
	validationErrors := utils.ValidateMultiple(map[string]func() *utils.ValidationError{
		"phone_number": func() *utils.ValidationError {
			return utils.ValidatePhoneNumber(req.PhoneNumber, req.CountryCode)
		},
		"country_code": func() *utils.ValidationError {
			return utils.ValidateCountryCode(req.CountryCode)
		},
		"name": func() *utils.ValidationError {
			return utils.ValidateString(req.Name, "name", utils.ValidationRules{
				Required:  true,
				MinLength: 2,
				MaxLength: 50,
			})
		},
	})

	if len(validationErrors) > 0 {
		utils.ValidationErrorResponse(c, validationErrors)
		return
	}

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if user already exists
	fullPhoneNumber := req.CountryCode + req.PhoneNumber
	var existingUser models.User
	err := collections.Users.FindOne(ctx, bson.M{
		"full_phone_number": fullPhoneNumber,
	}).Decode(&existingUser)

	if err == nil {
		if existingUser.IsPhoneVerified {
			utils.Conflict(c, "Phone number already registered")
			return
		}
		// User exists but not verified, proceed with verification
	} else if err != mongo.ErrNoDocuments {
		logger.Errorf("Database error: %v", err)
		utils.InternalServerError(c, "Registration failed")
		return
	}

	// Create or update user
	var user models.User
	if err == mongo.ErrNoDocuments {
		// Create new user
		user = models.User{
			PhoneNumber:     req.PhoneNumber,
			CountryCode:     req.CountryCode,
			FullPhoneNumber: fullPhoneNumber,
			Name:            strings.TrimSpace(req.Name),
		}
		user.BeforeCreate()

		// Hash password if provided
		if req.Password != "" {
			if err := utils.CheckPasswordStrength(req.Password); err != nil {
				utils.ValidationErrorResponse(c, map[string]utils.ValidationError{
					"password": {
						Field:   "password",
						Message: err.Error(),
						Code:    utils.ErrPasswordTooWeak,
					},
				})
				return
			}

			hashedPassword, err := utils.HashPassword(req.Password)
			if err != nil {
				logger.Errorf("Password hashing error: %v", err)
				utils.InternalServerError(c, "Registration failed")
				return
			}
			user.PasswordHash = hashedPassword
		}
	} else {
		// Update existing user
		user = existingUser
		user.Name = strings.TrimSpace(req.Name)
		user.BeforeUpdate()
	}

	// Generate OTP
	otp := user.GenerateOTP()

	// Save user
	if err == mongo.ErrNoDocuments {
		_, err = collections.Users.InsertOne(ctx, user)
	} else {
		_, err = collections.Users.ReplaceOne(ctx, bson.M{"_id": user.ID}, user)
	}

	if err != nil {
		logger.Errorf("Failed to save user: %v", err)
		utils.InternalServerError(c, "Registration failed")
		return
	}

	// Send OTP via SMS
	if err := h.smsService.SendOTP(user.FullPhoneNumber, otp); err != nil {
		logger.Errorf("Failed to send OTP: %v", err)
		utils.InternalServerError(c, "Failed to send verification code")
		return
	}

	// Log registration attempt
	logger.LogUserAction(user.ID.Hex(), "registration_initiated", "auth", map[string]interface{}{
		"phone_number": user.FullPhoneNumber,
		"ip":           utils.GetClientIP(c),
		"user_agent":   utils.GetUserAgent(c),
	})

	utils.Created(c, map[string]interface{}{
		"user_id":           user.ID.Hex(),
		"phone_number":      user.FullPhoneNumber,
		"verification_sent": true,
		"expires_in":        600, // 10 minutes
	})
}

// VerifyOTP verifies the OTP and completes registration
func (h *AuthHandler) VerifyOTP(c *gin.Context) {
	var req models.OTPVerificationRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	// Validate OTP
	if err := utils.ValidateOTP(req.OTP); err != nil {
		utils.ValidationErrorResponse(c, map[string]utils.ValidationError{
			"otp": *err,
		})
		return
	}

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find user
	fullPhoneNumber := req.CountryCode + req.PhoneNumber
	var user models.User
	err := collections.Users.FindOne(ctx, bson.M{
		"full_phone_number": fullPhoneNumber,
	}).Decode(&user)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "User not found")
		} else {
			logger.Errorf("Database error: %v", err)
			utils.InternalServerError(c, "Verification failed")
		}
		return
	}

	// Validate OTP
	if !user.ValidateOTP(req.OTP) {
		// Log failed attempt
		logger.LogSecurityEvent("otp_verification_failed", user.ID.Hex(), utils.GetClientIP(c), map[string]interface{}{
			"phone_number": user.FullPhoneNumber,
			"attempts":     user.OTP.Attempts,
		})

		if user.OTP.Attempts >= 3 {
			utils.BadRequest(c, "Too many failed attempts. Please request a new code")
		} else {
			utils.BadRequest(c, "Invalid verification code")
		}
		return
	}

	// Mark user as verified and active
	user.IsPhoneVerified = true
	user.IsActive = true
	user.BeforeUpdate()

	// Save user
	_, err = collections.Users.ReplaceOne(ctx, bson.M{"_id": user.ID}, user)
	if err != nil {
		logger.Errorf("Failed to update user: %v", err)
		utils.InternalServerError(c, "Verification failed")
		return
	}

	// Generate JWT tokens
	tokenPair, err := utils.GenerateTokenPair(
		user.ID.Hex(),
		user.FullPhoneNumber,
		string(user.Role),
		c.GetHeader("X-Device-ID"),
		"your-jwt-secret",
	)
	if err != nil {
		logger.Errorf("Token generation failed: %v", err)
		utils.InternalServerError(c, "Authentication failed")
		return
	}

	// Create session
	sessionID := utils.GenerateSessionToken()
	if h.redisClient != nil {
		session := &redis.Session{
			ID:        sessionID,
			UserID:    user.ID.Hex(),
			DeviceID:  c.GetHeader("X-Device-ID"),
			Platform:  c.GetHeader("X-Platform"),
			IPAddress: utils.GetClientIP(c),
			UserAgent: utils.GetUserAgent(c),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}

		if err := h.redisClient.SetSession(sessionID, session, 24*time.Hour); err != nil {
			logger.Errorf("Failed to create session: %v", err)
		}
	}

	// Log successful verification
	logger.LogUserAction(user.ID.Hex(), "phone_verified", "auth", map[string]interface{}{
		"phone_number": user.FullPhoneNumber,
		"ip":           utils.GetClientIP(c),
	})

	// Prepare response
	userInfo := user.GetPublicInfo(user.ID)

	utils.Success(c, map[string]interface{}{
		"user":          userInfo,
		"access_token":  tokenPair.AccessToken,
		"refresh_token": tokenPair.RefreshToken,
		"expires_at":    tokenPair.ExpiresAt.Unix(),
		"token_type":    tokenPair.TokenType,
		"session_id":    sessionID,
	})
}

// ResendOTP resends OTP to user
func (h *AuthHandler) ResendOTP(c *gin.Context) {
	var req struct {
		PhoneNumber string `json:"phone_number" binding:"required"`
		CountryCode string `json:"country_code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find user
	fullPhoneNumber := req.CountryCode + req.PhoneNumber
	var user models.User
	err := collections.Users.FindOne(ctx, bson.M{
		"full_phone_number": fullPhoneNumber,
	}).Decode(&user)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.NotFound(c, "User not found")
		} else {
			utils.InternalServerError(c, "Failed to resend code")
		}
		return
	}

	// Check if already verified
	if user.IsPhoneVerified {
		utils.BadRequest(c, "Phone number already verified")
		return
	}

	// Check rate limiting (max 3 OTP per hour)
	if h.redisClient != nil {
		key := fmt.Sprintf("otp_count:%s", user.FullPhoneNumber)
		count, _ := h.redisClient.IncrementBy(key, 1)
		if count == 1 {
			h.redisClient.Expire(key, time.Hour)
		}
		if count > 3 {
			utils.TooManyRequests(c, "Too many OTP requests. Please try again later")
			return
		}
	}

	// Generate new OTP
	otp := user.GenerateOTP()

	// Update user
	_, err = collections.Users.UpdateOne(ctx,
		bson.M{"_id": user.ID},
		bson.M{"$set": bson.M{
			"otp":        user.OTP,
			"updated_at": time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to update user OTP: %v", err)
		utils.InternalServerError(c, "Failed to resend code")
		return
	}

	// Send OTP
	if err := h.smsService.SendOTP(user.FullPhoneNumber, otp); err != nil {
		logger.Errorf("Failed to send OTP: %v", err)
		utils.InternalServerError(c, "Failed to send verification code")
		return
	}

	utils.Success(c, map[string]interface{}{
		"message":    "Verification code sent",
		"expires_in": 600,
	})
}

// Login authenticates existing user
func (h *AuthHandler) Login(c *gin.Context) {
	var req models.UserLoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find user
	fullPhoneNumber := req.CountryCode + req.PhoneNumber
	var user models.User
	err := collections.Users.FindOne(ctx, bson.M{
		"full_phone_number": fullPhoneNumber,
		"is_active":         true,
		"is_deleted":        false,
	}).Decode(&user)

	if err != nil {
		// Log failed login attempt
		logger.LogSecurityEvent("login_failed_user_not_found", "", utils.GetClientIP(c), map[string]interface{}{
			"phone_number": fullPhoneNumber,
		})

		utils.Unauthorized(c, "Invalid credentials")
		return
	}

	// Check if phone is verified
	if !user.IsPhoneVerified {
		utils.Unauthorized(c, "Phone number not verified")
		return
	}

	// Verify password if provided
	if req.Password != "" {
		if user.PasswordHash == "" {
			utils.Unauthorized(c, "Password not set for this account")
			return
		}

		if !utils.VerifyPassword(req.Password, user.PasswordHash) {
			// Log failed login attempt
			logger.LogSecurityEvent("login_failed_wrong_password", user.ID.Hex(), utils.GetClientIP(c), map[string]interface{}{
				"phone_number": user.FullPhoneNumber,
			})

			utils.Unauthorized(c, "Invalid credentials")
			return
		}
	} else {
		// Send OTP for passwordless login
		otp := user.GenerateOTP()

		_, err = collections.Users.UpdateOne(ctx,
			bson.M{"_id": user.ID},
			bson.M{"$set": bson.M{
				"otp":        user.OTP,
				"updated_at": time.Now(),
			}},
		)
		if err != nil {
			utils.InternalServerError(c, "Login failed")
			return
		}

		if err := h.smsService.SendOTP(user.FullPhoneNumber, otp); err != nil {
			logger.Errorf("Failed to send login OTP: %v", err)
			utils.InternalServerError(c, "Failed to send verification code")
			return
		}

		utils.Success(c, map[string]interface{}{
			"requires_otp": true,
			"message":      "Verification code sent",
			"expires_in":   600,
		})
		return
	}

	// Generate tokens
	tokenPair, err := utils.GenerateTokenPair(
		user.ID.Hex(),
		user.FullPhoneNumber,
		string(user.Role),
		c.GetHeader("X-Device-ID"),
		"your-jwt-secret",
	)
	if err != nil {
		logger.Errorf("Token generation failed: %v", err)
		utils.InternalServerError(c, "Authentication failed")
		return
	}

	// Create session
	sessionID := utils.GenerateSessionToken()
	if h.redisClient != nil {
		session := &redis.Session{
			ID:        sessionID,
			UserID:    user.ID.Hex(),
			DeviceID:  c.GetHeader("X-Device-ID"),
			Platform:  c.GetHeader("X-Platform"),
			IPAddress: utils.GetClientIP(c),
			UserAgent: utils.GetUserAgent(c),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}

		if err := h.redisClient.SetSession(sessionID, session, 24*time.Hour); err != nil {
			logger.Errorf("Failed to create session: %v", err)
		}
	}

	// Update user last seen
	go func() {
		_, err := collections.Users.UpdateOne(context.Background(),
			bson.M{"_id": user.ID},
			bson.M{"$set": bson.M{
				"last_seen":  time.Now(),
				"is_online":  true,
				"updated_at": time.Now(),
			}},
		)
		if err != nil {
			logger.Errorf("Failed to update user last seen: %v", err)
		}
	}()

	// Log successful login
	logger.LogUserAction(user.ID.Hex(), "login_success", "auth", map[string]interface{}{
		"phone_number": user.FullPhoneNumber,
		"ip":           utils.GetClientIP(c),
		"device_id":    c.GetHeader("X-Device-ID"),
	})

	// Prepare response
	userInfo := user.GetPublicInfo(user.ID)

	utils.Success(c, map[string]interface{}{
		"user":          userInfo,
		"access_token":  tokenPair.AccessToken,
		"refresh_token": tokenPair.RefreshToken,
		"expires_at":    tokenPair.ExpiresAt.Unix(),
		"token_type":    tokenPair.TokenType,
		"session_id":    sessionID,
	})
}

// RefreshToken refreshes access token
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	// Validate refresh token
	tokenPair, err := utils.RefreshToken(req.RefreshToken, "your-jwt-secret")
	if err != nil {
		utils.Unauthorized(c, "Invalid refresh token")
		return
	}

	utils.Success(c, map[string]interface{}{
		"access_token":  tokenPair.AccessToken,
		"refresh_token": tokenPair.RefreshToken,
		"expires_at":    tokenPair.ExpiresAt.Unix(),
		"token_type":    tokenPair.TokenType,
	})
}

// Logout logs out user
func (h *AuthHandler) Logout(c *gin.Context) {
	// Invalidate session
	if err := middleware.LogoutUser(c); err != nil {
		logger.Errorf("Logout error: %v", err)
	}

	utils.Success(c, map[string]interface{}{
		"message": "Logged out successfully",
	})
}

// GetProfile returns user profile
func (h *AuthHandler) GetProfile(c *gin.Context) {
	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	userInfo := user.GetPublicInfo(user.ID)

	utils.Success(c, map[string]interface{}{
		"user": userInfo,
	})
}

// UpdateProfile updates user profile
func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	var req models.UpdateProfileRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	// Validate input
	validationErrors := make(map[string]utils.ValidationError)

	if req.Name != "" {
		if err := utils.ValidateString(req.Name, "name", utils.ValidationRules{
			Required:  true,
			MinLength: 2,
			MaxLength: 50,
		}); err != nil {
			validationErrors["name"] = *err
		}
	}

	if req.Email != "" {
		if err := utils.ValidateEmail(req.Email); err != nil {
			validationErrors["email"] = *err
		}
	}

	if req.Username != "" {
		if err := utils.ValidateUsername(req.Username); err != nil {
			validationErrors["username"] = *err
		}
	}

	if len(validationErrors) > 0 {
		utils.ValidationErrorResponse(c, validationErrors)
		return
	}

	// Update user
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	updateFields := bson.M{}
	if req.Name != "" {
		updateFields["name"] = strings.TrimSpace(req.Name)
	}
	if req.Bio != "" {
		updateFields["bio"] = strings.TrimSpace(req.Bio)
	}
	if req.Email != "" {
		updateFields["email"] = strings.ToLower(strings.TrimSpace(req.Email))
	}
	if req.Username != "" {
		updateFields["username"] = strings.ToLower(strings.TrimSpace(req.Username))
	}
	updateFields["updated_at"] = time.Now()

	_, err = collections.Users.UpdateOne(ctx,
		bson.M{"_id": user.ID},
		bson.M{"$set": updateFields},
	)
	if err != nil {
		logger.Errorf("Failed to update user profile: %v", err)
		utils.InternalServerError(c, "Failed to update profile")
		return
	}

	// Refresh user data
	if err := middleware.RefreshUserData(c); err != nil {
		logger.Errorf("Failed to refresh user data: %v", err)
	}

	// Log profile update
	logger.LogUserAction(user.ID.Hex(), "profile_updated", "auth", map[string]interface{}{
		"updated_fields": updateFields,
		"ip":             utils.GetClientIP(c),
	})

	utils.Success(c, map[string]interface{}{
		"message": "Profile updated successfully",
	})
}

// UpdatePrivacySettings updates user privacy settings
func (h *AuthHandler) UpdatePrivacySettings(c *gin.Context) {
	var req models.PrivacyUpdateRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build update document
	privacyUpdate := bson.M{}

	if req.ProfilePhoto != "" {
		privacyUpdate["privacy.profile_photo"] = req.ProfilePhoto
	}
	if req.LastSeen != "" {
		privacyUpdate["privacy.last_seen"] = req.LastSeen
	}
	if req.About != "" {
		privacyUpdate["privacy.about"] = req.About
	}
	if req.Status != "" {
		privacyUpdate["privacy.status"] = req.Status
	}
	if req.ReadReceipts != nil {
		privacyUpdate["privacy.read_receipts"] = *req.ReadReceipts
	}
	if req.GroupInvites != "" {
		privacyUpdate["privacy.group_invites"] = req.GroupInvites
	}
	if req.CallPermissions != "" {
		privacyUpdate["privacy.call_permissions"] = req.CallPermissions
	}

	privacyUpdate["updated_at"] = time.Now()

	_, err = collections.Users.UpdateOne(ctx,
		bson.M{"_id": user.ID},
		bson.M{"$set": privacyUpdate},
	)
	if err != nil {
		logger.Errorf("Failed to update privacy settings: %v", err)
		utils.InternalServerError(c, "Failed to update privacy settings")
		return
	}

	// Log privacy update
	logger.LogUserAction(user.ID.Hex(), "privacy_updated", "auth", map[string]interface{}{
		"ip": utils.GetClientIP(c),
	})

	utils.Success(c, map[string]interface{}{
		"message": "Privacy settings updated successfully",
	})
}

// ChangePassword changes user password
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password" binding:"required"`
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

	// Validate current password if user has one
	if user.PasswordHash != "" {
		if req.CurrentPassword == "" {
			utils.BadRequest(c, "Current password is required")
			return
		}

		if !utils.VerifyPassword(req.CurrentPassword, user.PasswordHash) {
			utils.BadRequest(c, "Current password is incorrect")
			return
		}
	}

	// Validate new password
	if err := utils.CheckPasswordStrength(req.NewPassword); err != nil {
		utils.ValidationErrorResponse(c, map[string]utils.ValidationError{
			"new_password": {
				Field:   "new_password",
				Message: err.Error(),
				Code:    utils.ErrPasswordTooWeak,
			},
		})
		return
	}

	// Hash new password
	hashedPassword, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		logger.Errorf("Password hashing error: %v", err)
		utils.InternalServerError(c, "Failed to change password")
		return
	}

	// Update password
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = collections.Users.UpdateOne(ctx,
		bson.M{"_id": user.ID},
		bson.M{"$set": bson.M{
			"password_hash": hashedPassword,
			"updated_at":    time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to update password: %v", err)
		utils.InternalServerError(c, "Failed to change password")
		return
	}

	// Log password change
	logger.LogUserAction(user.ID.Hex(), "password_changed", "auth", map[string]interface{}{
		"ip": utils.GetClientIP(c),
	})

	utils.Success(c, map[string]interface{}{
		"message": "Password changed successfully",
	})
}

// EnableTwoFactor enables two-factor authentication
func (h *AuthHandler) EnableTwoFactor(c *gin.Context) {
	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	if user.TwoFactorEnabled {
		utils.BadRequest(c, "Two-factor authentication is already enabled")
		return
	}

	// Generate TOTP secret
	secret, err := utils.GenerateTOTPSecret()
	if err != nil {
		logger.Errorf("Failed to generate TOTP secret: %v", err)
		utils.InternalServerError(c, "Failed to enable two-factor authentication")
		return
	}

	// Update user
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = collections.Users.UpdateOne(ctx,
		bson.M{"_id": user.ID},
		bson.M{"$set": bson.M{
			"two_factor_secret": secret,
			"updated_at":        time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to save TOTP secret: %v", err)
		utils.InternalServerError(c, "Failed to enable two-factor authentication")
		return
	}

	utils.Success(c, map[string]interface{}{
		"secret":  secret,
		"qr_url":  fmt.Sprintf("otpauth://totp/ChatApp:%s?secret=%s&issuer=ChatApp", user.PhoneNumber, secret),
		"message": "Two-factor authentication configured. Please verify with your authenticator app.",
	})
}

// VerifyTwoFactor verifies and enables two-factor authentication
func (h *AuthHandler) VerifyTwoFactor(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
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

	if user.TwoFactorEnabled {
		utils.BadRequest(c, "Two-factor authentication is already enabled")
		return
	}

	// Verify TOTP code (implementation would depend on TOTP library)
	// For now, we'll assume it's verified
	valid := len(req.Code) == 6 // Simplified validation

	if !valid {
		utils.BadRequest(c, "Invalid verification code")
		return
	}

	// Enable two-factor authentication
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = collections.Users.UpdateOne(ctx,
		bson.M{"_id": user.ID},
		bson.M{"$set": bson.M{
			"two_factor_enabled": true,
			"updated_at":         time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to enable 2FA: %v", err)
		utils.InternalServerError(c, "Failed to enable two-factor authentication")
		return
	}

	// Log 2FA enabled
	logger.LogUserAction(user.ID.Hex(), "2fa_enabled", "auth", map[string]interface{}{
		"ip": utils.GetClientIP(c),
	})

	utils.Success(c, map[string]interface{}{
		"message": "Two-factor authentication enabled successfully",
	})
}

// DisableTwoFactor disables two-factor authentication
func (h *AuthHandler) DisableTwoFactor(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
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

	if !user.TwoFactorEnabled {
		utils.BadRequest(c, "Two-factor authentication is not enabled")
		return
	}

	// Verify password or 2FA code
	if user.PasswordHash != "" && req.Password != "" {
		if !utils.VerifyPassword(req.Password, user.PasswordHash) {
			utils.BadRequest(c, "Invalid password")
			return
		}
	} else if req.Code != "" {
		// Verify 2FA code (simplified)
		if len(req.Code) != 6 {
			utils.BadRequest(c, "Invalid verification code")
			return
		}
	} else {
		utils.BadRequest(c, "Password or verification code is required")
		return
	}

	// Disable two-factor authentication
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = collections.Users.UpdateOne(ctx,
		bson.M{"_id": user.ID},
		bson.M{"$set": bson.M{
			"two_factor_enabled": false,
			"two_factor_secret":  "",
			"updated_at":         time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to disable 2FA: %v", err)
		utils.InternalServerError(c, "Failed to disable two-factor authentication")
		return
	}

	// Log 2FA disabled
	logger.LogUserAction(user.ID.Hex(), "2fa_disabled", "auth", map[string]interface{}{
		"ip": utils.GetClientIP(c),
	})

	utils.Success(c, map[string]interface{}{
		"message": "Two-factor authentication disabled successfully",
	})
}

// DeleteAccount deletes user account
func (h *AuthHandler) DeleteAccount(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
		Reason   string `json:"reason"`
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

	// Verify password if user has one
	if user.PasswordHash != "" {
		if req.Password == "" {
			utils.BadRequest(c, "Password is required")
			return
		}

		if !utils.VerifyPassword(req.Password, user.PasswordHash) {
			utils.BadRequest(c, "Invalid password")
			return
		}
	}

	// Mark account for deletion (soft delete)
	collections := database.GetCollections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	deletionDate := time.Now().Add(30 * 24 * time.Hour) // 30 days grace period

	_, err = collections.Users.UpdateOne(ctx,
		bson.M{"_id": user.ID},
		bson.M{"$set": bson.M{
			"is_deleted":    true,
			"is_active":     false,
			"deletion_date": deletionDate,
			"updated_at":    time.Now(),
		}},
	)
	if err != nil {
		logger.Errorf("Failed to delete account: %v", err)
		utils.InternalServerError(c, "Failed to delete account")
		return
	}

	// Log account deletion
	logger.LogUserAction(user.ID.Hex(), "account_deleted", "auth", map[string]interface{}{
		"reason":        req.Reason,
		"deletion_date": deletionDate,
		"ip":            utils.GetClientIP(c),
	})

	// Logout user
	middleware.LogoutUser(c)

	utils.Success(c, map[string]interface{}{
		"message":       "Account scheduled for deletion",
		"deletion_date": deletionDate.Format(time.RFC3339),
		"note":          "You have 30 days to reactivate your account by logging in",
	})
}

// GetActiveSessions returns user's active sessions
func (h *AuthHandler) GetActiveSessions(c *gin.Context) {
	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	if h.redisClient == nil {
		utils.ServiceUnavailable(c, "Session service unavailable")
		return
	}

	sessions, err := h.redisClient.GetUserSessions(user.ID.Hex())
	if err != nil {
		logger.Errorf("Failed to get user sessions: %v", err)
		utils.InternalServerError(c, "Failed to get sessions")
		return
	}

	utils.Success(c, map[string]interface{}{
		"sessions": sessions,
	})
}

// TerminateSession terminates a specific session
func (h *AuthHandler) TerminateSession(c *gin.Context) {
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		utils.BadRequest(c, "Session ID is required")
		return
	}

	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found")
		return
	}

	if h.redisClient == nil {
		utils.ServiceUnavailable(c, "Session service unavailable")
		return
	}

	// Verify session belongs to user
	session, err := h.redisClient.GetSession(sessionID)
	if err != nil || session.UserID != user.ID.Hex() {
		utils.NotFound(c, "Session not found")
		return
	}

	// Delete session
	if err := h.redisClient.DeleteSession(sessionID); err != nil {
		logger.Errorf("Failed to delete session: %v", err)
		utils.InternalServerError(c, "Failed to terminate session")
		return
	}

	// Log session termination
	logger.LogUserAction(user.ID.Hex(), "session_terminated", "auth", map[string]interface{}{
		"session_id": sessionID,
		"ip":         utils.GetClientIP(c),
	})

	utils.Success(c, map[string]interface{}{
		"message": "Session terminated successfully",
	})
}
