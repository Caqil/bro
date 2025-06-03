package webrtc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"bro/internal/models"
	"bro/internal/websocket"
	"bro/pkg/database"
	"bro/pkg/logger"
	"bro/pkg/redis"
)

// Room represents a WebRTC call room/session
type Room struct {
	// Room identification
	ID     string             `json:"id"`
	CallID primitive.ObjectID `json:"call_id"`
	ChatID primitive.ObjectID `json:"chat_id"`

	// Room metadata
	Type        models.CallType    `json:"type"`
	Status      models.CallStatus  `json:"status"`
	InitiatorID primitive.ObjectID `json:"initiator_id"`

	// Participants management
	Peers           map[string]*PeerConnection             `json:"-"`
	PeersByUserID   map[primitive.ObjectID]*PeerConnection `json:"-"`
	MaxParticipants int                                    `json:"max_participants"`

	// Room settings and features
	Settings models.CallSettings `json:"settings"`
	Features models.CallFeatures `json:"features"`

	// Recording
	Recording   *models.RecordingInfo `json:"recording,omitempty"`
	IsRecording bool                  `json:"is_recording"`

	// Quality and analytics
	QualitySettings models.CallQualitySettings `json:"quality_settings"`
	Analytics       models.CallAnalytics       `json:"analytics"`

	// Technical configuration
	WebRTCConfig models.WebRTCServiceConfig `json:"webrtc_config"`
	TURNServers  []models.TURNServerConfig  `json:"turn_servers"`

	// Timestamps
	CreatedAt time.Time  `json:"created_at"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`

	// Communication
	Hub         *websocket.Hub `json:"-"`
	RedisClient *redis.Client  `json:"-"`

	// Database
	CallsCollection *mongo.Collection `json:"-"`

	// Room state
	mutex       sync.RWMutex
	closed      bool
	subscribers map[string]chan RoomEvent

	// Background tasks
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Statistics
	stats *RoomStatistics
}

// RoomStatistics contains room-specific statistics
type RoomStatistics struct {
	TotalPeers        int       `json:"total_peers"`
	ConnectedPeers    int       `json:"connected_peers"`
	PeakParticipants  int       `json:"peak_participants"`
	TotalDuration     int64     `json:"total_duration_seconds"`
	AverageQuality    float64   `json:"average_quality"`
	TotalBytes        uint64    `json:"total_bytes"`
	MessagesExchanged int64     `json:"messages_exchanged"`
	LastUpdated       time.Time `json:"last_updated"`
	mutex             sync.RWMutex
}

// RoomEvent represents events that happen in the room
type RoomEvent struct {
	Type      string                 `json:"type"`
	PeerID    string                 `json:"peer_id,omitempty"`
	UserID    primitive.ObjectID     `json:"user_id,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// RoomConfig contains room configuration
type RoomConfig struct {
	MaxParticipants   int
	WebRTCConfig      models.WebRTCServiceConfig
	DefaultSettings   models.CallSettings
	DefaultFeatures   models.CallFeatures
	QualitySettings   models.CallQualitySettings
	EnableRecording   bool
	EnableAnalytics   bool
	HeartbeatInterval time.Duration
	StatsInterval     time.Duration
}

// NewRoom creates a new WebRTC room
func NewRoom(callID primitive.ObjectID, chatID primitive.ObjectID, callType models.CallType, initiatorID primitive.ObjectID, hub *websocket.Hub, config *RoomConfig) (*Room, error) {
	if config == nil {
		config = DefaultRoomConfig()
	}

	collections := database.GetCollections()
	if collections == nil {
		return nil, fmt.Errorf("database collections not available")
	}

	ctx, cancel := context.WithCancel(context.Background())

	room := &Room{
		ID:              primitive.NewObjectID().Hex(),
		CallID:          callID,
		ChatID:          chatID,
		Type:            callType,
		Status:          models.CallStatusInitiating,
		InitiatorID:     initiatorID,
		Peers:           make(map[string]*PeerConnection),
		PeersByUserID:   make(map[primitive.ObjectID]*PeerConnection),
		MaxParticipants: config.MaxParticipants,
		Settings:        config.DefaultSettings,
		Features:        config.DefaultFeatures,
		QualitySettings: config.QualitySettings,
		WebRTCConfig:    config.WebRTCConfig,
		CreatedAt:       time.Now(),
		Hub:             hub,
		RedisClient:     redis.GetClient(),
		CallsCollection: collections.Calls,
		subscribers:     make(map[string]chan RoomEvent),
		ctx:             ctx,
		cancel:          cancel,
		stats:           &RoomStatistics{LastUpdated: time.Now()},
	}

	// Initialize analytics
	room.Analytics = models.CallAnalytics{
		ConnectionAttempts:      0,
		SuccessfulConnections:   0,
		FailedConnections:       0,
		AverageQuality:          0,
		QualityDistribution:     make(map[string]int),
		PeakParticipants:        0,
		TotalParticipantMinutes: 0,
		FeaturesUsed:            []string{},
		ScreenShareDuration:     0,
		RecordingDuration:       0,
		ParticipantLocations:    []string{},
	}

	// Load TURN servers from WebRTC config
	room.TURNServers = config.WebRTCConfig.TURNServers

	// Start background processes
	room.wg.Add(3)
	go room.heartbeatManager()
	go room.statsCollector()
	go room.qualityMonitor()

	logger.LogUserAction(initiatorID.Hex(), "room_created", "webrtc", map[string]interface{}{
		"room_id":   room.ID,
		"call_id":   callID.Hex(),
		"call_type": callType,
	})

	return room, nil
}

// DefaultRoomConfig returns default room configuration
func DefaultRoomConfig() *RoomConfig {
	return &RoomConfig{
		MaxParticipants: 10,
		WebRTCConfig: models.WebRTCServiceConfig{
			ICEConnectionTimeout: 30 * time.Second,
			DTLSTimeout:          10 * time.Second,
			EnableIPv6:           true,
			MaxBitrate:           2000000,
			MinBitrate:           100000,
			AdaptiveBitrate:      true,
			EnableRecording:      true,
			RecordingFormat:      "mp4",
		},
		DefaultSettings: models.CallSettings{
			MaxParticipants:    10,
			RequirePermission:  false,
			AutoAccept:         false,
			RecordingEnabled:   false,
			MaxVideoBitrate:    2000000,
			MaxAudioBitrate:    128000,
			AdaptiveQuality:    true,
			EndToEndEncryption: true,
			RequirePassword:    false,
		},
		DefaultFeatures: models.CallFeatures{
			VideoCall:         true,
			ScreenShare:       true,
			Recording:         true,
			ChatDuringCall:    true,
			FileSharing:       true,
			BackgroundEffects: true,
			NoiseReduction:    true,
			EchoCancellation:  true,
		},
		QualitySettings: models.CallQualitySettings{
			VideoMinBitrate:  100000,
			VideoMaxBitrate:  2000000,
			AudioMinBitrate:  32000,
			AudioMaxBitrate:  128000,
			MinFrameRate:     15,
			MaxFrameRate:     30,
			AdaptiveQuality:  true,
			NoiseReduction:   true,
			EchoCancellation: true,
		},
		EnableRecording:   true,
		EnableAnalytics:   true,
		HeartbeatInterval: 30 * time.Second,
		StatsInterval:     10 * time.Second,
	}
}

// JoinRoom adds a peer to the room
func (r *Room) JoinRoom(userID primitive.ObjectID, wsClient *websocket.Client) (*PeerConnection, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.closed {
		return nil, fmt.Errorf("room is closed")
	}

	// Check if user is already in the room
	if _, exists := r.PeersByUserID[userID]; exists {
		return nil, fmt.Errorf("user already in room")
	}

	// Check room capacity
	if len(r.Peers) >= r.MaxParticipants {
		return nil, fmt.Errorf("room is full")
	}

	// Create WebRTC configuration
	iceServers := make([]webrtc.ICEServer, 0)

	// Add STUN servers
	for _, stunServer := range r.WebRTCConfig.STUNServers {
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs: []string{stunServer.URL},
		})
	}

	// Add TURN servers
	for _, turnServer := range r.TURNServers {
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs:       turnServer.URLs,
			Username:   turnServer.Username,
			Credential: turnServer.Credential,
		})
	}

	peerConfig := &PeerConfig{
		ICEServers:           iceServers,
		ICETransportPolicy:   webrtc.ICETransportPolicyAll,
		BundlePolicy:         webrtc.BundlePolicyMaxBundle,
		RTCPMuxPolicy:        webrtc.RTCPMuxPolicyRequire,
		ICECandidatePoolSize: 0,
		SDPSemantics:         webrtc.SDPSemanticsUnifiedPlan,
	}

	// Create peer connection
	peer, err := NewPeerConnection(userID, r.CallID, r.ID, wsClient, peerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %w", err)
	}

	// Set room reference
	peer.Room = r

	// Add peer to room
	r.Peers[peer.ID] = peer
	r.PeersByUserID[userID] = peer

	// Update statistics
	r.updatePeerStats(1)
	r.Analytics.ConnectionAttempts++

	// Join WebSocket chat for this call
	if r.Hub != nil {
		r.Hub.JoinChat(wsClient, r.ChatID)
	}

	// Notify other peers about new participant
	r.broadcastToRoom("peer_joined", map[string]interface{}{
		"peer_id":   peer.ID,
		"user_id":   userID.Hex(),
		"peer_info": peer.GetInfo(),
	}, peer.ID)

	// Start call if this is the first connection after initiator
	if len(r.Peers) == 2 && r.Status == models.CallStatusRinging {
		r.startCall()
	}

	// Emit room event
	r.emitEvent(RoomEvent{
		Type:      "peer_joined",
		PeerID:    peer.ID,
		UserID:    userID,
		Data:      map[string]interface{}{"peer_info": peer.GetInfo()},
		Timestamp: time.Now(),
	})

	logger.LogUserAction(userID.Hex(), "joined_room", "webrtc", map[string]interface{}{
		"room_id":      r.ID,
		"call_id":      r.CallID.Hex(),
		"peer_id":      peer.ID,
		"participants": len(r.Peers),
	})

	return peer, nil
}

// CanUserJoin checks if user can join the room
func (r *Room) CanUserJoin(userID primitive.ObjectID) bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Check if room is closed
	if r.closed {
		return false
	}

	// Check if room is full
	if len(r.Peers) >= r.MaxParticipants {
		return false
	}

	// Check if user is already in room
	if _, exists := r.PeersByUserID[userID]; exists {
		return false
	}

	// TODO: Add additional permission checks here
	// - Check if user is participant of the call
	// - Check if call requires permission and user has it

	return true
}

// LeaveRoom removes a peer from the room
func (r *Room) LeaveRoom(userID primitive.ObjectID) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	peer, exists := r.PeersByUserID[userID]
	if !exists {
		return fmt.Errorf("user not in room")
	}

	// Remove peer from room
	delete(r.Peers, peer.ID)
	delete(r.PeersByUserID, userID)

	// Close peer connection
	peer.Close()

	// Update statistics
	r.updatePeerStats(-1)

	// Leave WebSocket chat
	if r.Hub != nil && peer.WSClient != nil {
		r.Hub.LeaveChat(peer.WSClient, r.ChatID)
	}

	// Notify other peers about participant leaving
	r.broadcastToRoom("peer_left", map[string]interface{}{
		"peer_id": peer.ID,
		"user_id": userID.Hex(),
		"reason":  "user_left",
	}, "")

	// End call if no peers left or only initiator remains
	if len(r.Peers) == 0 || (len(r.Peers) == 1 && r.Type == models.CallTypeConference) {
		r.endCall(primitive.NilObjectID, models.EndReasonNormal)
	}

	// Emit room event
	r.emitEvent(RoomEvent{
		Type:      "peer_left",
		PeerID:    peer.ID,
		UserID:    userID,
		Data:      map[string]interface{}{"reason": "user_left"},
		Timestamp: time.Now(),
	})

	logger.LogUserAction(userID.Hex(), "left_room", "webrtc", map[string]interface{}{
		"room_id":      r.ID,
		"call_id":      r.CallID.Hex(),
		"peer_id":      peer.ID,
		"participants": len(r.Peers),
	})

	return nil
}

// GetPeer returns a peer by user ID
func (r *Room) GetPeer(userID primitive.ObjectID) (*PeerConnection, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	peer, exists := r.PeersByUserID[userID]
	return peer, exists
}

// GetPeerByID returns a peer by peer ID
func (r *Room) GetPeerByID(peerID string) (*PeerConnection, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	peer, exists := r.Peers[peerID]
	return peer, exists
}

// GetAllPeers returns all peers in the room
func (r *Room) GetAllPeers() []*PeerConnection {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	peers := make([]*PeerConnection, 0, len(r.Peers))
	for _, peer := range r.Peers {
		peers = append(peers, peer)
	}
	return peers
}

// GetConnectedPeers returns only connected peers
func (r *Room) GetConnectedPeers() []*PeerConnection {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	peers := make([]*PeerConnection, 0)
	for _, peer := range r.Peers {
		if peer.State == webrtc.PeerConnectionStateConnected {
			peers = append(peers, peer)
		}
	}
	return peers
}

// Call state management

// startCall starts the call
func (r *Room) startCall() {
	if r.StartedAt != nil {
		return
	}

	now := time.Now()
	r.StartedAt = &now
	r.Status = models.CallStatusActive

	// Update call in database
	r.updateCallInDatabase()

	// Notify all peers
	r.broadcastToRoom("call_started", map[string]interface{}{
		"started_at": now,
	}, "")

	// Emit room event
	r.emitEvent(RoomEvent{
		Type:      "call_started",
		Data:      map[string]interface{}{"started_at": now},
		Timestamp: time.Now(),
	})

	logger.LogUserAction(r.InitiatorID.Hex(), "call_started", "webrtc", map[string]interface{}{
		"room_id":      r.ID,
		"call_id":      r.CallID.Hex(),
		"participants": len(r.Peers),
	})
}

// endCall ends the call
func (r *Room) endCall(endedBy primitive.ObjectID, reason models.EndReason) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.EndedAt != nil {
		return
	}

	now := time.Now()
	r.EndedAt = &now
	r.Status = models.CallStatusEnded

	// Calculate duration
	duration := int64(0)
	if r.StartedAt != nil {
		duration = int64(now.Sub(*r.StartedAt).Seconds())
	} else {
		duration = int64(now.Sub(r.CreatedAt).Seconds())
	}

	// Stop recording if active
	if r.IsRecording {
		r.stopRecording()
	}

	// Update call in database
	r.updateCallInDatabase()

	// Notify all peers
	r.broadcastToRoom("call_ended", map[string]interface{}{
		"ended_at": now,
		"ended_by": endedBy.Hex(),
		"reason":   reason,
		"duration": duration,
	}, "")

	// Close all peer connections
	for _, peer := range r.Peers {
		peer.Close()
	}

	// Emit room event
	r.emitEvent(RoomEvent{
		Type:   "call_ended",
		UserID: endedBy,
		Data: map[string]interface{}{
			"ended_at": now,
			"reason":   reason,
			"duration": duration,
		},
		Timestamp: time.Now(),
	})

	logger.LogUserAction(endedBy.Hex(), "call_ended", "webrtc", map[string]interface{}{
		"room_id":  r.ID,
		"call_id":  r.CallID.Hex(),
		"reason":   reason,
		"duration": duration,
	})

	// Schedule room cleanup
	go func() {
		time.Sleep(30 * time.Second)
		r.Close()
	}()
}

// EndCall ends the call (public method)
func (r *Room) EndCall(userID primitive.ObjectID, reason models.EndReason) error {
	if r.closed {
		return fmt.Errorf("room is already closed")
	}

	// Check if user has permission to end call
	if userID != r.InitiatorID {
		// Check if user is admin in group calls
		if r.Type == models.CallTypeGroup || r.Type == models.CallTypeConference {
			// TODO: Check group admin permissions from database
		} else {
			return fmt.Errorf("insufficient permissions to end call")
		}
	}

	r.endCall(userID, reason)
	return nil
}

// Recording management

// StartRecording starts call recording
func (r *Room) StartRecording(userID primitive.ObjectID) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if !r.Features.Recording {
		return fmt.Errorf("recording not enabled for this call")
	}

	if r.IsRecording {
		return fmt.Errorf("recording already in progress")
	}

	now := time.Now()
	r.Recording = &models.RecordingInfo{
		IsRecording: true,
		StartedAt:   &now,
		Quality:     models.VideoQualityHigh,
		Format:      r.WebRTCConfig.RecordingFormat,
		IsPublic:    false,
		RecordedBy:  userID,
	}
	r.IsRecording = true

	// Notify all peers
	r.broadcastToRoom("recording_started", map[string]interface{}{
		"started_by": userID.Hex(),
		"started_at": now,
	}, "")

	// Update analytics
	r.Analytics.FeaturesUsed = append(r.Analytics.FeaturesUsed, "recording")

	logger.LogUserAction(userID.Hex(), "recording_started", "webrtc", map[string]interface{}{
		"room_id": r.ID,
		"call_id": r.CallID.Hex(),
	})

	return nil
}

// StopRecording stops call recording
func (r *Room) StopRecording() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.stopRecording()
}

// stopRecording internal method to stop recording
func (r *Room) stopRecording() error {
	if !r.IsRecording {
		return fmt.Errorf("no recording in progress")
	}

	now := time.Now()
	r.Recording.IsRecording = false
	r.Recording.EndedAt = &now
	r.IsRecording = false

	if r.Recording.StartedAt != nil {
		r.Recording.Duration = int64(now.Sub(*r.Recording.StartedAt).Seconds())
		r.Analytics.RecordingDuration += r.Recording.Duration
	}

	// Notify all peers
	r.broadcastToRoom("recording_stopped", map[string]interface{}{
		"stopped_at": now,
		"duration":   r.Recording.Duration,
	}, "")

	logger.Infof("Recording stopped for room %s, duration: %d seconds", r.ID, r.Recording.Duration)

	return nil
}

// Peer event handlers

// OnPeerConnected handles peer connection events
func (r *Room) OnPeerConnected(peer *PeerConnection) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.Analytics.SuccessfulConnections++

	// Update peak participants
	connectedCount := len(r.GetConnectedPeers())
	if connectedCount > r.Analytics.PeakParticipants {
		r.Analytics.PeakParticipants = connectedCount
	}

	// Notify other peers
	r.broadcastToRoom("peer_connected", map[string]interface{}{
		"peer_id": peer.ID,
		"user_id": peer.UserID.Hex(),
	}, peer.ID)
}

// OnPeerDisconnected handles peer disconnection events
func (r *Room) OnPeerDisconnected(peer *PeerConnection) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Notify other peers
	r.broadcastToRoom("peer_disconnected", map[string]interface{}{
		"peer_id": peer.ID,
		"user_id": peer.UserID.Hex(),
	}, peer.ID)
}

// OnPeerStateChange handles peer state changes
func (r *Room) OnPeerStateChange(peer *PeerConnection, changeType string, data interface{}) {
	// Forward state changes to other peers
	r.broadcastToRoom("peer_state_change", map[string]interface{}{
		"peer_id":     peer.ID,
		"user_id":     peer.UserID.Hex(),
		"change_type": changeType,
		"data":        data,
	}, peer.ID)

	// Handle specific state changes
	switch changeType {
	case "media_state":
		r.handleMediaStateChange(peer, data)
	case "connection_state":
		r.handleConnectionStateChange(peer, data)
	case "stats_update":
		r.handleStatsUpdate(peer, data)
	}
}

// handleMediaStateChange handles media state changes
func (r *Room) handleMediaStateChange(peer *PeerConnection, data interface{}) {
	mediaState, ok := data.(map[string]interface{})["media_state"].(models.MediaState)
	if !ok {
		return
	}

	// Update analytics based on media state
	if mediaState.ScreenSharing {
		if !contains(r.Analytics.FeaturesUsed, "screen_share") {
			r.Analytics.FeaturesUsed = append(r.Analytics.FeaturesUsed, "screen_share")
		}
	}

	if mediaState.VideoEnabled {
		if !contains(r.Analytics.FeaturesUsed, "video") {
			r.Analytics.FeaturesUsed = append(r.Analytics.FeaturesUsed, "video")
		}
	}
}

// handleConnectionStateChange handles connection state changes
func (r *Room) handleConnectionStateChange(peer *PeerConnection, data interface{}) {
	state, ok := data.(map[string]interface{})["state"].(string)
	if !ok {
		return
	}

	if state == "failed" || state == "disconnected" {
		r.Analytics.FailedConnections++
	}
}

// handleStatsUpdate handles statistics updates
func (r *Room) handleStatsUpdate(peer *PeerConnection, data interface{}) {
	stats, ok := data.(map[string]interface{})["stats"].(*PeerStats)
	if !ok {
		return
	}

	// Update room-level statistics
	r.stats.mutex.Lock()
	r.stats.TotalBytes += stats.BytesSent + stats.BytesReceived
	r.stats.LastUpdated = time.Now()
	r.stats.mutex.Unlock()
}

// Data channel message handling

// HandleDataChannelMessage handles data channel messages from peers
func (r *Room) HandleDataChannelMessage(peer *PeerConnection, messageType string, data map[string]interface{}) {
	switch messageType {
	case "chat_message":
		r.handleChatMessage(peer, data)
	case "file_transfer":
		r.handleFileTransfer(peer, data)
	case "typing_indicator":
		r.handleTypingIndicator(peer, data)
	}

	r.stats.mutex.Lock()
	r.stats.MessagesExchanged++
	r.stats.mutex.Unlock()
}

// handleChatMessage handles chat messages through data channel
func (r *Room) handleChatMessage(peer *PeerConnection, data map[string]interface{}) {
	// Broadcast chat message to all other peers
	r.broadcastToRoom("chat_message", map[string]interface{}{
		"from":      peer.UserID.Hex(),
		"message":   data["message"],
		"timestamp": time.Now().Unix(),
	}, peer.ID)

	if !contains(r.Analytics.FeaturesUsed, "chat_during_call") {
		r.Analytics.FeaturesUsed = append(r.Analytics.FeaturesUsed, "chat_during_call")
	}
}

// handleFileTransfer handles file transfer through data channel
func (r *Room) handleFileTransfer(peer *PeerConnection, data map[string]interface{}) {
	// Forward file transfer to specified recipient or all peers
	r.broadcastToRoom("file_transfer", map[string]interface{}{
		"from": peer.UserID.Hex(),
		"data": data,
	}, peer.ID)

	if !contains(r.Analytics.FeaturesUsed, "file_sharing") {
		r.Analytics.FeaturesUsed = append(r.Analytics.FeaturesUsed, "file_sharing")
	}
}

// handleTypingIndicator handles typing indicators
func (r *Room) handleTypingIndicator(peer *PeerConnection, data map[string]interface{}) {
	r.broadcastToRoom("typing_indicator", map[string]interface{}{
		"from":   peer.UserID.Hex(),
		"typing": data["typing"],
	}, peer.ID)
}

// Communication helpers

// broadcastToRoom broadcasts a message to all peers in the room except sender
func (r *Room) broadcastToRoom(messageType string, data interface{}, exceptPeerID string) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for peerID, peer := range r.Peers {
		if peerID != exceptPeerID && peer.WSClient != nil {
			peer.WSClient.SendJSON("room_message", map[string]interface{}{
				"room_id": r.ID,
				"type":    messageType,
				"data":    data,
			})
		}
	}

	// Also broadcast through WebSocket hub to chat participants
	if r.Hub != nil {
		r.Hub.SendToChat(r.ChatID, "room_message", map[string]interface{}{
			"room_id": r.ID,
			"type":    messageType,
			"data":    data,
		})
	}
}

// Database operations

// updateCallInDatabase updates the call document in database
func (r *Room) updateCallInDatabase() {
	if r.CallsCollection == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"status":     r.Status,
			"started_at": r.StartedAt,
			"ended_at":   r.EndedAt,
			"recording":  r.Recording,
			"analytics":  r.Analytics,
			"updated_at": time.Now(),
		},
	}

	if _, err := r.CallsCollection.UpdateOne(ctx, bson.M{"_id": r.CallID}, update); err != nil {
		logger.Errorf("Failed to update call in database: %v", err)
	}
}

// Background processes

// heartbeatManager sends periodic heartbeats to peers
func (r *Room) heartbeatManager() {
	defer r.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if r.closed {
				return
			}
			r.sendHeartbeats()
		case <-r.ctx.Done():
			return
		}
	}
}

// sendHeartbeats sends heartbeat to all peers
func (r *Room) sendHeartbeats() {
	r.mutex.RLock()
	peers := make([]*PeerConnection, 0, len(r.Peers))
	for _, peer := range r.Peers {
		peers = append(peers, peer)
	}
	r.mutex.RUnlock()

	for _, peer := range peers {
		if peer.WSClient != nil {
			peer.WSClient.SendJSON("room_heartbeat", map[string]interface{}{
				"room_id":   r.ID,
				"timestamp": time.Now().Unix(),
			})
		}
	}
}

// statsCollector collects room statistics
func (r *Room) statsCollector() {
	defer r.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if r.closed {
				return
			}
			r.collectStatistics()
		case <-r.ctx.Done():
			return
		}
	}
}

// collectStatistics collects and updates room statistics
func (r *Room) collectStatistics() {
	r.mutex.RLock()
	totalPeers := len(r.Peers)
	connectedPeers := len(r.GetConnectedPeers())
	r.mutex.RUnlock()

	r.stats.mutex.Lock()
	r.stats.TotalPeers = totalPeers
	r.stats.ConnectedPeers = connectedPeers
	if r.StartedAt != nil {
		r.stats.TotalDuration = int64(time.Since(*r.StartedAt).Seconds())
	}
	r.stats.LastUpdated = time.Now()
	r.stats.mutex.Unlock()

	// Calculate average quality
	var totalQuality float64
	qualityCount := 0

	for _, peer := range r.GetConnectedPeers() {
		if peer.QualityMetrics.QualityScore > 0 {
			totalQuality += peer.QualityMetrics.QualityScore
			qualityCount++
		}
	}

	if qualityCount > 0 {
		r.Analytics.AverageQuality = totalQuality / float64(qualityCount)
	}
}

// qualityMonitor monitors call quality
func (r *Room) qualityMonitor() {
	defer r.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if r.closed {
				return
			}
			r.monitorQuality()
		case <-r.ctx.Done():
			return
		}
	}
}

// monitorQuality monitors and adjusts call quality
func (r *Room) monitorQuality() {
	peers := r.GetConnectedPeers()

	for _, peer := range peers {
		stats, err := peer.GetStats()
		if err != nil {
			continue
		}

		// Check for quality issues
		if stats.PacketsLost > 0 && stats.PacketsReceived > 0 {
			lossRate := float64(stats.PacketsLost) / float64(stats.PacketsReceived)
			if lossRate > 0.05 { // 5% packet loss
				r.handleQualityIssue(peer, "high_packet_loss", lossRate)
			}
		}

		if stats.RTT > 300 { // High RTT
			r.handleQualityIssue(peer, "high_rtt", float64(stats.RTT))
		}

		if stats.Jitter > 50 { // High jitter
			r.handleQualityIssue(peer, "high_jitter", stats.Jitter)
		}
	}
}

// handleQualityIssue handles quality issues
func (r *Room) handleQualityIssue(peer *PeerConnection, issueType string, value float64) {
	logger.Warnf("Quality issue detected for peer %s: %s = %f", peer.ID, issueType, value)

	// Notify peer about quality issue
	peer.sendSignalingMessage("quality_issue", map[string]interface{}{
		"issue_type": issueType,
		"value":      value,
		"suggestion": r.getQualityImprovementSuggestion(issueType),
	})

	// Record quality issue
	issue := models.QualityIssue{
		Type:        issueType,
		Severity:    r.getIssueSeverity(issueType, value),
		Timestamp:   time.Now(),
		Description: fmt.Sprintf("%s detected with value %f", issueType, value),
	}

	r.mutex.Lock()
	// Add to analytics (you can store quality issues here)
	if r.Analytics.QualityDistribution == nil {
		r.Analytics.QualityDistribution = make(map[string]int)
	}
	r.Analytics.QualityDistribution[issue.Severity]++

	// You could also store the issue in a slice if you add a QualityIssues field to Room
	// r.QualityIssues = append(r.QualityIssues, issue)
	r.mutex.Unlock()
}

// getQualityImprovementSuggestion returns suggestion for quality improvement
func (r *Room) getQualityImprovementSuggestion(issueType string) string {
	switch issueType {
	case "high_packet_loss":
		return "Check your network connection and consider reducing video quality"
	case "high_rtt":
		return "You may be far from the server, consider using a VPN or check your internet connection"
	case "high_jitter":
		return "Network instability detected, consider switching to a more stable connection"
	default:
		return "Check your network connection"
	}
}

// getIssueSeverity returns severity based on issue type and value
func (r *Room) getIssueSeverity(issueType string, value float64) string {
	switch issueType {
	case "high_packet_loss":
		if value > 0.1 {
			return "high"
		} else if value > 0.05 {
			return "medium"
		}
		return "low"
	case "high_rtt":
		if value > 500 {
			return "high"
		} else if value > 300 {
			return "medium"
		}
		return "low"
	case "high_jitter":
		if value > 100 {
			return "high"
		} else if value > 50 {
			return "medium"
		}
		return "low"
	}
	return "low"
}

// Event management

// Subscribe subscribes to room events
func (r *Room) Subscribe(subscriberID string) <-chan RoomEvent {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	ch := make(chan RoomEvent, 100)
	r.subscribers[subscriberID] = ch
	return ch
}

// Unsubscribe unsubscribes from room events
func (r *Room) Unsubscribe(subscriberID string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if ch, exists := r.subscribers[subscriberID]; exists {
		close(ch)
		delete(r.subscribers, subscriberID)
	}
}

// emitEvent emits an event to all subscribers
func (r *Room) emitEvent(event RoomEvent) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for _, ch := range r.subscribers {
		select {
		case ch <- event:
		default:
			// Channel is full, skip
		}
	}
}

// Utility methods

// updatePeerStats updates peer statistics
func (r *Room) updatePeerStats(delta int) {
	r.stats.mutex.Lock()
	r.stats.TotalPeers += delta
	if r.stats.TotalPeers > r.stats.PeakParticipants {
		r.stats.PeakParticipants = r.stats.TotalPeers
	}
	r.stats.mutex.Unlock()
}

// GetStatistics returns room statistics
func (r *Room) GetStatistics() *RoomStatistics {
	r.stats.mutex.RLock()
	defer r.stats.mutex.RUnlock()

	stats := *r.stats
	return &stats
}

// GetInfo returns room information
func (r *Room) GetInfo() map[string]interface{} {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return map[string]interface{}{
		"id":               r.ID,
		"call_id":          r.CallID.Hex(),
		"chat_id":          r.ChatID.Hex(),
		"type":             r.Type,
		"status":           r.Status,
		"initiator_id":     r.InitiatorID.Hex(),
		"max_participants": r.MaxParticipants,
		"current_peers":    len(r.Peers),
		"connected_peers":  len(r.GetConnectedPeers()),
		"created_at":       r.CreatedAt,
		"started_at":       r.StartedAt,
		"ended_at":         r.EndedAt,
		"is_recording":     r.IsRecording,
		"features":         r.Features,
		"analytics":        r.Analytics,
	}
}

// IsActive checks if the room is active
func (r *Room) IsActive() bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return !r.closed && (r.Status == models.CallStatusActive || r.Status == models.CallStatusRinging)
}

// IsClosed checks if the room is closed
func (r *Room) IsClosed() bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.closed
}

// Close gracefully closes the room
func (r *Room) Close() error {
	r.mutex.Lock()
	if r.closed {
		r.mutex.Unlock()
		return nil
	}
	r.closed = true
	r.mutex.Unlock()

	// Cancel context to stop background processes
	r.cancel()

	// Close all peer connections
	for _, peer := range r.Peers {
		peer.Close()
	}

	// Close all subscriber channels
	for _, ch := range r.subscribers {
		close(ch)
	}

	// Wait for background processes to finish
	r.wg.Wait()

	// Final database update
	r.updateCallInDatabase()

	logger.LogUserAction(r.InitiatorID.Hex(), "room_closed", "webrtc", map[string]interface{}{
		"room_id":  r.ID,
		"call_id":  r.CallID.Hex(),
		"duration": time.Since(r.CreatedAt).Seconds(),
	})

	return nil
}

// Helper functions

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
