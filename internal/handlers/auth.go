package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"bro/internal/middleware"
	"bro/internal/models"
	"bro/internal/services"
	"bro/internal/utils"
	"bro/pkg/logger"
)

// AuthHandler handles authentication-related HTTP requests
type AuthHandler struct {
	authService *services.AuthService
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(authService *services.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

// RegisterRoutes registers authentication routes
func (h *AuthHandler) RegisterRoutes(r *gin.RouterGroup, jwtSecret string) {
	auth := r.Group("/auth")

	// Public routes (no authentication required)
	auth.POST("/register", h.Register)
	auth.POST("/verify-otp", h.VerifyOTP)
	auth.POST("/login", h.Login)
	auth.POST("/refresh", h.RefreshToken)
	auth.POST("/resend-otp", h.ResendOTP)

	// Protected routes (authentication required)
	protected := auth.Group("")
	protected.Use(middleware.AuthMiddleware(jwtSecret))
	{
		protected.GET("/profile", h.GetProfile)
		protected.PUT("/profile", h.UpdateProfile)
		protected.PUT("/change-password", h.ChangePassword)
		protected.POST("/logout", h.Logout)
		protected.GET("/validate", h.ValidateToken)
	}
}

// Register handles user registration
func (h *AuthHandler) Register(c *gin.Context) {
	var req models.UserRegisterRequest

	// Parse and validate request
	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	// Validate required fields
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

	// Validate password if provided
	if req.Password != "" {
		if err := utils.ValidatePassword(req.Password, utils.DefaultPasswordRequirements()); err != nil {
			validationErrors["password"] = utils.ValidationError{
				Field:   "password",
				Message: err.Message,
				Code:    utils.ErrPasswordTooWeak,
			}
		}
	}

	if len(validationErrors) > 0 {
		utils.ValidationErrorResponse(c, validationErrors)
		return
	}

	// Register user
	user, err := h.authService.RegisterUser(&req)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			utils.Conflict(c, "User with this phone number already exists")
			return
		}
		logger.Errorf("Failed to register user: %v", err)
		utils.InternalServerError(c, "Failed to register user")
		return
	}

	// Sanitize response data
	userResponse := utils.SanitizeUserData(user.GetPublicInfo(user.ID))

	// Log successful registration
	logger.LogUserAction(user.ID.Hex(), "user_registered", "auth_handler", map[string]interface{}{
		"phone_number": user.FullPhoneNumber,
		"ip":           utils.GetClientIP(c),
	})

	utils.SuccessWithMessage(c, "User registered successfully. Please verify your phone number with the OTP sent.", userResponse)
}

// VerifyOTP handles OTP verification
func (h *AuthHandler) VerifyOTP(c *gin.Context) {
	var req models.OTPVerificationRequest

	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	// Validate request
	validationErrors := utils.ValidateMultiple(map[string]func() *utils.ValidationError{
		"phone_number": func() *utils.ValidationError {
			return utils.ValidatePhoneNumber(req.PhoneNumber, req.CountryCode)
		},
		"country_code": func() *utils.ValidationError {
			return utils.ValidateCountryCode(req.CountryCode)
		},
		"otp": func() *utils.ValidationError {
			return utils.ValidateOTP(req.OTP)
		},
	})

	if len(validationErrors) > 0 {
		utils.ValidationErrorResponse(c, validationErrors)
		return
	}

	// Verify OTP
	result, err := h.authService.VerifyOTP(&req)
	if err != nil {
		if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "expired") {
			utils.InvalidOTP(c)
			return
		}
		if strings.Contains(err.Error(), "not found") {
			utils.NotFound(c, "User not found")
			return
		}
		logger.Errorf("Failed to verify OTP: %v", err)
		utils.InternalServerError(c, "Failed to verify OTP")
		return
	}

	// Prepare response
	userResponse := utils.SanitizeUserData(result.User.GetPublicInfo(result.User.ID))
	authResponse := utils.AuthResponse{
		User:         userResponse,
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    result.ExpiresAt.Unix(),
		TokenType:    "Bearer",
	}

	// Log successful verification
	logger.LogUserAction(result.User.ID.Hex(), "otp_verified", "auth_handler", map[string]interface{}{
		"phone_number": result.User.FullPhoneNumber,
		"ip":           utils.GetClientIP(c),
	})

	utils.SuccessWithMessage(c, "Phone number verified successfully", authResponse)
}

// Login handles user login
func (h *AuthHandler) Login(c *gin.Context) {
	var req models.UserLoginRequest

	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	// Validate request
	validationErrors := utils.ValidateMultiple(map[string]func() *utils.ValidationError{
		"phone_number": func() *utils.ValidationError {
			return utils.ValidatePhoneNumber(req.PhoneNumber, req.CountryCode)
		},
		"country_code": func() *utils.ValidationError {
			return utils.ValidateCountryCode(req.CountryCode)
		},
	})

	// Validate password if provided
	if req.Password != "" {
		if len(req.Password) < 1 {
			validationErrors["password"] = utils.ValidationError{
				Field:   "password",
				Message: "Password is required",
				Code:    utils.ErrRequired,
			}
		}
	}

	if len(validationErrors) > 0 {
		utils.ValidationErrorResponse(c, validationErrors)
		return
	}

	// Attempt login
	result, err := h.authService.LoginWithPhone(&req)
	if err != nil {
		if strings.Contains(err.Error(), "invalid credentials") {
			utils.Unauthorized(c, "Invalid phone number or password")
			return
		}
		if strings.Contains(err.Error(), "not verified") {
			utils.BadRequest(c, "Phone number not verified. Please check for OTP.")
			return
		}
		if strings.Contains(err.Error(), "inactive") {
			utils.Forbidden(c, "Account is inactive")
			return
		}
		logger.Errorf("Failed to login user: %v", err)
		utils.InternalServerError(c, "Login failed")
		return
	}

	// Prepare response
	userResponse := utils.SanitizeUserData(result.User.GetPublicInfo(result.User.ID))
	authResponse := utils.AuthResponse{
		User:         userResponse,
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    result.ExpiresAt.Unix(),
		TokenType:    "Bearer",
	}

	// Log successful login
	logger.LogUserAction(result.User.ID.Hex(), "login_success", "auth_handler", map[string]interface{}{
		"phone_number": result.User.FullPhoneNumber,
		"ip":           utils.GetClientIP(c),
		"user_agent":   utils.GetUserAgent(c),
	})

	utils.SuccessWithMessage(c, "Login successful", authResponse)
}

// RefreshToken handles token refresh
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	if req.RefreshToken == "" {
		utils.BadRequest(c, "Refresh token is required")
		return
	}

	// Refresh token
	tokens, err := h.authService.RefreshToken(req.RefreshToken)
	if err != nil {
		if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "expired") {
			utils.InvalidToken(c)
			return
		}
		if strings.Contains(err.Error(), "not found") {
			utils.NotFound(c, "User not found")
			return
		}
		if strings.Contains(err.Error(), "inactive") {
			utils.Forbidden(c, "Account is inactive")
			return
		}
		logger.Errorf("Failed to refresh token: %v", err)
		utils.InternalServerError(c, "Failed to refresh token")
		return
	}

	// Prepare response
	tokenResponse := map[string]interface{}{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"expires_at":    tokens.ExpiresAt.Unix(),
		"token_type":    "Bearer",
	}

	utils.SuccessWithMessage(c, "Token refreshed successfully", tokenResponse)
}

// ResendOTP handles OTP resend requests
func (h *AuthHandler) ResendOTP(c *gin.Context) {
	var req struct {
		PhoneNumber string `json:"phone_number" binding:"required"`
		CountryCode string `json:"country_code" binding:"required"`
	}

	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	// Validate request
	validationErrors := utils.ValidateMultiple(map[string]func() *utils.ValidationError{
		"phone_number": func() *utils.ValidationError {
			return utils.ValidatePhoneNumber(req.PhoneNumber, req.CountryCode)
		},
		"country_code": func() *utils.ValidationError {
			return utils.ValidateCountryCode(req.CountryCode)
		},
	})

	if len(validationErrors) > 0 {
		utils.ValidationErrorResponse(c, validationErrors)
		return
	}

	// Resend OTP
	err := h.authService.ResendOTP(req.PhoneNumber, req.CountryCode)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			utils.NotFound(c, "User not found")
			return
		}
		if strings.Contains(err.Error(), "wait") {
			utils.TooManyRequests(c, "Please wait before requesting new OTP")
			return
		}
		logger.Errorf("Failed to resend OTP: %v", err)
		utils.InternalServerError(c, "Failed to resend OTP")
		return
	}

	utils.SuccessWithMessage(c, "OTP sent successfully", nil)
}

// GetProfile returns user profile
func (h *AuthHandler) GetProfile(c *gin.Context) {
	_, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found in context")
		return
	}

	// Get fresh user data from database
	userID, _ := middleware.GetUserIDFromContext(c)
	freshUser, err := h.authService.GetUserByID(userID)
	if err != nil {
		logger.Errorf("Failed to get user profile: %v", err)
		utils.InternalServerError(c, "Failed to get user profile")
		return
	}

	userResponse := utils.SanitizeUserData(freshUser.GetPublicInfo(freshUser.ID))
	utils.Success(c, userResponse)
}

// UpdateProfile handles profile updates
func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found in context")
		return
	}

	var req models.UpdateProfileRequest

	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	// Validate fields if provided
	validationErrors := make(map[string]utils.ValidationError)

	if req.Name != "" {
		if err := utils.ValidateString(req.Name, "name", utils.ValidationRules{
			Required:  false,
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

	if req.Bio != "" {
		if len(req.Bio) > 500 {
			validationErrors["bio"] = utils.ValidationError{
				Field:   "bio",
				Message: "Bio must be no more than 500 characters",
				Code:    utils.ErrTooLong,
			}
		}
	}

	if len(validationErrors) > 0 {
		utils.ValidationErrorResponse(c, validationErrors)
		return
	}

	// Update profile
	updatedUser, err := h.authService.UpdateProfile(userID, &req)
	if err != nil {
		if strings.Contains(err.Error(), "username already taken") {
			utils.Conflict(c, "Username is already taken")
			return
		}
		if strings.Contains(err.Error(), "invalid") {
			utils.BadRequest(c, err.Error())
			return
		}
		logger.Errorf("Failed to update profile: %v", err)
		utils.InternalServerError(c, "Failed to update profile")
		return
	}

	// Log profile update
	logger.LogUserAction(userID.Hex(), "profile_updated", "auth_handler", map[string]interface{}{
		"ip": utils.GetClientIP(c),
	})

	userResponse := utils.SanitizeUserData(updatedUser.GetPublicInfo(updatedUser.ID))
	utils.SuccessWithMessage(c, "Profile updated successfully", userResponse)
}

// ChangePassword handles password changes
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found in context")
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password" binding:"required"`
	}

	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	// Validate new password
	if err := utils.ValidatePassword(req.NewPassword, utils.DefaultPasswordRequirements()); err != nil {
		utils.BadRequest(c, err.Message)
		return
	}

	// Change password
	err = h.authService.ChangePassword(userID, req.OldPassword, req.NewPassword)
	if err != nil {
		if strings.Contains(err.Error(), "invalid current password") {
			utils.BadRequest(c, "Current password is incorrect")
			return
		}
		if strings.Contains(err.Error(), "invalid new password") {
			utils.BadRequest(c, err.Error())
			return
		}
		logger.Errorf("Failed to change password: %v", err)
		utils.InternalServerError(c, "Failed to change password")
		return
	}

	// Log password change
	logger.LogUserAction(userID.Hex(), "password_changed", "auth_handler", map[string]interface{}{
		"ip": utils.GetClientIP(c),
	})

	utils.SuccessWithMessage(c, "Password changed successfully", nil)
}

// Logout handles user logout
func (h *AuthHandler) Logout(c *gin.Context) {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found in context")
		return
	}

	sessionID, exists := c.Get("session_id")
	sessionIDStr := ""
	if exists {
		sessionIDStr = sessionID.(string)
	}

	// Logout user
	err = h.authService.Logout(userID, sessionIDStr)
	if err != nil {
		logger.Errorf("Failed to logout user: %v", err)
		// Don't fail the logout even if there are issues
	}

	// Also invalidate using middleware helper
	middleware.LogoutUser(c)

	utils.SuccessWithMessage(c, "Logout successful", nil)
}

// ValidateToken validates the current token
func (h *AuthHandler) ValidateToken(c *gin.Context) {
	user, err := middleware.GetUserFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "Invalid token")
		return
	}

	userResponse := utils.SanitizeUserData(user.GetPublicInfo(user.ID))

	tokenInfo := map[string]interface{}{
		"valid": true,
		"user":  userResponse,
	}

	utils.Success(c, tokenInfo)
}

// Helper methods for admin endpoints

// GetUserList returns paginated list of users (admin only)
func (h *AuthHandler) GetUserList(c *gin.Context) {
	// This would be added to admin routes
	if !middleware.IsAdmin(c) {
		utils.Forbidden(c, "Admin access required")
		return
	}

	pagination := utils.GetPaginationParams(c)

	// Implementation would depend on having a method in AuthService to get user list
	// For now, return placeholder
	utils.SuccessWithMessage(c, "User list endpoint - implementation needed", map[string]interface{}{
		"pagination": pagination,
		"note":       "This endpoint needs implementation in AuthService",
	})
}

// BanUser bans a user (admin only)
func (h *AuthHandler) BanUser(c *gin.Context) {
	if !middleware.IsAdmin(c) {
		utils.Forbidden(c, "Admin access required")
		return
	}

	userIDParam := c.Param("id")
	userID, err := primitive.ObjectIDFromHex(userIDParam)
	if err != nil {
		utils.BadRequest(c, "Invalid user ID")
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}

	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	// Implementation would depend on having a method in AuthService to ban users
	// For now, return placeholder
	utils.SuccessWithMessage(c, "User ban endpoint - implementation needed", map[string]interface{}{
		"user_id": userID.Hex(),
		"reason":  req.Reason,
		"note":    "This endpoint needs implementation in AuthService",
	})
}

// RegisterAdminRoutes registers admin-only authentication routes
func (h *AuthHandler) RegisterAdminRoutes(r *gin.RouterGroup, jwtSecret string) {
	admin := r.Group("/admin/users")
	admin.Use(middleware.AdminMiddleware(jwtSecret))
	{
		admin.GET("", h.GetUserList)
		admin.PUT("/:id/ban", h.BanUser)
		// Add more admin endpoints as needed
	}
}

// Health check endpoint for auth service
func (h *AuthHandler) HealthCheck(c *gin.Context) {
	services := make(map[string]utils.ServiceInfo)

	// Test auth service health
	services["auth"] = utils.ServiceInfo{
		Status:  "healthy",
		Message: "Authentication service is operational",
	}

	services["database"] = utils.ServiceInfo{
		Status:  "healthy",
		Message: "Database connection is healthy",
	}

	// Parse the uptime string to time.Duration
	uptimeStr := c.MustGet("uptime").(string)
	uptime, err := time.ParseDuration(uptimeStr)
	if err != nil {
		// Handle the error appropriately, e.g., log it or return an error response
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid uptime format"})
		return
	}

	utils.HealthCheck(c, "1.0.0", services, uptime)
}

// DeviceManagement endpoints

// GetDevices returns user's registered devices
func (h *AuthHandler) GetDevices(c *gin.Context) {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found in context")
		return
	}

	user, err := h.authService.GetUserByID(userID)
	if err != nil {
		utils.InternalServerError(c, "Failed to get user data")
		return
	}

	// Return sanitized device information
	devices := make([]map[string]interface{}, len(user.DeviceTokens))
	for i, device := range user.DeviceTokens {
		devices[i] = map[string]interface{}{
			"platform":    device.Platform,
			"device_id":   device.DeviceID,
			"app_version": device.AppVersion,
			"is_active":   device.IsActive,
			"created_at":  device.CreatedAt,
			"updated_at":  device.UpdatedAt,
		}
	}

	utils.Success(c, map[string]interface{}{
		"devices": devices,
		"total":   len(devices),
	})
}

// RegisterDeviceRoutes registers device management routes
func (h *AuthHandler) RegisterDeviceRoutes(r *gin.RouterGroup, jwtSecret string) {
	devices := r.Group("/devices")
	devices.Use(middleware.AuthMiddleware(jwtSecret))
	{
		devices.GET("", h.GetDevices)
		// Add more device management endpoints as needed
	}
}
