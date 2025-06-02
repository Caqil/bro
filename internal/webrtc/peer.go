package webrtc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"bro/internal/models"
	"bro/pkg/logger"
)

// Peer represents a WebRTC peer connection
type Peer struct {
	ID           string
	UserID       primitive.ObjectID
	ConnectionID string
	RoomID       string
	CallID       primitive.ObjectID

	// WebRTC connection
	PeerConnection *webrtc.PeerConnection
	DataChannel    *webrtc.DataChannel

	// Media state
	MediaState models.MediaState
	DeviceInfo models.DeviceInfo

	// Connection state
	ConnectionState webrtc.PeerConnectionState
	ICEState        webrtc.ICEConnectionState
	SignalingState  webrtc.SignalingState

	// Tracks
	LocalTracks  []*webrtc.TrackLocalStaticRTP
	RemoteTracks []*webrtc.TrackRemote

	// Metrics and quality
	Stats          *ConnectionStats
	QualityMetrics models.QualityMetrics

	// Signaling
	SignalingChannel chan *SignalingMessage
	ICECandidates    []*webrtc.ICECandidate

	// Lifecycle
	CreatedAt    time.Time
	ConnectedAt  *time.Time
	DisconnectAt *time.Time

	// Synchronization
	mutex   sync.RWMutex
	closed  bool
	cleanup func()

	// Callbacks
	OnConnectionStateChange func(state webrtc.PeerConnectionState)
	OnICECandidate          func(candidate *webrtc.ICECandidate)
	OnTrack                 func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver)
	OnDataChannelMessage    func(msg webrtc.DataChannelMessage)
	OnStatsUpdate           func(stats *ConnectionStats)
}

// ConnectionStats represents connection statistics
type ConnectionStats struct {
	BytesSent        uint64
	BytesReceived    uint64
	PacketsSent      uint64
	PacketsReceived  uint64
	PacketsLost      uint64
	RTT              time.Duration
	Jitter           time.Duration
	Bitrate          uint64
	FrameRate        float64
	VideoResolution  models.Resolution
	AudioLevel       float64
	NetworkType      string
	TotalFreezeTime  time.Duration
	TotalPauseTime   time.Duration
	FramesDropped    uint64
	FramesDecoded    uint64
	FramesRendered   uint64
	KeyFramesDecoded uint64
	LastStatsUpdate  time.Time
}

// SignalingMessage represents a signaling message
type SignalingMessage struct {
	Type       string                 `json:"type"`
	FromPeerID string                 `json:"from_peer_id"`
	ToPeerID   string                 `json:"to_peer_id,omitempty"`
	RoomID     string                 `json:"room_id"`
	CallID     string                 `json:"call_id"`
	Data       map[string]interface{} `json:"data"`
	Timestamp  time.Time              `json:"timestamp"`
	MessageID  string                 `json:"message_id"`
}

// PeerConfig represents peer configuration
type PeerConfig struct {
	ICEServers            []webrtc.ICEServer
	EnableDataChannel     bool
	MaxBitrate            uint32
	VideoCodec            webrtc.RTPCodecCapability
	AudioCodec            webrtc.RTPCodecCapability
	EnableAdaptiveBitrate bool
	StatsInterval         time.Duration
}

// DefaultPeerConfig returns default peer configuration
func DefaultPeerConfig(iceServers []webrtc.ICEServer) *PeerConfig {
	return &PeerConfig{
		ICEServers:        iceServers,
		EnableDataChannel: true,
		MaxBitrate:        2000000, // 2 Mbps
		VideoCodec: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeVP8,
			ClockRate: 90000,
		},
		AudioCodec: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeOpus,
			ClockRate: 48000,
			Channels:  2,
		},
		EnableAdaptiveBitrate: true,
		StatsInterval:         5 * time.Second,
	}
}

// NewPeer creates a new WebRTC peer
func NewPeer(userID primitive.ObjectID, connectionID, roomID string, callID primitive.ObjectID, config *PeerConfig) (*Peer, error) {
	peerID := fmt.Sprintf("%s_%s_%d", userID.Hex(), connectionID, time.Now().Unix())

	// Create WebRTC configuration
	webrtcConfig := webrtc.Configuration{
		ICEServers:         config.ICEServers,
		ICETransportPolicy: webrtc.ICETransportPolicyAll,
	}

	// Create MediaEngine with preferred codecs
	mediaEngine := &webrtc.MediaEngine{}

	// Register video codecs
	if err := mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: config.VideoCodec,
		PayloadType:        96,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		return nil, fmt.Errorf("failed to register video codec: %w", err)
	}

	// Register audio codecs
	if err := mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: config.AudioCodec,
		PayloadType:        111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, fmt.Errorf("failed to register audio codec: %w", err)
	}

	// Create API with media engine
	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))

	// Create peer connection
	peerConnection, err := api.NewPeerConnection(webrtcConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %w", err)
	}

	peer := &Peer{
		ID:               peerID,
		UserID:           userID,
		ConnectionID:     connectionID,
		RoomID:           roomID,
		CallID:           callID,
		PeerConnection:   peerConnection,
		MediaState:       models.MediaState{},
		ConnectionState:  webrtc.PeerConnectionStateNew,
		ICEState:         webrtc.ICEConnectionStateNew,
		SignalingState:   webrtc.SignalingStateStable,
		SignalingChannel: make(chan *SignalingMessage, 100),
		ICECandidates:    []*webrtc.ICECandidate{},
		CreatedAt:        time.Now(),
		Stats:            &ConnectionStats{},
		LocalTracks:      []*webrtc.TrackLocalStaticRTP{},
		RemoteTracks:     []*webrtc.TrackRemote{},
	}

	// Set up WebRTC event handlers
	peer.setupEventHandlers()

	// Create data channel if enabled
	if config.EnableDataChannel {
		if err := peer.createDataChannel(); err != nil {
			logger.Errorf("Failed to create data channel: %v", err)
		}
	}

	// Start statistics collection
	go peer.startStatsCollection(config.StatsInterval)

	logger.LogWebRTCEvent("peer_created", callID.Hex(), userID.Hex(), map[string]interface{}{
		"peer_id":       peerID,
		"room_id":       roomID,
		"connection_id": connectionID,
	})

	return peer, nil
}

// setupEventHandlers sets up WebRTC event handlers
func (p *Peer) setupEventHandlers() {
	// ICE candidate handler
	p.PeerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			p.mutex.Lock()
			p.ICECandidates = append(p.ICECandidates, candidate)
			p.mutex.Unlock()

			if p.OnICECandidate != nil {
				p.OnICECandidate(candidate)
			}

			logger.LogWebRTCEvent("ice_candidate_generated", p.CallID.Hex(), p.UserID.Hex(), map[string]interface{}{
				"peer_id":   p.ID,
				"candidate": candidate.ToJSON(),
			})
		}
	})

	// Connection state change handler
	p.PeerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		p.mutex.Lock()
		p.ConnectionState = state
		if state == webrtc.PeerConnectionStateConnected && p.ConnectedAt == nil {
			now := time.Now()
			p.ConnectedAt = &now
		} else if state == webrtc.PeerConnectionStateDisconnected || state == webrtc.PeerConnectionStateFailed {
			if p.DisconnectAt == nil {
				now := time.Now()
				p.DisconnectAt = &now
			}
		}
		p.mutex.Unlock()

		if p.OnConnectionStateChange != nil {
			p.OnConnectionStateChange(state)
		}

		logger.LogWebRTCEvent("connection_state_change", p.CallID.Hex(), p.UserID.Hex(), map[string]interface{}{
			"peer_id":          p.ID,
			"connection_state": state.String(),
		})

		// Handle failed connections
		if state == webrtc.PeerConnectionStateFailed {
			p.handleConnectionFailure()
		}
	})

	// ICE connection state change handler
	p.PeerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		p.mutex.Lock()
		p.ICEState = state
		p.mutex.Unlock()

		logger.LogWebRTCEvent("ice_connection_state_change", p.CallID.Hex(), p.UserID.Hex(), map[string]interface{}{
			"peer_id":   p.ID,
			"ice_state": state.String(),
		})
	})

	// Signaling state change handler
	p.PeerConnection.OnSignalingStateChange(func(state webrtc.SignalingState) {
		p.mutex.Lock()
		p.SignalingState = state
		p.mutex.Unlock()

		logger.LogWebRTCEvent("signaling_state_change", p.CallID.Hex(), p.UserID.Hex(), map[string]interface{}{
			"peer_id":         p.ID,
			"signaling_state": state.String(),
		})
	})

	// Track handler
	p.PeerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		p.mutex.Lock()
		p.RemoteTracks = append(p.RemoteTracks, track)
		p.mutex.Unlock()

		if p.OnTrack != nil {
			p.OnTrack(track, receiver)
		}

		logger.LogWebRTCEvent("track_received", p.CallID.Hex(), p.UserID.Hex(), map[string]interface{}{
			"peer_id":    p.ID,
			"track_kind": track.Kind().String(),
			"track_id":   track.ID(),
		})

		// Start reading from track to keep it alive
		go p.readTrack(track)
	})

	// Data channel handler
	p.PeerConnection.OnDataChannel(func(dataChannel *webrtc.DataChannel) {
		p.DataChannel = dataChannel
		p.setupDataChannelHandlers(dataChannel)

		logger.LogWebRTCEvent("data_channel_received", p.CallID.Hex(), p.UserID.Hex(), map[string]interface{}{
			"peer_id":       p.ID,
			"channel_label": dataChannel.Label(),
		})
	})
}

// createDataChannel creates a data channel
func (p *Peer) createDataChannel() error {
	dataChannel, err := p.PeerConnection.CreateDataChannel("messages", nil)
	if err != nil {
		return fmt.Errorf("failed to create data channel: %w", err)
	}

	p.DataChannel = dataChannel
	p.setupDataChannelHandlers(dataChannel)

	return nil
}

// setupDataChannelHandlers sets up data channel event handlers
func (p *Peer) setupDataChannelHandlers(dataChannel *webrtc.DataChannel) {
	dataChannel.OnOpen(func() {
		logger.LogWebRTCEvent("data_channel_opened", p.CallID.Hex(), p.UserID.Hex(), map[string]interface{}{
			"peer_id": p.ID,
		})
	})

	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		if p.OnDataChannelMessage != nil {
			p.OnDataChannelMessage(msg)
		}

		logger.LogWebRTCEvent("data_channel_message", p.CallID.Hex(), p.UserID.Hex(), map[string]interface{}{
			"peer_id":      p.ID,
			"message_size": len(msg.Data),
		})
	})

	dataChannel.OnClose(func() {
		logger.LogWebRTCEvent("data_channel_closed", p.CallID.Hex(), p.UserID.Hex(), map[string]interface{}{
			"peer_id": p.ID,
		})
	})

	dataChannel.OnError(func(err error) {
		logger.LogWebRTCEvent("data_channel_error", p.CallID.Hex(), p.UserID.Hex(), map[string]interface{}{
			"peer_id": p.ID,
			"error":   err.Error(),
		})
	})
}

// AddTrack adds a local track to the peer connection
func (p *Peer) AddTrack(track *webrtc.TrackLocalStaticRTP) (*webrtc.RTPSender, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed {
		return nil, fmt.Errorf("peer connection is closed")
	}

	sender, err := p.PeerConnection.AddTrack(track)
	if err != nil {
		return nil, fmt.Errorf("failed to add track: %w", err)
	}

	p.LocalTracks = append(p.LocalTracks, track)

	logger.LogWebRTCEvent("track_added", p.CallID.Hex(), p.UserID.Hex(), map[string]interface{}{
		"peer_id":    p.ID,
		"track_kind": track.Kind().String(),
		"track_id":   track.ID(),
	})

	return sender, nil
}

// RemoveTrack removes a local track from the peer connection
func (p *Peer) RemoveTrack(sender *webrtc.RTPSender) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed {
		return fmt.Errorf("peer connection is closed")
	}

	err := p.PeerConnection.RemoveTrack(sender)
	if err != nil {
		return fmt.Errorf("failed to remove track: %w", err)
	}

	// Remove from local tracks array
	for i, track := range p.LocalTracks {
		if track.ID() == sender.Track().ID() {
			p.LocalTracks = append(p.LocalTracks[:i], p.LocalTracks[i+1:]...)
			break
		}
	}

	logger.LogWebRTCEvent("track_removed", p.CallID.Hex(), p.UserID.Hex(), map[string]interface{}{
		"peer_id":  p.ID,
		"track_id": sender.Track().ID(),
	})

	return nil
}

// CreateOffer creates a WebRTC offer
func (p *Peer) CreateOffer() (*webrtc.SessionDescription, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed {
		return nil, fmt.Errorf("peer connection is closed")
	}

	offer, err := p.PeerConnection.CreateOffer(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create offer: %w", err)
	}

	if err := p.PeerConnection.SetLocalDescription(offer); err != nil {
		return nil, fmt.Errorf("failed to set local description: %w", err)
	}

	logger.LogWebRTCEvent("offer_created", p.CallID.Hex(), p.UserID.Hex(), map[string]interface{}{
		"peer_id": p.ID,
	})

	return &offer, nil
}

// CreateAnswer creates a WebRTC answer
func (p *Peer) CreateAnswer() (*webrtc.SessionDescription, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed {
		return nil, fmt.Errorf("peer connection is closed")
	}

	answer, err := p.PeerConnection.CreateAnswer(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create answer: %w", err)
	}

	if err := p.PeerConnection.SetLocalDescription(answer); err != nil {
		return nil, fmt.Errorf("failed to set local description: %w", err)
	}

	logger.LogWebRTCEvent("answer_created", p.CallID.Hex(), p.UserID.Hex(), map[string]interface{}{
		"peer_id": p.ID,
	})

	return &answer, nil
}

// SetRemoteDescription sets the remote session description
func (p *Peer) SetRemoteDescription(desc webrtc.SessionDescription) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed {
		return fmt.Errorf("peer connection is closed")
	}

	err := p.PeerConnection.SetRemoteDescription(desc)
	if err != nil {
		return fmt.Errorf("failed to set remote description: %w", err)
	}

	logger.LogWebRTCEvent("remote_description_set", p.CallID.Hex(), p.UserID.Hex(), map[string]interface{}{
		"peer_id": p.ID,
		"type":    desc.Type.String(),
	})

	return nil
}

// AddICECandidate adds an ICE candidate
func (p *Peer) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed {
		return fmt.Errorf("peer connection is closed")
	}

	err := p.PeerConnection.AddICECandidate(candidate)
	if err != nil {
		return fmt.Errorf("failed to add ICE candidate: %w", err)
	}

	logger.LogWebRTCEvent("ice_candidate_added", p.CallID.Hex(), p.UserID.Hex(), map[string]interface{}{
		"peer_id":   p.ID,
		"candidate": candidate.Candidate,
	})

	return nil
}

// SendDataChannelMessage sends a message through the data channel
func (p *Peer) SendDataChannelMessage(message []byte) error {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	if p.closed || p.DataChannel == nil {
		return fmt.Errorf("data channel not available")
	}

	if p.DataChannel.ReadyState() != webrtc.DataChannelStateOpen {
		return fmt.Errorf("data channel not open")
	}

	return p.DataChannel.Send(message)
}

// GetStats returns current connection statistics
func (p *Peer) GetStats() *ConnectionStats {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	// Create a copy of the stats
	stats := *p.Stats
	return &stats
}

// UpdateMediaState updates the media state
func (p *Peer) UpdateMediaState(state models.MediaState) {
	p.mutex.Lock()
	p.MediaState = state
	p.mutex.Unlock()

	logger.LogWebRTCEvent("media_state_updated", p.CallID.Hex(), p.UserID.Hex(), map[string]interface{}{
		"peer_id":        p.ID,
		"video_enabled":  state.VideoEnabled,
		"audio_enabled":  state.AudioEnabled,
		"screen_sharing": state.ScreenSharing,
	})
}

// IsConnected returns true if the peer is connected
func (p *Peer) IsConnected() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.ConnectionState == webrtc.PeerConnectionStateConnected
}

// IsClosed returns true if the peer is closed
func (p *Peer) IsClosed() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.closed
}

// Close closes the peer connection
func (p *Peer) Close() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true

	// Close data channel
	if p.DataChannel != nil {
		p.DataChannel.Close()
	}

	// Close peer connection
	if p.PeerConnection != nil {
		if err := p.PeerConnection.Close(); err != nil {
			logger.Errorf("Failed to close peer connection: %v", err)
		}
	}

	// Close signaling channel
	close(p.SignalingChannel)

	// Run cleanup callback if set
	if p.cleanup != nil {
		p.cleanup()
	}

	logger.LogWebRTCEvent("peer_closed", p.CallID.Hex(), p.UserID.Hex(), map[string]interface{}{
		"peer_id": p.ID,
	})

	return nil
}

// readTrack reads from a remote track to keep it alive
func (p *Peer) readTrack(track *webrtc.TrackRemote) {
	for {
		_, _, err := track.ReadRTP()
		if err != nil {
			logger.Debugf("Error reading from track %s: %v", track.ID(), err)
			return
		}
	}
}

// startStatsCollection starts periodic statistics collection
func (p *Peer) startStatsCollection(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if p.IsClosed() {
				return
			}
			p.collectStats()
		}
	}
}

// collectStats collects WebRTC statistics
func (p *Peer) collectStats() {
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	statsReport := p.PeerConnection.GetStats()

	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Process statistics
	for _, stat := range statsReport {
		switch s := stat.(type) {
		case *webrtc.InboundRTPStreamStats:
			p.Stats.BytesReceived = s.BytesReceived
			p.Stats.PacketsReceived = uint64(s.PacketsReceived)
			p.Stats.PacketsLost = uint64(s.PacketsLost)
			p.Stats.Jitter = time.Duration(s.Jitter * float64(time.Second))
		case *webrtc.OutboundRTPStreamStats:
			p.Stats.BytesSent = s.BytesSent
			p.Stats.PacketsSent = uint64(s.PacketsSent)
		case *webrtc.RemoteInboundRTPStreamStats:
			if s.RoundTripTime != nil {
				p.Stats.RTT = time.Duration(*&s.RoundTripTime * float64(time.Second))
			}
		}
	}

	p.Stats.LastStatsUpdate = time.Now()

	// Call stats callback if set
	if p.OnStatsUpdate != nil {
		statsCopy := *p.Stats
		go p.OnStatsUpdate(&statsCopy)
	}
}

// handleConnectionFailure handles connection failures
func (p *Peer) handleConnectionFailure() {
	logger.LogWebRTCEvent("connection_failed", p.CallID.Hex(), p.UserID.Hex(), map[string]interface{}{
		"peer_id": p.ID,
	})

	// Try to restart ICE connection
	go func() {
		if err := p.PeerConnection.RestartICE(); err != nil {
			logger.Errorf("Failed to restart ICE for peer %s: %v", p.ID, err)
		}
	}()
}

// GetConnectionInfo returns detailed connection information
func (p *Peer) GetConnectionInfo() map[string]interface{} {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	info := map[string]interface{}{
		"peer_id":          p.ID,
		"user_id":          p.UserID.Hex(),
		"connection_id":    p.ConnectionID,
		"room_id":          p.RoomID,
		"call_id":          p.CallID.Hex(),
		"connection_state": p.ConnectionState.String(),
		"ice_state":        p.ICEState.String(),
		"signaling_state":  p.SignalingState.String(),
		"media_state":      p.MediaState,
		"created_at":       p.CreatedAt,
		"connected_at":     p.ConnectedAt,
		"disconnect_at":    p.DisconnectAt,
		"local_tracks":     len(p.LocalTracks),
		"remote_tracks":    len(p.RemoteTracks),
		"ice_candidates":   len(p.ICECandidates),
		"stats":            p.Stats,
	}

	return info
}

// SetCleanupCallback sets a callback function to be called when the peer is closed
func (p *Peer) SetCleanupCallback(callback func()) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.cleanup = callback
}

// GetMediaState returns current media state
func (p *Peer) GetMediaState() models.MediaState {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.MediaState
}
