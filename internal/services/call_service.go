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

// CallService handles call operations
type CallService struct {
	config      *config.Config
	db          *mongo.Database
	collections *database.Collections
	redisClient *redis.Client
	pushService *PushService
}

// CallSession represents an active call session
type CallSession struct {
	CallID       primitive.ObjectID             `json:"call_id"`
	RoomID       string                         `json:"room_id"`
	Participants map[string]*SessionParticipant `json:"participants"`
	State        CallState                      `json:"state"`
	CreatedAt    time.Time                      `json:"created_at"`
	UpdatedAt    time.Time                      `json:"updated_at"`
}

// SessionParticipant represents a participant in a call session
type SessionParticipant struct {
	UserID       primitive.ObjectID `json:"user_id"`
	ConnectionID string             `json:"connection_id"`
	Status       string             `json:"status"` // "connecting", "connected", "disconnected"
	JoinedAt     *time.Time         `json:"joined_at,omitempty"`
	MediaState   MediaState         `json:"media_state"`
	DeviceInfo   DeviceInfo         `json:"device_info"`
	NetworkStats *NetworkStats      `json:"network_stats,omitempty"`
	LastPingAt   time.Time          `json:"last_ping_at"`
}

// MediaState represents participant's media state
type MediaState struct {
	VideoEnabled  bool   `json:"video_enabled"`
	AudioEnabled  bool   `json:"audio_enabled"`
	ScreenSharing bool   `json:"screen_sharing"`
	VideoQuality  string `json:"video_quality"`
	AudioQuality  string `json:"audio_quality"`
}

// DeviceInfo represents device information
type DeviceInfo struct {
	Platform     string   `json:"platform"`
	Browser      string   `json:"browser,omitempty"`
	Version      string   `json:"version,omitempty"`
	Capabilities []string `json:"capabilities"`
}

// NetworkStats represents network statistics
type NetworkStats struct {
	Bitrate     int       `json:"bitrate"`
	PacketLoss  float64   `json:"packet_loss"`
	Jitter      float64   `json:"jitter"`
	RTT         int       `json:"rtt"`
	LastUpdated time.Time `json:"last_updated"`
}

// CallState represents call session state
type CallState string

const (
	CallStateInitiating CallState = "initiating"
	CallStateRinging    CallState = "ringing"
	CallStateActive     CallState = "active"
	CallStateEnding     CallState = "ending"
	CallStateEnded      CallState = "ended"
)

// SignalingMessage represents WebRTC signaling message
type SignalingMessage struct {
	Type       string                 `json:"type"`
	CallID     string                 `json:"call_id"`
	FromUserID string                 `json:"from_user_id"`
	ToUserID   string                 `json:"to_user_id,omitempty"`
	Data       map[string]interface{} `json:"data"`
	Timestamp  time.Time              `json:"timestamp"`
}

// CallRequest represents call initiation request
type CallRequest struct {
	Type         models.CallType      `json:"type"`
	Participants []primitive.ObjectID `json:"participants"`
	ChatID       primitive.ObjectID   `json:"chat_id"`
	VideoEnabled bool                 `json:"video_enabled"`
	Settings     *models.CallSettings `json:"settings,omitempty"`
}

// CallAnswer represents call answer
type CallAnswer struct {
	Accept       bool                 `json:"accept"`
	VideoEnabled bool                 `json:"video_enabled"`
	RejectReason *models.RejectReason `json:"reject_reason,omitempty"`
}

// CallControlAction represents call control action
type CallControlAction struct {
	Action string                 `json:"action"`
	Data   map[string]interface{} `json:"data,omitempty"`
}

// ICEServers represents ICE servers configuration
type ICEServers struct {
	STUNServers []STUNServer `json:"stun_servers"`
	TURNServers []TURNServer `json:"turn_servers"`
}

// STUNServer represents STUN server configuration
type STUNServer struct {
	URL string `json:"url"`
}

// TURNServer represents TURN server configuration
type TURNServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username"`
	Credential string   `json:"credential"`
}

// NewCallService creates a new call service
func NewCallService(config *config.Config, pushService *PushService) *CallService {
	db := database.GetDB()
	collections := database.GetCollections()
	redisClient := redis.GetClient()

	service := &CallService{
		config:      config,
		db:          db,
		collections: collections,
		redisClient: redisClient,
		pushService: pushService,
	}

	// Start background tasks
	go service.cleanupStaleConnections()
	go service.updateCallMetrics()

	logger.Info("Call service initialized successfully")
	return service
}

// Public Call Service Methods

// InitiateCall initiates a new call
func (s *CallService) InitiateCall(initiatorID primitive.ObjectID, request *CallRequest) (*models.Call, *ICEServers, error) {
	startTime := time.Now()

	// Validate request
	if err := s.validateCallRequest(request); err != nil {
		return nil, nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check if initiator can make calls
	if err := s.checkCallPermissions(initiatorID, request.ChatID); err != nil {
		return nil, nil, fmt.Errorf("permission denied: %w", err)
	}

	// Check participant availability
	unavailableUsers, err := s.checkParticipantAvailability(request.Participants)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to check availability: %w", err)
	}
	if len(unavailableUsers) > 0 {
		return nil, nil, fmt.Errorf("some participants are unavailable")
	}

	// Create call
	call := &models.Call{
		Type:        request.Type,
		Status:      models.CallStatusInitiating,
		InitiatorID: initiatorID,
		ChatID:      request.ChatID,
		SessionID:   primitive.NewObjectID().Hex(),
	}

	// Add participants
	for _, participantID := range request.Participants {
		role := models.ParticipantRoleParticipant
		if participantID == initiatorID {
			role = models.ParticipantRoleInitiator
		}
		call.AddParticipant(participantID, role)
	}

	// Set call settings
	if request.Settings != nil {
		call.Settings = *request.Settings
	}

	call.BeforeCreate()

	// Save call to database
	if err := s.saveCall(call); err != nil {
		return nil, nil, fmt.Errorf("failed to save call: %w", err)
	}

	// Create call session in Redis
	session := &CallSession{
		CallID:       call.ID,
		RoomID:       call.SessionID,
		Participants: make(map[string]*SessionParticipant),
		State:        CallStateInitiating,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.saveCallSession(session); err != nil {
		logger.Errorf("Failed to save call session: %v", err)
	}

	// Send call notifications to participants
	go s.sendCallNotifications(call, "incoming_call")

	// Get ICE servers configuration
	iceServers := s.getICEServers()

	// Log call initiation
	duration := time.Since(startTime)
	s.logCallEvent("call_initiated", call, map[string]interface{}{
		"duration_ms":       duration.Milliseconds(),
		"participant_count": len(request.Participants),
		"video_enabled":     request.VideoEnabled,
	})

	logger.Infof("Call initiated: %s (Type: %s, Participants: %d)",
		call.ID.Hex(), call.Type, len(call.Participants))

	return call, iceServers, nil
}

// AnswerCall answers an incoming call
func (s *CallService) AnswerCall(callID primitive.ObjectID, userID primitive.ObjectID, answer *CallAnswer) error {
	startTime := time.Now()

	// Get call
	call, err := s.getCall(callID)
	if err != nil {
		return fmt.Errorf("call not found: %w", err)
	}

	// Check if user is a participant
	participant := call.GetParticipant(userID)
	if participant == nil {
		return fmt.Errorf("user is not a participant in this call")
	}

	// Check call state
	if !call.IsActive() {
		return fmt.Errorf("call is not active")
	}

	// Update participant status
	if answer.Accept {
		call.UpdateParticipantStatus(userID, models.ParticipantStatusConnected)

		// Update media state
		participant.MediaState.VideoEnabled = answer.VideoEnabled
		participant.MediaState.AudioEnabled = true

		// Set call status to active if this is the first acceptance
		if call.Status == models.CallStatusRinging {
			call.Status = models.CallStatusActive
			call.StartCall()
		}
	} else {
		call.UpdateParticipantStatus(userID, models.ParticipantStatusRejected)

		// Set reject reason
		if answer.RejectReason != nil {
			participant.RejectReason = answer.RejectReason
		}

		// End call if all participants rejected
		if s.allParticipantsRejected(call) {
			call.EndCall(userID, models.EndReasonRejected)
		}
	}

	// Update call in database
	if err := s.updateCall(call); err != nil {
		return fmt.Errorf("failed to update call: %w", err)
	}

	// Update call session
	if session, err := s.getCallSession(call.ID); err == nil {
		if answer.Accept {
			session.State = CallStateActive
		}
		session.UpdatedAt = time.Now()
		s.saveCallSession(session)
	}

	// Send notifications
	eventType := "call_accepted"
	if !answer.Accept {
		eventType = "call_rejected"
	}
	go s.sendCallNotifications(call, eventType)

	// Log call answer
	duration := time.Since(startTime)
	s.logCallEvent("call_answered", call, map[string]interface{}{
		"user_id":       userID.Hex(),
		"accepted":      answer.Accept,
		"duration_ms":   duration.Milliseconds(),
		"video_enabled": answer.VideoEnabled,
	})

	logger.Infof("Call answered: %s (User: %s, Accepted: %v)",
		call.ID.Hex(), userID.Hex(), answer.Accept)

	return nil
}

// EndCall ends a call
func (s *CallService) EndCall(callID primitive.ObjectID, userID primitive.ObjectID, reason models.EndReason) error {
	startTime := time.Now()

	// Get call
	call, err := s.getCall(callID)
	if err != nil {
		return fmt.Errorf("call not found: %w", err)
	}

	// Check if user can end the call
	participant := call.GetParticipant(userID)
	if participant == nil {
		return fmt.Errorf("user is not a participant in this call")
	}

	// End the call
	call.EndCall(userID, reason)

	// Update call in database
	if err := s.updateCall(call); err != nil {
		return fmt.Errorf("failed to update call: %w", err)
	}

	// Clean up call session
	if err := s.deleteCallSession(call.ID); err != nil {
		logger.Errorf("Failed to delete call session: %v", err)
	}

	// Send call ended notifications
	go s.sendCallNotifications(call, "call_ended")

	// Process call for analytics
	go s.processCallAnalytics(call)

	// Log call end
	duration := time.Since(startTime)
	s.logCallEvent("call_ended", call, map[string]interface{}{
		"ended_by":      userID.Hex(),
		"reason":        string(reason),
		"call_duration": call.Duration,
		"duration_ms":   duration.Milliseconds(),
	})

	logger.Infof("Call ended: %s (Duration: %s, Reason: %s)",
		call.ID.Hex(), utils.FormatDuration(call.Duration), reason)

	return nil
}

// JoinCall allows a user to join an ongoing call
func (s *CallService) JoinCall(callID primitive.ObjectID, userID primitive.ObjectID, connectionID string, mediaState MediaState, deviceInfo DeviceInfo) error {
	// Get call
	call, err := s.getCall(callID)
	if err != nil {
		return fmt.Errorf("call not found: %w", err)
	}

	// Check if user can join
	if !call.CanUserJoin(userID) {
		return fmt.Errorf("cannot join call")
	}

	// Get call session
	session, err := s.getCallSession(call.ID)
	if err != nil {
		return fmt.Errorf("call session not found: %w", err)
	}

	// Add participant to session
	participant := &SessionParticipant{
		UserID:       userID,
		ConnectionID: connectionID,
		Status:       "connecting",
		MediaState:   mediaState,
		DeviceInfo:   deviceInfo,
		LastPingAt:   time.Now(),
	}

	session.Participants[connectionID] = participant
	session.UpdatedAt = time.Now()

	// Save session
	if err := s.saveCallSession(session); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	// Update call participant status
	call.UpdateParticipantStatus(userID, models.ParticipantStatusConnecting)
	s.updateCall(call)

	s.logCallEvent("user_joined", call, map[string]interface{}{
		"user_id":       userID.Hex(),
		"connection_id": connectionID,
		"media_state":   mediaState,
	})

	return nil
}

// LeaveCall allows a user to leave a call
func (s *CallService) LeaveCall(callID primitive.ObjectID, userID primitive.ObjectID, connectionID string) error {
	// Get call session
	session, err := s.getCallSession(callID)
	if err != nil {
		return fmt.Errorf("call session not found: %w", err)
	}

	// Remove participant from session
	if participant, exists := session.Participants[connectionID]; exists {
		participant.Status = "disconnected"
		now := time.Now()
		participant.JoinedAt = &now

		// Remove after a short delay to allow for reconnection
		go func() {
			time.Sleep(30 * time.Second)
			delete(session.Participants, connectionID)
			s.saveCallSession(session)
		}()
	}

	session.UpdatedAt = time.Now()
	s.saveCallSession(session)

	// Update call
	call, err := s.getCall(callID)
	if err == nil {
		call.UpdateParticipantStatus(userID, models.ParticipantStatusDisconnected)
		s.updateCall(call)

		// End call if no participants left
		if len(s.getConnectedParticipants(session)) == 0 {
			call.EndCall(userID, models.EndReasonNormal)
			s.updateCall(call)
			s.deleteCallSession(callID)
		}
	}

	s.logCallEvent("user_left", call, map[string]interface{}{
		"user_id":       userID.Hex(),
		"connection_id": connectionID,
	})

	return nil
}

// UpdateMediaState updates participant's media state
func (s *CallService) UpdateMediaState(callID primitive.ObjectID, connectionID string, mediaState MediaState) error {
	// Get call session
	session, err := s.getCallSession(callID)
	if err != nil {
		return fmt.Errorf("call session not found: %w", err)
	}

	// Update participant media state
	if participant, exists := session.Participants[connectionID]; exists {
		participant.MediaState = mediaState
		participant.LastPingAt = time.Now()
		session.UpdatedAt = time.Now()

		// Save session
		if err := s.saveCallSession(session); err != nil {
			return fmt.Errorf("failed to save session: %w", err)
		}
	}

	return nil
}

// HandleSignaling handles WebRTC signaling messages
func (s *CallService) HandleSignaling(message *SignalingMessage) error {
	// Get call session
	callID, err := primitive.ObjectIDFromHex(message.CallID)
	if err != nil {
		return fmt.Errorf("invalid call ID: %w", err)
	}

	session, err := s.getCallSession(callID)
	if err != nil {
		return fmt.Errorf("call session not found: %w", err)
	}

	// Log signaling message
	s.logCallEvent("signaling_message", nil, map[string]interface{}{
		"call_id":   message.CallID,
		"type":      message.Type,
		"from_user": message.FromUserID,
		"to_user":   message.ToUserID,
		"data_size": len(fmt.Sprintf("%v", message.Data)),
	})

	// Process based on message type
	switch message.Type {
	case "offer", "answer":
		// Forward to target participant
		return s.forwardSignalingMessage(session, message)
	case "ice-candidate":
		// Forward ICE candidate
		return s.forwardSignalingMessage(session, message)
	case "ping":
		// Handle ping for keep-alive
		return s.handlePing(session, message)
	default:
		logger.Warnf("Unknown signaling message type: %s", message.Type)
	}

	return nil
}

// GetCallHistory gets call history for user
func (s *CallService) GetCallHistory(userID primitive.ObjectID, limit, offset int) ([]*models.Call, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build filter for calls where user is a participant
	filter := bson.M{
		"participants.user_id": userID,
	}

	// Get total count
	total, err := s.collections.Calls.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count calls: %w", err)
	}

	// Get calls
	opts := options.Find().
		SetSort(bson.D{{"created_at", -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(offset))

	cursor, err := s.collections.Calls.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to find calls: %w", err)
	}
	defer cursor.Close(ctx)

	var calls []*models.Call
	if err := cursor.All(ctx, &calls); err != nil {
		return nil, 0, fmt.Errorf("failed to decode calls: %w", err)
	}

	return calls, total, nil
}

// GetActiveCall gets active call for user
func (s *CallService) GetActiveCall(userID primitive.ObjectID) (*models.Call, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{
		"participants.user_id": userID,
		"status": bson.M{
			"$in": []models.CallStatus{
				models.CallStatusInitiating,
				models.CallStatusRinging,
				models.CallStatusConnecting,
				models.CallStatusActive,
			},
		},
	}

	var call models.Call
	err := s.collections.Calls.FindOne(ctx, filter).Decode(&call)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find active call: %w", err)
	}

	return &call, nil
}

// StartRecording starts call recording
func (s *CallService) StartRecording(callID primitive.ObjectID, userID primitive.ObjectID) error {
	call, err := s.getCall(callID)
	if err != nil {
		return fmt.Errorf("call not found: %w", err)
	}

	// Check permissions
	if !s.canRecordCall(call, userID) {
		return fmt.Errorf("recording not allowed")
	}

	// Start recording
	call.StartRecording(userID)

	// Update call
	if err := s.updateCall(call); err != nil {
		return fmt.Errorf("failed to update call: %w", err)
	}

	s.logCallEvent("recording_started", call, map[string]interface{}{
		"started_by": userID.Hex(),
	})

	return nil
}

// StopRecording stops call recording
func (s *CallService) StopRecording(callID primitive.ObjectID, userID primitive.ObjectID) error {
	call, err := s.getCall(callID)
	if err != nil {
		return fmt.Errorf("call not found: %w", err)
	}

	// Check if recording is active
	if call.Recording == nil || !call.Recording.IsRecording {
		return fmt.Errorf("recording is not active")
	}

	// Stop recording
	call.StopRecording()

	// Update call
	if err := s.updateCall(call); err != nil {
		return fmt.Errorf("failed to update call: %w", err)
	}

	s.logCallEvent("recording_stopped", call, map[string]interface{}{
		"stopped_by": userID.Hex(),
		"duration":   call.Recording.Duration,
	})

	return nil
}

// Helper Methods

// validateCallRequest validates call request
func (s *CallService) validateCallRequest(request *CallRequest) error {
	if len(request.Participants) == 0 {
		return fmt.Errorf("participants are required")
	}

	if len(request.Participants) > 10 { // Max 10 participants
		return fmt.Errorf("too many participants (max 10)")
	}

	if request.Type == "" {
		return fmt.Errorf("call type is required")
	}

	return nil
}

// checkCallPermissions checks if user can make calls
func (s *CallService) checkCallPermissions(userID, chatID primitive.ObjectID) error {
	// Check if user is member of the chat
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := s.collections.Chats.CountDocuments(ctx, bson.M{
		"_id":          chatID,
		"participants": userID,
		"is_active":    true,
	})
	if err != nil {
		return fmt.Errorf("failed to check chat access: %w", err)
	}

	if count == 0 {
		return fmt.Errorf("user is not a member of this chat")
	}

	return nil
}

// checkParticipantAvailability checks if participants are available
func (s *CallService) checkParticipantAvailability(participants []primitive.ObjectID) ([]primitive.ObjectID, error) {
	var unavailable []primitive.ObjectID

	for _, participantID := range participants {
		// Check if user has an active call
		activeCall, err := s.GetActiveCall(participantID)
		if err != nil {
			continue // Ignore errors, assume available
		}

		if activeCall != nil {
			unavailable = append(unavailable, participantID)
		}
	}

	return unavailable, nil
}

// allParticipantsRejected checks if all participants rejected the call
func (s *CallService) allParticipantsRejected(call *models.Call) bool {
	for _, participant := range call.Participants {
		if participant.Status != models.ParticipantStatusRejected &&
			participant.Status != models.ParticipantStatusMissed {
			return false
		}
	}
	return true
}

// getICEServers gets ICE servers configuration
func (s *CallService) getICEServers() *ICEServers {
	iceServers := &ICEServers{
		STUNServers: []STUNServer{
			{URL: "stun:stun.l.google.com:19302"},
			{URL: "stun:stun1.l.google.com:19302"},
		},
		TURNServers: []TURNServer{
			{
				URLs: []string{
					fmt.Sprintf("turn:%s:%d", s.config.COTURNConfig.Host, s.config.COTURNConfig.Port),
				},
				Username:   s.config.COTURNConfig.Username,
				Credential: s.config.COTURNConfig.Password,
			},
		},
	}

	return iceServers
}

// sendCallNotifications sends notifications for call events
func (s *CallService) sendCallNotifications(call *models.Call, eventType string) {
	if s.pushService == nil {
		return
	}

	// Get initiator user
	initiator, err := s.getUserByID(call.InitiatorID)
	if err != nil {
		logger.Errorf("Failed to get initiator user: %v", err)
		return
	}

	// Send notifications to participants
	for _, participant := range call.Participants {
		if participant.UserID == call.InitiatorID {
			continue // Don't notify initiator
		}

		recipient, err := s.getUserByID(participant.UserID)
		if err != nil {
			logger.Errorf("Failed to get recipient user: %v", err)
			continue
		}

		switch eventType {
		case "incoming_call":
			s.pushService.SendCallNotification(initiator, recipient, call)
		case "call_accepted", "call_rejected", "call_ended":
			// These might be handled differently or not send push notifications
		}
	}
}

// canRecordCall checks if user can record the call
func (s *CallService) canRecordCall(call *models.Call, userID primitive.ObjectID) bool {
	// Only initiator or admin can start recording
	if call.InitiatorID == userID {
		return true
	}

	// Check if user is admin of the group (if group call)
	// This would require checking group permissions
	return false
}

// getConnectedParticipants gets list of connected participants
func (s *CallService) getConnectedParticipants(session *CallSession) []*SessionParticipant {
	var connected []*SessionParticipant
	for _, participant := range session.Participants {
		if participant.Status == "connected" {
			connected = append(connected, participant)
		}
	}
	return connected
}

// forwardSignalingMessage forwards signaling message to target participant
func (s *CallService) forwardSignalingMessage(session *CallSession, message *SignalingMessage) error {
	// This would typically use WebSocket or other real-time communication
	// to forward the message to the target participant
	logger.Debugf("Forwarding signaling message: %s -> %s", message.FromUserID, message.ToUserID)
	return nil
}

// handlePing handles ping message for connection keep-alive
func (s *CallService) handlePing(session *CallSession, message *SignalingMessage) error {
	// Update last ping time for the participant
	for _, participant := range session.Participants {
		if participant.UserID.Hex() == message.FromUserID {
			participant.LastPingAt = time.Now()
			break
		}
	}

	session.UpdatedAt = time.Now()
	return s.saveCallSession(session)
}

// processCallAnalytics processes call for analytics
func (s *CallService) processCallAnalytics(call *models.Call) {
	// Calculate call metrics
	call.Analytics.TotalParticipantMinutes = int(call.Duration/60) * len(call.Participants)
	call.Analytics.PeakParticipants = len(call.GetConnectedParticipants())

	// Update feature usage
	if call.Features.VideoCall {
		call.Analytics.FeaturesUsed = append(call.Analytics.FeaturesUsed, "video")
	}
	if call.Features.ScreenShare {
		call.Analytics.FeaturesUsed = append(call.Analytics.FeaturesUsed, "screen_share")
	}
	if call.Features.Recording {
		call.Analytics.FeaturesUsed = append(call.Analytics.FeaturesUsed, "recording")
	}

	// Save analytics
	s.updateCall(call)
}

// Database Operations

// saveCall saves call to database
func (s *CallService) saveCall(call *models.Call) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := s.collections.Calls.InsertOne(ctx, call)
	if err != nil {
		return err
	}

	call.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

// getCall gets call from database
func (s *CallService) getCall(callID primitive.ObjectID) (*models.Call, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var call models.Call
	err := s.collections.Calls.FindOne(ctx, bson.M{"_id": callID}).Decode(&call)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("call not found")
		}
		return nil, err
	}

	return &call, nil
}

// updateCall updates call in database
func (s *CallService) updateCall(call *models.Call) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	call.BeforeUpdate()
	_, err := s.collections.Calls.ReplaceOne(ctx, bson.M{"_id": call.ID}, call)
	return err
}

// getUserByID gets user by ID
func (s *CallService) getUserByID(userID primitive.ObjectID) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err := s.collections.Users.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// Redis Operations

// saveCallSession saves call session to Redis
func (s *CallService) saveCallSession(session *CallSession) error {
	if s.redisClient == nil {
		return nil
	}

	key := fmt.Sprintf("call_session:%s", session.CallID.Hex())
	return s.redisClient.SetEX(key, session, 4*time.Hour)
}

// getCallSession gets call session from Redis
func (s *CallService) getCallSession(callID primitive.ObjectID) (*CallSession, error) {
	if s.redisClient == nil {
		return nil, fmt.Errorf("Redis not available")
	}

	key := fmt.Sprintf("call_session:%s", callID.Hex())
	var session CallSession
	if err := s.redisClient.GetJSON(key, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

// deleteCallSession deletes call session from Redis
func (s *CallService) deleteCallSession(callID primitive.ObjectID) error {
	if s.redisClient == nil {
		return nil
	}

	key := fmt.Sprintf("call_session:%s", callID.Hex())
	return s.redisClient.Delete(key)
}

// Background Tasks

// cleanupStaleConnections cleans up stale connections
func (s *CallService) cleanupStaleConnections() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if s.redisClient == nil {
			continue
		}

		// Get all call sessions
		pattern := "call_session:*"
		keys, err := s.redisClient.Scan(pattern)
		if err != nil {
			logger.Errorf("Failed to scan call sessions: %v", err)
			continue
		}

		for _, key := range keys {
			var session CallSession
			if err := s.redisClient.GetJSON(key, &session); err != nil {
				continue
			}

			// Check for stale participants
			now := time.Now()
			staleThreshold := 60 * time.Second

			for connectionID, participant := range session.Participants {
				if now.Sub(participant.LastPingAt) > staleThreshold {
					logger.Infof("Removing stale participant: %s from call %s",
						participant.UserID.Hex(), session.CallID.Hex())

					delete(session.Participants, connectionID)
					session.UpdatedAt = now
				}
			}

			// Remove session if no participants left
			if len(session.Participants) == 0 {
				logger.Infof("Removing empty call session: %s", session.CallID.Hex())
				s.redisClient.Delete(key)

				// End the call
				if call, err := s.getCall(session.CallID); err == nil {
					call.EndCall(primitive.NilObjectID, models.EndReasonTimeout)
					s.updateCall(call)
				}
			} else {
				// Update session
				s.redisClient.SetEX(key, &session, 4*time.Hour)
			}
		}
	}
}

// updateCallMetrics updates call metrics
func (s *CallService) updateCallMetrics() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		logger.Debug("Updating call metrics")

		// Update active call metrics
		if s.redisClient != nil {
			activeCalls := 0
			activeParticipants := 0

			pattern := "call_session:*"
			keys, err := s.redisClient.Scan(pattern)
			if err == nil {
				activeCalls = len(keys)

				for _, key := range keys {
					var session CallSession
					if err := s.redisClient.GetJSON(key, &session); err == nil {
						activeParticipants += len(s.getConnectedParticipants(&session))
					}
				}
			}

			// Store metrics in Redis
			metricsKey := "call_metrics"
			metrics := map[string]interface{}{
				"active_calls":        activeCalls,
				"active_participants": activeParticipants,
				"last_updated":        time.Now(),
			}
			s.redisClient.SetEX(metricsKey, metrics, 10*time.Minute)
		}
	}
}

// Logging

// logCallEvent logs call-related events
func (s *CallService) logCallEvent(event string, call *models.Call, metadata map[string]interface{}) {
	fields := map[string]interface{}{
		"event": event,
		"type":  "call_event",
	}

	if call != nil {
		fields["call_id"] = call.ID.Hex()
		fields["call_type"] = string(call.Type)
		fields["call_status"] = string(call.Status)
		fields["initiator_id"] = call.InitiatorID.Hex()
		fields["participant_count"] = len(call.Participants)
	}

	for k, v := range metadata {
		fields[k] = v
	}

	logger.WithFields(fields).Info("Call event")
}

// Utility Functions

// GetCallMetrics gets current call metrics
func (s *CallService) GetCallMetrics() (map[string]interface{}, error) {
	if s.redisClient == nil {
		return map[string]interface{}{}, nil
	}

	var metrics map[string]interface{}
	if err := s.redisClient.GetJSON("call_metrics", &metrics); err != nil {
		return map[string]interface{}{}, nil
	}

	return metrics, nil
}

// IsUserInCall checks if user is currently in a call
func (s *CallService) IsUserInCall(userID primitive.ObjectID) bool {
	activeCall, err := s.GetActiveCall(userID)
	return err == nil && activeCall != nil
}

// Global service instance
var globalCallService *CallService

func GetCallService() *CallService {
	return globalCallService
}

func SetCallService(service *CallService) {
	globalCallService = service
}
