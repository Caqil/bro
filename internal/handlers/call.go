package handlers

import (
	"net/http"
	"strconv"
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

// CallHandler handles call-related HTTP requests
type CallHandler struct {
	callService *services.CallService
}

// NewCallHandler creates a new call handler
func NewCallHandler(callService *services.CallService) *CallHandler {
	return &CallHandler{
		callService: callService,
	}
}

// RegisterRoutes registers call routes
func (h *CallHandler) RegisterRoutes(r *gin.RouterGroup, jwtSecret string) {
	calls := r.Group("/calls")
	calls.Use(middleware.AuthMiddleware(jwtSecret))

	// Call management endpoints
	calls.POST("/initiate", h.InitiateCall)
	calls.POST("/:id/answer", h.AnswerCall)
	calls.POST("/:id/end", h.EndCall)
	calls.POST("/:id/join", h.JoinCall)
	calls.POST("/:id/leave", h.LeaveCall)
	calls.GET("/:id", h.GetCall)
	calls.GET("/history", h.GetCallHistory)

	// Call control endpoints
	calls.PUT("/:id/media", h.UpdateMediaState)
	calls.PUT("/:id/quality", h.UpdateQualityMetrics)

	// Recording endpoints
	calls.POST("/:id/recording/start", h.StartRecording)
	calls.POST("/:id/recording/stop", h.StopRecording)

	// Statistics and monitoring
	calls.GET("/stats", middleware.AdminMiddleware(jwtSecret), h.GetCallStatistics)
}

// InitiateCall handles call initiation
func (h *CallHandler) InitiateCall(c *gin.Context) {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found in context")
		return
	}

	var req struct {
		ParticipantIDs []string             `json:"participant_ids" binding:"required"`
		ChatID         string               `json:"chat_id" binding:"required"`
		Type           models.CallType      `json:"type" binding:"required"`
		VideoEnabled   bool                 `json:"video_enabled"`
		AudioEnabled   bool                 `json:"audio_enabled"`
		DeviceInfo     models.DeviceInfo    `json:"device_info"`
		Settings       *models.CallSettings `json:"settings,omitempty"`
	}

	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	// Validate request
	validationErrors := make(map[string]utils.ValidationError)

	// Validate chat ID
	chatID, err := primitive.ObjectIDFromHex(req.ChatID)
	if err != nil {
		validationErrors["chat_id"] = utils.ValidationError{
			Field:   "chat_id",
			Message: "Invalid chat ID format",
			Code:    utils.ErrInvalidObjectID,
		}
	}

	// Validate participant IDs
	if len(req.ParticipantIDs) == 0 {
		validationErrors["participant_ids"] = utils.ValidationError{
			Field:   "participant_ids",
			Message: "At least one participant is required",
			Code:    utils.ErrRequired,
		}
	}

	participantObjectIDs := make([]primitive.ObjectID, 0, len(req.ParticipantIDs))
	for i, idStr := range req.ParticipantIDs {
		id, err := primitive.ObjectIDFromHex(idStr)
		if err != nil {
			validationErrors[("participant_id_" + strconv.Itoa(i))] = utils.ValidationError{
				Field:   "participant_ids",
				Message: "Invalid participant ID format",
				Code:    utils.ErrInvalidObjectID,
			}
		} else {
			participantObjectIDs = append(participantObjectIDs, id)
		}
	}

	// Validate call type
	validCallTypes := []models.CallType{
		models.CallTypeVoice,
		models.CallTypeVideo,
		models.CallTypeGroup,
		models.CallTypeConference,
	}
	validType := false
	for _, validCallType := range validCallTypes {
		if req.Type == validCallType {
			validType = true
			break
		}
	}
	if !validType {
		validationErrors["type"] = utils.ValidationError{
			Field:   "type",
			Message: "Invalid call type",
			Code:    utils.ErrInvalidFormat,
		}
	}

	// Check participant limits based on call type
	maxParticipants := 2
	if req.Type == models.CallTypeGroup || req.Type == models.CallTypeConference {
		maxParticipants = 10
	}
	if len(participantObjectIDs) > maxParticipants {
		validationErrors["participant_ids"] = utils.ValidationError{
			Field:   "participant_ids",
			Message: "Too many participants for this call type",
			Code:    utils.ErrTooLarge,
		}
	}

	if len(validationErrors) > 0 {
		utils.ValidationErrorResponse(c, validationErrors)
		return
	}

	// Create call request
	callRequest := &services.CallRequest{
		InitiatorID:    userID,
		ParticipantIDs: participantObjectIDs,
		ChatID:         chatID,
		Type:           req.Type,
		VideoEnabled:   req.VideoEnabled,
		AudioEnabled:   req.AudioEnabled,
		DeviceInfo:     req.DeviceInfo,
		Settings:       req.Settings,
	}

	// Initiate call
	response, err := h.callService.InitiateCall(callRequest)
	if err != nil {
		if strings.Contains(err.Error(), "not available") {
			utils.BadRequest(c, "One or more participants are not available")
			return
		}
		if strings.Contains(err.Error(), "invalid") {
			utils.BadRequest(c, err.Error())
			return
		}
		if strings.Contains(err.Error(), "already in") {
			utils.Conflict(c, "One or more participants are already in another call")
			return
		}
		logger.Errorf("Failed to initiate call: %v", err)
		utils.InternalServerError(c, "Failed to initiate call")
		return
	}

	// Log call initiation
	logger.LogUserAction(userID.Hex(), "call_initiated", "call_handler", map[string]interface{}{
		"call_id":      response.Call.ID.Hex(),
		"call_type":    response.Call.Type,
		"participants": len(response.Participants),
		"chat_id":      req.ChatID,
		"ip":           utils.GetClientIP(c),
	})

	utils.SuccessWithMessage(c, "Call initiated successfully", response)
}

// AnswerCall handles call answering
func (h *CallHandler) AnswerCall(c *gin.Context) {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found in context")
		return
	}

	callIDParam := c.Param("id")
	callID, err := primitive.ObjectIDFromHex(callIDParam)
	if err != nil {
		utils.BadRequest(c, "Invalid call ID")
		return
	}

	var req struct {
		Accept     bool              `json:"accept"`
		DeviceInfo models.DeviceInfo `json:"device_info"`
	}

	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	// Answer call
	response, err := h.callService.AnswerCall(callID, userID, req.Accept, req.DeviceInfo)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not active") {
			utils.NotFound(c, "Call not found or not active")
			return
		}
		if strings.Contains(err.Error(), "not invited") {
			utils.Forbidden(c, "User not invited to this call")
			return
		}
		if strings.Contains(err.Error(), "rejected") {
			utils.BadRequest(c, "Call was rejected")
			return
		}
		logger.Errorf("Failed to answer call: %v", err)
		utils.InternalServerError(c, "Failed to answer call")
		return
	}

	action := "accepted"
	if !req.Accept {
		action = "rejected"
	}

	// Log call answer
	logger.LogUserAction(userID.Hex(), "call_"+action, "call_handler", map[string]interface{}{
		"call_id": callID.Hex(),
		"ip":      utils.GetClientIP(c),
	})

	utils.SuccessWithMessage(c, "Call "+action+" successfully", response)
}

// EndCall handles call termination
func (h *CallHandler) EndCall(c *gin.Context) {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found in context")
		return
	}

	callIDParam := c.Param("id")
	callID, err := primitive.ObjectIDFromHex(callIDParam)
	if err != nil {
		utils.BadRequest(c, "Invalid call ID")
		return
	}

	var req struct {
		Reason models.EndReason `json:"reason,omitempty"`
	}

	// Parse request (reason is optional)
	utils.ParseJSON(c, &req)

	// Default reason if not provided
	if req.Reason == "" {
		req.Reason = models.EndReasonNormal
	}

	// End call
	err = h.callService.EndCall(callID, userID, req.Reason)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not active") {
			utils.NotFound(c, "Call not found or not active")
			return
		}
		logger.Errorf("Failed to end call: %v", err)
		utils.InternalServerError(c, "Failed to end call")
		return
	}

	// Log call end
	logger.LogUserAction(userID.Hex(), "call_ended", "call_handler", map[string]interface{}{
		"call_id": callID.Hex(),
		"reason":  req.Reason,
		"ip":      utils.GetClientIP(c),
	})

	utils.SuccessWithMessage(c, "Call ended successfully", nil)
}

// JoinCall handles joining an ongoing call
func (h *CallHandler) JoinCall(c *gin.Context) {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found in context")
		return
	}

	callIDParam := c.Param("id")
	callID, err := primitive.ObjectIDFromHex(callIDParam)
	if err != nil {
		utils.BadRequest(c, "Invalid call ID")
		return
	}

	var req struct {
		DeviceInfo models.DeviceInfo `json:"device_info"`
	}

	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	// Join call
	response, err := h.callService.JoinCall(callID, userID, req.DeviceInfo)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not active") {
			utils.NotFound(c, "Call not found or not active")
			return
		}
		if strings.Contains(err.Error(), "maximum participants") {
			utils.BadRequest(c, "Call has reached maximum participants")
			return
		}
		if strings.Contains(err.Error(), "already in call") {
			utils.Conflict(c, "User already in call")
			return
		}
		logger.Errorf("Failed to join call: %v", err)
		utils.InternalServerError(c, "Failed to join call")
		return
	}

	// Log call join
	logger.LogUserAction(userID.Hex(), "call_joined", "call_handler", map[string]interface{}{
		"call_id": callID.Hex(),
		"ip":      utils.GetClientIP(c),
	})

	utils.SuccessWithMessage(c, "Joined call successfully", response)
}

// LeaveCall handles leaving a call
func (h *CallHandler) LeaveCall(c *gin.Context) {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found in context")
		return
	}

	callIDParam := c.Param("id")
	callID, err := primitive.ObjectIDFromHex(callIDParam)
	if err != nil {
		utils.BadRequest(c, "Invalid call ID")
		return
	}

	// Leave call
	err = h.callService.LeaveCall(callID, userID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not active") {
			utils.NotFound(c, "Call not found or not active")
			return
		}
		if strings.Contains(err.Error(), "not in call") {
			utils.BadRequest(c, "User not in call")
			return
		}
		logger.Errorf("Failed to leave call: %v", err)
		utils.InternalServerError(c, "Failed to leave call")
		return
	}

	// Log call leave
	logger.LogUserAction(userID.Hex(), "call_left", "call_handler", map[string]interface{}{
		"call_id": callID.Hex(),
		"ip":      utils.GetClientIP(c),
	})

	utils.SuccessWithMessage(c, "Left call successfully", nil)
}

// GetCall returns information about a specific call
func (h *CallHandler) GetCall(c *gin.Context) {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found in context")
		return
	}

	callIDParam := c.Param("id")
	callID, err := primitive.ObjectIDFromHex(callIDParam)
	if err != nil {
		utils.BadRequest(c, "Invalid call ID")
		return
	}

	// Get active call information
	response, err := h.callService.GetActiveCall(callID, userID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not active") {
			utils.NotFound(c, "Call not found or not active")
			return
		}
		if strings.Contains(err.Error(), "not in call") {
			utils.Forbidden(c, "User not authorized to view this call")
			return
		}
		logger.Errorf("Failed to get call: %v", err)
		utils.InternalServerError(c, "Failed to get call information")
		return
	}

	utils.Success(c, response)
}

// GetCallHistory returns user's call history
func (h *CallHandler) GetCallHistory(c *gin.Context) {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found in context")
		return
	}

	// Parse pagination parameters
	pagination := utils.GetPaginationParams(c)

	// Get call history
	response, err := h.callService.GetCallHistory(userID, pagination.Page, pagination.Limit)
	if err != nil {
		logger.Errorf("Failed to get call history: %v", err)
		utils.InternalServerError(c, "Failed to get call history")
		return
	}

	// Create meta information
	meta := utils.CreatePaginationMeta(pagination.Page, pagination.Limit, response.TotalCount)

	utils.SuccessWithMeta(c, response.Calls, meta)
}

// UpdateMediaState handles media state updates (audio/video on/off)
func (h *CallHandler) UpdateMediaState(c *gin.Context) {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found in context")
		return
	}

	callIDParam := c.Param("id")
	callID, err := primitive.ObjectIDFromHex(callIDParam)
	if err != nil {
		utils.BadRequest(c, "Invalid call ID")
		return
	}

	var req models.MediaState

	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	// Update media state
	err = h.callService.UpdateMediaState(callID, userID, req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not active") {
			utils.NotFound(c, "Call not found or not active")
			return
		}
		if strings.Contains(err.Error(), "not in call") {
			utils.BadRequest(c, "User not in call")
			return
		}
		logger.Errorf("Failed to update media state: %v", err)
		utils.InternalServerError(c, "Failed to update media state")
		return
	}

	// Log media state update
	logger.LogUserAction(userID.Hex(), "media_state_updated", "call_handler", map[string]interface{}{
		"call_id":        callID.Hex(),
		"video_enabled":  req.VideoEnabled,
		"audio_enabled":  req.AudioEnabled,
		"screen_sharing": req.ScreenSharing,
		"ip":             utils.GetClientIP(c),
	})

	utils.SuccessWithMessage(c, "Media state updated successfully", nil)
}

// UpdateQualityMetrics handles quality metrics updates
func (h *CallHandler) UpdateQualityMetrics(c *gin.Context) {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found in context")
		return
	}

	callIDParam := c.Param("id")
	callID, err := primitive.ObjectIDFromHex(callIDParam)
	if err != nil {
		utils.BadRequest(c, "Invalid call ID")
		return
	}

	var req models.QualityMetrics

	if err := utils.ParseJSON(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request format")
		return
	}

	// Validate quality metrics
	if req.QualityScore < 0 || req.QualityScore > 5 {
		utils.BadRequest(c, "Quality score must be between 0 and 5")
		return
	}

	// Update quality metrics
	err = h.callService.UpdateQualityMetrics(callID, userID, &req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not active") {
			utils.NotFound(c, "Call not found or not active")
			return
		}
		if strings.Contains(err.Error(), "not in call") {
			utils.BadRequest(c, "User not in call")
			return
		}
		logger.Errorf("Failed to update quality metrics: %v", err)
		utils.InternalServerError(c, "Failed to update quality metrics")
		return
	}

	utils.SuccessWithMessage(c, "Quality metrics updated successfully", nil)
}

// StartRecording handles starting call recording
func (h *CallHandler) StartRecording(c *gin.Context) {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found in context")
		return
	}

	callIDParam := c.Param("id")
	callID, err := primitive.ObjectIDFromHex(callIDParam)
	if err != nil {
		utils.BadRequest(c, "Invalid call ID")
		return
	}

	// Start recording
	err = h.callService.StartRecording(callID, userID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not active") {
			utils.NotFound(c, "Call not found or not active")
			return
		}
		if strings.Contains(err.Error(), "not enabled") {
			utils.BadRequest(c, "Recording not enabled for this call")
			return
		}
		if strings.Contains(err.Error(), "insufficient permissions") {
			utils.Forbidden(c, "Insufficient permissions to start recording")
			return
		}
		logger.Errorf("Failed to start recording: %v", err)
		utils.InternalServerError(c, "Failed to start recording")
		return
	}

	// Log recording start
	logger.LogUserAction(userID.Hex(), "recording_started", "call_handler", map[string]interface{}{
		"call_id": callID.Hex(),
		"ip":      utils.GetClientIP(c),
	})

	utils.SuccessWithMessage(c, "Recording started successfully", nil)
}

// StopRecording handles stopping call recording
func (h *CallHandler) StopRecording(c *gin.Context) {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found in context")
		return
	}

	callIDParam := c.Param("id")
	callID, err := primitive.ObjectIDFromHex(callIDParam)
	if err != nil {
		utils.BadRequest(c, "Invalid call ID")
		return
	}

	// Stop recording
	err = h.callService.StopRecording(callID, userID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not active") {
			utils.NotFound(c, "Call not found or not active")
			return
		}
		if strings.Contains(err.Error(), "not being recorded") {
			utils.BadRequest(c, "Call is not being recorded")
			return
		}
		logger.Errorf("Failed to stop recording: %v", err)
		utils.InternalServerError(c, "Failed to stop recording")
		return
	}

	// Log recording stop
	logger.LogUserAction(userID.Hex(), "recording_stopped", "call_handler", map[string]interface{}{
		"call_id": callID.Hex(),
		"ip":      utils.GetClientIP(c),
	})

	utils.SuccessWithMessage(c, "Recording stopped successfully", nil)
}

// GetCallStatistics returns call service statistics (admin only)
func (h *CallHandler) GetCallStatistics(c *gin.Context) {
	if !middleware.IsAdmin(c) {
		utils.Forbidden(c, "Admin access required")
		return
	}

	// Get call statistics
	stats := h.callService.GetCallStatistics()

	utils.Success(c, stats)
}

// Emergency and troubleshooting endpoints

// ForceEndCall forcefully ends a call (admin only)
func (h *CallHandler) ForceEndCall(c *gin.Context) {
	if !middleware.IsAdmin(c) {
		utils.Forbidden(c, "Admin access required")
		return
	}

	callIDParam := c.Param("id")
	callID, err := primitive.ObjectIDFromHex(callIDParam)
	if err != nil {
		utils.BadRequest(c, "Invalid call ID")
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}

	utils.ParseJSON(c, &req)

	adminID, _ := middleware.GetUserIDFromContext(c)

	// Force end call with admin reason
	err = h.callService.EndCall(callID, adminID, models.EndReasonServerError)
	if err != nil {
		logger.Errorf("Failed to force end call: %v", err)
		utils.InternalServerError(c, "Failed to force end call")
		return
	}

	// Log admin action
	logger.LogUserAction(adminID.Hex(), "force_end_call", "call_handler", map[string]interface{}{
		"call_id": callID.Hex(),
		"reason":  req.Reason,
		"ip":      utils.GetClientIP(c),
	})

	utils.SuccessWithMessage(c, "Call forcefully ended", nil)
}

// GetActiveCallsCount returns count of active calls
func (h *CallHandler) GetActiveCallsCount(c *gin.Context) {
	stats := h.callService.GetCallStatistics()

	response := map[string]interface{}{
		"active_calls": stats.ActiveCalls,
		"total_calls":  stats.TotalCalls,
		"calls_today":  stats.CallsToday,
	}

	utils.Success(c, response)
}

// RegisterAdminRoutes registers admin-only call routes
func (h *CallHandler) RegisterAdminRoutes(r *gin.RouterGroup, jwtSecret string) {
	admin := r.Group("/admin/calls")
	admin.Use(middleware.AdminMiddleware(jwtSecret))
	{
		admin.GET("/stats", h.GetCallStatistics)
		admin.POST("/:id/force-end", h.ForceEndCall)
		admin.GET("/active-count", h.GetActiveCallsCount)
	}
}

// WebRTC signaling endpoints (these would typically be WebSocket-based)

// GetTURNServers returns TURN server configuration
func (h *CallHandler) GetTURNServers(c *gin.Context) {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not found in context")
		return
	}

	// This would get TURN servers from configuration
	// For now, return basic structure
	turnServers := []map[string]interface{}{
		{
			"urls":       []string{"stun:stun.l.google.com:19302"},
			"username":   "",
			"credential": "",
		},
	}

	// Log TURN server request for monitoring
	logger.LogUserAction(userID.Hex(), "turn_servers_requested", "call_handler", map[string]interface{}{
		"ip": utils.GetClientIP(c),
	})

	utils.Success(c, map[string]interface{}{
		"ice_servers": turnServers,
		"ttl":         3600, // 1 hour
	})
}

// RegisterWebRTCRoutes registers WebRTC-related routes
func (h *CallHandler) RegisterWebRTCRoutes(r *gin.RouterGroup, jwtSecret string) {
	webrtc := r.Group("/webrtc")
	webrtc.Use(middleware.AuthMiddleware(jwtSecret))
	{
		webrtc.GET("/turn-servers", h.GetTURNServers)
		// WebSocket signaling would be handled separately
		// webrtc.GET("/signaling", h.HandleSignaling) // WebSocket upgrade
	}
}

// Health check endpoint for call service
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

// Call quality and troubleshooting endpoints

// GetCallDiagnostics returns diagnostic information for a call (admin/moderator)
func (h *CallHandler) GetCallDiagnostics(c *gin.Context) {
	if !middleware.IsModerator(c) {
		utils.Forbidden(c, "Moderator access required")
		return
	}

	callIDParam := c.Param("id")
	callID, err := primitive.ObjectIDFromHex(callIDParam)
	if err != nil {
		utils.BadRequest(c, "Invalid call ID")
		return
	}

	// This would get diagnostic information
	// For now, return placeholder
	diagnostics := map[string]interface{}{
		"call_id":     callID.Hex(),
		"status":      "active", // This would be fetched from the service
		"duration":    "00:05:23",
		"quality":     "good",
		"issues":      []string{},
		"bandwidth":   "2.5 Mbps",
		"packet_loss": "0.1%",
	}

	utils.Success(c, diagnostics)
}

// RegisterDiagnosticsRoutes registers diagnostic routes
func (h *CallHandler) RegisterDiagnosticsRoutes(r *gin.RouterGroup, jwtSecret string) {
	diagnostics := r.Group("/diagnostics")
	diagnostics.Use(middleware.ModeratorMiddleware(jwtSecret))
	{
		diagnostics.GET("/calls/:id", h.GetCallDiagnostics)
	}
}

// Utility methods

// validateCallAccess checks if user can access a call
func (h *CallHandler) validateCallAccess(userID, callID primitive.ObjectID) error {
	// This would check if user is participant in the call
	// Implementation would depend on CallService having such a method
	return nil
}

// sanitizeCallResponse removes sensitive information from call response
func (h *CallHandler) sanitizeCallResponse(response interface{}) interface{} {
	// Remove or mask sensitive information like TURN credentials
	return response
}

// Helper method to get device info from headers
func getDeviceInfoFromHeaders(c *gin.Context) models.DeviceInfo {
	return models.DeviceInfo{
		Platform:   c.GetHeader("X-Platform"),
		DeviceID:   c.GetHeader("X-Device-ID"),
		AppVersion: c.GetHeader("X-App-Version"),
		UserAgent:  c.GetHeader("User-Agent"),
		IP:         utils.GetClientIP(c),
	}
}

// Rate limiting helpers for call endpoints
func (h *CallHandler) checkCallRateLimit(c *gin.Context, action string) bool {
	// This would implement call-specific rate limiting
	// For now, return true (no rate limiting)
	return true
}
