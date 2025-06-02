package services

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"bro/internal/config"
	"bro/internal/models"
	"bro/internal/utils"
	"bro/pkg/database"
	"bro/pkg/logger"
	"bro/pkg/redis"
)

// AuthService handles authentication operations
type AuthService struct {
	config      *config.Config
	db          *mongo.Database
	collections *database.Collections
	redisClient *redis.Client
	encryption  *utils.EncryptionService
}

// LoginResult represents the result of a login attempt
type LoginResult struct {
	User         *models.User
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	IsNewUser    bool
}

// NewAuthService creates a new authentication service
func NewAuthService(db *mongo.Database, redisClient *redis.Client, config *config.Config) *AuthService {
	collections := database.GetCollections()

	encryption, err := utils.NewEncryptionService(config.EncryptionKey)
	if err != nil {
		logger.Fatalf("Failed to initialize encryption service: %v", err)
	}

	return &AuthService{
		config:      config,
		db:          db,
		collections: collections,
		redisClient: redisClient,
		encryption:  encryption,
	}
}

// RegisterUser registers a new user with phone number
func (as *AuthService) RegisterUser(req *models.UserRegisterRequest) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Validate phone number
	if err := utils.ValidatePhoneNumber(req.PhoneNumber, req.CountryCode); err != nil {
		return nil, fmt.Errorf("invalid phone number: %w", err)
	}

	// Check if user already exists
	fullPhoneNumber := req.CountryCode + req.PhoneNumber
	existingUser, err := as.GetUserByPhone(fullPhoneNumber)
	if err == nil && existingUser != nil {
		return nil, fmt.Errorf("user with phone number already exists")
	}

	// Create new user
	user := &models.User{
		PhoneNumber:     req.PhoneNumber,
		CountryCode:     req.CountryCode,
		FullPhoneNumber: fullPhoneNumber,
		Name:            req.Name,
		IsPhoneVerified: false,
		IsActive:        true,
	}

	// Hash password if provided
	if req.Password != "" {
		hashedPassword, err := utils.HashPassword(req.Password)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		user.PasswordHash = hashedPassword
	}

	// Set default values
	user.BeforeCreate()

	// Generate OTP
	otp := user.GenerateOTP()

	// Insert user into database
	result, err := as.collections.Users.InsertOne(ctx, user)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, fmt.Errorf("user with phone number already exists")
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	user.ID = result.InsertedID.(primitive.ObjectID)

	// Log user creation
	logger.LogUserAction(user.ID.Hex(), "user_registered", "auth", map[string]interface{}{
		"phone_number": fullPhoneNumber,
		"name":         user.Name,
	})

	// Return user and OTP (OTP should be sent via SMS in production)
	logger.Debugf("Generated OTP for user %s: %s", user.ID.Hex(), otp)

	return user, nil
}

// VerifyOTP verifies OTP and activates user account
func (as *AuthService) VerifyOTP(req *models.OTPVerificationRequest) (*LoginResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Find user by phone number
	fullPhoneNumber := req.CountryCode + req.PhoneNumber
	user, err := as.GetUserByPhone(fullPhoneNumber)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	// Validate OTP
	if !user.ValidateOTP(req.OTP) {
		logger.LogSecurityEvent("invalid_otp_attempt", user.ID.Hex(), "", map[string]interface{}{
			"phone_number": fullPhoneNumber,
			"attempts":     user.OTP.Attempts,
		})
		return nil, fmt.Errorf("invalid or expired OTP")
	}

	// Update user verification status
	update := bson.M{
		"$set": bson.M{
			"is_phone_verified": true,
			"otp.is_used":       true,
			"updated_at":        time.Now(),
		},
	}

	_, err = as.collections.Users.UpdateOne(ctx, bson.M{"_id": user.ID}, update)
	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	// Update user object
	user.IsPhoneVerified = true
	user.OTP.IsUsed = true

	// Generate JWT tokens
	tokens, err := as.generateTokenPair(user, "")
	if err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	// Create session
	err = as.createSession(user.ID, tokens.AccessToken, "")
	if err != nil {
		logger.Errorf("Failed to create session: %v", err)
	}

	// Log successful verification
	logger.LogUserAction(user.ID.Hex(), "otp_verified", "auth", map[string]interface{}{
		"phone_number": fullPhoneNumber,
	})

	return &LoginResult{
		User:         user,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    tokens.ExpiresAt,
		IsNewUser:    true,
	}, nil
}

// LoginWithPhone logs in a user with phone number and password
func (as *AuthService) LoginWithPhone(req *models.UserLoginRequest) (*LoginResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Find user by phone number
	fullPhoneNumber := req.CountryCode + req.PhoneNumber
	user, err := as.GetUserByPhone(fullPhoneNumber)
	if err != nil {
		logger.LogSecurityEvent("login_attempt_invalid_phone", "", "", map[string]interface{}{
			"phone_number": fullPhoneNumber,
		})
		return nil, fmt.Errorf("invalid credentials")
	}

	// Check if user is active
	if !user.IsActive || user.IsDeleted {
		logger.LogSecurityEvent("login_attempt_inactive_user", user.ID.Hex(), "", map[string]interface{}{
			"phone_number": fullPhoneNumber,
			"is_active":    user.IsActive,
			"is_deleted":   user.IsDeleted,
		})
		return nil, fmt.Errorf("account is inactive")
	}

	// Check password if provided
	if req.Password != "" && user.PasswordHash != "" {
		if !utils.VerifyPassword(req.Password, user.PasswordHash) {
			logger.LogSecurityEvent("login_attempt_invalid_password", user.ID.Hex(), "", map[string]interface{}{
				"phone_number": fullPhoneNumber,
			})
			return nil, fmt.Errorf("invalid credentials")
		}
	}

	// Check if phone is verified
	if !user.IsPhoneVerified {
		// Generate new OTP for verification
		otp := user.GenerateOTP()

		// Update user with new OTP
		update := bson.M{
			"$set": bson.M{
				"otp":        user.OTP,
				"updated_at": time.Now(),
			},
		}

		_, err = as.collections.Users.UpdateOne(ctx, bson.M{"_id": user.ID}, update)
		if err != nil {
			logger.Errorf("Failed to update OTP: %v", err)
		}

		logger.Debugf("Generated OTP for unverified user %s: %s", user.ID.Hex(), otp)
		return nil, fmt.Errorf("phone number not verified, OTP sent")
	}

	// Generate JWT tokens
	tokens, err := as.generateTokenPair(user, "")
	if err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	// Create session
	err = as.createSession(user.ID, tokens.AccessToken, "")
	if err != nil {
		logger.Errorf("Failed to create session: %v", err)
	}

	// Update last login
	_, err = as.collections.Users.UpdateOne(ctx, bson.M{"_id": user.ID}, bson.M{
		"$set": bson.M{
			"last_seen":  time.Now(),
			"is_online":  true,
			"updated_at": time.Now(),
		},
	})
	if err != nil {
		logger.Errorf("Failed to update last login: %v", err)
	}

	// Log successful login
	logger.LogUserAction(user.ID.Hex(), "login_success", "auth", map[string]interface{}{
		"phone_number": fullPhoneNumber,
		"method":       "phone",
	})

	return &LoginResult{
		User:         user,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    tokens.ExpiresAt,
		IsNewUser:    false,
	}, nil
}

// RefreshToken generates new access token from refresh token
func (as *AuthService) RefreshToken(refreshToken string) (*utils.TokenPair, error) {
	// Validate refresh token
	claims, err := utils.ValidateToken(refreshToken, as.config.JWTSecret)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	// Get user
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID in token")
	}

	user, err := as.GetUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	if !user.IsActive || user.IsDeleted {
		return nil, fmt.Errorf("user account is inactive")
	}

	// Generate new token pair
	tokens, err := as.generateTokenPair(user, claims.DeviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	// Update session
	err = as.updateSession(claims.ID, tokens.AccessToken)
	if err != nil {
		logger.Errorf("Failed to update session: %v", err)
	}

	logger.LogUserAction(user.ID.Hex(), "token_refreshed", "auth", map[string]interface{}{
		"device_id": claims.DeviceID,
	})

	return tokens, nil
}

// Logout logs out a user and invalidates session
func (as *AuthService) Logout(userID primitive.ObjectID, sessionID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Update user online status
	_, err := as.collections.Users.UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{
			"is_online":  false,
			"last_seen":  time.Now(),
			"updated_at": time.Now(),
		},
	})
	if err != nil {
		logger.Errorf("Failed to update user online status: %v", err)
	}

	// Delete session from Redis
	if as.redisClient != nil && sessionID != "" {
		err = as.redisClient.DeleteSession(sessionID)
		if err != nil {
			logger.Errorf("Failed to delete session: %v", err)
		}
	}

	logger.LogUserAction(userID.Hex(), "logout", "auth", map[string]interface{}{
		"session_id": sessionID,
	})

	return nil
}

// ResendOTP generates and resends OTP for a user
func (as *AuthService) ResendOTP(phoneNumber, countryCode string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Find user
	fullPhoneNumber := countryCode + phoneNumber
	user, err := as.GetUserByPhone(fullPhoneNumber)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	// Check if last OTP was sent recently (rate limiting)
	if time.Since(user.OTP.CreatedAt) < 60*time.Second {
		return fmt.Errorf("please wait before requesting new OTP")
	}

	// Generate new OTP
	otp := user.GenerateOTP()

	// Update user with new OTP
	update := bson.M{
		"$set": bson.M{
			"otp":        user.OTP,
			"updated_at": time.Now(),
		},
	}

	_, err = as.collections.Users.UpdateOne(ctx, bson.M{"_id": user.ID}, update)
	if err != nil {
		return fmt.Errorf("failed to update OTP: %w", err)
	}

	logger.LogUserAction(user.ID.Hex(), "otp_resent", "auth", map[string]interface{}{
		"phone_number": fullPhoneNumber,
	})

	// In production, send OTP via SMS
	logger.Debugf("Resent OTP for user %s: %s", user.ID.Hex(), otp)

	return nil
}

// GetUserByID retrieves user by ID
func (as *AuthService) GetUserByID(userID primitive.ObjectID) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err := as.collections.Users.FindOne(ctx, bson.M{
		"_id":        userID,
		"is_active":  true,
		"is_deleted": false,
	}).Decode(&user)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// GetUserByPhone retrieves user by phone number
func (as *AuthService) GetUserByPhone(fullPhoneNumber string) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err := as.collections.Users.FindOne(ctx, bson.M{
		"full_phone_number": fullPhoneNumber,
		"is_active":         true,
		"is_deleted":        false,
	}).Decode(&user)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// UpdateProfile updates user profile information
func (as *AuthService) UpdateProfile(userID primitive.ObjectID, req *models.UpdateProfileRequest) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Build update document
	update := bson.M{
		"$set": bson.M{
			"updated_at": time.Now(),
		},
	}

	if req.Name != "" {
		update["$set"].(bson.M)["name"] = req.Name
	}

	if req.Bio != "" {
		update["$set"].(bson.M)["bio"] = req.Bio
	}

	if req.Email != "" {
		// Validate email
		if err := utils.ValidateEmail(req.Email); err != nil {
			return nil, fmt.Errorf("invalid email: %w", err)
		}
		update["$set"].(bson.M)["email"] = req.Email
	}

	if req.Username != "" {
		// Validate username
		if err := utils.ValidateUsername(req.Username); err != nil {
			return nil, fmt.Errorf("invalid username: %w", err)
		}

		// Check if username is taken
		exists, err := as.isUsernameExists(req.Username, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to check username: %w", err)
		}
		if exists {
			return nil, fmt.Errorf("username already taken")
		}

		update["$set"].(bson.M)["username"] = req.Username
	}

	// Update user
	_, err := as.collections.Users.UpdateOne(ctx, bson.M{"_id": userID}, update)
	if err != nil {
		return nil, fmt.Errorf("failed to update profile: %w", err)
	}

	// Get updated user
	user, err := as.GetUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get updated user: %w", err)
	}

	logger.LogUserAction(userID.Hex(), "profile_updated", "auth", map[string]interface{}{
		"fields_updated": getUpdatedFields(req),
	})

	return user, nil
}

// ChangePassword changes user password
func (as *AuthService) ChangePassword(userID primitive.ObjectID, oldPassword, newPassword string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get user
	user, err := as.GetUserByID(userID)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	// Verify old password
	if user.PasswordHash != "" && !utils.VerifyPassword(oldPassword, user.PasswordHash) {
		return fmt.Errorf("invalid current password")
	}

	// Validate new password
	requirements := utils.DefaultPasswordRequirements()
	if err := utils.ValidatePassword(newPassword, requirements); err != nil {
		return fmt.Errorf("invalid new password: %w", err)
	}

	// Hash new password
	hashedPassword, err := utils.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password
	update := bson.M{
		"$set": bson.M{
			"password_hash": hashedPassword,
			"updated_at":    time.Now(),
		},
	}

	_, err = as.collections.Users.UpdateOne(ctx, bson.M{"_id": userID}, update)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	logger.LogUserAction(userID.Hex(), "password_changed", "auth", map[string]interface{}{})

	return nil
}

// Private helper methods

// generateTokenPair generates JWT access and refresh tokens
func (as *AuthService) generateTokenPair(user *models.User, deviceID string) (*utils.TokenPair, error) {
	return utils.GenerateTokenPair(
		user.ID.Hex(),
		user.FullPhoneNumber,
		string(user.Role),
		deviceID,
		as.config.JWTSecret,
	)
}

// createSession creates a new session in Redis
func (as *AuthService) createSession(userID primitive.ObjectID, accessToken, deviceID string) error {
	if as.redisClient == nil {
		return nil // Skip if Redis is not available
	}

	// Extract session ID from token
	claims, err := utils.ValidateToken(accessToken, as.config.JWTSecret)
	if err != nil {
		return err
	}

	session := &redis.SessionData{
		UserID:    userID.Hex(),
		DeviceID:  deviceID,
		Platform:  "", // This should be provided by client
		IP:        "",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		IsActive:  true,
	}

	return as.redisClient.SetSession(claims.ID, session, 24*time.Hour)
}

// updateSession updates existing session
func (as *AuthService) updateSession(sessionID, accessToken string) error {
	if as.redisClient == nil {
		return nil
	}

	return as.redisClient.UpdateSessionActivity(sessionID)
}

// isUsernameExists checks if username already exists
func (as *AuthService) isUsernameExists(username string, excludeUserID primitive.ObjectID) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{
		"username":   username,
		"is_active":  true,
		"is_deleted": false,
	}

	// Exclude current user if provided
	if !excludeUserID.IsZero() {
		filter["_id"] = bson.M{"$ne": excludeUserID}
	}

	count, err := as.collections.Users.CountDocuments(ctx, filter)
	return count > 0, err
}

// getUpdatedFields returns list of updated fields for logging
func getUpdatedFields(req *models.UpdateProfileRequest) []string {
	var fields []string
	if req.Name != "" {
		fields = append(fields, "name")
	}
	if req.Bio != "" {
		fields = append(fields, "bio")
	}
	if req.Email != "" {
		fields = append(fields, "email")
	}
	if req.Username != "" {
		fields = append(fields, "username")
	}
	return fields
}
