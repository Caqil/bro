package webrtc

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"bro/internal/models"
	"bro/internal/websocket"
	"bro/pkg/logger"
)

// PeerConnection represents a WebRTC peer connection
type PeerConnection struct {
	// Identifiers
	ID       string             `json:"id"`
	UserID   primitive.ObjectID `json:"user_id"`
	CallID   primitive.ObjectID `json:"call_id"`
	RoomID   string             `json:"room_id"`
	DeviceID string             `json:"device_id"`
	Platform string             `json:"platform"`

	// WebRTC connection
	PeerConnection *webrtc.PeerConnection `json:"-"`

	// Connection state
	State          webrtc.PeerConnectionState `json:"state"`
	ICEState       webrtc.ICEConnectionState  `json:"ice_state"`
	SignalingState webrtc.SignalingState      `json:"signaling_state"`

	// Media tracks
	LocalTracks  map[string]*webrtc.TrackLocalStaticRTP `json:"-"`
	RemoteTracks map[string]*webrtc.TrackRemote         `json:"-"`

	// Media state
	MediaState models.MediaState `json:"media_state"`

	// Quality metrics
	QualityMetrics models.QualityMetrics `json:"quality_metrics"`

	// Technical details
	TechDetails models.TechnicalDetails `json:"tech_details"`

	// Connection info
	ConnectionInfo models.ConnectionInfo `json:"connection_info"`

	// Timestamps
	CreatedAt   time.Time  `json:"created_at"`
	ConnectedAt *time.Time `json:"connected_at,omitempty"`

	// Communication
	WSClient *websocket.Client `json:"-"`
	Room     *Room             `json:"-"`

	// Statistics
	Stats *PeerStats `json:"stats"`

	// Concurrency control
	mutex  sync.RWMutex
	closed bool

	// Channels for coordination
	done         chan struct{}
	statsReady   chan *webrtc.StatsReport
	reconnecting bool
}

// PeerStats contains peer connection statistics
type PeerStats struct {
	BytesSent          uint64    `json:"bytes_sent"`
	BytesReceived      uint64    `json:"bytes_received"`
	PacketsSent        uint64    `json:"packets_sent"`
	PacketsReceived    uint64    `json:"packets_received"`
	PacketsLost        uint64    `json:"packets_lost"`
	AudioBitrate       uint32    `json:"audio_bitrate"`
	VideoBitrate       uint32    `json:"video_bitrate"`
	Jitter             float64   `json:"jitter"`
	RTT                uint32    `json:"rtt"`
	AvailableBandwidth uint64    `json:"available_bandwidth"`
	LastStatsUpdate    time.Time `json:"last_stats_update"`
}

// PeerConfig contains peer connection configuration
type PeerConfig struct {
	ICEServers           []webrtc.ICEServer
	ICETransportPolicy   webrtc.ICETransportPolicy
	BundlePolicy         webrtc.BundlePolicy
	RTCPMuxPolicy        webrtc.RTCPMuxPolicy
	PeerIdentity         string
	Certificates         []webrtc.Certificate
	ICECandidatePoolSize uint8
	SDPSemantics         webrtc.SDPSemantics
}

// NewPeerConnection creates a new WebRTC peer connection
func NewPeerConnection(userID primitive.ObjectID, callID primitive.ObjectID, roomID string, wsClient *websocket.Client, config *PeerConfig) (*PeerConnection, error) {
	// Create WebRTC configuration
	webrtcConfig := webrtc.Configuration{
		ICEServers:           config.ICEServers,
		ICETransportPolicy:   config.ICETransportPolicy,
		BundlePolicy:         config.BundlePolicy,
		RTCPMuxPolicy:        config.RTCPMuxPolicy,
		PeerIdentity:         config.PeerIdentity,
		Certificates:         config.Certificates,
		ICECandidatePoolSize: config.ICECandidatePoolSize,
		SDPSemantics:         config.SDPSemantics,
	}

	// Create WebRTC peer connection
	peerConn, err := webrtc.NewPeerConnection(webrtcConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %w", err)
	}

	peer := &PeerConnection{
		ID:             primitive.NewObjectID().Hex(),
		UserID:         userID,
		CallID:         callID,
		RoomID:         roomID,
		PeerConnection: peerConn,
		State:          webrtc.PeerConnectionStateNew,
		ICEState:       webrtc.ICEConnectionStateNew,
		SignalingState: webrtc.SignalingStateStable,
		LocalTracks:    make(map[string]*webrtc.TrackLocalStaticRTP),
		RemoteTracks:   make(map[string]*webrtc.TrackRemote),
		MediaState: models.MediaState{
			VideoEnabled:      false,
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
		CreatedAt:  time.Now(),
		WSClient:   wsClient,
		Stats:      &PeerStats{},
		done:       make(chan struct{}),
		statsReady: make(chan *webrtc.StatsReport, 1),
	}

	// Get device and platform info from websocket client
	if wsClient != nil {
		clientInfo := wsClient.GetInfo()
		peer.DeviceID = clientInfo.DeviceID
		peer.Platform = clientInfo.Platform
	}

	// Set up WebRTC event handlers
	if err := peer.setupEventHandlers(); err != nil {
		peerConn.Close()
		return nil, fmt.Errorf("failed to setup event handlers: %w", err)
	}

	logger.LogUserAction(userID.Hex(), "peer_created", "webrtc", map[string]interface{}{
		"peer_id": peer.ID,
		"call_id": callID.Hex(),
		"room_id": roomID,
	})

	return peer, nil
}

// setupEventHandlers sets up WebRTC event handlers
func (p *PeerConnection) setupEventHandlers() error {
	// Connection state change
	p.PeerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		p.mutex.Lock()
		p.State = state
		p.mutex.Unlock()

		switch state {
		case webrtc.PeerConnectionStateConnected:
			now := time.Now()
			p.ConnectedAt = &now
			p.onConnected()
		case webrtc.PeerConnectionStateDisconnected:
			p.onDisconnected()
		case webrtc.PeerConnectionStateFailed:
			p.onFailed()
		case webrtc.PeerConnectionStateClosed:
			p.onClosed()
		}

		p.notifyStateChange("connection_state", map[string]interface{}{
			"state": state.String(),
		})
	})

	// ICE connection state change
	p.PeerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		p.mutex.Lock()
		p.ICEState = state
		p.mutex.Unlock()

		p.notifyStateChange("ice_state", map[string]interface{}{
			"state": state.String(),
		})

		switch state {
		case webrtc.ICEConnectionStateConnected:
			logger.Infof("ICE connected for peer %s", p.ID)
		case webrtc.ICEConnectionStateFailed:
			logger.Errorf("ICE connection failed for peer %s", p.ID)
			p.handleConnectionFailure()
		case webrtc.ICEConnectionStateDisconnected:
			logger.Warnf("ICE disconnected for peer %s", p.ID)
		}
	})

	// Signaling state change
	p.PeerConnection.OnSignalingStateChange(func(state webrtc.SignalingState) {
		p.mutex.Lock()
		p.SignalingState = state
		p.mutex.Unlock()

		p.notifyStateChange("signaling_state", map[string]interface{}{
			"state": state.String(),
		})
	})

	// ICE candidate
	p.PeerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}

		p.sendSignalingMessage("ice_candidate", map[string]interface{}{
			"candidate": candidate.ToJSON(),
		})
	})

	// Track handling
	p.PeerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		p.handleRemoteTrack(track, receiver)
	})

	// Data channel handling
	p.PeerConnection.OnDataChannel(func(channel *webrtc.DataChannel) {
		p.handleDataChannel(channel)
	})

	return nil
}

// CreateOffer creates an SDP offer
func (p *PeerConnection) CreateOffer(options *webrtc.OfferOptions) (*webrtc.SessionDescription, error) {
	if p.isClosed() {
		return nil, fmt.Errorf("peer connection is closed")
	}

	offer, err := p.PeerConnection.CreateOffer(options)
	if err != nil {
		return nil, fmt.Errorf("failed to create offer: %w", err)
	}

	if err := p.PeerConnection.SetLocalDescription(offer); err != nil {
		return nil, fmt.Errorf("failed to set local description: %w", err)
	}

	logger.LogUserAction(p.UserID.Hex(), "offer_created", "webrtc", map[string]interface{}{
		"peer_id": p.ID,
		"call_id": p.CallID.Hex(),
	})

	return &offer, nil
}

// CreateAnswer creates an SDP answer
func (p *PeerConnection) CreateAnswer(options *webrtc.AnswerOptions) (*webrtc.SessionDescription, error) {
	if p.isClosed() {
		return nil, fmt.Errorf("peer connection is closed")
	}

	answer, err := p.PeerConnection.CreateAnswer(options)
	if err != nil {
		return nil, fmt.Errorf("failed to create answer: %w", err)
	}

	if err := p.PeerConnection.SetLocalDescription(answer); err != nil {
		return nil, fmt.Errorf("failed to set local description: %w", err)
	}

	logger.LogUserAction(p.UserID.Hex(), "answer_created", "webrtc", map[string]interface{}{
		"peer_id": p.ID,
		"call_id": p.CallID.Hex(),
	})

	return &answer, nil
}

// SetRemoteDescription sets the remote SDP description
func (p *PeerConnection) SetRemoteDescription(sdp webrtc.SessionDescription) error {
	if p.isClosed() {
		return fmt.Errorf("peer connection is closed")
	}

	if err := p.PeerConnection.SetRemoteDescription(sdp); err != nil {
		return fmt.Errorf("failed to set remote description: %w", err)
	}

	logger.LogUserAction(p.UserID.Hex(), "remote_description_set", "webrtc", map[string]interface{}{
		"peer_id": p.ID,
		"type":    sdp.Type.String(),
	})

	return nil
}

// AddICECandidate adds an ICE candidate
func (p *PeerConnection) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	if p.isClosed() {
		return fmt.Errorf("peer connection is closed")
	}

	if err := p.PeerConnection.AddICECandidate(candidate); err != nil {
		return fmt.Errorf("failed to add ICE candidate: %w", err)
	}

	return nil
}

// AddLocalTrack adds a local media track
func (p *PeerConnection) AddLocalTrack(trackType string, codec webrtc.RTPCodecCapability) (*webrtc.TrackLocalStaticRTP, error) {
	if p.isClosed() {
		return nil, fmt.Errorf("peer connection is closed")
	}

	track, err := webrtc.NewTrackLocalStaticRTP(codec, trackType, fmt.Sprintf("%s_%s", p.UserID.Hex(), trackType))
	if err != nil {
		return nil, fmt.Errorf("failed to create local track: %w", err)
	}

	sender, err := p.PeerConnection.AddTrack(track)
	if err != nil {
		return nil, fmt.Errorf("failed to add track: %w", err)
	}

	p.mutex.Lock()
	p.LocalTracks[trackType] = track
	p.mutex.Unlock()

	// Handle RTCP packets
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if p.isClosed() {
				return
			}
			_, _, err := sender.Read(rtcpBuf)
			if err != nil {
				return
			}
		}
	}()

	// Update media state
	p.updateMediaState(trackType, true)

	logger.LogUserAction(p.UserID.Hex(), "local_track_added", "webrtc", map[string]interface{}{
		"peer_id":    p.ID,
		"track_type": trackType,
		"codec":      codec.MimeType,
	})

	return track, nil
}

// RemoveLocalTrack removes a local media track
func (p *PeerConnection) RemoveLocalTrack(trackType string) error {
	if p.isClosed() {
		return fmt.Errorf("peer connection is closed")
	}

	p.mutex.Lock()
	track, exists := p.LocalTracks[trackType]
	if !exists {
		p.mutex.Unlock()
		return fmt.Errorf("track %s not found", trackType)
	}
	delete(p.LocalTracks, trackType)
	p.mutex.Unlock()

	// Find and remove the sender
	for _, sender := range p.PeerConnection.GetSenders() {
		if sender.Track() == track {
			if err := p.PeerConnection.RemoveTrack(sender); err != nil {
				return fmt.Errorf("failed to remove track: %w", err)
			}
			break
		}
	}

	// Update media state
	p.updateMediaState(trackType, false)

	logger.LogUserAction(p.UserID.Hex(), "local_track_removed", "webrtc", map[string]interface{}{
		"peer_id":    p.ID,
		"track_type": trackType,
	})

	return nil
}

// updateMediaState updates the media state based on track type
func (p *PeerConnection) updateMediaState(trackType string, enabled bool) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	switch trackType {
	case "video":
		p.MediaState.VideoEnabled = enabled
	case "audio":
		p.MediaState.AudioEnabled = enabled
	case "screen":
		p.MediaState.ScreenSharing = enabled
	}

	// Notify room about media state change
	p.notifyStateChange("media_state", map[string]interface{}{
		"media_state": p.MediaState,
	})
}

// handleRemoteTrack handles incoming remote tracks
func (p *PeerConnection) handleRemoteTrack(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
	trackID := track.ID()

	p.mutex.Lock()
	p.RemoteTracks[trackID] = track
	p.mutex.Unlock()

	logger.LogUserAction(p.UserID.Hex(), "remote_track_received", "webrtc", map[string]interface{}{
		"peer_id":     p.ID,
		"track_id":    trackID,
		"track_kind":  track.Kind().String(),
		"track_codec": track.Codec().MimeType,
	})

	// Notify room about new remote track
	p.notifyStateChange("remote_track", map[string]interface{}{
		"track_id":    trackID,
		"track_kind":  track.Kind().String(),
		"track_codec": track.Codec().MimeType,
	})

	// Start reading packets (for statistics and processing)
	go p.handleTrackPackets(track)
}

// handleTrackPackets reads and processes track packets
func (p *PeerConnection) handleTrackPackets(track *webrtc.TrackRemote) {
	for {
		if p.isClosed() {
			return
		}

		_, _, err := track.ReadRTP()
		if err != nil {
			logger.Errorf("Error reading RTP packet: %v", err)
			return
		}

		// Update statistics
		p.updatePacketStats()
	}
}

// handleDataChannel handles incoming data channels
func (p *PeerConnection) handleDataChannel(channel *webrtc.DataChannel) {
	logger.LogUserAction(p.UserID.Hex(), "data_channel_received", "webrtc", map[string]interface{}{
		"peer_id":       p.ID,
		"channel_id":    channel.ID(),
		"channel_label": channel.Label(),
	})

	// Handle data channel messages
	channel.OnMessage(func(msg webrtc.DataChannelMessage) {
		p.handleDataChannelMessage(channel, msg)
	})
}

// handleDataChannelMessage handles data channel messages
func (p *PeerConnection) handleDataChannelMessage(channel *webrtc.DataChannel, msg webrtc.DataChannelMessage) {
	logger.Debugf("Data channel message from peer %s: %s", p.ID, string(msg.Data))

	// Parse and handle different message types
	var message map[string]interface{}
	if err := json.Unmarshal(msg.Data, &message); err != nil {
		logger.Errorf("Failed to parse data channel message: %v", err)
		return
	}

	msgType, ok := message["type"].(string)
	if !ok {
		return
	}

	switch msgType {
	case "chat_message":
		p.handleChatMessage(message)
	case "file_transfer":
		p.handleFileTransfer(message)
	case "typing_indicator":
		p.handleTypingIndicator(message)
	}
}

// Statistics and monitoring

// GetStats returns current connection statistics
func (p *PeerConnection) GetStats() (*PeerStats, error) {
	if p.isClosed() {
		return nil, fmt.Errorf("peer connection is closed")
	}

	stats := p.PeerConnection.GetStats()

	p.mutex.RLock()
	currentStats := *p.Stats
	p.mutex.RUnlock()

	// Parse WebRTC stats
	for _, stat := range stats {
		switch s := stat.(type) {
		case *webrtc.InboundRTPStreamStats:
			currentStats.BytesReceived = s.BytesReceived
			currentStats.PacketsReceived = uint64(s.PacketsReceived)
			currentStats.PacketsLost = uint64(s.PacketsLost)
			currentStats.Jitter = s.Jitter
		case *webrtc.OutboundRTPStreamStats:
			currentStats.BytesSent = s.BytesSent
			currentStats.PacketsSent = uint64(s.PacketsSent)
		case *webrtc.ICECandidatePairStats:
			currentStats.RTT = uint32(s.CurrentRoundTripTime * 1000)
		}
	}

	currentStats.LastStatsUpdate = time.Now()

	p.mutex.Lock()
	p.Stats = &currentStats
	p.mutex.Unlock()

	return &currentStats, nil
}

// updatePacketStats updates packet statistics
func (p *PeerConnection) updatePacketStats() {
	p.mutex.Lock()
	p.Stats.PacketsReceived++
	p.mutex.Unlock()
}

// startStatsCollection starts periodic statistics collection
func (p *PeerConnection) startStatsCollection() {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if p.isClosed() {
					return
				}

				stats, err := p.GetStats()
				if err != nil {
					logger.Errorf("Failed to get stats for peer %s: %v", p.ID, err)
					continue
				}

				// Update quality metrics based on stats
				p.updateQualityMetrics(stats)

				// Notify room about stats update
				p.notifyStateChange("stats_update", map[string]interface{}{
					"stats": stats,
				})

			case <-p.done:
				return
			}
		}
	}()
}

// updateQualityMetrics updates quality metrics based on statistics
func (p *PeerConnection) updateQualityMetrics(stats *PeerStats) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.QualityMetrics = models.QualityMetrics{
		AudioBitrate:     int64(stats.AudioBitrate),
		AudioPacketsLost: int64(stats.PacketsLost),
		AudioJitter:      stats.Jitter,
		VideoBitrate:     int64(stats.VideoBitrate),
		VideoPacketsLost: int64(stats.PacketsLost),
		VideoFramerate:   float64(p.MediaState.VideoFrameRate),
		VideoResolution:  p.MediaState.VideoResolution,
		RTT:              int(stats.RTT),
		NetworkType:      p.ConnectionInfo.ConnectionType,
		QualityScore:     p.calculateQualityScore(stats),
	}
}

// calculateQualityScore calculates overall quality score (1-5)
func (p *PeerConnection) calculateQualityScore(stats *PeerStats) float64 {
	score := 5.0

	// Penalize for packet loss
	if stats.PacketsLost > 0 && stats.PacketsReceived > 0 {
		lossRate := float64(stats.PacketsLost) / float64(stats.PacketsReceived)
		score -= lossRate * 2.0
	}

	// Penalize for high RTT
	if stats.RTT > 100 {
		score -= float64(stats.RTT-100) / 100.0
	}

	// Penalize for high jitter
	if stats.Jitter > 30 {
		score -= (stats.Jitter - 30) / 50.0
	}

	if score < 1.0 {
		score = 1.0
	}

	return score
}

// Event handlers

// onConnected handles successful connection
func (p *PeerConnection) onConnected() {
	logger.LogUserAction(p.UserID.Hex(), "peer_connected", "webrtc", map[string]interface{}{
		"peer_id":  p.ID,
		"call_id":  p.CallID.Hex(),
		"duration": time.Since(p.CreatedAt).Seconds(),
	})

	// Start statistics collection
	p.startStatsCollection()

	// Notify room about connection
	if p.Room != nil {
		p.Room.OnPeerConnected(p)
	}
}

// onDisconnected handles disconnection
func (p *PeerConnection) onDisconnected() {
	logger.LogUserAction(p.UserID.Hex(), "peer_disconnected", "webrtc", map[string]interface{}{
		"peer_id": p.ID,
		"call_id": p.CallID.Hex(),
	})

	// Attempt reconnection if not intentionally closed
	if !p.isClosed() && !p.reconnecting {
		p.attemptReconnection()
	}
}

// onFailed handles connection failure
func (p *PeerConnection) onFailed() {
	logger.LogUserAction(p.UserID.Hex(), "peer_failed", "webrtc", map[string]interface{}{
		"peer_id": p.ID,
		"call_id": p.CallID.Hex(),
	})

	p.handleConnectionFailure()
}

// onClosed handles connection closure
func (p *PeerConnection) onClosed() {
	logger.LogUserAction(p.UserID.Hex(), "peer_closed", "webrtc", map[string]interface{}{
		"peer_id": p.ID,
		"call_id": p.CallID.Hex(),
	})

	if p.Room != nil {
		p.Room.OnPeerDisconnected(p)
	}
}

// handleConnectionFailure handles connection failures
func (p *PeerConnection) handleConnectionFailure() {
	// Attempt reconnection
	p.attemptReconnection()
}

// attemptReconnection attempts to reconnect the peer
func (p *PeerConnection) attemptReconnection() {
	if p.reconnecting || p.isClosed() {
		return
	}

	p.mutex.Lock()
	p.reconnecting = true
	p.mutex.Unlock()

	go func() {
		defer func() {
			p.mutex.Lock()
			p.reconnecting = false
			p.mutex.Unlock()
		}()

		logger.Infof("Attempting reconnection for peer %s", p.ID)

		// Create new peer connection with same configuration
		// This would involve recreating the WebRTC connection
		// and re-negotiating with the remote peer

		// For now, just notify about reconnection attempt
		p.sendSignalingMessage("reconnection_needed", map[string]interface{}{
			"reason": "connection_failed",
		})
	}()
}

// Communication helpers

// sendSignalingMessage sends a signaling message through WebSocket
func (p *PeerConnection) sendSignalingMessage(messageType string, data interface{}) {
	if p.WSClient == nil {
		return
	}

	message := map[string]interface{}{
		"type":    messageType,
		"peer_id": p.ID,
		"call_id": p.CallID.Hex(),
		"room_id": p.RoomID,
		"data":    data,
	}

	p.WSClient.SendJSON("webrtc_signaling", message)
}

// notifyStateChange notifies about state changes
func (p *PeerConnection) notifyStateChange(changeType string, data interface{}) {
	if p.Room != nil {
		p.Room.OnPeerStateChange(p, changeType, data)
	}

	// Also send through WebSocket
	p.sendSignalingMessage("state_change", map[string]interface{}{
		"change_type": changeType,
		"data":        data,
	})
}

// Message handlers for data channel

// handleChatMessage handles chat messages through data channel
func (p *PeerConnection) handleChatMessage(message map[string]interface{}) {
	// Forward to room for processing
	if p.Room != nil {
		p.Room.HandleDataChannelMessage(p, "chat_message", message)
	}
}

// handleFileTransfer handles file transfer through data channel
func (p *PeerConnection) handleFileTransfer(message map[string]interface{}) {
	// Forward to room for processing
	if p.Room != nil {
		p.Room.HandleDataChannelMessage(p, "file_transfer", message)
	}
}

// handleTypingIndicator handles typing indicators
func (p *PeerConnection) handleTypingIndicator(message map[string]interface{}) {
	// Forward to room for processing
	if p.Room != nil {
		p.Room.HandleDataChannelMessage(p, "typing_indicator", message)
	}
}

// Utility methods

// isClosed checks if the peer connection is closed
func (p *PeerConnection) isClosed() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.closed
}

// GetInfo returns peer connection information
func (p *PeerConnection) GetInfo() map[string]interface{} {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return map[string]interface{}{
		"id":              p.ID,
		"user_id":         p.UserID.Hex(),
		"call_id":         p.CallID.Hex(),
		"room_id":         p.RoomID,
		"device_id":       p.DeviceID,
		"platform":        p.Platform,
		"state":           p.State.String(),
		"ice_state":       p.ICEState.String(),
		"signaling_state": p.SignalingState.String(),
		"media_state":     p.MediaState,
		"created_at":      p.CreatedAt,
		"connected_at":    p.ConnectedAt,
		"local_tracks":    len(p.LocalTracks),
		"remote_tracks":   len(p.RemoteTracks),
		"reconnecting":    p.reconnecting,
	}
}

// Close gracefully closes the peer connection
func (p *PeerConnection) Close() error {
	p.mutex.Lock()
	if p.closed {
		p.mutex.Unlock()
		return nil
	}
	p.closed = true
	p.mutex.Unlock()

	// Close done channel to stop goroutines
	close(p.done)

	// Close WebRTC connection
	if p.PeerConnection != nil {
		if err := p.PeerConnection.Close(); err != nil {
			logger.Errorf("Error closing peer connection: %v", err)
		}
	}

	logger.LogUserAction(p.UserID.Hex(), "peer_connection_closed", "webrtc", map[string]interface{}{
		"peer_id":  p.ID,
		"call_id":  p.CallID.Hex(),
		"duration": time.Since(p.CreatedAt).Seconds(),
	})

	return nil
}
