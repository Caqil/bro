package webrtc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"bro/internal/config"
	"bro/internal/models"
	"bro/internal/services"
	"bro/internal/websocket"
	"bro/pkg/database"
	"bro/pkg/logger"
	"bro/pkg/redis"
)

// SignalingServer manages WebRTC signaling for all calls
type SignalingServer struct {
	// Configuration
	config *config.Config

	// Database collections
	callsCollection  *mongo.Collection
	chatsCollection  *mongo.Collection
	usersCollection  *mongo.Collection
	groupsCollection *mongo.Collection

	// Communication
	hub         *websocket.Hub
	redisClient *redis.Client

	// Services
	chatService *services.ChatService
	pushService *services.PushService

	// Room management
	rooms       map[primitive.ObjectID]*Room // CallID -> Room
	roomsByUser map[primitive.ObjectID]*Room // UserID -> Room (active call)
	roomsMutex  sync.RWMutex

	// WebRTC configuration
	webrtcConfig models.WebRTCServiceConfig

	// Message handlers
	messageHandlers map[string]SignalingHandler

	// Statistics
	stats *SignalingStatistics

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Timeouts and intervals
	callTimeout    time.Duration
	ringingTimeout time.Duration
}

// SignalingStatistics contains signaling server statistics
type SignalingStatistics struct {
	TotalCalls        int64     `json:"total_calls"`
	ActiveCalls       int64     `json:"active_calls"`
	CompletedCalls    int64     `json:"completed_calls"`
	FailedCalls       int64     `json:"failed_calls"`
	TotalSignalsSent  int64     `json:"total_signals_sent"`
	AverageCallLength float64   `json:"average_call_length"`
	LastUpdated       time.Time `json:"last_updated"`
	mutex             sync.RWMutex
}

// SignalingMessage represents a WebRTC signaling message
type SignalingMessage struct {
	Type     string                 `json:"type"`
	CallID   primitive.ObjectID     `json:"call_id"`
	RoomID   string                 `json:"room_id,omitempty"`
	PeerID   string                 `json:"peer_id,omitempty"`
	From     primitive.ObjectID     `json:"from"`
	To       primitive.ObjectID     `json:"to,omitempty"`
	Data     map[string]interface{} `json:"data"`
	Metadata SignalingMetadata      `json:"metadata"`
}

// SignalingMetadata contains metadata for signaling messages
type SignalingMetadata struct {
	Timestamp   time.Time `json:"timestamp"`
	MessageID   string    `json:"message_id"`
	UserAgent   string    `json:"user_agent,omitempty"`
	Platform    string    `json:"platform,omitempty"`
	DeviceID    string    `json:"device_id,omitempty"`
	NetworkType string    `json:"network_type,omitempty"`
}

// SignalingHandler interface for handling specific signaling message types
type SignalingHandler interface {
	HandleMessage(server *SignalingServer, client *websocket.Client, message *SignalingMessage) error
	GetMessageType() string
}

// CallRequest represents a call initiation request
type CallRequest struct {
	Type         models.CallType      `json:"type"`
	ChatID       primitive.ObjectID   `json:"chat_id"`
	Participants []primitive.ObjectID `json:"participants"`
	VideoEnabled bool                 `json:"video_enabled"`
	Settings     *models.CallSettings `json:"settings,omitempty"`
}

// CallResponse represents a call response (accept/reject)
type CallResponse struct {
	CallID       primitive.ObjectID   `json:"call_id"`
	Accept       bool                 `json:"accept"`
	VideoEnabled bool                 `json:"video_enabled"`
	RejectReason *models.RejectReason `json:"reject_reason,omitempty"`
}

// NewSignalingServer creates a new WebRTC signaling server
func NewSignalingServer(
	cfg *config.Config,
	hub *websocket.Hub,
	chatService *services.ChatService,
	pushService *services.PushService,
) (*SignalingServer, error) {

	collections := database.GetCollections()
	if collections == nil {
		return nil, fmt.Errorf("database collections not available")
	}

	ctx, cancel := context.WithCancel(context.Background())

	server := &SignalingServer{
		config:           cfg,
		callsCollection:  collections.Calls,
		chatsCollection:  collections.Chats,
		usersCollection:  collections.Users,
		groupsCollection: collections.Groups,
		hub:              hub,
		redisClient:      redis.GetClient(),
		chatService:      chatService,
		pushService:      pushService,
		rooms:            make(map[primitive.ObjectID]*Room),
		roomsByUser:      make(map[primitive.ObjectID]*Room),
		messageHandlers:  make(map[string]SignalingHandler),
		stats:            &SignalingStatistics{LastUpdated: time.Now()},
		ctx:              ctx,
		cancel:           cancel,
		callTimeout:      5 * time.Minute,
		ringingTimeout:   60 * time.Second,
	}

	// Load WebRTC configuration
	// This would typically come from admin config or environment
	server.webrtcConfig = models.WebRTCServiceConfig{
		STUNServers: []models.STUNServer{
			{URL: "stun:stun.l.google.com:19302"},
			{URL: "stun:stun1.l.google.com:19302"},
		},
		TURNServers: []models.TURNServerConfig{
			// TURN servers would be configured here
		},
		ICEConnectionTimeout: 30 * time.Second,
		DTLSTimeout:          10 * time.Second,
		EnableIPv6:           true,
		MaxBitrate:           2000000,
		MinBitrate:           100000,
		AdaptiveBitrate:      true,
		EnableRecording:      true,
		RecordingFormat:      "mp4",
	}

	// Register message handlers
	server.registerMessageHandlers()

	// Start background processes
	server.wg.Add(2)
	go server.statsCollector()
	go server.callTimeoutManager()

	logger.Info("WebRTC Signaling Server initialized successfully")
	return server, nil
}

// registerMessageHandlers registers all signaling message handlers
func (s *SignalingServer) registerMessageHandlers() {
	handlers := []SignalingHandler{
		&CallInitiateHandler{},
		&CallResponseHandler{},
		&CallEndHandler{},
		&SDPOfferHandler{},
		&SDPAnswerHandler{},
		&ICECandidateHandler{},
		&MediaControlHandler{},
		&CallStatusHandler{},
		&ReconnectHandler{},
	}

	for _, handler := range handlers {
		s.messageHandlers[handler.GetMessageType()] = handler
		logger.Debugf("Registered signaling handler: %s", handler.GetMessageType())
	}
}

// HandleSignalingMessage handles incoming signaling messages
func (s *SignalingServer) HandleSignalingMessage(client *websocket.Client, rawMessage []byte) error {
	var message SignalingMessage
	if err := json.Unmarshal(rawMessage, &message); err != nil {
		return fmt.Errorf("failed to unmarshal signaling message: %w", err)
	}

	// Set metadata
	message.Metadata = SignalingMetadata{
		Timestamp: time.Now(),
		MessageID: primitive.NewObjectID().Hex(),
		Platform:  client.GetInfo().Platform,
		DeviceID:  client.GetInfo().DeviceID,
	}

	// Set sender
	message.From = client.GetUserID()

	// Find handler
	handler, exists := s.messageHandlers[message.Type]
	if !exists {
		return fmt.Errorf("no handler found for signaling message type: %s", message.Type)
	}

	// Handle message
	if err := handler.HandleMessage(s, client, &message); err != nil {
		logger.Errorf("Error handling signaling message %s: %v", message.Type, err)
		return err
	}

	// Update statistics
	s.updateSignalingStats()

	return nil
}

// InitiateCall initiates a new call
func (s *SignalingServer) InitiateCall(initiatorID primitive.ObjectID, request *CallRequest) (*models.Call, error) {
	// Validate request
	if err := s.validateCallRequest(initiatorID, request); err != nil {
		return nil, fmt.Errorf("invalid call request: %w", err)
	}

	// Create call in database
	call, err := s.createCallInDatabase(initiatorID, request)
	if err != nil {
		return nil, fmt.Errorf("failed to create call: %w", err)
	}

	// Create room
	room, err := s.createRoom(call)
	if err != nil {
		s.deleteCallFromDatabase(call.ID)
		return nil, fmt.Errorf("failed to create room: %w", err)
	}

	// Store room
	s.roomsMutex.Lock()
	s.rooms[call.ID] = room
	s.roomsByUser[initiatorID] = room
	s.roomsMutex.Unlock()

	// Notify participants
	go s.notifyParticipantsAboutCall(call, "call_initiated")

	// Send push notifications
	go s.sendCallNotifications(call, "incoming_call")

	// Set call timeout
	go s.setCallTimeout(call.ID, s.ringingTimeout)

	// Update statistics
	s.updateCallStats("initiated")

	logger.LogUserAction(initiatorID.Hex(), "call_initiated", "webrtc_signaling", map[string]interface{}{
		"call_id":      call.ID.Hex(),
		"call_type":    call.Type,
		"participants": len(call.Participants),
	})

	return call, nil
}

// RespondToCall handles call response (accept/reject)
func (s *SignalingServer) RespondToCall(userID primitive.ObjectID, response *CallResponse) error {
	// Get call from database
	call, err := s.getCallFromDatabase(response.CallID)
	if err != nil {
		return fmt.Errorf("call not found: %w", err)
	}

	// Check if user is a participant
	participant := call.GetParticipant(userID)
	if participant == nil {
		return fmt.Errorf("user is not a participant in this call")
	}

	// Update participant status
	if response.Accept {
		call.UpdateParticipantStatus(userID, models.ParticipantStatusConnected)
		participant.MediaState.VideoEnabled = response.VideoEnabled
	} else {
		call.UpdateParticipantStatus(userID, models.ParticipantStatusRejected)
		if response.RejectReason != nil {
			participant.RejectReason = response.RejectReason
		}
	}

	// Update call in database
	if err := s.updateCallInDatabase(call); err != nil {
		logger.Errorf("Failed to update call in database: %v", err)
	}

	// Handle response
	if response.Accept {
		// Join room
		s.roomsMutex.RLock()
		room, exists := s.rooms[call.ID]
		s.roomsMutex.RUnlock()

		if !exists {
			return fmt.Errorf("call room not found")
		}

		// Add user to room
		s.roomsMutex.Lock()
		s.roomsByUser[userID] = room
		s.roomsMutex.Unlock()

		// Notify other participants
		s.notifyParticipantsAboutCall(call, "call_accepted")

	} else {
		// Notify other participants about rejection
		s.notifyParticipantsAboutCall(call, "call_rejected")

		// End call if all participants rejected or no one accepted
		if s.shouldEndCallAfterReject(call) {
			s.EndCall(call.ID, userID, models.EndReasonRejected)
		}
	}

	action := "call_rejected"
	if response.Accept {
		action = "call_accepted"
	}

	logger.LogUserAction(userID.Hex(), action, "webrtc_signaling", map[string]interface{}{
		"call_id": call.ID.Hex(),
	})

	return nil
}

// EndCall ends a call
func (s *SignalingServer) EndCall(callID primitive.ObjectID, userID primitive.ObjectID, reason models.EndReason) error {
	// Get room
	s.roomsMutex.RLock()
	room, exists := s.rooms[callID]
	s.roomsMutex.RUnlock()

	if !exists {
		return fmt.Errorf("call not found")
	}

	// End the call in room
	if err := room.EndCall(userID, reason); err != nil {
		return fmt.Errorf("failed to end call: %w", err)
	}

	// Remove room from active rooms
	s.roomsMutex.Lock()
	delete(s.rooms, callID)

	// Remove from user rooms
	for participantUserID := range room.PeersByUserID {
		if s.roomsByUser[participantUserID] != nil && s.roomsByUser[participantUserID].ID == room.ID {
			delete(s.roomsByUser, participantUserID)
		}
	}
	s.roomsMutex.Unlock()

	// Update statistics
	s.updateCallStats("ended")

	logger.LogUserAction(userID.Hex(), "call_ended", "webrtc_signaling", map[string]interface{}{
		"call_id": callID.Hex(),
		"reason":  reason,
	})

	return nil
}

// JoinCall allows a user to join an ongoing call
func (s *SignalingServer) JoinCall(userID primitive.ObjectID, callID primitive.ObjectID, wsClient *websocket.Client) (*PeerConnection, error) {
	// Get room
	s.roomsMutex.RLock()
	room, exists := s.rooms[callID]
	s.roomsMutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("call not found")
	}

	// Check if user can join
	if !room.CanUserJoin(userID) {
		return nil, fmt.Errorf("user cannot join this call")
	}

	// Join room
	peer, err := room.JoinRoom(userID, wsClient)
	if err != nil {
		return nil, fmt.Errorf("failed to join room: %w", err)
	}

	// Add to user rooms mapping
	s.roomsMutex.Lock()
	s.roomsByUser[userID] = room
	s.roomsMutex.Unlock()

	logger.LogUserAction(userID.Hex(), "joined_call", "webrtc_signaling", map[string]interface{}{
		"call_id": callID.Hex(),
		"peer_id": peer.ID,
	})

	return peer, nil
}

// LeaveCall allows a user to leave a call
func (s *SignalingServer) LeaveCall(userID primitive.ObjectID) error {
	// Get user's active room
	s.roomsMutex.RLock()
	room, exists := s.roomsByUser[userID]
	s.roomsMutex.RUnlock()

	if !exists {
		return fmt.Errorf("user is not in any call")
	}

	// Leave room
	if err := room.LeaveRoom(userID); err != nil {
		return fmt.Errorf("failed to leave room: %w", err)
	}

	// Remove from user rooms mapping
	s.roomsMutex.Lock()
	delete(s.roomsByUser, userID)
	s.roomsMutex.Unlock()

	logger.LogUserAction(userID.Hex(), "left_call", "webrtc_signaling", map[string]interface{}{
		"call_id": room.CallID.Hex(),
		"room_id": room.ID,
	})

	return nil
}

// GetUserActiveCall returns user's active call
func (s *SignalingServer) GetUserActiveCall(userID primitive.ObjectID) (*Room, bool) {
	s.roomsMutex.RLock()
	defer s.roomsMutex.RUnlock()

	room, exists := s.roomsByUser[userID]
	return room, exists
}

// GetRoom returns a room by call ID
func (s *SignalingServer) GetRoom(callID primitive.ObjectID) (*Room, bool) {
	s.roomsMutex.RLock()
	defer s.roomsMutex.RUnlock()

	room, exists := s.rooms[callID]
	return room, exists
}

// GetActiveRooms returns all active rooms
func (s *SignalingServer) GetActiveRooms() []*Room {
	s.roomsMutex.RLock()
	defer s.roomsMutex.RUnlock()

	rooms := make([]*Room, 0, len(s.rooms))
	for _, room := range s.rooms {
		if room.IsActive() {
			rooms = append(rooms, room)
		}
	}
	return rooms
}

// Message Handlers Implementation

// CallInitiateHandler handles call initiation
type CallInitiateHandler struct{}

func (h *CallInitiateHandler) GetMessageType() string { return "call_initiate" }

func (h *CallInitiateHandler) HandleMessage(server *SignalingServer, client *websocket.Client, message *SignalingMessage) error {
	var request CallRequest
	if err := mapToStruct(message.Data, &request); err != nil {
		return fmt.Errorf("invalid call request: %w", err)
	}

	call, err := server.InitiateCall(client.GetUserID(), &request)
	if err != nil {
		client.SendError("CALL_INITIATE_FAILED", "Failed to initiate call", err.Error())
		return err
	}

	// Send response to initiator
	client.SendJSON("call_initiated", map[string]interface{}{
		"call_id": call.ID.Hex(),
		"room_id": server.rooms[call.ID].ID,
		"call":    call,
	})

	return nil
}

// CallResponseHandler handles call responses
type CallResponseHandler struct{}

func (h *CallResponseHandler) GetMessageType() string { return "call_response" }

func (h *CallResponseHandler) HandleMessage(server *SignalingServer, client *websocket.Client, message *SignalingMessage) error {
	var response CallResponse
	if err := mapToStruct(message.Data, &response); err != nil {
		return fmt.Errorf("invalid call response: %w", err)
	}

	if err := server.RespondToCall(client.GetUserID(), &response); err != nil {
		client.SendError("CALL_RESPONSE_FAILED", "Failed to respond to call", err.Error())
		return err
	}

	client.SendJSON("call_response_sent", map[string]interface{}{
		"call_id":  response.CallID.Hex(),
		"accepted": response.Accept,
	})

	return nil
}

// CallEndHandler handles call termination
type CallEndHandler struct{}

func (h *CallEndHandler) GetMessageType() string { return "call_end" }

func (h *CallEndHandler) HandleMessage(server *SignalingServer, client *websocket.Client, message *SignalingMessage) error {
	reason := models.EndReasonNormal
	if reasonStr, ok := message.Data["reason"].(string); ok {
		reason = models.EndReason(reasonStr)
	}

	if err := server.EndCall(message.CallID, client.GetUserID(), reason); err != nil {
		client.SendError("CALL_END_FAILED", "Failed to end call", err.Error())
		return err
	}

	client.SendJSON("call_ended", map[string]interface{}{
		"call_id": message.CallID.Hex(),
		"reason":  reason,
	})

	return nil
}

// SDPOfferHandler handles SDP offers
type SDPOfferHandler struct{}

func (h *SDPOfferHandler) GetMessageType() string { return "sdp_offer" }

func (h *SDPOfferHandler) HandleMessage(server *SignalingServer, client *websocket.Client, message *SignalingMessage) error {
	// Get room and peer
	room, exists := server.GetRoom(message.CallID)
	if !exists {
		return fmt.Errorf("call room not found")
	}

	peer, exists := room.GetPeer(client.GetUserID())
	if !exists {
		return fmt.Errorf("peer not found in room")
	}

	// Extract SDP
	sdpData, ok := message.Data["sdp"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid SDP data")
	}

	sdp := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdpData["sdp"].(string),
	}

	// Set remote description
	if err := peer.SetRemoteDescription(sdp); err != nil {
		return fmt.Errorf("failed to set remote description: %w", err)
	}

	// Create answer
	answer, err := peer.CreateAnswer(nil)
	if err != nil {
		return fmt.Errorf("failed to create answer: %w", err)
	}

	// Send answer back
	client.SendJSON("sdp_answer", map[string]interface{}{
		"call_id": message.CallID.Hex(),
		"peer_id": peer.ID,
		"sdp": map[string]interface{}{
			"type": answer.Type.String(),
			"sdp":  answer.SDP,
		},
	})

	return nil
}

// SDPAnswerHandler handles SDP answers
type SDPAnswerHandler struct{}

func (h *SDPAnswerHandler) GetMessageType() string { return "sdp_answer" }

func (h *SDPAnswerHandler) HandleMessage(server *SignalingServer, client *websocket.Client, message *SignalingMessage) error {
	// Get room and peer
	room, exists := server.GetRoom(message.CallID)
	if !exists {
		return fmt.Errorf("call room not found")
	}

	peer, exists := room.GetPeer(client.GetUserID())
	if !exists {
		return fmt.Errorf("peer not found in room")
	}

	// Extract SDP
	sdpData, ok := message.Data["sdp"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid SDP data")
	}

	sdp := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  sdpData["sdp"].(string),
	}

	// Set remote description
	if err := peer.SetRemoteDescription(sdp); err != nil {
		return fmt.Errorf("failed to set remote description: %w", err)
	}

	return nil
}

// ICECandidateHandler handles ICE candidates
type ICECandidateHandler struct{}

func (h *ICECandidateHandler) GetMessageType() string { return "ice_candidate" }

func (h *ICECandidateHandler) HandleMessage(server *SignalingServer, client *websocket.Client, message *SignalingMessage) error {
	// Get room and peer
	room, exists := server.GetRoom(message.CallID)
	if !exists {
		return fmt.Errorf("call room not found")
	}

	peer, exists := room.GetPeer(client.GetUserID())
	if !exists {
		return fmt.Errorf("peer not found in room")
	}

	// Extract ICE candidate
	candidateData, ok := message.Data["candidate"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid ICE candidate data")
	}

	candidate := webrtc.ICECandidateInit{
		Candidate:     candidateData["candidate"].(string),
		SDPMid:        stringPtr(candidateData["sdpMid"].(string)),
		SDPMLineIndex: uint16Ptr(uint16(candidateData["sdpMLineIndex"].(float64))),
	}

	// Add ICE candidate
	if err := peer.AddICECandidate(candidate); err != nil {
		return fmt.Errorf("failed to add ICE candidate: %w", err)
	}

	return nil
}

// MediaControlHandler handles media control (mute/unmute, video on/off)
type MediaControlHandler struct{}

func (h *MediaControlHandler) GetMessageType() string { return "media_control" }

func (h *MediaControlHandler) HandleMessage(server *SignalingServer, client *websocket.Client, message *SignalingMessage) error {
	// Get room and peer
	room, exists := server.GetRoom(message.CallID)
	if !exists {
		return fmt.Errorf("call room not found")
	}

	peer, exists := room.GetPeer(client.GetUserID())
	if !exists {
		return fmt.Errorf("peer not found in room")
	}

	action, ok := message.Data["action"].(string)
	if !ok {
		return fmt.Errorf("invalid media control action")
	}

	// Handle different media control actions
	switch action {
	case "mute_audio":
		peer.MediaState.AudioEnabled = false
	case "unmute_audio":
		peer.MediaState.AudioEnabled = true
	case "disable_video":
		peer.MediaState.VideoEnabled = false
	case "enable_video":
		peer.MediaState.VideoEnabled = true
	case "start_screen_share":
		peer.MediaState.ScreenSharing = true
	case "stop_screen_share":
		peer.MediaState.ScreenSharing = false
	default:
		return fmt.Errorf("unknown media control action: %s", action)
	}

	// Notify other peers about media state change
	room.OnPeerStateChange(peer, "media_control", map[string]interface{}{
		"action":      action,
		"media_state": peer.MediaState,
	})

	return nil
}

// CallStatusHandler handles call status updates
type CallStatusHandler struct{}

func (h *CallStatusHandler) GetMessageType() string { return "call_status" }

func (h *CallStatusHandler) HandleMessage(server *SignalingServer, client *websocket.Client, message *SignalingMessage) error {
	// This can be used for status updates, ping/pong, etc.
	status, ok := message.Data["status"].(string)
	if !ok {
		return fmt.Errorf("invalid status data")
	}

	logger.Debugf("Call status update from %s: %s", client.GetUserID().Hex(), status)

	// Echo status back
	client.SendJSON("call_status_ack", map[string]interface{}{
		"call_id": message.CallID.Hex(),
		"status":  status,
	})

	return nil
}

// ReconnectHandler handles reconnection requests
type ReconnectHandler struct{}

func (h *ReconnectHandler) GetMessageType() string { return "reconnect" }

func (h *ReconnectHandler) HandleMessage(server *SignalingServer, client *websocket.Client, message *SignalingMessage) error {
	// Get room
	room, exists := server.GetRoom(message.CallID)
	if !exists {
		return fmt.Errorf("call room not found")
	}

	// Try to rejoin the call
	_, err := server.JoinCall(client.GetUserID(), message.CallID, client)
	if err != nil {
		client.SendError("RECONNECT_FAILED", "Failed to reconnect to call", err.Error())
		return err
	}

	client.SendJSON("reconnected", map[string]interface{}{
		"call_id": message.CallID.Hex(),
		"room_id": room.ID,
	})

	return nil
}

// Database operations

// createCallInDatabase creates a call document in the database
func (s *SignalingServer) createCallInDatabase(initiatorID primitive.ObjectID, request *CallRequest) (*models.Call, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	call := &models.Call{
		Type:        request.Type,
		Status:      models.CallStatusRinging,
		InitiatorID: initiatorID,
		ChatID:      request.ChatID,
		SessionID:   primitive.NewObjectID().Hex(),
	}

	// Add participants
	allParticipants := append([]primitive.ObjectID{initiatorID}, request.Participants...)
	for _, participantID := range allParticipants {
		role := models.ParticipantRoleParticipant
		if participantID == initiatorID {
			role = models.ParticipantRoleInitiator
		}

		participant := models.CallParticipant{
			UserID: participantID,
			Status: models.ParticipantStatusInvited,
			Role:   role,
			MediaState: models.MediaState{
				VideoEnabled:      request.VideoEnabled,
				AudioEnabled:      true,
				ScreenSharing:     false,
				RecordingLocal:    false,
				VideoQuality:      models.VideoQualityMedium,
				VideoResolution:   models.Resolution{Width: 1280, Height: 720},
				VideoFrameRate:    30,
				AudioQuality:      models.AudioQualityMedium,
				AudioCodec:        "opus",
				NetworkAdaptation: true,
			},
			DeviceInfo: models.DeviceInfo{
				SupportsVideo:       true,
				SupportsAudio:       true,
				SupportsScreenShare: true,
			},
			Settings: models.ParticipantSettings{
				AutoMute:         false,
				AutoVideo:        request.VideoEnabled,
				NoiseReduction:   true,
				EchoCancellation: true,
				BackgroundBlur:   false,
			},
		}

		call.Participants = append(call.Participants, participant)
	}

	// Set before create
	call.BeforeCreate()

	// Apply custom settings if provided
	if request.Settings != nil {
		call.Settings = *request.Settings
	}

	// Insert into database
	result, err := s.callsCollection.InsertOne(ctx, call)
	if err != nil {
		return nil, fmt.Errorf("failed to insert call: %w", err)
	}

	call.ID = result.InsertedID.(primitive.ObjectID)
	return call, nil
}

// getCallFromDatabase retrieves a call from the database
func (s *SignalingServer) getCallFromDatabase(callID primitive.ObjectID) (*models.Call, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var call models.Call
	err := s.callsCollection.FindOne(ctx, bson.M{"_id": callID}).Decode(&call)
	if err != nil {
		return nil, fmt.Errorf("call not found: %w", err)
	}

	return &call, nil
}

// updateCallInDatabase updates a call in the database
func (s *SignalingServer) updateCallInDatabase(call *models.Call) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	call.BeforeUpdate()

	_, err := s.callsCollection.ReplaceOne(ctx, bson.M{"_id": call.ID}, call)
	return err
}

// deleteCallFromDatabase deletes a call from the database
func (s *SignalingServer) deleteCallFromDatabase(callID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.callsCollection.DeleteOne(ctx, bson.M{"_id": callID})
	return err
}

// Helper methods

// validateCallRequest validates a call request
func (s *SignalingServer) validateCallRequest(initiatorID primitive.ObjectID, request *CallRequest) error {
	if request.Type == "" {
		return fmt.Errorf("call type is required")
	}

	if len(request.Participants) == 0 {
		return fmt.Errorf("at least one participant is required")
	}

	// Check if initiator is not calling themselves
	for _, participantID := range request.Participants {
		if participantID == initiatorID {
			return fmt.Errorf("cannot call yourself")
		}
	}

	// Check participant limits
	maxParticipants := 10
	if request.Type == models.CallTypePrivate {
		maxParticipants = 1
	} else if request.Type == models.CallTypeGroup {
		maxParticipants = 50
	} else if request.Type == models.CallTypeConference {
		maxParticipants = 100
	}

	if len(request.Participants) > maxParticipants {
		return fmt.Errorf("too many participants for call type %s", request.Type)
	}

	// Check if participants exist and are available
	return s.validateParticipants(append(request.Participants, initiatorID))
}

// validateParticipants validates that participants exist and are available
func (s *SignalingServer) validateParticipants(participantIDs []primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if all users exist and are active
	count, err := s.usersCollection.CountDocuments(ctx, bson.M{
		"_id":        bson.M{"$in": participantIDs},
		"is_active":  true,
		"is_deleted": false,
	})
	if err != nil {
		return fmt.Errorf("failed to validate participants: %w", err)
	}

	if int(count) != len(participantIDs) {
		return fmt.Errorf("one or more participants not found or inactive")
	}

	return nil
}

// createRoom creates a new room for the call
func (s *SignalingServer) createRoom(call *models.Call) (*Room, error) {
	roomConfig := &RoomConfig{
		MaxParticipants:   call.Settings.MaxParticipants,
		WebRTCConfig:      s.webrtcConfig,
		DefaultSettings:   call.Settings,
		DefaultFeatures:   call.Features,
		QualitySettings:   call.QualitySettings,
		EnableRecording:   call.Settings.RecordingEnabled,
		EnableAnalytics:   true,
		HeartbeatInterval: 30 * time.Second,
		StatsInterval:     10 * time.Second,
	}

	return NewRoom(call.ID, call.ChatID, call.Type, call.InitiatorID, s.hub, roomConfig)
}

// shouldEndCallAfterReject determines if call should end after rejection
func (s *SignalingServer) shouldEndCallAfterReject(call *models.Call) bool {
	acceptedCount := 0
	rejectedCount := 0
	pendingCount := 0

	for _, participant := range call.Participants {
		switch participant.Status {
		case models.ParticipantStatusConnected:
			acceptedCount++
		case models.ParticipantStatusRejected:
			rejectedCount++
		case models.ParticipantStatusInvited, models.ParticipantStatusRinging:
			pendingCount++
		}
	}

	// For private calls, end if rejected
	if call.Type == models.CallTypePrivate && rejectedCount > 0 {
		return true
	}

	// For group calls, end if all participants rejected or only initiator remains
	if acceptedCount <= 1 && pendingCount == 0 {
		return true
	}

	return false
}

// notifyParticipantsAboutCall notifies participants about call events
func (s *SignalingServer) notifyParticipantsAboutCall(call *models.Call, eventType string) {
	for _, participant := range call.Participants {
		if s.hub != nil {
			data := map[string]interface{}{
				"call_id":    call.ID.Hex(),
				"call_type":  call.Type,
				"event_type": eventType,
				"call":       call,
			}

			s.hub.SendToUser(participant.UserID, "call_event", data)
		}
	}
}

// sendCallNotifications sends push notifications for call events
func (s *SignalingServer) sendCallNotifications(call *models.Call, notificationType string) {
	if s.pushService == nil {
		return
	}

	for _, participant := range call.Participants {
		// Skip initiator for incoming call notifications
		if notificationType == "incoming_call" && participant.UserID == call.InitiatorID {
			continue
		}

		go func(userID primitive.ObjectID) {
			// Get user for notification
			user, err := s.getUserFromDatabase(userID)
			if err != nil {
				logger.Errorf("Failed to get user for notification: %v", err)
				return
			}

			var title, message string
			switch notificationType {
			case "incoming_call":
				initiator, _ := s.getUserFromDatabase(call.InitiatorID)
				initiatorName := "Unknown"
				if initiator != nil {
					initiatorName = initiator.Name
				}

				if call.Type == models.CallTypePrivate {
					title = "Incoming Call"
					message = fmt.Sprintf("%s is calling you", initiatorName)
				} else {
					title = "Incoming Group Call"
					message = fmt.Sprintf("%s started a group call", initiatorName)
				}
			}

			// TODO: Send push notification through push service
			// This depends on your actual PushService implementation
			logger.Infof("Sending %s notification to user %s: %s", notificationType, userID.Hex(), message)
		}(participant.UserID)
	}
}

// getUserFromDatabase gets user from database
func (s *SignalingServer) getUserFromDatabase(userID primitive.ObjectID) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var user models.User
	err := s.usersCollection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	return &user, nil
}

// setCallTimeout sets a timeout for call response
func (s *SignalingServer) setCallTimeout(callID primitive.ObjectID, timeout time.Duration) {
	time.AfterFunc(timeout, func() {
		// Check if call is still ringing
		call, err := s.getCallFromDatabase(callID)
		if err != nil {
			return
		}

		if call.Status == models.CallStatusRinging {
			// End call due to timeout
			s.EndCall(callID, call.InitiatorID, models.EndReasonTimeout)
		}
	})
}

// Background processes

// statsCollector collects signaling statistics
func (s *SignalingServer) statsCollector() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.collectStatistics()
		case <-s.ctx.Done():
			return
		}
	}
}

// collectStatistics collects and updates statistics
func (s *SignalingServer) collectStatistics() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s.stats.mutex.Lock()
	defer s.stats.mutex.Unlock()

	// Count total calls
	total, _ := s.callsCollection.CountDocuments(ctx, bson.M{})
	s.stats.TotalCalls = total

	// Count active calls
	s.roomsMutex.RLock()
	s.stats.ActiveCalls = int64(len(s.rooms))
	s.roomsMutex.RUnlock()

	// Count completed calls
	completed, _ := s.callsCollection.CountDocuments(ctx, bson.M{
		"status": models.CallStatusEnded,
	})
	s.stats.CompletedCalls = completed

	// Count failed calls
	failed, _ := s.callsCollection.CountDocuments(ctx, bson.M{
		"status": bson.M{"$in": []models.CallStatus{
			models.CallStatusFailed,
			models.CallStatusRejected,
			models.CallStatusMissed,
		}},
	})
	s.stats.FailedCalls = failed

	s.stats.LastUpdated = time.Now()
}

// callTimeoutManager manages call timeouts
func (s *SignalingServer) callTimeoutManager() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkCallTimeouts()
		case <-s.ctx.Done():
			return
		}
	}
}

// checkCallTimeouts checks for timed out calls
func (s *SignalingServer) checkCallTimeouts() {
	s.roomsMutex.RLock()
	rooms := make([]*Room, 0, len(s.rooms))
	for _, room := range s.rooms {
		rooms = append(rooms, room)
	}
	s.roomsMutex.RUnlock()

	now := time.Now()

	for _, room := range rooms {
		// Check if call has been ringing too long
		if room.Status == models.CallStatusRinging {
			if now.Sub(room.CreatedAt) > s.ringingTimeout {
				logger.Infof("Call %s timed out (ringing too long)", room.CallID.Hex())
				s.EndCall(room.CallID, room.InitiatorID, models.EndReasonTimeout)
			}
		}

		// Check if call has been active too long
		if room.Status == models.CallStatusActive && room.StartedAt != nil {
			if now.Sub(*room.StartedAt) > s.callTimeout {
				logger.Infof("Call %s timed out (active too long)", room.CallID.Hex())
				s.EndCall(room.CallID, room.InitiatorID, models.EndReasonTimeout)
			}
		}
	}
}

// updateSignalingStats updates signaling statistics
func (s *SignalingServer) updateSignalingStats() {
	s.stats.mutex.Lock()
	s.stats.TotalSignalsSent++
	s.stats.mutex.Unlock()
}

// updateCallStats updates call statistics
func (s *SignalingServer) updateCallStats(eventType string) {
	s.stats.mutex.Lock()
	defer s.stats.mutex.Unlock()

	switch eventType {
	case "initiated":
		// Statistics are updated in collectStatistics
	case "ended":
		// Statistics are updated in collectStatistics
	}

	s.stats.LastUpdated = time.Now()
}

// GetStatistics returns signaling server statistics
func (s *SignalingServer) GetStatistics() *SignalingStatistics {
	s.stats.mutex.RLock()
	defer s.stats.mutex.RUnlock()

	stats := *s.stats
	return &stats
}

// Close gracefully shuts down the signaling server
func (s *SignalingServer) Close() error {
	logger.Info("Shutting down WebRTC Signaling Server...")

	// Cancel context to stop background processes
	s.cancel()

	// Close all active rooms
	s.roomsMutex.Lock()
	for _, room := range s.rooms {
		room.Close()
	}
	s.rooms = make(map[primitive.ObjectID]*Room)
	s.roomsByUser = make(map[primitive.ObjectID]*Room)
	s.roomsMutex.Unlock()

	// Wait for background processes to finish
	s.wg.Wait()

	logger.Info("WebRTC Signaling Server shutdown complete")
	return nil
}

// Utility functions

// mapToStruct converts map to struct using JSON marshaling
func mapToStruct(m map[string]interface{}, v interface{}) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// stringPtr returns a pointer to string
func stringPtr(s string) *string {
	return &s
}

// uint16Ptr returns a pointer to uint16
func uint16Ptr(u uint16) *uint16 {
	return &u
}
