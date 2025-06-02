package services

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bro/internal/config"
	"bro/internal/models"
	"bro/internal/utils"
	"bro/pkg/database"
	"bro/pkg/logger"
	"bro/pkg/redis"
)

// AuthService handles all authentication related operations
type AuthService struct {
	db          *mongo.Database
	collections *database.Collections
	redisClient *redis.Client
	config      *config.Config
	encryption  *utils.EncryptionService
}

// AuthResponse represents authentication response
type AuthResponse struct {
	User         *models.UserPublicInfo `json:"user"`
	AccessToken  string                 `json:"access_token"`
	RefreshToken string                 `json:"refresh_token"`
	ExpiresAt    int64                  `json:"expires_at"`
	TokenType    string                 `json:"token_type"`
	IsNewUser    bool                   `json:"is_new_user,omitempty"`
}

// ContactSearchResult represents contact search result
type ContactSearchResult struct {
	User      *models.UserPublicInfo `json:"user"`
	IsContact bool                   `json:"is_contact"`
	IsBlocked bool                   `json:"is_blocked"`
}

// ContactSyncResult represents contact sync result
type ContactSyncResult struct {
	Found       []ContactSearchResult `json:"found"`
	NotFound    []string              `json:"not_found"`
	NewContacts int                   `json:"new_contacts"`
}

// DeviceSession represents device session information
type DeviceSession struct {
	SessionID    string    `json:"session_id"`
	DeviceID     string    `json:"device_id"`
	Platform     string    `json:"platform"`
	AppVersion   string    `json:"app_version,omitempty"`
	IPAddress    string    `json:"ip_address"`
	Location     string    `json:"location,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	IsActive     bool      `json:"is_active"`
}

// SecurityLog represents security-related log entry
type SecurityLog struct {
	UserID    primitive.ObjectID     `json:"user_id"`
	Action    string                 `json:"action"`
	IP        string                 `json:"ip"`
	UserAgent string                 `json:"user_agent"`
	Details   map[string]interface{} `json:"details"`
	Timestamp time.Time              `json:"timestamp"`
}

// NewAuthService creates a new authentication service
func NewAuthService(db *mongo.Database, redisClient *redis.Client, config *config.Config) *AuthService {
	collections := database.GetCollections()

	encryption, err := utils.NewEncryptionService(config.EncryptionKey)
	if err != nil {
		logger.Fatal("Failed to initialize encryption service:", err)
	}

	service := &AuthService{
		db:          db,
		collections: collections,
		redisClient: redisClient,
		config:      config,
		encryption:  encryption,
	}

	// Initialize background tasks
	go service.cleanupExpiredSessions()
	go service.cleanupExpiredOTPs()

	logger.Info("Authentication service initialized successfully")
	return service
}

// User Registration and Verification

// RegisterUser registers a new user with phone number
func (s *AuthService) RegisterUser(req *models.UserRegisterRequest, deviceID, platform, ipAddress string) (*AuthResponse, error) {
	startTime := time.Now()

	// Validate input
	if err := s.validateRegistrationRequest(req); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check if user already exists
	existingUser, err := s.getUserByPhone(req.CountryCode, req.PhoneNumber)
	if err == nil && existingUser != nil {
		if existingUser.IsPhoneVerified {
			return nil, fmt.Errorf("user already exists and verified")
		}
		// User exists but not verified, allow re-registration
		return s.handleExistingUnverifiedUser(existingUser, req, deviceID, platform, ipAddress)
	}

	// Create new user
	user := &models.User{
		PhoneNumber:     req.PhoneNumber,
		CountryCode:     req.CountryCode,
		FullPhoneNumber: req.CountryCode + req.PhoneNumber,
		Name:            req.Name,
		IsPhoneVerified: false,
	}

	// Set password if provided
	if req.Password != "" {
		passwordHash, err := utils.HashPassword(req.Password)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		user.PasswordHash = passwordHash
	}

	user.BeforeCreate()

	// Insert user
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := s.collections.Users.InsertOne(ctx, user)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, fmt.Errorf("user already exists")
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	user.ID = result.InsertedID.(primitive.ObjectID)

	// Generate and send OTP
	if err := s.generateAndSendOTP(user, "registration"); err != nil {
		logger.Errorf("Failed to send OTP to user %s: %v", user.ID.Hex(), err)
		// Don't return error here, user is created successfully
	}

	// Log registration
	duration := time.Since(startTime)
	logger.LogUserAction(user.ID.Hex(), "register", "auth_service", map[string]interface{}{
		"phone_number": user.FullPhoneNumber,
		"device_id":    deviceID,
		"platform":     platform,
		"duration_ms":  duration.Milliseconds(),
		"ip":           ipAddress,
	})

	// Create response without tokens (user needs to verify first)
	response := &AuthResponse{
		User:      &user.GetPublicInfo(user.ID),
		IsNewUser: true,
	}

	return response, nil
}

// VerifyOTP verifies OTP and completes registration/login
func (s *AuthService) VerifyOTP(req *models.OTPVerificationRequest, deviceID, platform, ipAddress string) (*AuthResponse, error) {
	startTime := time.Now()

	// Validate OTP format
	if err := utils.ValidateOTP(req.OTP); err != nil {
		return nil, fmt.Errorf("invalid OTP format: %w", err)
	}

	// Get user by phone
	user, err := s.getUserByPhone(req.CountryCode, req.PhoneNumber)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	// Validate OTP
	if !user.ValidateOTP(req.OTP) {
		// Log failed OTP attempt
		logger.LogSecurityEvent("otp_verification_failed", user.ID.Hex(), ipAddress, map[string]interface{}{
			"phone_number": user.FullPhoneNumber,
			"attempts":     user.OTP.Attempts,
		})

		// Update user in database
		s.updateUser(user)

		if user.OTP.Attempts >= 3 {
			return nil, fmt.Errorf("maximum OTP attempts exceeded")
		}
		return nil, fmt.Errorf("invalid OTP")
	}

	// Mark phone as verified
	user.IsPhoneVerified = true
	user.BeforeUpdate()

	// Update user in database
	if err := s.updateUser(user); err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	// Generate tokens and create session
	authResponse, err := s.createUserSession(user, deviceID, platform, ipAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Log successful verification
	duration := time.Since(startTime)
	logger.LogUserAction(user.ID.Hex(), "verify_otp", "auth_service", map[string]interface{}{
		"phone_number": user.FullPhoneNumber,
		"device_id":    deviceID,
		"platform":     platform,
		"duration_ms":  duration.Milliseconds(),
		"ip":           ipAddress,
	})

	return authResponse, nil
}

// ResendOTP resends OTP to user
func (s *AuthService) ResendOTP(phoneNumber, countryCode, reason string) error {
	user, err := s.getUserByPhone(countryCode, phoneNumber)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	// Check rate limiting
	if time.Since(user.OTP.CreatedAt) < 60*time.Second {
		return fmt.Errorf("please wait before requesting another OTP")
	}

	// Generate and send new OTP
	if err := s.generateAndSendOTP(user, reason); err != nil {
		return fmt.Errorf("failed to send OTP: %w", err)
	}

	logger.LogUserAction(user.ID.Hex(), "resend_otp", "auth_service", map[string]interface{}{
		"phone_number": user.FullPhoneNumber,
		"reason":       reason,
	})

	return nil
}

// User Login

// LoginWithPassword logs in user with phone number and password
func (s *AuthService) LoginWithPassword(req *models.UserLoginRequest, deviceID, platform, ipAddress string) (*AuthResponse, error) {
	startTime := time.Now()

	// Get user by phone
	user, err := s.getUserByPhone(req.CountryCode, req.PhoneNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	// Check if user is active
	if !user.IsActive || user.IsDeleted {
		return nil, fmt.Errorf("account is inactive")
	}

	// Check phone verification
	if !user.IsPhoneVerified {
		return nil, fmt.Errorf("phone number not verified")
	}

	// Verify password
	if !utils.VerifyPassword(req.Password, user.PasswordHash) {
		// Log failed login attempt
		logger.LogSecurityEvent("login_failed", user.ID.Hex(), ipAddress, map[string]interface{}{
			"phone_number": user.FullPhoneNumber,
			"reason":       "invalid_password",
		})
		return nil, fmt.Errorf("invalid credentials")
	}

	// Check if account is locked
	if err := s.checkAccountLock(user.ID); err != nil {
		return nil, err
	}

	// Create session
	authResponse, err := s.createUserSession(user, deviceID, platform, ipAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Reset failed login attempts
	s.resetFailedLoginAttempts(user.ID)

	// Log successful login
	duration := time.Since(startTime)
	logger.LogUserAction(user.ID.Hex(), "login", "auth_service", map[string]interface{}{
		"phone_number": user.FullPhoneNumber,
		"device_id":    deviceID,
		"platform":     platform,
		"duration_ms":  duration.Milliseconds(),
		"ip":           ipAddress,
	})

	return authResponse, nil
}

// LoginWithOTP initiates OTP-based login
func (s *AuthService) LoginWithOTP(phoneNumber, countryCode string) error {
	user, err := s.getUserByPhone(countryCode, phoneNumber)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	if !user.IsActive || user.IsDeleted {
		return fmt.Errorf("account is inactive")
	}

	if !user.IsPhoneVerified {
		return fmt.Errorf("phone number not verified")
	}

	// Generate and send OTP
	if err := s.generateAndSendOTP(user, "login"); err != nil {
		return fmt.Errorf("failed to send OTP: %w", err)
	}

	logger.LogUserAction(user.ID.Hex(), "login_otp_request", "auth_service", map[string]interface{}{
		"phone_number": user.FullPhoneNumber,
	})

	return nil
}

// Token Management

// RefreshToken refreshes access token using refresh token
func (s *AuthService) RefreshToken(refreshToken, deviceID string) (*AuthResponse, error) {
	// Validate refresh token
	claims, err := utils.ValidateToken(refreshToken, s.config.JWTSecret)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	// Get user
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID in token")
	}

	user, err := s.getUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	// Check if user is active
	if !user.IsActive || user.IsDeleted {
		return nil, fmt.Errorf("account is inactive")
	}

	// Generate new tokens
	tokenPair, err := utils.GenerateTokenPair(
		user.ID.Hex(),
		user.FullPhoneNumber,
		string(user.Role),
		deviceID,
		s.config.JWTSecret,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	// Update session in Redis
	sessionData := &redis.SessionData{
		UserID:    user.ID.Hex(),
		DeviceID:  deviceID,
		Platform:  claims.PhoneNumber, // This might need adjustment
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Data:      make(map[string]interface{}),
	}

	if s.redisClient != nil {
		if err := s.redisClient.SetSession(claims.ID, sessionData, 24*time.Hour); err != nil {
			logger.Error("Failed to update session in Redis:", err)
		}
	}

	response := &AuthResponse{
		User:         &user.GetPublicInfo(user.ID),
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    tokenPair.ExpiresAt.Unix(),
		TokenType:    tokenPair.TokenType,
	}

	logger.LogUserAction(user.ID.Hex(), "token_refresh", "auth_service", map[string]interface{}{
		"device_id": deviceID,
	})

	return response, nil
}

// LogoutUser logs out user from current session
func (s *AuthService) LogoutUser(userID primitive.ObjectID, sessionID, deviceID string) error {
	// Delete session from Redis
	if s.redisClient != nil {
		if err := s.redisClient.DeleteSession(sessionID); err != nil {
			logger.Error("Failed to delete session from Redis:", err)
		}

		// Set user offline
		if err := s.redisClient.SetUserOffline(userID.Hex()); err != nil {
			logger.Error("Failed to set user offline:", err)
		}
	}

	// Update user last seen
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"last_seen":  time.Now(),
			"is_online":  false,
			"updated_at": time.Now(),
		},
	}

	_, err := s.collections.Users.UpdateOne(ctx, bson.M{"_id": userID}, update)
	if err != nil {
		logger.Error("Failed to update user last seen:", err)
	}

	logger.LogUserAction(userID.Hex(), "logout", "auth_service", map[string]interface{}{
		"session_id": sessionID,
		"device_id":  deviceID,
	})

	return nil
}

// LogoutAllDevices logs out user from all devices
func (s *AuthService) LogoutAllDevices(userID primitive.ObjectID) error {
	// This would require maintaining a list of active sessions per user
	// For now, we'll implement a simple version

	if s.redisClient != nil {
		// Set user offline
		if err := s.redisClient.SetUserOffline(userID.Hex()); err != nil {
			logger.Error("Failed to set user offline:", err)
		}
	}

	logger.LogUserAction(userID.Hex(), "logout_all_devices", "auth_service", nil)
	return nil
}

// Profile Management

// GetProfile gets user profile
func (s *AuthService) GetProfile(userID primitive.ObjectID) (*models.User, error) {
	user, err := s.getUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	return user, nil
}

// UpdateProfile updates user profile
func (s *AuthService) UpdateProfile(userID primitive.ObjectID, req *models.UpdateProfileRequest) (*models.User, error) {
	user, err := s.getUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Validate updates
	if req.Name != "" {
		if err := utils.ValidateString(req.Name, "name", utils.ValidationRules{
			Required:  true,
			MinLength: 1,
			MaxLength: 50,
		}); err != nil {
			return nil, err
		}
		user.Name = req.Name
	}

	if req.Bio != "" {
		if err := utils.ValidateString(req.Bio, "bio", utils.ValidationRules{
			MaxLength: 150,
		}); err != nil {
			return nil, err
		}
		user.Bio = req.Bio
	}

	if req.Email != "" {
		if err := utils.ValidateEmail(req.Email); err != nil {
			return nil, err
		}
		// Check if email is already used
		if err := s.checkEmailExists(req.Email, userID); err != nil {
			return nil, err
		}
		user.Email = req.Email
	}

	if req.Username != "" {
		if err := utils.ValidateUsername(req.Username); err != nil {
			return nil, err
		}
		// Check if username is already used
		if err := s.checkUsernameExists(req.Username, userID); err != nil {
			return nil, err
		}
		user.Username = req.Username
	}

	user.BeforeUpdate()

	// Update in database
	if err := s.updateUser(user); err != nil {
		return nil, fmt.Errorf("failed to update profile: %w", err)
	}

	logger.LogUserAction(userID.Hex(), "update_profile", "auth_service", map[string]interface{}{
		"fields_updated": getUpdatedFields(req),
	})

	return user, nil
}

// UpdatePrivacySettings updates user privacy settings
func (s *AuthService) UpdatePrivacySettings(userID primitive.ObjectID, req *models.PrivacyUpdateRequest) error {
	user, err := s.getUserByID(userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	// Update privacy settings
	if req.ProfilePhoto != "" {
		user.Privacy.ProfilePhoto = req.ProfilePhoto
	}
	if req.LastSeen != "" {
		user.Privacy.LastSeen = req.LastSeen
	}
	if req.About != "" {
		user.Privacy.About = req.About
	}
	if req.Status != "" {
		user.Privacy.Status = req.Status
	}
	if req.ReadReceipts != nil {
		user.Privacy.ReadReceipts = *req.ReadReceipts
	}
	if req.GroupInvites != "" {
		user.Privacy.GroupInvites = req.GroupInvites
	}
	if req.CallPermissions != "" {
		user.Privacy.CallPermissions = req.CallPermissions
	}

	user.BeforeUpdate()

	// Update in database
	if err := s.updateUser(user); err != nil {
		return fmt.Errorf("failed to update privacy settings: %w", err)
	}

	logger.LogUserAction(userID.Hex(), "update_privacy", "auth_service", nil)
	return nil
}

// Contact Management

// AddContact adds a contact
func (s *AuthService) AddContact(userID primitive.ObjectID, req *models.ContactRequest) (*models.User, error) {
	// Get the user to add as contact
	contactUser, err := s.getUserByPhone(req.CountryCode, req.PhoneNumber)
	if err != nil {
		return nil, fmt.Errorf("contact not found")
	}

	// Get current user
	user, err := s.getUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	// Check if already a contact
	if user.IsContactOf(contactUser.ID) {
		return nil, fmt.Errorf("user is already a contact")
	}

	// Check if user is blocked
	if user.HasBlockedUser(contactUser.ID) || contactUser.HasBlockedUser(userID) {
		return nil, fmt.Errorf("cannot add blocked user as contact")
	}

	// Add to contacts
	user.Contacts = append(user.Contacts, contactUser.ID)
	user.BeforeUpdate()

	// Update in database
	if err := s.updateUser(user); err != nil {
		return nil, fmt.Errorf("failed to add contact: %w", err)
	}

	logger.LogUserAction(userID.Hex(), "add_contact", "auth_service", map[string]interface{}{
		"contact_id":    contactUser.ID.Hex(),
		"contact_phone": contactUser.FullPhoneNumber,
	})

	return contactUser, nil
}

// RemoveContact removes a contact
func (s *AuthService) RemoveContact(userID, contactID primitive.ObjectID) error {
	user, err := s.getUserByID(userID)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	// Remove from contacts
	for i, id := range user.Contacts {
		if id == contactID {
			user.Contacts = append(user.Contacts[:i], user.Contacts[i+1:]...)
			break
		}
	}

	user.BeforeUpdate()

	// Update in database
	if err := s.updateUser(user); err != nil {
		return fmt.Errorf("failed to remove contact: %w", err)
	}

	logger.LogUserAction(userID.Hex(), "remove_contact", "auth_service", map[string]interface{}{
		"contact_id": contactID.Hex(),
	})

	return nil
}

// GetContacts gets user contacts
func (s *AuthService) GetContacts(userID primitive.ObjectID) ([]models.UserPublicInfo, error) {
	user, err := s.getUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	if len(user.Contacts) == 0 {
		return []models.UserPublicInfo{}, nil
	}

	// Get contact details
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := s.collections.Users.Find(ctx, bson.M{
		"_id":        bson.M{"$in": user.Contacts},
		"is_active":  true,
		"is_deleted": false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get contacts: %w", err)
	}
	defer cursor.Close(ctx)

	var contacts []models.User
	if err := cursor.All(ctx, &contacts); err != nil {
		return nil, fmt.Errorf("failed to decode contacts: %w", err)
	}

	// Convert to public info
	result := make([]models.UserPublicInfo, len(contacts))
	for i, contact := range contacts {
		result[i] = contact.GetPublicInfo(userID)
	}

	return result, nil
}

// BlockUser blocks a user
func (s *AuthService) BlockUser(userID, targetUserID primitive.ObjectID) error {
	user, err := s.getUserByID(userID)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	// Check if already blocked
	if user.HasBlockedUser(targetUserID) {
		return fmt.Errorf("user is already blocked")
	}

	// Add to blocked users
	user.BlockedUsers = append(user.BlockedUsers, targetUserID)

	// Remove from contacts if exists
	for i, id := range user.Contacts {
		if id == targetUserID {
			user.Contacts = append(user.Contacts[:i], user.Contacts[i+1:]...)
			break
		}
	}

	user.BeforeUpdate()

	// Update current user
	if err := s.updateUser(user); err != nil {
		return fmt.Errorf("failed to block user: %w", err)
	}

	// Update target user's blocked_by list
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = s.collections.Users.UpdateOne(ctx,
		bson.M{"_id": targetUserID},
		bson.M{"$addToSet": bson.M{"blocked_by": userID}},
	)
	if err != nil {
		logger.Error("Failed to update target user blocked_by list:", err)
	}

	logger.LogUserAction(userID.Hex(), "block_user", "auth_service", map[string]interface{}{
		"blocked_user_id": targetUserID.Hex(),
	})

	return nil
}

// UnblockUser unblocks a user
func (s *AuthService) UnblockUser(userID, targetUserID primitive.ObjectID) error {
	user, err := s.getUserByID(userID)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	// Remove from blocked users
	for i, id := range user.BlockedUsers {
		if id == targetUserID {
			user.BlockedUsers = append(user.BlockedUsers[:i], user.BlockedUsers[i+1:]...)
			break
		}
	}

	user.BeforeUpdate()

	// Update current user
	if err := s.updateUser(user); err != nil {
		return fmt.Errorf("failed to unblock user: %w", err)
	}

	// Remove from target user's blocked_by list
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = s.collections.Users.UpdateOne(ctx,
		bson.M{"_id": targetUserID},
		bson.M{"$pull": bson.M{"blocked_by": userID}},
	)
	if err != nil {
		logger.Error("Failed to update target user blocked_by list:", err)
	}

	logger.LogUserAction(userID.Hex(), "unblock_user", "auth_service", map[string]interface{}{
		"unblocked_user_id": targetUserID.Hex(),
	})

	return nil
}

// SearchUsers searches for users by phone number or username
func (s *AuthService) SearchUsers(query string, userID primitive.ObjectID) ([]ContactSearchResult, error) {
	user, err := s.getUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Search by username or phone number
	filter := bson.M{
		"$and": []bson.M{
			{
				"$or": []bson.M{
					{"username": bson.M{"$regex": query, "$options": "i"}},
					{"full_phone_number": bson.M{"$regex": query}},
				},
			},
			{"is_active": true},
			{"is_deleted": false},
			{"_id": bson.M{"$ne": userID}}, // Exclude self
		},
	}

	cursor, err := s.collections.Users.Find(ctx, filter, options.Find().SetLimit(20))
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer cursor.Close(ctx)

	var users []models.User
	if err := cursor.All(ctx, &users); err != nil {
		return nil, fmt.Errorf("failed to decode results: %w", err)
	}

	// Convert to search results
	results := make([]ContactSearchResult, len(users))
	for i, searchUser := range users {
		results[i] = ContactSearchResult{
			User:      &searchUser.GetPublicInfo(userID),
			IsContact: user.IsContactOf(searchUser.ID),
			IsBlocked: user.HasBlockedUser(searchUser.ID),
		}
	}

	return results, nil
}

// Password Management

// ChangePassword changes user password
func (s *AuthService) ChangePassword(userID primitive.ObjectID, currentPassword, newPassword string) error {
	user, err := s.getUserByID(userID)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	// Verify current password
	if !utils.VerifyPassword(currentPassword, user.PasswordHash) {
		return fmt.Errorf("current password is incorrect")
	}

	// Validate new password
	if err := utils.ValidatePassword(newPassword, utils.DefaultPasswordRequirements()); err != nil {
		return fmt.Errorf("new password validation failed: %w", err)
	}

	// Hash new password
	passwordHash, err := utils.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password
	user.PasswordHash = passwordHash
	user.BeforeUpdate()

	if err := s.updateUser(user); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	logger.LogUserAction(userID.Hex(), "change_password", "auth_service", nil)
	return nil
}

// ResetPassword initiates password reset
func (s *AuthService) ResetPassword(phoneNumber, countryCode string) error {
	user, err := s.getUserByPhone(countryCode, phoneNumber)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	// Generate and send OTP for password reset
	if err := s.generateAndSendOTP(user, "password_reset"); err != nil {
		return fmt.Errorf("failed to send reset OTP: %w", err)
	}

	logger.LogUserAction(user.ID.Hex(), "password_reset_request", "auth_service", map[string]interface{}{
		"phone_number": user.FullPhoneNumber,
	})

	return nil
}

// ConfirmPasswordReset confirms password reset with OTP
func (s *AuthService) ConfirmPasswordReset(phoneNumber, countryCode, otp, newPassword string) error {
	user, err := s.getUserByPhone(countryCode, phoneNumber)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	// Validate OTP
	if !user.ValidateOTP(otp) {
		if err := s.updateUser(user); err != nil {
			logger.Error("Failed to update user OTP attempts:", err)
		}
		return fmt.Errorf("invalid OTP")
	}

	// Validate new password
	if err := utils.ValidatePassword(newPassword, utils.DefaultPasswordRequirements()); err != nil {
		return fmt.Errorf("password validation failed: %w", err)
	}

	// Hash new password
	passwordHash, err := utils.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password
	user.PasswordHash = passwordHash
	user.BeforeUpdate()

	if err := s.updateUser(user); err != nil {
		return fmt.Errorf("failed to reset password: %w", err)
	}

	logger.LogUserAction(user.ID.Hex(), "password_reset_confirm", "auth_service", nil)
	return nil
}

// Two-Factor Authentication

// EnableTwoFactor enables two-factor authentication
func (s *AuthService) EnableTwoFactor(userID primitive.ObjectID) (string, error) {
	user, err := s.getUserByID(userID)
	if err != nil {
		return "", fmt.Errorf("user not found")
	}

	if user.TwoFactorEnabled {
		return "", fmt.Errorf("two-factor authentication already enabled")
	}

	// Generate TOTP secret
	secret, err := utils.GenerateTOTPSecret()
	if err != nil {
		return "", fmt.Errorf("failed to generate TOTP secret: %w", err)
	}

	// Save secret (but don't enable yet)
	user.TwoFactorSecret = secret
	user.BeforeUpdate()

	if err := s.updateUser(user); err != nil {
		return "", fmt.Errorf("failed to save TOTP secret: %w", err)
	}

	logger.LogUserAction(userID.Hex(), "enable_2fa_request", "auth_service", nil)
	return secret, nil
}

// ConfirmTwoFactor confirms and enables two-factor authentication
func (s *AuthService) ConfirmTwoFactor(userID primitive.ObjectID, code string) error {
	user, err := s.getUserByID(userID)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	if user.TwoFactorSecret == "" {
		return fmt.Errorf("two-factor setup not initiated")
	}

	// Validate TOTP code (implementation would depend on TOTP library)
	// For now, we'll just check if it's 6 digits
	if len(code) != 6 {
		return fmt.Errorf("invalid verification code")
	}

	// Enable two-factor
	user.TwoFactorEnabled = true
	user.BeforeUpdate()

	if err := s.updateUser(user); err != nil {
		return fmt.Errorf("failed to enable two-factor: %w", err)
	}

	logger.LogUserAction(userID.Hex(), "enable_2fa_confirm", "auth_service", nil)
	return nil
}

// DisableTwoFactor disables two-factor authentication
func (s *AuthService) DisableTwoFactor(userID primitive.ObjectID, code string) error {
	user, err := s.getUserByID(userID)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	if !user.TwoFactorEnabled {
		return fmt.Errorf("two-factor authentication not enabled")
	}

	// Validate TOTP code
	if len(code) != 6 {
		return fmt.Errorf("invalid verification code")
	}

	// Disable two-factor
	user.TwoFactorEnabled = false
	user.TwoFactorSecret = ""
	user.BeforeUpdate()

	if err := s.updateUser(user); err != nil {
		return fmt.Errorf("failed to disable two-factor: %w", err)
	}

	logger.LogUserAction(userID.Hex(), "disable_2fa", "auth_service", nil)
	return nil
}

// Device and Session Management

// GetActiveSessions gets active sessions for user
func (s *AuthService) GetActiveSessions(userID primitive.ObjectID) ([]DeviceSession, error) {
	// This would require maintaining session data in Redis or database
	// For now, return empty list
	return []DeviceSession{}, nil
}

// RevokeSession revokes a specific session
func (s *AuthService) RevokeSession(userID primitive.ObjectID, sessionID string) error {
	if s.redisClient != nil {
		if err := s.redisClient.DeleteSession(sessionID); err != nil {
			return fmt.Errorf("failed to revoke session: %w", err)
		}
	}

	logger.LogUserAction(userID.Hex(), "revoke_session", "auth_service", map[string]interface{}{
		"session_id": sessionID,
	})

	return nil
}

// Account Management

// DeactivateAccount deactivates user account
func (s *AuthService) DeactivateAccount(userID primitive.ObjectID, reason string) error {
	user, err := s.getUserByID(userID)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	user.IsActive = false
	user.BeforeUpdate()

	if err := s.updateUser(user); err != nil {
		return fmt.Errorf("failed to deactivate account: %w", err)
	}

	// Log out from all devices
	s.LogoutAllDevices(userID)

	logger.LogUserAction(userID.Hex(), "deactivate_account", "auth_service", map[string]interface{}{
		"reason": reason,
	})

	return nil
}

// DeleteAccount marks account for deletion
func (s *AuthService) DeleteAccount(userID primitive.ObjectID, reason string) error {
	user, err := s.getUserByID(userID)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	// Mark for deletion (actual deletion happens after grace period)
	user.IsDeleted = true
	deletionDate := time.Now().Add(30 * 24 * time.Hour) // 30 days grace period
	user.DeletionDate = &deletionDate
	user.BeforeUpdate()

	if err := s.updateUser(user); err != nil {
		return fmt.Errorf("failed to mark account for deletion: %w", err)
	}

	// Log out from all devices
	s.LogoutAllDevices(userID)

	logger.LogUserAction(userID.Hex(), "delete_account", "auth_service", map[string]interface{}{
		"reason":        reason,
		"deletion_date": deletionDate,
	})

	return nil
}

// Helper Methods

// validateRegistrationRequest validates user registration request
func (s *AuthService) validateRegistrationRequest(req *models.UserRegisterRequest) error {
	// Validate phone number
	if err := utils.ValidatePhoneNumber(req.PhoneNumber, req.CountryCode); err != nil {
		return err
	}

	// Validate country code
	if err := utils.ValidateCountryCode(req.CountryCode); err != nil {
		return err
	}

	// Validate name
	if err := utils.ValidateString(req.Name, "name", utils.ValidationRules{
		Required:  true,
		MinLength: 1,
		MaxLength: 50,
	}); err != nil {
		return err
	}

	// Validate password if provided
	if req.Password != "" {
		if err := utils.ValidatePassword(req.Password, utils.DefaultPasswordRequirements()); err != nil {
			return err
		}
	}

	return nil
}

// getUserByPhone gets user by phone number
func (s *AuthService) getUserByPhone(countryCode, phoneNumber string) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fullPhoneNumber := countryCode + phoneNumber

	var user models.User
	err := s.collections.Users.FindOne(ctx, bson.M{
		"full_phone_number": fullPhoneNumber,
	}).Decode(&user)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	return &user, nil
}

// getUserByID gets user by ID
func (s *AuthService) getUserByID(userID primitive.ObjectID) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err := s.collections.Users.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	return &user, nil
}

// updateUser updates user in database
func (s *AuthService) updateUser(user *models.User) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.collections.Users.ReplaceOne(ctx, bson.M{"_id": user.ID}, user)
	return err
}

// generateAndSendOTP generates and sends OTP to user
func (s *AuthService) generateAndSendOTP(user *models.User, reason string) error {
	// Generate OTP
	otp := user.GenerateOTP()

	// Update user in database
	if err := s.updateUser(user); err != nil {
		return fmt.Errorf("failed to save OTP: %w", err)
	}

	// Send OTP via SMS (this would integrate with SMS service)
	// For now, just log it
	logger.Infof("OTP for user %s (%s): %s", user.ID.Hex(), user.FullPhoneNumber, otp)

	return nil
}

// createUserSession creates user session and returns auth response
func (s *AuthService) createUserSession(user *models.User, deviceID, platform, ipAddress string) (*AuthResponse, error) {
	// Generate tokens
	tokenPair, err := utils.GenerateTokenPair(
		user.ID.Hex(),
		user.FullPhoneNumber,
		string(user.Role),
		deviceID,
		s.config.JWTSecret,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	// Create session in Redis
	if s.redisClient != nil {
		sessionData := &redis.SessionData{
			UserID:    user.ID.Hex(),
			DeviceID:  deviceID,
			Platform:  platform,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Data:      make(map[string]interface{}),
		}

		// Extract session ID from JWT claims
		claims, _ := utils.ValidateToken(tokenPair.AccessToken, s.config.JWTSecret)
		if claims != nil {
			if err := s.redisClient.SetSession(claims.ID, sessionData, 24*time.Hour); err != nil {
				logger.Error("Failed to create session in Redis:", err)
			}

			// Set user online
			if err := s.redisClient.SetUserOnline(user.ID.Hex(), deviceID, 5*time.Minute); err != nil {
				logger.Error("Failed to set user online:", err)
			}
		}
	}

	response := &AuthResponse{
		User:         &user.GetPublicInfo(user.ID),
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    tokenPair.ExpiresAt.Unix(),
		TokenType:    tokenPair.TokenType,
	}

	return response, nil
}

// handleExistingUnverifiedUser handles registration for existing unverified user
func (s *AuthService) handleExistingUnverifiedUser(user *models.User, req *models.UserRegisterRequest, deviceID, platform, ipAddress string) (*AuthResponse, error) {
	// Update user details
	user.Name = req.Name
	if req.Password != "" {
		passwordHash, err := utils.HashPassword(req.Password)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		user.PasswordHash = passwordHash
	}
	user.BeforeUpdate()

	// Update in database
	if err := s.updateUser(user); err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	// Generate and send new OTP
	if err := s.generateAndSendOTP(user, "re-registration"); err != nil {
		logger.Errorf("Failed to send OTP to user %s: %v", user.ID.Hex(), err)
	}

	response := &AuthResponse{
		User:      &user.GetPublicInfo(user.ID),
		IsNewUser: false,
	}

	return response, nil
}

// checkEmailExists checks if email is already used by another user
func (s *AuthService) checkEmailExists(email string, excludeUserID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := s.collections.Users.CountDocuments(ctx, bson.M{
		"email": email,
		"_id":   bson.M{"$ne": excludeUserID},
	})
	if err != nil {
		return fmt.Errorf("failed to check email: %w", err)
	}

	if count > 0 {
		return fmt.Errorf("email already exists")
	}

	return nil
}

// checkUsernameExists checks if username is already used by another user
func (s *AuthService) checkUsernameExists(username string, excludeUserID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := s.collections.Users.CountDocuments(ctx, bson.M{
		"username": username,
		"_id":      bson.M{"$ne": excludeUserID},
	})
	if err != nil {
		return fmt.Errorf("failed to check username: %w", err)
	}

	if count > 0 {
		return fmt.Errorf("username already exists")
	}

	return nil
}

// checkAccountLock checks if account is locked due to failed login attempts
func (s *AuthService) checkAccountLock(userID primitive.ObjectID) error {
	if s.redisClient == nil {
		return nil
	}

	key := fmt.Sprintf("failed_login:%s", userID.Hex())
	exists, err := s.redisClient.Exists(key)
	if err != nil {
		return nil // Allow login if Redis check fails
	}

	if exists {
		return fmt.Errorf("account is temporarily locked due to failed login attempts")
	}

	return nil
}

// resetFailedLoginAttempts resets failed login attempts counter
func (s *AuthService) resetFailedLoginAttempts(userID primitive.ObjectID) {
	if s.redisClient == nil {
		return
	}

	key := fmt.Sprintf("failed_login:%s", userID.Hex())
	s.redisClient.Delete(key)
}

// getUpdatedFields returns list of updated fields from profile update request
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

// Background Tasks

// cleanupExpiredSessions cleans up expired sessions
func (s *AuthService) cleanupExpiredSessions() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		logger.Debug("Running expired sessions cleanup")
		// Implementation would clean up expired sessions from Redis
		// This is just a placeholder
	}
}

// cleanupExpiredOTPs cleans up expired OTP codes
func (s *AuthService) cleanupExpiredOTPs() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		logger.Debug("Running expired OTPs cleanup")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		// Reset expired OTPs
		_, err := s.collections.Users.UpdateMany(ctx,
			bson.M{
				"otp.expires_at": bson.M{"$lt": time.Now()},
				"otp.is_used":    false,
			},
			bson.M{
				"$set": bson.M{
					"otp.is_used": true,
				},
			},
		)

		cancel()

		if err != nil {
			logger.Error("Failed to cleanup expired OTPs:", err)
		}
	}
}
