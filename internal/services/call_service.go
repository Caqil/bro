package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bro/internal/config"
	"bro/internal/models"
	"bro/internal/webrtc"
	"bro/internal/websocket"
	"bro/pkg/database"
	"bro/pkg/logger"
	"bro/pkg/redis"
)

// CallService handles all call-related operations
type CallService struct {
	// Configuration
	config *config.Config

	// Database collections
	callsCollection    *mongo.Collection
	usersCollection    *mongo.Collection
	chatsCollection    *mongo.Collection
	messagesCollection *mongo.Collection

	// External services
	signalingServer *webrtc.SignalingServer
	hub             *websocket.Hub
	redisClient     *redis.Client
	pushService     *PushService
	smsService      *SMSService

	// Call management
	activeCalls   map[primitive.ObjectID]*CallSession
	callsMutex    sync.RWMutex
	callTimeouts  map[primitive.ObjectID]*time.Timer
	timeoutsMutex sync.RWMutex

	// Statistics
	callStats *CallStatistics

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// CallSession represents an active call session
type CallSession struct {
	Call         *models.Call
	Room         *webrtc.Room
	Participants map[primitive.ObjectID]*CallParticipantInfo
	StartTime    time.Time
	IsActive     bool
	IsRecording  bool
	QualityData  map[primitive.ObjectID]*models.QualityMetrics
	Events       []CallEvent
	mutex        sync.RWMutex
}

// CallParticipantInfo contains participant information
type CallParticipantInfo struct {
	UserID         primitive.ObjectID
	PeerID         string
	ConnectionID   string
	Status         models.ParticipantStatus
	JoinedAt       *time.Time
	LeftAt         *time.Time
	MediaState     models.MediaState
	DeviceInfo     models.DeviceInfo
	QualityMetrics *models.QualityMetrics
	LastActivity   time.Time
}

// CallEvent represents a call event
type CallEvent struct {
	Type      string
	UserID    primitive.ObjectID
	Data      map[string]interface{}
	Timestamp time.Time
}

// CallStatistics contains call service statistics
type CallStatistics struct {
	TotalCalls          int64
	ActiveCalls         int
	CallsToday          int64
	AverageCallDuration time.Duration
	SuccessRate         float64
	LastUpdated         time.Time
	mutex               sync.RWMutex
}

// CallRequest represents a call initiation request
type CallRequest struct {
	InitiatorID    primitive.ObjectID
	ParticipantIDs []primitive.ObjectID
	ChatID         primitive.ObjectID
	Type           models.CallType
	VideoEnabled   bool
	AudioEnabled   bool
	DeviceInfo     models.DeviceInfo
	Settings       *models.CallSettings
}

// CallResponse represents call operation response
type CallResponse struct {
	Call         *models.Call              `json:"call"`
	Room         *CallRoomInfo             `json:"room,omitempty"`
	Participants []CallParticipantResponse `json:"participants"`
	TURNServers  []models.TURNServer       `json:"turn_servers,omitempty"`
	IceServers   []map[string]interface{}  `json:"ice_servers,omitempty"`
	SignalingURL string                    `json:"signaling_url,omitempty"`
}

// CallRoomInfo contains room information
type CallRoomInfo struct {
	RoomID      string                 `json:"room_id"`
	MaxPeers    int                    `json:"max_peers"`
	ActivePeers int                    `json:"active_peers"`
	Quality     string                 `json:"quality"`
	IsRecording bool                   `json:"is_recording"`
	Settings    map[string]interface{} `json:"settings"`
}

// CallParticipantResponse contains participant response data
type CallParticipantResponse struct {
	UserID     primitive.ObjectID       `json:"user_id"`
	UserInfo   models.UserPublicInfo    `json:"user_info"`
	Status     models.ParticipantStatus `json:"status"`
	JoinedAt   *time.Time               `json:"joined_at,omitempty"`
	MediaState models.MediaState        `json:"media_state"`
	DeviceInfo models.DeviceInfo        `json:"device_info"`
	Quality    *models.QualityMetrics   `json:"quality,omitempty"`
}

// NewCallService creates a new call service
func NewCallService(
	cfg *config.Config,
	signalingServer *webrtc.SignalingServer,
	hub *websocket.Hub,
	pushService *PushService,
	smsService *SMSService,
) (*CallService, error) {

	collections := database.GetCollections()
	if collections == nil {
		return nil, fmt.Errorf("database collections not available")
	}

	ctx, cancel := context.WithCancel(context.Background())

	service := &CallService{
		config:             cfg,
		callsCollection:    collections.Calls,
		usersCollection:    collections.Users,
		chatsCollection:    collections.Chats,
		messagesCollection: collections.Messages,
		signalingServer:    signalingServer,
		hub:                hub,
		redisClient:        redis.GetClient(),
		pushService:        pushService,
		smsService:         smsService,
		activeCalls:        make(map[primitive.ObjectID]*CallSession),
		callTimeouts:       make(map[primitive.ObjectID]*time.Timer),
		callStats:          &CallStatistics{},
		ctx:                ctx,
		cancel:             cancel,
	}

	// Start background processes
	service.wg.Add(3)
	go service.statsCollector()
	go service.callTimeoutManager()
	go service.qualityMonitor()

	logger.Info("Call Service initialized successfully")
	return service, nil
}

// InitiateCall starts a new call
func (cs *CallService) InitiateCall(req *CallRequest) (*CallResponse, error) {
	// Validate request
	if err := cs.validateCallRequest(req); err != nil {
		return nil, fmt.Errorf("invalid call request: %w", err)
	}

	// Check if users are available for calling
	if err := cs.checkParticipantsAvailability(req.ParticipantIDs); err != nil {
		return nil, fmt.Errorf("participants not available: %w", err)
	}

	// Create call in database
	call, err := cs.createCall(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create call: %w", err)
	}

	// Create call session
	session, err := cs.createCallSession(call)
	if err != nil {
		// Cleanup call if session creation fails
		cs.deleteCall(call.ID)
		return nil, fmt.Errorf("failed to create call session: %w", err)
	}

	// Store active call
	cs.callsMutex.Lock()
	cs.activeCalls[call.ID] = session
	cs.callsMutex.Unlock()

	// Set call timeout
	cs.setCallTimeout(call.ID, 30*time.Second) // 30 seconds to answer

	// Send notifications to participants
	go cs.notifyParticipants(call, "call_invitation")

	// Create system message in chat
	go cs.createCallMessage(call, "call_initiated")

	// Update statistics
	cs.updateCallStats("initiated")

	// Prepare response
	response := &CallResponse{
		Call:         call,
		Participants: cs.buildParticipantResponses(session),
		TURNServers:  cs.getTURNServers(),
		IceServers:   cs.config.GetTURNServers(),
		SignalingURL: cs.getSignalingURL(),
	}

	if session.Room != nil {
		response.Room = &CallRoomInfo{
			RoomID:      session.Room.ID,
			MaxPeers:    session.Room.MaxPeers,
			ActivePeers: session.Room.PeerCount,
			IsRecording: session.IsRecording,
		}
	}

	logger.LogUserAction(req.InitiatorID.Hex(), "call_initiated", "call_service", map[string]interface{}{
		"call_id":      call.ID.Hex(),
		"call_type":    call.Type,
		"participants": len(req.ParticipantIDs),
		"chat_id":      req.ChatID.Hex(),
	})

	return response, nil
}

// AnswerCall handles call answer
func (cs *CallService) AnswerCall(callID primitive.ObjectID, userID primitive.ObjectID, accept bool, deviceInfo models.DeviceInfo) (*CallResponse, error) {
	// Get active call session
	cs.callsMutex.RLock()
	session, exists := cs.activeCalls[callID]
	cs.callsMutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("call not found or not active")
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()

	// Check if user is a participant
	participantInfo, exists := session.Participants[userID]
	if !exists {
		return nil, fmt.Errorf("user not invited to this call")
	}

	if !accept {
		// Call rejected
		participantInfo.Status = models.ParticipantStatusRejected
		now := time.Now()
		participantInfo.LeftAt = &now

		// Update call in database
		cs.updateCallParticipant(callID, userID, models.ParticipantStatusRejected)

		// Notify other participants
		go cs.notifyParticipantsExcept(session.Call, "call_rejected", userID, map[string]interface{}{
			"user_id": userID.Hex(),
		})

		// Check if all participants rejected
		if cs.areAllParticipantsRejected(session) {
			go cs.endCall(callID, userID, models.EndReasonRejected)
		}

		logger.LogUserAction(userID.Hex(), "call_rejected", "call_service", map[string]interface{}{
			"call_id": callID.Hex(),
		})

		return nil, fmt.Errorf("call rejected")
	}

	// Call accepted
	participantInfo.Status = models.ParticipantStatusConnecting
	now := time.Now()
	participantInfo.JoinedAt = &now
	participantInfo.DeviceInfo = deviceInfo

	// Update call status if first acceptance
	if session.Call.Status == models.CallStatusRinging {
		session.Call.Status = models.CallStatusConnecting
		session.Call.BeforeUpdate()
		cs.updateCallStatus(callID, models.CallStatusConnecting)
	}

	// Update participant in database
	cs.updateCallParticipant(callID, userID, models.ParticipantStatusConnecting)

	// Clear call timeout
	cs.clearCallTimeout(callID)

	// Notify other participants
	go cs.notifyParticipantsExcept(session.Call, "call_accepted", userID, map[string]interface{}{
		"user_id": userID.Hex(),
	})

	// Create response
	response := &CallResponse{
		Call:         session.Call,
		Participants: cs.buildParticipantResponses(session),
		TURNServers:  cs.getTURNServers(),
		IceServers:   cs.config.GetTURNServers(),
		SignalingURL: cs.getSignalingURL(),
	}

	if session.Room != nil {
		response.Room = &CallRoomInfo{
			RoomID:      session.Room.ID,
			MaxPeers:    session.Room.MaxPeers,
			ActivePeers: session.Room.PeerCount,
			IsRecording: session.IsRecording,
		}
	}

	logger.LogUserAction(userID.Hex(), "call_accepted", "call_service", map[string]interface{}{
		"call_id": callID.Hex(),
	})

	return response, nil
}

// EndCall ends an active call
func (cs *CallService) EndCall(callID primitive.ObjectID, userID primitive.ObjectID, reason models.EndReason) error {
	return cs.endCall(callID, userID, reason)
}

// endCall internal method to end a call
func (cs *CallService) endCall(callID primitive.ObjectID, endedBy primitive.ObjectID, reason models.EndReason) error {
	// Get active call session
	cs.callsMutex.Lock()
	session, exists := cs.activeCalls[callID]
	if exists {
		delete(cs.activeCalls, callID)
	}
	cs.callsMutex.Unlock()

	if !exists {
		// Try to end call in database anyway
		return cs.endCallInDatabase(callID, endedBy, reason)
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()

	// Mark session as inactive
	session.IsActive = false

	// Calculate call duration
	var duration time.Duration
	if session.Call.StartedAt != nil {
		duration = time.Since(*session.Call.StartedAt)
	} else {
		duration = time.Since(session.StartTime)
	}

	// Update call in database
	now := time.Now()
	session.Call.EndCall(endedBy, reason)
	session.Call.Duration = int64(duration.Seconds())

	// Update all participants
	for userID, participant := range session.Participants {
		if participant.Status == models.ParticipantStatusConnected {
			participant.Status = models.ParticipantStatusDisconnected
			if participant.LeftAt == nil {
				participant.LeftAt = &now
			}
		}
	}

	// Save call to database
	if err := cs.saveCallToDatabase(session.Call); err != nil {
		logger.Errorf("Failed to save call to database: %v", err)
	}

	// Close WebRTC room if exists
	if session.Room != nil {
		go func() {
			if err := session.Room.Close(); err != nil {
				logger.Errorf("Failed to close WebRTC room: %v", err)
			}
		}()
	}

	// Clear timeout
	cs.clearCallTimeout(callID)

	// Notify all participants
	go cs.notifyAllParticipants(session.Call, "call_ended", map[string]interface{}{
		"ended_by": endedBy.Hex(),
		"reason":   reason,
		"duration": duration.Seconds(),
		"ended_at": now,
	})

	// Create system message
	go cs.createCallMessage(session.Call, "call_ended")

	// Update statistics
	cs.updateCallStats("ended")

	logger.LogUserAction(endedBy.Hex(), "call_ended", "call_service", map[string]interface{}{
		"call_id":  callID.Hex(),
		"reason":   reason,
		"duration": duration.Seconds(),
	})

	return nil
}

// JoinCall allows a user to join an ongoing call
func (cs *CallService) JoinCall(callID primitive.ObjectID, userID primitive.ObjectID, deviceInfo models.DeviceInfo) (*CallResponse, error) {
	// Get active call session
	cs.callsMutex.RLock()
	session, exists := cs.activeCalls[callID]
	cs.callsMutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("call not found or not active")
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()

	// Check if user can join
	if len(session.Participants) >= session.Call.Settings.MaxParticipants {
		return nil, fmt.Errorf("call has reached maximum participants")
	}

	// Check if user is already in call
	if _, exists := session.Participants[userID]; exists {
		return nil, fmt.Errorf("user already in call")
	}

	// Add participant to call
	now := time.Now()
	participantInfo := &CallParticipantInfo{
		UserID:         userID,
		Status:         models.ParticipantStatusConnecting,
		JoinedAt:       &now,
		DeviceInfo:     deviceInfo,
		MediaState:     models.MediaState{},
		QualityMetrics: &models.QualityMetrics{},
		LastActivity:   now,
	}

	session.Participants[userID] = participantInfo

	// Add to database
	participant := models.CallParticipant{
		UserID:     userID,
		Status:     models.ParticipantStatusConnecting,
		Role:       models.ParticipantRoleParticipant,
		JoinedAt:   &now,
		DeviceInfo: deviceInfo,
		MediaState: models.MediaState{},
	}

	session.Call.Participants = append(session.Call.Participants, participant)
	cs.saveCallToDatabase(session.Call)

	// Notify other participants
	go cs.notifyParticipantsExcept(session.Call, "participant_joined", userID, map[string]interface{}{
		"user_id":   userID.Hex(),
		"joined_at": now,
	})

	// Create response
	response := &CallResponse{
		Call:         session.Call,
		Participants: cs.buildParticipantResponses(session),
		TURNServers:  cs.getTURNServers(),
		IceServers:   cs.config.GetTURNServers(),
		SignalingURL: cs.getSignalingURL(),
	}

	if session.Room != nil {
		response.Room = &CallRoomInfo{
			RoomID:      session.Room.ID,
			MaxPeers:    session.Room.MaxPeers,
			ActivePeers: session.Room.PeerCount,
			IsRecording: session.IsRecording,
		}
	}

	logger.LogUserAction(userID.Hex(), "call_joined", "call_service", map[string]interface{}{
		"call_id": callID.Hex(),
	})

	return response, nil
}

// LeaveCall handles a user leaving a call
func (cs *CallService) LeaveCall(callID primitive.ObjectID, userID primitive.ObjectID) error {
	// Get active call session
	cs.callsMutex.RLock()
	session, exists := cs.activeCalls[callID]
	cs.callsMutex.RUnlock()

	if !exists {
		return fmt.Errorf("call not found or not active")
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()

	// Find participant
	participantInfo, exists := session.Participants[userID]
	if !exists {
		return fmt.Errorf("user not in call")
	}

	// Update participant status
	participantInfo.Status = models.ParticipantStatusDisconnected
	now := time.Now()
	participantInfo.LeftAt = &now

	// Update in database
	cs.updateCallParticipant(callID, userID, models.ParticipantStatusDisconnected)

	// Notify other participants
	go cs.notifyParticipantsExcept(session.Call, "participant_left", userID, map[string]interface{}{
		"user_id": userID.Hex(),
		"left_at": now,
	})

	// Check if call should end (no active participants)
	activeParticipants := 0
	for _, p := range session.Participants {
		if p.Status == models.ParticipantStatusConnected || p.Status == models.ParticipantStatusConnecting {
			activeParticipants++
		}
	}

	if activeParticipants <= 1 {
		// End call if only one or no participants left
		go cs.endCall(callID, userID, models.EndReasonNormal)
	}

	logger.LogUserAction(userID.Hex(), "call_left", "call_service", map[string]interface{}{
		"call_id": callID.Hex(),
	})

	return nil
}

// GetCallHistory returns user's call history
func (cs *CallService) GetCallHistory(userID primitive.ObjectID, page, limit int) (*models.CallHistoryResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Calculate skip
	skip := (page - 1) * limit

	// Build query to find calls where user is a participant
	filter := bson.M{
		"participants.user_id": userID,
	}

	// Count total calls
	total, err := cs.callsCollection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to count calls: %w", err)
	}

	// Find calls with pagination
	opts := options.Find().
		SetSort(bson.D{{Key: "initiated_at", Value: -1}}).
		SetSkip(int64(skip)).
		SetLimit(int64(limit))

	cursor, err := cs.callsCollection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find calls: %w", err)
	}
	defer cursor.Close(ctx)

	var calls []models.Call
	if err := cursor.All(ctx, &calls); err != nil {
		return nil, fmt.Errorf("failed to decode calls: %w", err)
	}

	// Convert to response format
	callResponses := make([]models.CallResponse, len(calls))
	for i, call := range calls {
		// Find user's participant info
		var myParticipant *models.CallParticipant
		for _, p := range call.Participants {
			if p.UserID == userID {
				myParticipant = &p
				break
			}
		}

		callResponses[i] = models.CallResponse{
			Call:          call,
			MyParticipant: myParticipant,
			CanControl:    call.InitiatorID == userID,
			CanRecord:     call.Settings.RecordingEnabled,
			CanInvite:     true,
			TURNServers:   cs.getTURNServers(),
		}
	}

	return &models.CallHistoryResponse{
		Calls:      callResponses,
		TotalCount: total,
		HasMore:    int64(skip+limit) < total,
	}, nil
}

// GetActiveCall returns information about an active call
func (cs *CallService) GetActiveCall(callID primitive.ObjectID, userID primitive.ObjectID) (*CallResponse, error) {
	// Get active call session
	cs.callsMutex.RLock()
	session, exists := cs.activeCalls[callID]
	cs.callsMutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("call not found or not active")
	}

	session.mutex.RLock()
	defer session.mutex.RUnlock()

	// Check if user is participant
	if _, exists := session.Participants[userID]; !exists {
		return nil, fmt.Errorf("user not in call")
	}

	// Create response
	response := &CallResponse{
		Call:         session.Call,
		Participants: cs.buildParticipantResponses(session),
		TURNServers:  cs.getTURNServers(),
		IceServers:   cs.config.GetTURNServers(),
		SignalingURL: cs.getSignalingURL(),
	}

	if session.Room != nil {
		response.Room = &CallRoomInfo{
			RoomID:      session.Room.ID,
			MaxPeers:    session.Room.MaxPeers,
			ActivePeers: session.Room.PeerCount,
			IsRecording: session.IsRecording,
		}
	}

	return response, nil
}

// StartRecording starts call recording
func (cs *CallService) StartRecording(callID primitive.ObjectID, userID primitive.ObjectID) error {
	// Get active call session
	cs.callsMutex.RLock()
	session, exists := cs.activeCalls[callID]
	cs.callsMutex.RUnlock()

	if !exists {
		return fmt.Errorf("call not found or not active")
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()

	// Check if user can start recording
	if !session.Call.Settings.RecordingEnabled {
		return fmt.Errorf("recording not enabled for this call")
	}

	// Check if user is call initiator or admin
	if session.Call.InitiatorID != userID {
		// Check if user is admin in group call
		canRecord := false
		for _, p := range session.Call.Participants {
			if p.UserID == userID && (p.Role == models.ParticipantRoleInitiator || p.Role == models.ParticipantRoleModerator) {
				canRecord = true
				break
			}
		}
		if !canRecord {
			return fmt.Errorf("insufficient permissions to start recording")
		}
	}

	// Start recording in room
	if session.Room != nil {
		if err := session.Room.StartRecording(userID); err != nil {
			return fmt.Errorf("failed to start recording: %w", err)
		}
	}

	// Update session
	session.IsRecording = true

	// Update call in database
	cs.updateCallRecording(callID, true, userID)

	// Notify all participants
	go cs.notifyAllParticipants(session.Call, "recording_started", map[string]interface{}{
		"started_by": userID.Hex(),
		"started_at": time.Now(),
	})

	logger.LogUserAction(userID.Hex(), "recording_started", "call_service", map[string]interface{}{
		"call_id": callID.Hex(),
	})

	return nil
}

// StopRecording stops call recording
func (cs *CallService) StopRecording(callID primitive.ObjectID, userID primitive.ObjectID) error {
	// Get active call session
	cs.callsMutex.RLock()
	session, exists := cs.activeCalls[callID]
	cs.callsMutex.RUnlock()

	if !exists {
		return fmt.Errorf("call not found or not active")
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()

	if !session.IsRecording {
		return fmt.Errorf("call is not being recorded")
	}

	// Stop recording in room
	if session.Room != nil {
		// The room will handle stopping recording internally
	}

	// Update session
	session.IsRecording = false

	// Update call in database
	cs.updateCallRecording(callID, false, primitive.NilObjectID)

	// Notify all participants
	go cs.notifyAllParticipants(session.Call, "recording_stopped", map[string]interface{}{
		"stopped_by": userID.Hex(),
		"stopped_at": time.Now(),
	})

	logger.LogUserAction(userID.Hex(), "recording_stopped", "call_service", map[string]interface{}{
		"call_id": callID.Hex(),
	})

	return nil
}

// UpdateMediaState updates a participant's media state
func (cs *CallService) UpdateMediaState(callID primitive.ObjectID, userID primitive.ObjectID, mediaState models.MediaState) error {
	// Get active call session
	cs.callsMutex.RLock()
	session, exists := cs.activeCalls[callID]
	cs.callsMutex.RUnlock()

	if !exists {
		return fmt.Errorf("call not found or not active")
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()

	// Find participant
	participantInfo, exists := session.Participants[userID]
	if !exists {
		return fmt.Errorf("user not in call")
	}

	// Update media state
	participantInfo.MediaState = mediaState
	participantInfo.LastActivity = time.Now()

	// Update in database
	cs.updateCallParticipantMedia(callID, userID, mediaState)

	// Notify other participants
	go cs.notifyParticipantsExcept(session.Call, "media_state_changed", userID, map[string]interface{}{
		"user_id":     userID.Hex(),
		"media_state": mediaState,
	})

	logger.LogUserAction(userID.Hex(), "media_state_updated", "call_service", map[string]interface{}{
		"call_id":        callID.Hex(),
		"video_enabled":  mediaState.VideoEnabled,
		"audio_enabled":  mediaState.AudioEnabled,
		"screen_sharing": mediaState.ScreenSharing,
	})

	return nil
}

// UpdateQualityMetrics updates call quality metrics for a participant
func (cs *CallService) UpdateQualityMetrics(callID primitive.ObjectID, userID primitive.ObjectID, quality *models.QualityMetrics) error {
	// Get active call session
	cs.callsMutex.RLock()
	session, exists := cs.activeCalls[callID]
	cs.callsMutex.RUnlock()

	if !exists {
		return fmt.Errorf("call not found or not active")
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()

	// Find participant
	participantInfo, exists := session.Participants[userID]
	if !exists {
		return fmt.Errorf("user not in call")
	}

	// Update quality metrics
	participantInfo.QualityMetrics = quality
	participantInfo.LastActivity = time.Now()

	// Store in session quality data
	session.QualityData[userID] = quality

	// Log quality issues if any
	if quality.QualityScore < 3.0 {
		logger.Warnf("Poor call quality detected for user %s in call %s: score %.2f",
			userID.Hex(), callID.Hex(), quality.QualityScore)
	}

	return nil
}

// GetCallStatistics returns call service statistics
func (cs *CallService) GetCallStatistics() *CallStatistics {
	cs.callStats.mutex.RLock()
	defer cs.callStats.mutex.RUnlock()

	// Create a copy
	stats := *cs.callStats
	return &stats
}

// Helper methods

// validateCallRequest validates call request
func (cs *CallService) validateCallRequest(req *CallRequest) error {
	if req.InitiatorID.IsZero() {
		return fmt.Errorf("initiator ID is required")
	}

	if len(req.ParticipantIDs) == 0 {
		return fmt.Errorf("at least one participant is required")
	}

	if req.ChatID.IsZero() {
		return fmt.Errorf("chat ID is required")
	}

	if req.Type == "" {
		return fmt.Errorf("call type is required")
	}

	// Check maximum participants based on call type
	maxParticipants := 2
	if req.Type == models.CallTypeGroup || req.Type == models.CallTypeConference {
		maxParticipants = 10
	}

	if len(req.ParticipantIDs) > maxParticipants {
		return fmt.Errorf("too many participants for call type %s", req.Type)
	}

	return nil
}

// checkParticipantsAvailability checks if participants are available
func (cs *CallService) checkParticipantsAvailability(participantIDs []primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if any participant is already in a call
	for _, userID := range participantIDs {
		// Check in active calls
		cs.callsMutex.RLock()
		for _, session := range cs.activeCalls {
			if _, exists := session.Participants[userID]; exists {
				cs.callsMutex.RUnlock()
				return fmt.Errorf("user %s is already in another call", userID.Hex())
			}
		}
		cs.callsMutex.RUnlock()

		// Check if user exists and is active
		var user models.User
		err := cs.usersCollection.FindOne(ctx, bson.M{
			"_id":        userID,
			"is_active":  true,
			"is_deleted": false,
		}).Decode(&user)
		if err != nil {
			return fmt.Errorf("user %s not found or inactive", userID.Hex())
		}
	}

	return nil
}

// createCall creates a new call in database
func (cs *CallService) createCall(req *CallRequest) (*models.Call, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create call model
	call := &models.Call{
		Type:        req.Type,
		Status:      models.CallStatusInitiating,
		InitiatorID: req.InitiatorID,
		ChatID:      req.ChatID,
	}

	// Set default settings if not provided
	if req.Settings == nil {
		call.Settings = models.CallSettings{
			MaxParticipants:   10,
			RequirePermission: false,
			AutoAccept:        false,
			RecordingEnabled:  false,
			MaxVideoBitrate:   2000000,
			MaxAudioBitrate:   128000,
			AdaptiveQuality:   true,
		}
	} else {
		call.Settings = *req.Settings
	}

	// Add participants
	for _, participantID := range req.ParticipantIDs {
		participant := models.CallParticipant{
			UserID: participantID,
			Status: models.ParticipantStatusInvited,
			Role:   models.ParticipantRoleParticipant,
			MediaState: models.MediaState{
				VideoEnabled: req.VideoEnabled,
				AudioEnabled: req.AudioEnabled,
			},
			DeviceInfo: req.DeviceInfo,
		}
		call.Participants = append(call.Participants, participant)
	}

	// Set initiator as initiator role
	if len(call.Participants) > 0 {
		for i := range call.Participants {
			if call.Participants[i].UserID == req.InitiatorID {
				call.Participants[i].Role = models.ParticipantRoleInitiator
				break
			}
		}
	}

	// Set before create
	call.BeforeCreate()

	// Insert into database
	result, err := cs.callsCollection.InsertOne(ctx, call)
	if err != nil {
		return nil, fmt.Errorf("failed to insert call: %w", err)
	}

	call.ID = result.InsertedID.(primitive.ObjectID)
	return call, nil
}

// createCallSession creates a call session with WebRTC room
func (cs *CallService) createCallSession(call *models.Call) (*CallSession, error) {
	// Create WebRTC room configuration
	roomConfig := webrtc.DefaultRoomConfig(call.Type)
	roomConfig.MaxParticipants = call.Settings.MaxParticipants
	roomConfig.VideoEnabled = call.Type == models.CallTypeVideo || call.Type == models.CallTypeGroup
	roomConfig.AudioEnabled = true
	roomConfig.EnableRecording = call.Settings.RecordingEnabled

	// Create WebRTC room
	room := webrtc.NewRoom(call.ID, call.ChatID, call.Type, roomConfig, cs.hub)

	// Create session
	session := &CallSession{
		Call:         call,
		Room:         room,
		Participants: make(map[primitive.ObjectID]*CallParticipantInfo),
		StartTime:    time.Now(),
		IsActive:     true,
		IsRecording:  false,
		QualityData:  make(map[primitive.ObjectID]*models.QualityMetrics),
		Events:       []CallEvent{},
	}

	// Add participants to session
	for _, participant := range call.Participants {
		participantInfo := &CallParticipantInfo{
			UserID:         participant.UserID,
			Status:         participant.Status,
			MediaState:     participant.MediaState,
			DeviceInfo:     participant.DeviceInfo,
			QualityMetrics: &models.QualityMetrics{},
			LastActivity:   time.Now(),
		}
		session.Participants[participant.UserID] = participantInfo
	}

	return session, nil
}

// notifyParticipants sends notifications to all call participants
func (cs *CallService) notifyParticipants(call *models.Call, eventType string) {
	for _, participant := range call.Participants {
		if participant.UserID == call.InitiatorID {
			continue // Don't notify initiator
		}

		// Send WebSocket notification
		if cs.hub != nil {
			data := map[string]interface{}{
				"call_id":      call.ID.Hex(),
				"initiator_id": call.InitiatorID.Hex(),
				"call_type":    call.Type,
				"chat_id":      call.ChatID.Hex(),
			}
			cs.hub.SendToUser(participant.UserID, eventType, data)
		}

		// Send push notification
		if cs.pushService != nil {
			go func(userID primitive.ObjectID) {
				initiatorName := cs.getUserName(call.InitiatorID)
				message := fmt.Sprintf("Incoming %s call from %s", call.Type, initiatorName)

				err := cs.pushService.SendCallNotification(userID, call.ID, message, map[string]interface{}{
					"call_id":   call.ID.Hex(),
					"call_type": call.Type,
					"action":    "incoming_call",
				})
				if err != nil {
					logger.Errorf("Failed to send push notification: %v", err)
				}
			}(participant.UserID)
		}
	}
}

// notifyParticipantsExcept sends notifications to all participants except one
func (cs *CallService) notifyParticipantsExcept(call *models.Call, eventType string, excludeUserID primitive.ObjectID, data map[string]interface{}) {
	for _, participant := range call.Participants {
		if participant.UserID == excludeUserID {
			continue
		}

		if cs.hub != nil {
			cs.hub.SendToUser(participant.UserID, eventType, data)
		}
	}
}

// notifyAllParticipants sends notifications to all participants
func (cs *CallService) notifyAllParticipants(call *models.Call, eventType string, data map[string]interface{}) {
	for _, participant := range call.Participants {
		if cs.hub != nil {
			cs.hub.SendToUser(participant.UserID, eventType, data)
		}
	}
}

// createCallMessage creates a system message for the call
func (cs *CallService) createCallMessage(call *models.Call, messageType string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var content string
	switch messageType {
	case "call_initiated":
		content = fmt.Sprintf("%s call started", call.Type)
	case "call_ended":
		duration := time.Duration(call.Duration) * time.Second
		content = fmt.Sprintf("Call ended • %s", cs.formatDuration(duration))
	default:
		content = "Call activity"
	}

	message := &models.Message{
		ChatID:   call.ChatID,
		SenderID: call.InitiatorID,
		Type:     models.MessageTypeCall,
		Content:  content,
		Status:   models.MessageStatusSent,
		Metadata: models.MessageMetadata{
			CallData: &models.CallMessageData{
				CallType:     string(call.Type),
				Duration:     int(call.Duration),
				Status:       "completed",
				Participants: []primitive.ObjectID{call.InitiatorID},
			},
		},
	}

	message.BeforeCreate()

	_, err := cs.messagesCollection.InsertOne(ctx, message)
	if err != nil {
		logger.Errorf("Failed to create call message: %v", err)
	}
}

// Database helper methods

// updateCallStatus updates call status in database
func (cs *CallService) updateCallStatus(callID primitive.ObjectID, status models.CallStatus) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"status":     status,
			"updated_at": time.Now(),
		},
	}

	_, err := cs.callsCollection.UpdateOne(ctx, bson.M{"_id": callID}, update)
	return err
}

// updateCallParticipant updates participant status
func (cs *CallService) updateCallParticipant(callID primitive.ObjectID, userID primitive.ObjectID, status models.ParticipantStatus) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"participants.$.status":     status,
			"participants.$.updated_at": time.Now(),
		},
	}

	filter := bson.M{
		"_id":                  callID,
		"participants.user_id": userID,
	}

	_, err := cs.callsCollection.UpdateOne(ctx, filter, update)
	return err
}

// updateCallParticipantMedia updates participant media state
func (cs *CallService) updateCallParticipantMedia(callID primitive.ObjectID, userID primitive.ObjectID, mediaState models.MediaState) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"participants.$.media_state": mediaState,
			"participants.$.updated_at":  time.Now(),
		},
	}

	filter := bson.M{
		"_id":                  callID,
		"participants.user_id": userID,
	}

	_, err := cs.callsCollection.UpdateOne(ctx, filter, update)
	return err
}

// updateCallRecording updates recording status
func (cs *CallService) updateCallRecording(callID primitive.ObjectID, isRecording bool, recordedBy primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	updateDoc := bson.M{
		"recording.is_recording": isRecording,
		"updated_at":             time.Now(),
	}

	if isRecording {
		now := time.Now()
		updateDoc["recording.started_at"] = &now
		updateDoc["recording.recorded_by"] = recordedBy
	} else {
		now := time.Now()
		updateDoc["recording.ended_at"] = &now
	}

	update := bson.M{"$set": updateDoc}

	_, err := cs.callsCollection.UpdateOne(ctx, bson.M{"_id": callID}, update)
	return err
}

// saveCallToDatabase saves call to database
func (cs *CallService) saveCallToDatabase(call *models.Call) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	call.BeforeUpdate()

	filter := bson.M{"_id": call.ID}
	update := bson.M{"$set": call}

	_, err := cs.callsCollection.UpdateOne(ctx, filter, update)
	return err
}

// deleteCall deletes a call from database
func (cs *CallService) deleteCall(callID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := cs.callsCollection.DeleteOne(ctx, bson.M{"_id": callID})
	return err
}

// endCallInDatabase ends call in database only
func (cs *CallService) endCallInDatabase(callID primitive.ObjectID, endedBy primitive.ObjectID, reason models.EndReason) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"status":     models.CallStatusEnded,
			"ended_at":   &now,
			"ended_by":   &endedBy,
			"end_reason": reason,
			"updated_at": now,
		},
	}

	_, err := cs.callsCollection.UpdateOne(ctx, bson.M{"_id": callID}, update)
	return err
}

// Utility methods

// buildParticipantResponses builds participant response data
func (cs *CallService) buildParticipantResponses(session *CallSession) []CallParticipantResponse {
	responses := make([]CallParticipantResponse, 0, len(session.Participants))

	for userID, participantInfo := range session.Participants {
		userInfo := cs.getUserInfo(userID)

		response := CallParticipantResponse{
			UserID:     userID,
			UserInfo:   userInfo,
			Status:     participantInfo.Status,
			JoinedAt:   participantInfo.JoinedAt,
			MediaState: participantInfo.MediaState,
			DeviceInfo: participantInfo.DeviceInfo,
			Quality:    participantInfo.QualityMetrics,
		}
		responses = append(responses, response)
	}

	return responses
}

// getUserInfo gets user public info
func (cs *CallService) getUserInfo(userID primitive.ObjectID) models.UserPublicInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var user models.User
	err := cs.usersCollection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		return models.UserPublicInfo{
			ID:   userID,
			Name: "Unknown User",
		}
	}

	return user.GetPublicInfo(userID) // Self info
}

// getUserName gets user name
func (cs *CallService) getUserName(userID primitive.ObjectID) string {
	userInfo := cs.getUserInfo(userID)
	return userInfo.Name
}

// getTURNServers gets TURN servers configuration
func (cs *CallService) getTURNServers() []models.TURNServer {
	servers := []models.TURNServer{}

	if cs.config.COTURNConfig.Host != "" {
		server := models.TURNServer{
			URLs: []string{
				fmt.Sprintf("turn:%s:%d", cs.config.COTURNConfig.Host, cs.config.COTURNConfig.Port),
				fmt.Sprintf("stun:%s:%d", cs.config.COTURNConfig.Host, cs.config.COTURNConfig.Port),
			},
			Username:   cs.config.COTURNConfig.Username,
			Credential: cs.config.COTURNConfig.Password,
		}
		servers = append(servers, server)
	}

	// Add public STUN servers as fallback
	servers = append(servers, models.TURNServer{
		URLs: []string{"stun:stun.l.google.com:19302"},
	})

	return servers
}

// getSignalingURL gets signaling server URL
func (cs *CallService) getSignalingURL() string {
	return "/api/webrtc/signaling" // WebSocket endpoint
}

// areAllParticipantsRejected checks if all participants rejected the call
func (cs *CallService) areAllParticipantsRejected(session *CallSession) bool {
	for _, participant := range session.Participants {
		if participant.Status != models.ParticipantStatusRejected &&
			participant.Status != models.ParticipantStatusMissed {
			return false
		}
	}
	return true
}

// formatDuration formats duration for display
func (cs *CallService) formatDuration(duration time.Duration) string {
	if duration < time.Minute {
		return fmt.Sprintf("%d sec", int(duration.Seconds()))
	}
	minutes := int(duration.Minutes())
	seconds := int(duration.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

// Background processes

// setCallTimeout sets a timeout for call
func (cs *CallService) setCallTimeout(callID primitive.ObjectID, duration time.Duration) {
	cs.timeoutsMutex.Lock()
	defer cs.timeoutsMutex.Unlock()

	// Clear existing timeout
	if timer, exists := cs.callTimeouts[callID]; exists {
		timer.Stop()
	}

	// Set new timeout
	timer := time.AfterFunc(duration, func() {
		cs.handleCallTimeout(callID)
	})
	cs.callTimeouts[callID] = timer
}

// clearCallTimeout clears call timeout
func (cs *CallService) clearCallTimeout(callID primitive.ObjectID) {
	cs.timeoutsMutex.Lock()
	defer cs.timeoutsMutex.Unlock()

	if timer, exists := cs.callTimeouts[callID]; exists {
		timer.Stop()
		delete(cs.callTimeouts, callID)
	}
}

// handleCallTimeout handles call timeout
func (cs *CallService) handleCallTimeout(callID primitive.ObjectID) {
	logger.Warnf("Call %s timed out", callID.Hex())

	// End call due to timeout
	cs.endCall(callID, primitive.NilObjectID, models.EndReasonTimeout)

	// Clear timeout
	cs.clearCallTimeout(callID)
}

// statsCollector collects call statistics
func (cs *CallService) statsCollector() {
	defer cs.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cs.collectStatistics()
		case <-cs.ctx.Done():
			return
		}
	}
}

// collectStatistics collects and updates statistics
func (cs *CallService) collectStatistics() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cs.callStats.mutex.Lock()
	defer cs.callStats.mutex.Unlock()

	// Count total calls
	total, _ := cs.callsCollection.CountDocuments(ctx, bson.M{})
	cs.callStats.TotalCalls = total

	// Count active calls
	cs.callsMutex.RLock()
	cs.callStats.ActiveCalls = len(cs.activeCalls)
	cs.callsMutex.RUnlock()

	// Count calls today
	today := time.Now().Truncate(24 * time.Hour)
	todayFilter := bson.M{
		"initiated_at": bson.M{
			"$gte": today,
		},
	}
	callsToday, _ := cs.callsCollection.CountDocuments(ctx, todayFilter)
	cs.callStats.CallsToday = callsToday

	// Calculate average call duration
	pipeline := []bson.M{
		{"$match": bson.M{"status": models.CallStatusEnded, "duration": bson.M{"$gt": 0}}},
		{"$group": bson.M{
			"_id":          nil,
			"avg_duration": bson.M{"$avg": "$duration"},
		}},
	}

	cursor, err := cs.callsCollection.Aggregate(ctx, pipeline)
	if err == nil {
		defer cursor.Close(ctx)
		if cursor.Next(ctx) {
			var result struct {
				AvgDuration float64 `bson:"avg_duration"`
			}
			if err := cursor.Decode(&result); err == nil {
				cs.callStats.AverageCallDuration = time.Duration(result.AvgDuration) * time.Second
			}
		}
	}

	cs.callStats.LastUpdated = time.Now()
}

// callTimeoutManager manages call timeouts
func (cs *CallService) callTimeoutManager() {
	defer cs.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cs.checkStaleCall()
		case <-cs.ctx.Done():
			return
		}
	}
}

// checkStaleCall checks for stale calls and cleans them up
func (cs *CallService) checkStaleCall() {
	cs.callsMutex.RLock()
	staleCalls := make([]primitive.ObjectID, 0)

	for callID, session := range cs.activeCalls {
		// Check if call has been active too long without progress
		if time.Since(session.StartTime) > 10*time.Minute {
			if session.Call.Status == models.CallStatusInitiating ||
				session.Call.Status == models.CallStatusRinging {
				staleCalls = append(staleCalls, callID)
			}
		}
	}
	cs.callsMutex.RUnlock()

	// End stale calls
	for _, callID := range staleCalls {
		logger.Warnf("Ending stale call %s", callID.Hex())
		cs.endCall(callID, primitive.NilObjectID, models.EndReasonTimeout)
	}
}

// qualityMonitor monitors call quality
func (cs *CallService) qualityMonitor() {
	defer cs.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cs.monitorCallQuality()
		case <-cs.ctx.Done():
			return
		}
	}
}

// monitorCallQuality monitors quality of active calls
func (cs *CallService) monitorCallQuality() {
	cs.callsMutex.RLock()
	defer cs.callsMutex.RUnlock()

	for callID, session := range cs.activeCalls {
		session.mutex.RLock()

		// Check quality metrics for each participant
		for userID, quality := range session.QualityData {
			if quality.QualityScore < 2.0 { // Poor quality threshold
				logger.Warnf("Poor call quality detected in call %s for user %s: score %.2f",
					callID.Hex(), userID.Hex(), quality.QualityScore)

				// Could implement automatic quality adjustment here
			}
		}

		session.mutex.RUnlock()
	}
}

// updateCallStats updates call statistics
func (cs *CallService) updateCallStats(eventType string) {
	cs.callStats.mutex.Lock()
	defer cs.callStats.mutex.Unlock()

	switch eventType {
	case "initiated":
		cs.callStats.CallsToday++
	case "ended":
		// Update success rate calculation could be added here
	}

	cs.callStats.LastUpdated = time.Now()
}

// Close gracefully shuts down the call service
func (cs *CallService) Close() error {
	logger.Info("Shutting down Call Service...")

	// Cancel context and wait for goroutines
	cs.cancel()
	cs.wg.Wait()

	// End all active calls
	cs.callsMutex.Lock()
	for callID := range cs.activeCalls {
		cs.endCall(callID, primitive.NilObjectID, models.EndReasonServerError)
	}
	cs.callsMutex.Unlock()

	// Clear all timeouts
	cs.timeoutsMutex.Lock()
	for _, timer := range cs.callTimeouts {
		timer.Stop()
	}
	cs.callTimeouts = make(map[primitive.ObjectID]*time.Timer)
	cs.timeoutsMutex.Unlock()

	logger.Info("Call Service shutdown complete")
	return nil
}
