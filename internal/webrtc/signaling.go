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

	"bro/internal/config"
	"bro/internal/models"
	"bro/internal/websocket"
	"bro/pkg/database"
	"bro/pkg/logger"
	"bro/pkg/redis"
)

// SignalingServer manages WebRTC signaling
type SignalingServer struct {
	// Configuration
	config *config.Config

	// Room management
	rooms      map[string]*Room
	roomsMutex sync.RWMutex

	// Peer management
	peers      map[string]*Peer
	peersMutex sync.RWMutex

	// Call management
	activeCalls map[primitive.ObjectID]*CallSession
	callsMutex  sync.RWMutex

	// Communication
	hub           *websocket.Hub
	redisClient   *redis.Client
	signalingChan chan *SignalingMessage

	// TURN/STUN servers
	iceServers []webrtc.ICEServer

	// Statistics
	stats *SignalingStats

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// CallSession represents an active call session
type CallSession struct {
	CallID      primitive.ObjectID
	ChatID      primitive.ObjectID
	RoomID      string
	InitiatorID primitive.ObjectID
	Type        models.CallType
	Status      models.CallStatus

	// Participants
	Participants map[primitive.ObjectID]*CallParticipant

	// Timing
	CreatedAt time.Time
	StartedAt *time.Time
	EndedAt   *time.Time

	// Configuration
	Config CallSessionConfig

	// Media state
	MediaState map[primitive.ObjectID]models.MediaState

	// Quality monitoring
	QualityStats map[primitive.ObjectID]*ConnectionStats
}

// CallParticipant represents a participant in a call
type CallParticipant struct {
	UserID       primitive.ObjectID
	PeerID       string
	ConnectionID string
	Status       models.ParticipantStatus
	JoinedAt     *time.Time
	LeftAt       *time.Time
	DeviceInfo   models.DeviceInfo
	MediaState   models.MediaState
}

// CallSessionConfig represents call session configuration
type CallSessionConfig struct {
	MaxParticipants    int
	VideoEnabled       bool
	AudioEnabled       bool
	RecordingEnabled   bool
	ScreenShareEnabled bool
	Quality            models.CallQualitySettings
}

// SignalingStats represents signaling server statistics
type SignalingStats struct {
	ActiveCalls         int
	ActiveRooms         int
	TotalPeers          int
	ConnectedPeers      int
	TotalCallsToday     int
	AverageCallDuration time.Duration
	LastUpdated         time.Time

	// Quality metrics
	AverageConnectionTime time.Duration
	FailureRate           float64
	ReconnectionRate      float64
}

// SignalingRequest represents a signaling request
type SignalingRequest struct {
	Type       string                 `json:"type"`
	CallID     string                 `json:"call_id"`
	FromUserID string                 `json:"from_user_id"`
	ToUserID   string                 `json:"to_user_id,omitempty"`
	Data       map[string]interface{} `json:"data"`
	Timestamp  time.Time              `json:"timestamp"`
	RequestID  string                 `json:"request_id"`
}

// SignalingResponse represents a signaling response
type SignalingResponse struct {
	Type      string                 `json:"type"`
	Success   bool                   `json:"success"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Error     string                 `json:"error,omitempty"`
	RequestID string                 `json:"request_id"`
	Timestamp time.Time              `json:"timestamp"`
}

// NewSignalingServer creates a new signaling server
func NewSignalingServer(cfg *config.Config, hub *websocket.Hub) (*SignalingServer, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Setup ICE servers
	iceServers := []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	}

	// Add TURN servers if configured
	if cfg.COTURNConfig.Host != "" {
		turnServer := webrtc.ICEServer{
			URLs:       []string{fmt.Sprintf("turn:%s:%d", cfg.COTURNConfig.Host, cfg.COTURNConfig.Port)},
			Username:   cfg.COTURNConfig.Username,
			Credential: cfg.COTURNConfig.Password,
		}
		iceServers = append(iceServers, turnServer)
	}

	server := &SignalingServer{
		config:        cfg,
		rooms:         make(map[string]*Room),
		peers:         make(map[string]*Peer),
		activeCalls:   make(map[primitive.ObjectID]*CallSession),
		hub:           hub,
		redisClient:   redis.GetClient(),
		signalingChan: make(chan *SignalingMessage, 1000),
		iceServers:    iceServers,
		stats:         &SignalingStats{},
		ctx:           ctx,
		cancel:        cancel,
	}

	// Start processing goroutines
	server.wg.Add(3)
	go server.processSignaling()
	go server.monitorStats()
	go server.cleanupRoutine()

	logger.Info("WebRTC Signaling Server initialized")
	return server, nil
}

// InitiateCall initiates a new call
func (s *SignalingServer) InitiateCall(req *SignalingRequest) (*SignalingResponse, error) {
	callIDStr := req.CallID
	callID, err := primitive.ObjectIDFromHex(callIDStr)
	if err != nil {
		return s.errorResponse(req.RequestID, "invalid_call_id", "Invalid call ID format")
	}

	fromUserID, err := primitive.ObjectIDFromHex(req.FromUserID)
	if err != nil {
		return s.errorResponse(req.RequestID, "invalid_user_id", "Invalid user ID format")
	}

	// Get call from database
	call, err := s.getCallFromDB(callID)
	if err != nil {
		return s.errorResponse(req.RequestID, "call_not_found", "Call not found")
	}

	// Verify user is authorized to join this call
	if !s.canUserJoinCall(fromUserID, call) {
		return s.errorResponse(req.RequestID, "unauthorized", "User not authorized to join this call")
	}

	// Create or get existing call session
	session, err := s.getOrCreateCallSession(call)
	if err != nil {
		return s.errorResponse(req.RequestID, "session_error", fmt.Sprintf("Failed to create call session: %v", err))
	}

	// Create room if it doesn't exist
	room, err := s.getOrCreateRoom(session)
	if err != nil {
		return s.errorResponse(req.RequestID, "room_error", fmt.Sprintf("Failed to create room: %v", err))
	}

	// Generate connection ID and peer configuration
	connectionID := fmt.Sprintf("conn_%s_%d", fromUserID.Hex(), time.Now().Unix())
	peerConfig := DefaultPeerConfig(s.iceServers)

	// Create peer
	peer, err := NewPeer(fromUserID, connectionID, room.ID, callID, peerConfig)
	if err != nil {
		return s.errorResponse(req.RequestID, "peer_error", fmt.Sprintf("Failed to create peer: %v", err))
	}

	// Add peer to room
	if err := room.AddPeer(peer); err != nil {
		peer.Close()
		return s.errorResponse(req.RequestID, "room_full", fmt.Sprintf("Failed to add peer to room: %v", err))
	}

	// Store peer
	s.peersMutex.Lock()
	s.peers[peer.ID] = peer
	s.peersMutex.Unlock()

	// Add participant to call session
	participant := &CallParticipant{
		UserID:       fromUserID,
		PeerID:       peer.ID,
		ConnectionID: connectionID,
		Status:       models.ParticipantStatusConnecting,
		DeviceInfo:   models.DeviceInfo{}, // This would be populated from request
		MediaState:   models.MediaState{},
	}

	s.callsMutex.Lock()
	session.Participants[fromUserID] = participant
	s.callsMutex.Unlock()

	// Update call status if this is the first participant
	if len(session.Participants) == 1 {
		session.Status = models.CallStatusRinging
		s.updateCallInDB(call.ID, models.CallStatusRinging)
	}

	// Prepare response data
	responseData := map[string]interface{}{
		"call_id":       callID.Hex(),
		"room_id":       room.ID,
		"peer_id":       peer.ID,
		"connection_id": connectionID,
		"ice_servers":   s.iceServers,
		"config": map[string]interface{}{
			"video_enabled":     session.Config.VideoEnabled,
			"audio_enabled":     session.Config.AudioEnabled,
			"recording_enabled": session.Config.RecordingEnabled,
			"max_participants":  session.Config.MaxParticipants,
		},
	}

	// Notify other participants about new participant
	s.notifyParticipants(session, "participant_joined", map[string]interface{}{
		"user_id":           fromUserID.Hex(),
		"peer_id":           peer.ID,
		"participant_count": len(session.Participants),
	}, fromUserID)

	logger.LogWebRTCEvent("call_initiated", callID.Hex(), fromUserID.Hex(), map[string]interface{}{
		"room_id":           room.ID,
		"peer_id":           peer.ID,
		"participant_count": len(session.Participants),
	})

	return s.successResponse(req.RequestID, responseData), nil
}

// JoinCall allows a user to join an existing call
func (s *SignalingServer) JoinCall(req *SignalingRequest) (*SignalingResponse, error) {
	callIDStr := req.CallID
	callID, err := primitive.ObjectIDFromHex(callIDStr)
	if err != nil {
		return s.errorResponse(req.RequestID, "invalid_call_id", "Invalid call ID format")
	}

	fromUserID, err := primitive.ObjectIDFromHex(req.FromUserID)
	if err != nil {
		return s.errorResponse(req.RequestID, "invalid_user_id", "Invalid user ID format")
	}

	// Get existing call session
	s.callsMutex.RLock()
	session, exists := s.activeCalls[callID]
	s.callsMutex.RUnlock()

	if !exists {
		return s.errorResponse(req.RequestID, "call_not_found", "Call session not found")
	}

	// Check if user can join
	if len(session.Participants) >= session.Config.MaxParticipants {
		return s.errorResponse(req.RequestID, "call_full", "Call has reached maximum participants")
	}

	// Check if user is already in the call
	if _, exists := session.Participants[fromUserID]; exists {
		return s.errorResponse(req.RequestID, "already_joined", "User already in call")
	}

	// Get room
	s.roomsMutex.RLock()
	room, exists := s.rooms[session.RoomID]
	s.roomsMutex.RUnlock()

	if !exists {
		return s.errorResponse(req.RequestID, "room_not_found", "Room not found")
	}

	// Create peer and add to room (similar to InitiateCall)
	connectionID := fmt.Sprintf("conn_%s_%d", fromUserID.Hex(), time.Now().Unix())
	peerConfig := DefaultPeerConfig(s.iceServers)

	peer, err := NewPeer(fromUserID, connectionID, room.ID, callID, peerConfig)
	if err != nil {
		return s.errorResponse(req.RequestID, "peer_error", fmt.Sprintf("Failed to create peer: %v", err))
	}

	if err := room.AddPeer(peer); err != nil {
		peer.Close()
		return s.errorResponse(req.RequestID, "join_failed", fmt.Sprintf("Failed to join room: %v", err))
	}

	// Store peer
	s.peersMutex.Lock()
	s.peers[peer.ID] = peer
	s.peersMutex.Unlock()

	// Add participant to session
	participant := &CallParticipant{
		UserID:       fromUserID,
		PeerID:       peer.ID,
		ConnectionID: connectionID,
		Status:       models.ParticipantStatusConnecting,
		DeviceInfo:   models.DeviceInfo{},
		MediaState:   models.MediaState{},
	}

	s.callsMutex.Lock()
	session.Participants[fromUserID] = participant
	s.callsMutex.Unlock()

	// Prepare response
	responseData := map[string]interface{}{
		"call_id":       callID.Hex(),
		"room_id":       room.ID,
		"peer_id":       peer.ID,
		"connection_id": connectionID,
		"ice_servers":   s.iceServers,
		"participants":  s.getParticipantList(session),
	}

	// Notify other participants
	s.notifyParticipants(session, "participant_joined", map[string]interface{}{
		"user_id":           fromUserID.Hex(),
		"peer_id":           peer.ID,
		"participant_count": len(session.Participants),
	}, fromUserID)

	logger.LogWebRTCEvent("call_joined", callID.Hex(), fromUserID.Hex(), map[string]interface{}{
		"room_id":           room.ID,
		"peer_id":           peer.ID,
		"participant_count": len(session.Participants),
	})

	return s.successResponse(req.RequestID, responseData), nil
}

// HandleOffer handles WebRTC offer
func (s *SignalingServer) HandleOffer(req *SignalingRequest) (*SignalingResponse, error) {
	peerID, ok := req.Data["peer_id"].(string)
	if !ok {
		return s.errorResponse(req.RequestID, "missing_peer_id", "Peer ID is required")
	}

	offer, ok := req.Data["offer"].(map[string]interface{})
	if !ok {
		return s.errorResponse(req.RequestID, "missing_offer", "SDP offer is required")
	}

	// Get peer
	s.peersMutex.RLock()
	peer, exists := s.peers[peerID]
	s.peersMutex.RUnlock()

	if !exists {
		return s.errorResponse(req.RequestID, "peer_not_found", "Peer not found")
	}

	// Create signaling message
	signalingMsg := &SignalingMessage{
		Type:       "offer",
		FromPeerID: peerID,
		ToPeerID:   req.Data["to_peer_id"].(string),
		RoomID:     peer.RoomID,
		CallID:     peer.CallID.Hex(),
		Data:       req.Data,
		Timestamp:  time.Now(),
		MessageID:  primitive.NewObjectID().Hex(),
	}

	// Send to signaling channel
	select {
	case s.signalingChan <- signalingMsg:
	default:
		return s.errorResponse(req.RequestID, "signaling_busy", "Signaling server is busy")
	}

	logger.LogWebRTCEvent("offer_processed", peer.CallID.Hex(), peer.UserID.Hex(), map[string]interface{}{
		"peer_id": peerID,
		"to_peer": signalingMsg.ToPeerID,
		"room_id": peer.RoomID,
	})

	return s.successResponse(req.RequestID, map[string]interface{}{
		"message": "Offer processed",
	}), nil
}

// HandleAnswer handles WebRTC answer
func (s *SignalingServer) HandleAnswer(req *SignalingRequest) (*SignalingResponse, error) {
	peerID, ok := req.Data["peer_id"].(string)
	if !ok {
		return s.errorResponse(req.RequestID, "missing_peer_id", "Peer ID is required")
	}

	answer, ok := req.Data["answer"].(map[string]interface{})
	if !ok {
		return s.errorResponse(req.RequestID, "missing_answer", "SDP answer is required")
	}

	// Get peer
	s.peersMutex.RLock()
	peer, exists := s.peers[peerID]
	s.peersMutex.RUnlock()

	if !exists {
		return s.errorResponse(req.RequestID, "peer_not_found", "Peer not found")
	}

	// Create signaling message
	signalingMsg := &SignalingMessage{
		Type:       "answer",
		FromPeerID: peerID,
		ToPeerID:   req.Data["to_peer_id"].(string),
		RoomID:     peer.RoomID,
		CallID:     peer.CallID.Hex(),
		Data:       req.Data,
		Timestamp:  time.Now(),
		MessageID:  primitive.NewObjectID().Hex(),
	}

	// Send to signaling channel
	select {
	case s.signalingChan <- signalingMsg:
	default:
		return s.errorResponse(req.RequestID, "signaling_busy", "Signaling server is busy")
	}

	logger.LogWebRTCEvent("answer_processed", peer.CallID.Hex(), peer.UserID.Hex(), map[string]interface{}{
		"peer_id": peerID,
		"to_peer": signalingMsg.ToPeerID,
		"room_id": peer.RoomID,
	})

	return s.successResponse(req.RequestID, map[string]interface{}{
		"message": "Answer processed",
	}), nil
}

// HandleICECandidate handles ICE candidates
func (s *SignalingServer) HandleICECandidate(req *SignalingRequest) (*SignalingResponse, error) {
	peerID, ok := req.Data["peer_id"].(string)
	if !ok {
		return s.errorResponse(req.RequestID, "missing_peer_id", "Peer ID is required")
	}

	candidate, ok := req.Data["candidate"].(map[string]interface{})
	if !ok {
		return s.errorResponse(req.RequestID, "missing_candidate", "ICE candidate is required")
	}

	// Get peer
	s.peersMutex.RLock()
	peer, exists := s.peers[peerID]
	s.peersMutex.RUnlock()

	if !exists {
		return s.errorResponse(req.RequestID, "peer_not_found", "Peer not found")
	}

	// Create signaling message
	signalingMsg := &SignalingMessage{
		Type:       "ice_candidate",
		FromPeerID: peerID,
		ToPeerID:   req.Data["to_peer_id"].(string),
		RoomID:     peer.RoomID,
		CallID:     peer.CallID.Hex(),
		Data:       req.Data,
		Timestamp:  time.Now(),
		MessageID:  primitive.NewObjectID().Hex(),
	}

	// Send to signaling channel
	select {
	case s.signalingChan <- signalingMsg:
	default:
		return s.errorResponse(req.RequestID, "signaling_busy", "Signaling server is busy")
	}

	return s.successResponse(req.RequestID, map[string]interface{}{
		"message": "ICE candidate processed",
	}), nil
}

// LeaveCall handles user leaving a call
func (s *SignalingServer) LeaveCall(req *SignalingRequest) (*SignalingResponse, error) {
	callIDStr := req.CallID
	callID, err := primitive.ObjectIDFromHex(callIDStr)
	if err != nil {
		return s.errorResponse(req.RequestID, "invalid_call_id", "Invalid call ID format")
	}

	fromUserID, err := primitive.ObjectIDFromHex(req.FromUserID)
	if err != nil {
		return s.errorResponse(req.RequestID, "invalid_user_id", "Invalid user ID format")
	}

	// Get call session
	s.callsMutex.Lock()
	session, exists := s.activeCalls[callID]
	if !exists {
		s.callsMutex.Unlock()
		return s.errorResponse(req.RequestID, "call_not_found", "Call session not found")
	}

	// Get participant
	participant, exists := session.Participants[fromUserID]
	if !exists {
		s.callsMutex.Unlock()
		return s.errorResponse(req.RequestID, "not_in_call", "User not in call")
	}

	// Remove participant
	delete(session.Participants, fromUserID)
	now := time.Now()
	participant.LeftAt = &now
	participant.Status = models.ParticipantStatusDisconnected
	s.callsMutex.Unlock()

	// Remove and close peer
	s.peersMutex.Lock()
	if peer, exists := s.peers[participant.PeerID]; exists {
		delete(s.peers, participant.PeerID)
		peer.Close()
	}
	s.peersMutex.Unlock()

	// Update call status and end call if no participants left
	if len(session.Participants) == 0 {
		session.Status = models.CallStatusEnded
		session.EndedAt = &now
		s.updateCallInDB(callID, models.CallStatusEnded)

		// Remove call session
		s.callsMutex.Lock()
		delete(s.activeCalls, callID)
		s.callsMutex.Unlock()

		// Close room
		s.roomsMutex.Lock()
		if room, exists := s.rooms[session.RoomID]; exists {
			delete(s.rooms, session.RoomID)
			room.Close()
		}
		s.roomsMutex.Unlock()
	} else {
		// Notify remaining participants
		s.notifyParticipants(session, "participant_left", map[string]interface{}{
			"user_id":           fromUserID.Hex(),
			"participant_count": len(session.Participants),
		}, fromUserID)
	}

	logger.LogWebRTCEvent("call_left", callID.Hex(), fromUserID.Hex(), map[string]interface{}{
		"peer_id":                participant.PeerID,
		"remaining_participants": len(session.Participants),
	})

	return s.successResponse(req.RequestID, map[string]interface{}{
		"message": "Left call successfully",
	}), nil
}

// GetCallStatus returns the current status of a call
func (s *SignalingServer) GetCallStatus(req *SignalingRequest) (*SignalingResponse, error) {
	callIDStr := req.CallID
	callID, err := primitive.ObjectIDFromHex(callIDStr)
	if err != nil {
		return s.errorResponse(req.RequestID, "invalid_call_id", "Invalid call ID format")
	}

	// Get call session
	s.callsMutex.RLock()
	session, exists := s.activeCalls[callID]
	s.callsMutex.RUnlock()

	if !exists {
		return s.errorResponse(req.RequestID, "call_not_found", "Call session not found")
	}

	// Prepare status data
	statusData := map[string]interface{}{
		"call_id":           callID.Hex(),
		"status":            session.Status,
		"participant_count": len(session.Participants),
		"participants":      s.getParticipantList(session),
		"created_at":        session.CreatedAt,
		"started_at":        session.StartedAt,
		"config":            session.Config,
	}

	return s.successResponse(req.RequestID, statusData), nil
}

// Close shuts down the signaling server
func (s *SignalingServer) Close() error {
	logger.Info("Shutting down WebRTC Signaling Server...")

	// Cancel context and wait for goroutines
	s.cancel()
	s.wg.Wait()

	// Close all rooms
	s.roomsMutex.Lock()
	for _, room := range s.rooms {
		room.Close()
	}
	s.rooms = make(map[string]*Room)
	s.roomsMutex.Unlock()

	// Close all peers
	s.peersMutex.Lock()
	for _, peer := range s.peers {
		peer.Close()
	}
	s.peers = make(map[string]*Peer)
	s.peersMutex.Unlock()

	// Clear active calls
	s.callsMutex.Lock()
	s.activeCalls = make(map[primitive.ObjectID]*CallSession)
	s.callsMutex.Unlock()

	// Close signaling channel
	close(s.signalingChan)

	logger.Info("WebRTC Signaling Server shut down complete")
	return nil
}

// Private helper methods

// getOrCreateCallSession gets or creates a call session
func (s *SignalingServer) getOrCreateCallSession(call *models.Call) (*CallSession, error) {
	s.callsMutex.Lock()
	defer s.callsMutex.Unlock()

	// Check if session already exists
	if session, exists := s.activeCalls[call.ID]; exists {
		return session, nil
	}

	// Create new session
	session := &CallSession{
		CallID:       call.ID,
		ChatID:       call.ChatID,
		RoomID:       fmt.Sprintf("room_%s_%d", call.ID.Hex(), time.Now().Unix()),
		InitiatorID:  call.InitiatorID,
		Type:         call.Type,
		Status:       call.Status,
		Participants: make(map[primitive.ObjectID]*CallParticipant),
		CreatedAt:    time.Now(),
		Config: CallSessionConfig{
			MaxParticipants:    call.Settings.MaxParticipants,
			VideoEnabled:       call.Type == models.CallTypeVideo || call.Type == models.CallTypeGroup,
			AudioEnabled:       true,
			RecordingEnabled:   call.Settings.RecordingEnabled,
			ScreenShareEnabled: true,
			Quality:            call.Settings.QualitySettings,
		},
		MediaState:   make(map[primitive.ObjectID]models.MediaState),
		QualityStats: make(map[primitive.ObjectID]*ConnectionStats),
	}

	s.activeCalls[call.ID] = session
	return session, nil
}

// getOrCreateRoom gets or creates a room for a call session
func (s *SignalingServer) getOrCreateRoom(session *CallSession) (*Room, error) {
	s.roomsMutex.Lock()
	defer s.roomsMutex.Unlock()

	// Check if room already exists
	if room, exists := s.rooms[session.RoomID]; exists {
		return room, nil
	}

	// Create room config
	roomConfig := DefaultRoomConfig(session.Type)
	roomConfig.MaxParticipants = session.Config.MaxParticipants
	roomConfig.VideoEnabled = session.Config.VideoEnabled
	roomConfig.AudioEnabled = session.Config.AudioEnabled
	roomConfig.EnableRecording = session.Config.RecordingEnabled
	roomConfig.ICEServers = s.iceServers

	// Create new room
	room := NewRoom(session.CallID, session.ChatID, session.Type, roomConfig, s.hub)
	s.rooms[session.RoomID] = room

	return room, nil
}

// processSignaling processes signaling messages
func (s *SignalingServer) processSignaling() {
	defer s.wg.Done()

	for {
		select {
		case msg := <-s.signalingChan:
			s.routeSignalingMessage(msg)
		case <-s.ctx.Done():
			return
		}
	}
}

// routeSignalingMessage routes signaling messages to appropriate rooms
func (s *SignalingServer) routeSignalingMessage(msg *SignalingMessage) {
	s.roomsMutex.RLock()
	room, exists := s.rooms[msg.RoomID]
	s.roomsMutex.RUnlock()

	if !exists {
		logger.Warnf("Room %s not found for signaling message", msg.RoomID)
		return
	}

	// Route message to room
	select {
	case room.SignalingChan <- msg:
	default:
		logger.Warnf("Failed to route signaling message to room %s: channel full", msg.RoomID)
	}
}

// monitorStats monitors and updates statistics
func (s *SignalingServer) monitorStats() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.updateStats()
		case <-s.ctx.Done():
			return
		}
	}
}

// updateStats updates server statistics
func (s *SignalingServer) updateStats() {
	s.callsMutex.RLock()
	activeCalls := len(s.activeCalls)
	s.callsMutex.RUnlock()

	s.roomsMutex.RLock()
	activeRooms := len(s.rooms)
	s.roomsMutex.RUnlock()

	s.peersMutex.RLock()
	totalPeers := len(s.peers)
	connectedPeers := 0
	for _, peer := range s.peers {
		if peer.IsConnected() {
			connectedPeers++
		}
	}
	s.peersMutex.RUnlock()

	// Update stats
	s.stats.ActiveCalls = activeCalls
	s.stats.ActiveRooms = activeRooms
	s.stats.TotalPeers = totalPeers
	s.stats.ConnectedPeers = connectedPeers
	s.stats.LastUpdated = time.Now()

	// Store stats in Redis if available
	if s.redisClient != nil {
		statsData, _ := json.Marshal(s.stats)
		s.redisClient.Set("webrtc:stats", string(statsData), 5*time.Minute)
	}
}

// cleanupRoutine performs periodic cleanup
func (s *SignalingServer) cleanupRoutine() {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.performCleanup()
		case <-s.ctx.Done():
			return
		}
	}
}

// performCleanup performs cleanup of stale connections and sessions
func (s *SignalingServer) performCleanup() {
	now := time.Now()
	staleThreshold := 5 * time.Minute

	// Cleanup stale peers
	s.peersMutex.Lock()
	var stalePeers []string
	for peerID, peer := range s.peers {
		if peer.IsClosed() || now.Sub(peer.CreatedAt) > staleThreshold {
			stalePeers = append(stalePeers, peerID)
		}
	}
	for _, peerID := range stalePeers {
		if peer, exists := s.peers[peerID]; exists {
			peer.Close()
			delete(s.peers, peerID)
		}
	}
	s.peersMutex.Unlock()

	// Cleanup stale call sessions
	s.callsMutex.Lock()
	var staleCalls []primitive.ObjectID
	for callID, session := range s.activeCalls {
		if session.Status == models.CallStatusEnded || now.Sub(session.CreatedAt) > 24*time.Hour {
			staleCalls = append(staleCalls, callID)
		}
	}
	for _, callID := range staleCalls {
		delete(s.activeCalls, callID)
	}
	s.callsMutex.Unlock()

	// Cleanup stale rooms
	s.roomsMutex.Lock()
	var staleRooms []string
	for roomID, room := range s.rooms {
		if room.State == RoomStateEnded || now.Sub(room.CreatedAt) > 24*time.Hour {
			staleRooms = append(staleRooms, roomID)
		}
	}
	for _, roomID := range staleRooms {
		if room, exists := s.rooms[roomID]; exists {
			room.Close()
			delete(s.rooms, roomID)
		}
	}
	s.roomsMutex.Unlock()

	if len(stalePeers) > 0 || len(staleCalls) > 0 || len(staleRooms) > 0 {
		logger.Debugf("Cleanup completed: %d peers, %d calls, %d rooms",
			len(stalePeers), len(staleCalls), len(staleRooms))
	}
}

// Helper methods for database operations

// getCallFromDB retrieves call from database
func (s *SignalingServer) getCallFromDB(callID primitive.ObjectID) (*models.Call, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	collections := database.GetCollections()
	if collections == nil {
		return nil, fmt.Errorf("database not available")
	}

	var call models.Call
	err := collections.Calls.FindOne(ctx, bson.M{"_id": callID}).Decode(&call)
	if err != nil {
		return nil, fmt.Errorf("call not found: %w", err)
	}

	return &call, nil
}

// updateCallInDB updates call status in database
func (s *SignalingServer) updateCallInDB(callID primitive.ObjectID, status models.CallStatus) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	collections := database.GetCollections()
	if collections == nil {
		return fmt.Errorf("database not available")
	}

	update := bson.M{
		"$set": bson.M{
			"status":     status,
			"updated_at": time.Now(),
		},
	}

	_, err := collections.Calls.UpdateOne(ctx, bson.M{"_id": callID}, update)
	return err
}

// canUserJoinCall checks if a user can join a call
func (s *SignalingServer) canUserJoinCall(userID primitive.ObjectID, call *models.Call) bool {
	// Check if user is a participant
	for _, participant := range call.Participants {
		if participant.UserID == userID {
			return true
		}
	}
	return false
}

// notifyParticipants notifies all participants except excluded user
func (s *SignalingServer) notifyParticipants(session *CallSession, eventType string, data map[string]interface{}, excludeUserID primitive.ObjectID) {
	for userID := range session.Participants {
		if userID != excludeUserID {
			if s.hub != nil {
				s.hub.SendToUser(userID, eventType, data)
			}
		}
	}
}

// getParticipantList returns list of participants
func (s *SignalingServer) getParticipantList(session *CallSession) []map[string]interface{} {
	participants := make([]map[string]interface{}, 0, len(session.Participants))
	for userID, participant := range session.Participants {
		participants = append(participants, map[string]interface{}{
			"user_id":       userID.Hex(),
			"peer_id":       participant.PeerID,
			"connection_id": participant.ConnectionID,
			"status":        participant.Status,
			"joined_at":     participant.JoinedAt,
			"media_state":   participant.MediaState,
		})
	}
	return participants
}

// Response helper methods

// successResponse creates a success response
func (s *SignalingServer) successResponse(requestID string, data map[string]interface{}) *SignalingResponse {
	return &SignalingResponse{
		Type:      "success",
		Success:   true,
		Data:      data,
		RequestID: requestID,
		Timestamp: time.Now(),
	}
}

// errorResponse creates an error response
func (s *SignalingServer) errorResponse(requestID, errorCode, errorMessage string) (*SignalingResponse, error) {
	return &SignalingResponse{
		Type:      "error",
		Success:   false,
		Error:     fmt.Sprintf("%s: %s", errorCode, errorMessage),
		RequestID: requestID,
		Timestamp: time.Now(),
	}, fmt.Errorf("%s: %s", errorCode, errorMessage)
}

// GetStats returns current signaling server statistics
func (s *SignalingServer) GetStats() *SignalingStats {
	statsCopy := *s.stats
	return &statsCopy
}
