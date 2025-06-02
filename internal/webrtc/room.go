package webrtc

import (
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"bro/internal/models"
	"bro/internal/websocket"
	"bro/pkg/logger"
)

// Room represents a WebRTC call room
type Room struct {
	ID     string
	CallID primitive.ObjectID
	ChatID primitive.ObjectID
	Type   models.CallType

	// Room configuration
	Config RoomConfig

	// Participants
	Peers     map[string]*Peer
	MaxPeers  int
	PeerCount int

	// Room state
	State     RoomState
	CreatedAt time.Time
	StartedAt *time.Time
	EndedAt   *time.Time

	// Media management
	ActiveSpeaker string
	MediaStreams  map[string]*MediaStream

	// Recording
	Recording *RoomRecording

	// Statistics
	Stats *RoomStats

	// Communication
	Hub           *websocket.Hub
	SignalingChan chan *SignalingMessage
	EventChan     chan *RoomEvent

	// Quality monitoring
	QualityMonitor *QualityMonitor

	// Synchronization
	mutex   sync.RWMutex
	closed  bool
	cleanup []func()
}

// RoomState represents the state of a room
type RoomState string

const (
	RoomStateWaiting    RoomState = "waiting"
	RoomStateConnecting RoomState = "connecting"
	RoomStateActive     RoomState = "active"
	RoomStateEnding     RoomState = "ending"
	RoomStateEnded      RoomState = "ended"
)

// RoomConfig represents room configuration
type RoomConfig struct {
	MaxParticipants   int
	AllowJoinAnytime  bool
	RequirePermission bool
	EnableRecording   bool
	EnableScreenShare bool
	VideoEnabled      bool
	AudioEnabled      bool
	MaxBitrate        uint32
	AdaptiveQuality   bool
	ICEServers        []webrtc.ICEServer
}

// MediaStream represents a media stream in the room
type MediaStream struct {
	ID        string
	PeerID    string
	UserID    primitive.ObjectID
	Type      StreamType
	Track     *webrtc.TrackLocalStaticRTP
	Enabled   bool
	Quality   StreamQuality
	CreatedAt time.Time
}

// StreamType represents the type of media stream
type StreamType string

const (
	StreamTypeAudio  StreamType = "audio"
	StreamTypeVideo  StreamType = "video"
	StreamTypeScreen StreamType = "screen"
)

// StreamQuality represents stream quality settings
type StreamQuality struct {
	Bitrate    uint32
	FrameRate  uint32
	Resolution models.Resolution
	Codec      string
}

// RoomRecording represents room recording information
type RoomRecording struct {
	IsRecording bool
	StartedAt   *time.Time
	EndedAt     *time.Time
	RecordedBy  primitive.ObjectID
	Streams     []RecordedStream
	OutputPath  string
	FileSize    int64
}

// RecordedStream represents a recorded stream
type RecordedStream struct {
	StreamID  string
	UserID    primitive.ObjectID
	Type      StreamType
	StartTime time.Time
	EndTime   *time.Time
	FilePath  string
}

// RoomStats represents room statistics
type RoomStats struct {
	TotalParticipants     int
	PeakParticipants      int
	TotalDuration         time.Duration
	TotalDataTransferred  uint64
	AverageQuality        float64
	ConnectionAttempts    int
	SuccessfulConnections int
	FailedConnections     int
	ReconnectionAttempts  int
	LastUpdated           time.Time
}

// QualityMonitor monitors call quality
type QualityMonitor struct {
	room            *Room
	monitorInterval time.Duration
	thresholds      QualityThresholds
	alerts          []QualityAlert
	mutex           sync.RWMutex
}

// QualityThresholds defines quality thresholds
type QualityThresholds struct {
	MinBitrate    uint64
	MaxRTT        time.Duration
	MaxPacketLoss float64
	MinFrameRate  float64
	MaxJitter     time.Duration
}

// QualityAlert represents a quality alert
type QualityAlert struct {
	Type      string
	PeerID    string
	Message   string
	Value     interface{}
	Threshold interface{}
	Timestamp time.Time
}

// RoomEvent represents a room event
type RoomEvent struct {
	Type      string
	RoomID    string
	PeerID    string
	Data      map[string]interface{}
	Timestamp time.Time
}

// DefaultRoomConfig returns default room configuration
func DefaultRoomConfig(callType models.CallType) RoomConfig {
	maxParticipants := 2
	if callType == models.CallTypeGroup || callType == models.CallTypeConference {
		maxParticipants = 10
	}

	return RoomConfig{
		MaxParticipants:   maxParticipants,
		AllowJoinAnytime:  true,
		RequirePermission: false,
		EnableRecording:   false,
		EnableScreenShare: true,
		VideoEnabled:      callType == models.CallTypeVideo || callType == models.CallTypeGroup,
		AudioEnabled:      true,
		MaxBitrate:        2000000, // 2 Mbps
		AdaptiveQuality:   true,
	}
}

// NewRoom creates a new WebRTC room
func NewRoom(callID, chatID primitive.ObjectID, callType models.CallType, config RoomConfig, hub *websocket.Hub) *Room {
	roomID := fmt.Sprintf("room_%s_%d", callID.Hex(), time.Now().Unix())

	room := &Room{
		ID:            roomID,
		CallID:        callID,
		ChatID:        chatID,
		Type:          callType,
		Config:        config,
		Peers:         make(map[string]*Peer),
		MaxPeers:      config.MaxParticipants,
		State:         RoomStateWaiting,
		CreatedAt:     time.Now(),
		MediaStreams:  make(map[string]*MediaStream),
		Hub:           hub,
		SignalingChan: make(chan *SignalingMessage, 1000),
		EventChan:     make(chan *RoomEvent, 100),
		Stats:         &RoomStats{},
		cleanup:       []func(){},
	}

	// Initialize quality monitor
	room.QualityMonitor = &QualityMonitor{
		room:            room,
		monitorInterval: 5 * time.Second,
		thresholds: QualityThresholds{
			MinBitrate:    100000, // 100 kbps
			MaxRTT:        500 * time.Millisecond,
			MaxPacketLoss: 5.0, // 5%
			MinFrameRate:  15.0,
			MaxJitter:     50 * time.Millisecond,
		},
		alerts: []QualityAlert{},
	}

	// Start room processing goroutines
	go room.processSignaling()
	go room.processEvents()
	go room.monitorQuality()

	logger.LogWebRTCEvent("room_created", callID.Hex(), "", map[string]interface{}{
		"room_id":          roomID,
		"chat_id":          chatID.Hex(),
		"call_type":        callType,
		"max_participants": config.MaxParticipants,
	})

	return room
}

// AddPeer adds a peer to the room
func (r *Room) AddPeer(peer *Peer) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.closed {
		return fmt.Errorf("room is closed")
	}

	if r.PeerCount >= r.MaxPeers {
		return fmt.Errorf("room is full")
	}

	if _, exists := r.Peers[peer.ID]; exists {
		return fmt.Errorf("peer already exists in room")
	}

	// Set up peer callbacks
	peer.OnConnectionStateChange = r.handlePeerConnectionStateChange
	peer.OnICECandidate = r.handlePeerICECandidate
	peer.OnTrack = r.handlePeerTrack
	peer.OnDataChannelMessage = r.handlePeerDataChannelMessage
	peer.OnStatsUpdate = r.handlePeerStatsUpdate

	// Set cleanup callback
	peer.SetCleanupCallback(func() {
		r.RemovePeer(peer.ID)
	})

	// Add peer to room
	r.Peers[peer.ID] = peer
	r.PeerCount++

	// Update statistics
	r.Stats.TotalParticipants++
	if r.PeerCount > r.Stats.PeakParticipants {
		r.Stats.PeakParticipants = r.PeerCount
	}

	// Start room if this is the first connection
	if r.State == RoomStateWaiting && r.PeerCount > 0 {
		r.State = RoomStateConnecting
	}

	// Emit room event
	r.emitEvent("peer_joined", peer.ID, map[string]interface{}{
		"user_id":    peer.UserID.Hex(),
		"peer_count": r.PeerCount,
	})

	logger.LogWebRTCEvent("peer_joined_room", r.CallID.Hex(), peer.UserID.Hex(), map[string]interface{}{
		"room_id":    r.ID,
		"peer_id":    peer.ID,
		"peer_count": r.PeerCount,
	})

	return nil
}

// RemovePeer removes a peer from the room
func (r *Room) RemovePeer(peerID string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	peer, exists := r.Peers[peerID]
	if !exists {
		return fmt.Errorf("peer not found in room")
	}

	// Remove peer's media streams
	r.removePeerStreams(peerID)

	// Remove peer from room
	delete(r.Peers, peerID)
	r.PeerCount--

	// Stop recording if the recorder left
	if r.Recording != nil && r.Recording.IsRecording && r.Recording.RecordedBy == peer.UserID {
		r.stopRecording()
	}

	// End room if no peers left
	if r.PeerCount == 0 && r.State != RoomStateEnded {
		r.endRoom()
	}

	// Emit room event
	r.emitEvent("peer_left", peerID, map[string]interface{}{
		"user_id":    peer.UserID.Hex(),
		"peer_count": r.PeerCount,
	})

	logger.LogWebRTCEvent("peer_left_room", r.CallID.Hex(), peer.UserID.Hex(), map[string]interface{}{
		"room_id":    r.ID,
		"peer_id":    peerID,
		"peer_count": r.PeerCount,
	})

	return nil
}

// GetPeer returns a peer by ID
func (r *Room) GetPeer(peerID string) (*Peer, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	peer, exists := r.Peers[peerID]
	return peer, exists
}

// GetPeers returns all peers in the room
func (r *Room) GetPeers() map[string]*Peer {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	peers := make(map[string]*Peer)
	for id, peer := range r.Peers {
		peers[id] = peer
	}
	return peers
}

// BroadcastSignalingMessage broadcasts a signaling message to all peers except sender
func (r *Room) BroadcastSignalingMessage(msg *SignalingMessage, excludePeerID string) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for peerID, peer := range r.Peers {
		if peerID != excludePeerID && !peer.IsClosed() {
			select {
			case peer.SignalingChannel <- msg:
			default:
				logger.Warnf("Failed to send signaling message to peer %s: channel full", peerID)
			}
		}
	}
}

// SendSignalingMessage sends a signaling message to a specific peer
func (r *Room) SendSignalingMessage(msg *SignalingMessage, peerID string) error {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	peer, exists := r.Peers[peerID]
	if !exists {
		return fmt.Errorf("peer not found")
	}

	if peer.IsClosed() {
		return fmt.Errorf("peer connection is closed")
	}

	select {
	case peer.SignalingChannel <- msg:
		return nil
	default:
		return fmt.Errorf("peer signaling channel is full")
	}
}

// StartRecording starts room recording
func (r *Room) StartRecording(recordedBy primitive.ObjectID) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if !r.Config.EnableRecording {
		return fmt.Errorf("recording is not enabled for this room")
	}

	if r.Recording != nil && r.Recording.IsRecording {
		return fmt.Errorf("recording is already in progress")
	}

	now := time.Now()
	r.Recording = &RoomRecording{
		IsRecording: true,
		StartedAt:   &now,
		RecordedBy:  recordedBy,
		Streams:     []RecordedStream{},
		OutputPath:  fmt.Sprintf("recordings/call_%s_%d.mp4", r.CallID.Hex(), now.Unix()),
	}

	// Emit recording started event
	r.emitEvent("recording_started", "", map[string]interface{}{
		"recorded_by": recordedBy.Hex(),
		"started_at":  now,
	})

	logger.LogWebRTCEvent("recording_started", r.CallID.Hex(), recordedBy.Hex(), map[string]interface{}{
		"room_id": r.ID,
	})

	return nil
}

// StopRecording stops room recording
func (r *Room) stopRecording() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.Recording == nil || !r.Recording.IsRecording {
		return fmt.Errorf("no recording in progress")
	}

	now := time.Now()
	r.Recording.IsRecording = false
	r.Recording.EndedAt = &now

	// Calculate recording duration and update streams
	if r.Recording.StartedAt != nil {
		now.Sub(*r.Recording.StartedAt)
		for i := range r.Recording.Streams {
			if r.Recording.Streams[i].EndTime == nil {
				r.Recording.Streams[i].EndTime = &now
			}
		}
	}

	// Emit recording stopped event
	r.emitEvent("recording_stopped", "", map[string]interface{}{
		"ended_at":  now,
		"file_path": r.Recording.OutputPath,
	})

	logger.LogWebRTCEvent("recording_stopped", r.CallID.Hex(), r.Recording.RecordedBy.Hex(), map[string]interface{}{
		"room_id":   r.ID,
		"duration":  now.Sub(*r.Recording.StartedAt).Seconds(),
		"file_path": r.Recording.OutputPath,
	})

	return nil
}

// AddMediaStream adds a media stream to the room
func (r *Room) AddMediaStream(stream *MediaStream) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.MediaStreams[stream.ID] = stream

	// Broadcast stream to other peers
	r.broadcastStreamToOtherPeers(stream)

	logger.LogWebRTCEvent("stream_added", r.CallID.Hex(), stream.UserID.Hex(), map[string]interface{}{
		"room_id":     r.ID,
		"stream_id":   stream.ID,
		"stream_type": stream.Type,
	})
}

// RemoveMediaStream removes a media stream from the room
func (r *Room) RemoveMediaStream(streamID string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	stream, exists := r.MediaStreams[streamID]
	if !exists {
		return
	}

	delete(r.MediaStreams, streamID)

	logger.LogWebRTCEvent("stream_removed", r.CallID.Hex(), stream.UserID.Hex(), map[string]interface{}{
		"room_id":     r.ID,
		"stream_id":   streamID,
		"stream_type": stream.Type,
	})
}

// SetActiveSpeaker sets the active speaker
func (r *Room) SetActiveSpeaker(peerID string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.ActiveSpeaker != peerID {
		r.ActiveSpeaker = peerID

		// Emit active speaker changed event
		r.emitEvent("active_speaker_changed", peerID, map[string]interface{}{
			"peer_id": peerID,
		})
	}
}

// EndRoom ends the room
func (r *Room) endRoom() {
	if r.State == RoomStateEnded {
		return
	}

	r.State = RoomStateEnding
	now := time.Now()
	r.EndedAt = &now

	// Stop recording if active
	if r.Recording != nil && r.Recording.IsRecording {
		r.stopRecording()
	}

	// Calculate total duration
	r.Stats.TotalDuration = now.Sub(r.CreatedAt)
	r.Stats.LastUpdated = now

	// Close all peers
	for _, peer := range r.Peers {
		peer.Close()
	}

	r.State = RoomStateEnded

	// Emit room ended event
	r.emitEvent("room_ended", "", map[string]interface{}{
		"ended_at":           now,
		"total_duration":     r.Stats.TotalDuration.Seconds(),
		"total_participants": r.Stats.TotalParticipants,
	})

	logger.LogWebRTCEvent("room_ended", r.CallID.Hex(), "", map[string]interface{}{
		"room_id":           r.ID,
		"duration":          r.Stats.TotalDuration.Seconds(),
		"participants":      r.Stats.TotalParticipants,
		"peak_participants": r.Stats.PeakParticipants,
	})
}

// Close closes the room and cleans up resources
func (r *Room) Close() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.closed {
		return nil
	}

	r.closed = true

	// End room if not already ended
	if r.State != RoomStateEnded {
		r.endRoom()
	}

	// Close channels
	close(r.SignalingChan)
	close(r.EventChan)

	// Run cleanup functions
	for _, cleanup := range r.cleanup {
		cleanup()
	}

	logger.LogWebRTCEvent("room_closed", r.CallID.Hex(), "", map[string]interface{}{
		"room_id": r.ID,
	})

	return nil
}

// GetRoomInfo returns room information
func (r *Room) GetRoomInfo() map[string]interface{} {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	info := map[string]interface{}{
		"room_id":        r.ID,
		"call_id":        r.CallID.Hex(),
		"chat_id":        r.ChatID.Hex(),
		"type":           r.Type,
		"state":          r.State,
		"peer_count":     r.PeerCount,
		"max_peers":      r.MaxPeers,
		"created_at":     r.CreatedAt,
		"started_at":     r.StartedAt,
		"ended_at":       r.EndedAt,
		"active_speaker": r.ActiveSpeaker,
		"recording":      r.Recording,
		"stats":          r.Stats,
		"media_streams":  len(r.MediaStreams),
	}

	// Add peer information
	peers := make([]map[string]interface{}, 0, len(r.Peers))
	for _, peer := range r.Peers {
		peers = append(peers, peer.GetConnectionInfo())
	}
	info["peers"] = peers

	return info
}

// Private methods

// processSignaling processes signaling messages
func (r *Room) processSignaling() {
	for msg := range r.SignalingChan {
		r.handleSignalingMessage(msg)
	}
}

// processEvents processes room events
func (r *Room) processEvents() {
	for event := range r.EventChan {
		r.handleRoomEvent(event)
	}
}

// monitorQuality monitors call quality
func (r *Room) monitorQuality() {
	ticker := time.NewTicker(r.QualityMonitor.monitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if r.closed {
				return
			}
			r.QualityMonitor.checkQuality()
		}
	}
}

// handleSignalingMessage handles signaling messages
func (r *Room) handleSignalingMessage(msg *SignalingMessage) {
	switch msg.Type {
	case "offer":
		r.handleOffer(msg)
	case "answer":
		r.handleAnswer(msg)
	case "ice_candidate":
		r.handleICECandidate(msg)
	case "renegotiation":
		r.handleRenegotiation(msg)
	}
}

// handleOffer handles WebRTC offer
func (r *Room) handleOffer(msg *SignalingMessage) {
	// Forward offer to target peer or broadcast
	if msg.ToPeerID != "" {
		r.SendSignalingMessage(msg, msg.ToPeerID)
	} else {
		r.BroadcastSignalingMessage(msg, msg.FromPeerID)
	}
}

// handleAnswer handles WebRTC answer
func (r *Room) handleAnswer(msg *SignalingMessage) {
	// Forward answer to target peer
	if msg.ToPeerID != "" {
		r.SendSignalingMessage(msg, msg.ToPeerID)
	}
}

// handleICECandidate handles ICE candidate
func (r *Room) handleICECandidate(msg *SignalingMessage) {
	// Forward ICE candidate to target peer or broadcast
	if msg.ToPeerID != "" {
		r.SendSignalingMessage(msg, msg.ToPeerID)
	} else {
		r.BroadcastSignalingMessage(msg, msg.FromPeerID)
	}
}

// handleRenegotiation handles renegotiation
func (r *Room) handleRenegotiation(msg *SignalingMessage) {
	// Handle renegotiation for dynamic track addition/removal
	logger.LogWebRTCEvent("renegotiation_requested", r.CallID.Hex(), "", map[string]interface{}{
		"room_id":   r.ID,
		"from_peer": msg.FromPeerID,
		"to_peer":   msg.ToPeerID,
	})
}

// handleRoomEvent handles room events
func (r *Room) handleRoomEvent(event *RoomEvent) {
	// Process room events and emit to WebSocket clients
	if r.Hub != nil {
		r.Hub.SendToChat(r.ChatID, event.Type, event.Data, nil)
	}
}

// emitEvent emits a room event
func (r *Room) emitEvent(eventType, peerID string, data map[string]interface{}) {
	event := &RoomEvent{
		Type:      eventType,
		RoomID:    r.ID,
		PeerID:    peerID,
		Data:      data,
		Timestamp: time.Now(),
	}

	select {
	case r.EventChan <- event:
	default:
		logger.Warnf("Failed to emit room event: channel full")
	}
}

// Peer event handlers

// handlePeerConnectionStateChange handles peer connection state changes
func (r *Room) handlePeerConnectionStateChange(state webrtc.PeerConnectionState) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Update room state based on peer connections
	connectedPeers := 0
	for _, peer := range r.Peers {
		if peer.IsConnected() {
			connectedPeers++
		}
	}

	// Start room when first peer connects
	if connectedPeers > 0 && r.State == RoomStateConnecting && r.StartedAt == nil {
		now := time.Now()
		r.StartedAt = &now
		r.State = RoomStateActive

		r.emitEvent("room_started", "", map[string]interface{}{
			"started_at": now,
		})
	}

	// Update statistics
	if state == webrtc.PeerConnectionStateConnected {
		r.Stats.SuccessfulConnections++
	} else if state == webrtc.PeerConnectionStateFailed {
		r.Stats.FailedConnections++
	}
}

// handlePeerICECandidate handles peer ICE candidates
func (r *Room) handlePeerICECandidate(candidate *webrtc.ICECandidate) {
	// Store candidate for late joiners or renegotiation
	// This could be stored in Redis for persistence
}

// handlePeerTrack handles peer tracks
func (r *Room) handlePeerTrack(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
	// Handle incoming media tracks
	logger.LogWebRTCEvent("track_received_in_room", r.CallID.Hex(), "", map[string]interface{}{
		"room_id":    r.ID,
		"track_kind": track.Kind().String(),
		"track_id":   track.ID(),
	})
}

// handlePeerDataChannelMessage handles peer data channel messages
func (r *Room) handlePeerDataChannelMessage(msg webrtc.DataChannelMessage) {
	// Handle data channel messages (chat, file transfer, etc.)
	logger.LogWebRTCEvent("data_channel_message_in_room", r.CallID.Hex(), "", map[string]interface{}{
		"room_id":      r.ID,
		"message_size": len(msg.Data),
	})
}

// handlePeerStatsUpdate handles peer statistics updates
func (r *Room) handlePeerStatsUpdate(stats *ConnectionStats) {
	// Aggregate peer statistics for room-level metrics
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.Stats.TotalDataTransferred += stats.BytesSent + stats.BytesReceived
	r.Stats.LastUpdated = time.Now()
}

// removePeerStreams removes all streams belonging to a peer
func (r *Room) removePeerStreams(peerID string) {
	var streamsToRemove []string
	for streamID, stream := range r.MediaStreams {
		if stream.PeerID == peerID {
			streamsToRemove = append(streamsToRemove, streamID)
		}
	}

	for _, streamID := range streamsToRemove {
		delete(r.MediaStreams, streamID)
	}
}

// broadcastStreamToOtherPeers broadcasts a stream to other peers
func (r *Room) broadcastStreamToOtherPeers(stream *MediaStream) {
	for peerID, peer := range r.Peers {
		if peerID != stream.PeerID && !peer.IsClosed() {
			// Add stream track to peer connection
			if stream.Track != nil {
				_, err := peer.AddTrack(stream.Track)
				if err != nil {
					logger.Errorf("Failed to add track to peer %s: %v", peerID, err)
				}
			}
		}
	}
}

// Quality monitoring

// checkQuality checks overall room quality
func (qm *QualityMonitor) checkQuality() {
	qm.mutex.Lock()
	defer qm.mutex.Unlock()

	room := qm.room
	room.mutex.RLock()
	defer room.mutex.RUnlock()

	// Check quality for each peer
	for peerID, peer := range room.Peers {
		stats := peer.GetStats()
		qm.checkPeerQuality(peerID, stats)
	}

	// Calculate overall room quality
	qm.calculateRoomQuality()
}

// checkPeerQuality checks quality for a specific peer
func (qm *QualityMonitor) checkPeerQuality(peerID string, stats *ConnectionStats) {
	// Check bitrate
	if stats.Bitrate < qm.thresholds.MinBitrate {
		alert := QualityAlert{
			Type:      "low_bitrate",
			PeerID:    peerID,
			Message:   "Bitrate below threshold",
			Value:     stats.Bitrate,
			Threshold: qm.thresholds.MinBitrate,
			Timestamp: time.Now(),
		}
		qm.alerts = append(qm.alerts, alert)
	}

	// Check RTT
	if stats.RTT > qm.thresholds.MaxRTT {
		alert := QualityAlert{
			Type:      "high_rtt",
			PeerID:    peerID,
			Message:   "RTT above threshold",
			Value:     stats.RTT,
			Threshold: qm.thresholds.MaxRTT,
			Timestamp: time.Now(),
		}
		qm.alerts = append(qm.alerts, alert)
	}

	// Check packet loss
	if stats.PacketsReceived > 0 {
		packetLoss := float64(stats.PacketsLost) / float64(stats.PacketsReceived) * 100
		if packetLoss > qm.thresholds.MaxPacketLoss {
			alert := QualityAlert{
				Type:      "high_packet_loss",
				PeerID:    peerID,
				Message:   "Packet loss above threshold",
				Value:     packetLoss,
				Threshold: qm.thresholds.MaxPacketLoss,
				Timestamp: time.Now(),
			}
			qm.alerts = append(qm.alerts, alert)
		}
	}

	// Cleanup old alerts (keep only last 100)
	if len(qm.alerts) > 100 {
		qm.alerts = qm.alerts[len(qm.alerts)-100:]
	}
}

// calculateRoomQuality calculates overall room quality
func (qm *QualityMonitor) calculateRoomQuality() {
	room := qm.room

	totalQuality := 0.0
	peerCount := 0

	for _, peer := range room.Peers {
		if peer.IsConnected() {
			stats := peer.GetStats()

			// Calculate peer quality score (0-100)
			quality := 100.0

			// Reduce quality based on issues
			if stats.Bitrate < qm.thresholds.MinBitrate {
				quality -= 20
			}
			if stats.RTT > qm.thresholds.MaxRTT {
				quality -= 15
			}
			if stats.Jitter > qm.thresholds.MaxJitter {
				quality -= 10
			}

			totalQuality += quality
			peerCount++
		}
	}

	if peerCount > 0 {
		room.Stats.AverageQuality = totalQuality / float64(peerCount)
	}
}
