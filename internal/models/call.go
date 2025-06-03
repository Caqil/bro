package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Call struct {
	ID primitive.ObjectID `bson:"_id,omitempty" json:"id"`

	// Basic Call Information
	Type   CallType   `bson:"type" json:"type"`
	Status CallStatus `bson:"status" json:"status"`

	// Participants
	InitiatorID  primitive.ObjectID `bson:"initiator_id" json:"initiator_id"`
	Participants []CallParticipant  `bson:"participants" json:"participants"`

	// Chat Information
	ChatID  primitive.ObjectID  `bson:"chat_id" json:"chat_id"`
	GroupID *primitive.ObjectID `bson:"group_id,omitempty" json:"group_id,omitempty"`

	// Call Session Details
	SessionID string `bson:"session_id" json:"session_id"`
	RoomID    string `bson:"room_id,omitempty" json:"room_id,omitempty"`

	// Timing Information
	InitiatedAt time.Time  `bson:"initiated_at" json:"initiated_at"`
	StartedAt   *time.Time `bson:"started_at,omitempty" json:"started_at,omitempty"`
	EndedAt     *time.Time `bson:"ended_at,omitempty" json:"ended_at,omitempty"`
	Duration    int64      `bson:"duration" json:"duration"` // Duration in seconds

	// Call Quality and Technical Details
	Quality     CallQuality      `bson:"quality" json:"quality"`
	TechDetails TechnicalDetails `bson:"tech_details" json:"tech_details"`

	// Features and Settings
	Features CallFeatures `bson:"features" json:"features"`
	Settings CallSettings `bson:"settings" json:"settings"`

	// Recording Information
	Recording *RecordingInfo `bson:"recording,omitempty" json:"recording,omitempty"`

	// End Call Information
	EndReason EndReason           `bson:"end_reason,omitempty" json:"end_reason,omitempty"`
	EndedBy   *primitive.ObjectID `bson:"ended_by,omitempty" json:"ended_by,omitempty"`

	// Billing and Cost (for premium features)
	BillingInfo *BillingInfo `bson:"billing_info,omitempty" json:"billing_info,omitempty"`

	// Analytics and Metrics
	Analytics CallAnalytics `bson:"analytics" json:"analytics"`

	// Admin and Moderation
	IsReported bool         `bson:"is_reported" json:"is_reported"`
	Reports    []CallReport `bson:"reports,omitempty" json:"reports,omitempty"`

	// Timestamps
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`

	// Metadata
	Metadata map[string]interface{} `bson:"metadata,omitempty" json:"metadata,omitempty"`
}

type CallParticipant struct {
	UserID primitive.ObjectID `bson:"user_id" json:"user_id"`

	// Join/Leave Information
	JoinedAt *time.Time `bson:"joined_at,omitempty" json:"joined_at,omitempty"`
	LeftAt   *time.Time `bson:"left_at,omitempty" json:"left_at,omitempty"`
	Duration int64      `bson:"duration" json:"duration"` // Participant duration in seconds

	// Participant Status
	Status ParticipantStatus `bson:"status" json:"status"`
	Role   ParticipantRole   `bson:"role" json:"role"`

	// Media State
	MediaState MediaState `bson:"media_state" json:"media_state"`

	// Device and Connection Info
	DeviceInfo     DeviceInfo     `bson:"device_info" json:"device_info"`
	ConnectionInfo ConnectionInfo `bson:"connection_info" json:"connection_info"`

	// Quality Metrics
	QualityMetrics QualityMetrics `bson:"quality_metrics" json:"quality_metrics"`

	// Participant Settings
	Settings ParticipantSettings `bson:"settings" json:"settings"`

	// Rejection/Miss Information
	ResponseTime *time.Duration `bson:"response_time,omitempty" json:"response_time,omitempty"`
	RejectReason *RejectReason  `bson:"reject_reason,omitempty" json:"reject_reason,omitempty"`

	// Screen Sharing
	ScreenShare *ScreenShareInfo `bson:"screen_share,omitempty" json:"screen_share,omitempty"`
}

type CallType string

const (
	CallTypeVoice       CallType = "voice"
	CallTypeVideo       CallType = "video"
	CallTypeGroup       CallType = "group"
	CallTypeConference  CallType = "conference"
	CallTypeScreenShare CallType = "screen_share"
	CallTypePrivate     CallType = "private"
)

type CallStatus string

const (
	CallStatusInitiating CallStatus = "initiating"
	CallStatusRinging    CallStatus = "ringing"
	CallStatusConnecting CallStatus = "connecting"
	CallStatusActive     CallStatus = "active"
	CallStatusHolding    CallStatus = "holding"
	CallStatusEnded      CallStatus = "ended"
	CallStatusFailed     CallStatus = "failed"
	CallStatusRejected   CallStatus = "rejected"
	CallStatusMissed     CallStatus = "missed"
	CallStatusBusy       CallStatus = "busy"
	CallStatusCancelled  CallStatus = "cancelled"
)

type ParticipantStatus string

const (
	ParticipantStatusInvited      ParticipantStatus = "invited"
	ParticipantStatusRinging      ParticipantStatus = "ringing"
	ParticipantStatusConnecting   ParticipantStatus = "connecting"
	ParticipantStatusConnected    ParticipantStatus = "connected"
	ParticipantStatusDisconnected ParticipantStatus = "disconnected"
	ParticipantStatusRejected     ParticipantStatus = "rejected"
	ParticipantStatusMissed       ParticipantStatus = "missed"
	ParticipantStatusBusy         ParticipantStatus = "busy"
)

type ParticipantRole string

const (
	ParticipantRoleInitiator   ParticipantRole = "initiator"
	ParticipantRoleParticipant ParticipantRole = "participant"
	ParticipantRoleModerator   ParticipantRole = "moderator"
	ParticipantRoleObserver    ParticipantRole = "observer"
)

type MediaState struct {
	VideoEnabled   bool `bson:"video_enabled" json:"video_enabled"`
	AudioEnabled   bool `bson:"audio_enabled" json:"audio_enabled"`
	ScreenSharing  bool `bson:"screen_sharing" json:"screen_sharing"`
	RecordingLocal bool `bson:"recording_local" json:"recording_local"`

	// Video settings
	VideoQuality    VideoQuality `bson:"video_quality" json:"video_quality"`
	VideoResolution Resolution   `bson:"video_resolution" json:"video_resolution"`
	VideoFrameRate  int          `bson:"video_frame_rate" json:"video_frame_rate"`

	// Audio settings
	AudioQuality AudioQuality `bson:"audio_quality" json:"audio_quality"`
	AudioCodec   string       `bson:"audio_codec" json:"audio_codec"`

	// Network adaptation
	NetworkAdaptation bool `bson:"network_adaptation" json:"network_adaptation"`
}

type DeviceInfo struct {
	Platform       string `bson:"platform" json:"platform"`       // "web", "ios", "android", "desktop"
	DeviceType     string `bson:"device_type" json:"device_type"` // "mobile", "tablet", "desktop"
	Browser        string `bson:"browser,omitempty" json:"browser,omitempty"`
	BrowserVersion string `bson:"browser_version,omitempty" json:"browser_version,omitempty"`
	AppVersion     string `bson:"app_version,omitempty" json:"app_version,omitempty"`
	OSVersion      string `bson:"os_version,omitempty" json:"os_version,omitempty"`

	// Camera and Microphone
	CameraDevice     string `bson:"camera_device,omitempty" json:"camera_device,omitempty"`
	MicrophoneDevice string `bson:"microphone_device,omitempty" json:"microphone_device,omitempty"`
	SpeakerDevice    string `bson:"speaker_device,omitempty" json:"speaker_device,omitempty"`

	// Capabilities
	SupportsVideo       bool `bson:"supports_video" json:"supports_video"`
	SupportsAudio       bool `bson:"supports_audio" json:"supports_audio"`
	SupportsScreenShare bool `bson:"supports_screen_share" json:"supports_screen_share"`
}

type ConnectionInfo struct {
	IPAddress      string        `bson:"ip_address,omitempty" json:"ip_address,omitempty"`
	UserAgent      string        `bson:"user_agent,omitempty" json:"user_agent,omitempty"`
	ConnectionType string        `bson:"connection_type" json:"connection_type"` // "wifi", "cellular", "ethernet"
	Bandwidth      BandwidthInfo `bson:"bandwidth" json:"bandwidth"`

	// WebRTC Connection Info
	ICEConnectionState string `bson:"ice_connection_state" json:"ice_connection_state"`
	DTLSState          string `bson:"dtls_state" json:"dtls_state"`
	SignalingState     string `bson:"signaling_state" json:"signaling_state"`

	// TURN/STUN usage
	UsingTURN  bool   `bson:"using_turn" json:"using_turn"`
	TURNServer string `bson:"turn_server,omitempty" json:"turn_server,omitempty"`
}

type BandwidthInfo struct {
	Download   int64   `bson:"download" json:"download"`       // Kbps
	Upload     int64   `bson:"upload" json:"upload"`           // Kbps
	Ping       int     `bson:"ping" json:"ping"`               // ms
	Jitter     float64 `bson:"jitter" json:"jitter"`           // ms
	PacketLoss float64 `bson:"packet_loss" json:"packet_loss"` // percentage
}

type QualityMetrics struct {
	// Audio Metrics
	AudioBitrate     int64   `bson:"audio_bitrate" json:"audio_bitrate"`
	AudioPacketsLost int64   `bson:"audio_packets_lost" json:"audio_packets_lost"`
	AudioJitter      float64 `bson:"audio_jitter" json:"audio_jitter"`

	// Video Metrics
	VideoBitrate     int64      `bson:"video_bitrate" json:"video_bitrate"`
	VideoPacketsLost int64      `bson:"video_packets_lost" json:"video_packets_lost"`
	VideoFramerate   float64    `bson:"video_framerate" json:"video_framerate"`
	VideoResolution  Resolution `bson:"video_resolution" json:"video_resolution"`

	// Network Metrics
	RTT         int    `bson:"rtt" json:"rtt"` // Round trip time in ms
	NetworkType string `bson:"network_type" json:"network_type"`

	// Overall Quality Score (1-5)
	QualityScore float64 `bson:"quality_score" json:"quality_score"`
}

type ParticipantSettings struct {
	AutoMute         bool `bson:"auto_mute" json:"auto_mute"`
	AutoVideo        bool `bson:"auto_video" json:"auto_video"`
	NoiseReduction   bool `bson:"noise_reduction" json:"noise_reduction"`
	EchoCancellation bool `bson:"echo_cancellation" json:"echo_cancellation"`
	BackgroundBlur   bool `bson:"background_blur" json:"background_blur"`
}

type ScreenShareInfo struct {
	IsSharing  bool       `bson:"is_sharing" json:"is_sharing"`
	StartedAt  time.Time  `bson:"started_at" json:"started_at"`
	EndedAt    *time.Time `bson:"ended_at,omitempty" json:"ended_at,omitempty"`
	ScreenType string     `bson:"screen_type" json:"screen_type"` // "screen", "window", "tab"
	Resolution Resolution `bson:"resolution" json:"resolution"`
	FrameRate  int        `bson:"frame_rate" json:"frame_rate"`
}

type CallQuality struct {
	OverallRating     float64 `bson:"overall_rating" json:"overall_rating"` // 1-5 scale
	AudioQuality      float64 `bson:"audio_quality" json:"audio_quality"`
	VideoQuality      float64 `bson:"video_quality" json:"video_quality"`
	ConnectionQuality float64 `bson:"connection_quality" json:"connection_quality"`

	// Issues encountered
	Issues []QualityIssue `bson:"issues,omitempty" json:"issues,omitempty"`
}

type QualityIssue struct {
	Type        string        `bson:"type" json:"type"`         // "audio_drop", "video_freeze", "connection_lost"
	Severity    string        `bson:"severity" json:"severity"` // "low", "medium", "high"
	Timestamp   time.Time     `bson:"timestamp" json:"timestamp"`
	Duration    time.Duration `bson:"duration" json:"duration"`
	Description string        `bson:"description,omitempty" json:"description,omitempty"`
}

type TechnicalDetails struct {
	// WebRTC Stats
	WebRTCStats map[string]interface{} `bson:"webrtc_stats,omitempty" json:"webrtc_stats,omitempty"`

	// Codec Information
	AudioCodec string `bson:"audio_codec" json:"audio_codec"`
	VideoCodec string `bson:"video_codec" json:"video_codec"`

	// Server Information
	MediaServerID  string `bson:"media_server_id,omitempty" json:"media_server_id,omitempty"`
	TURNServerUsed string `bson:"turn_server_used,omitempty" json:"turn_server_used,omitempty"`

	// Bandwidth Usage
	TotalBandwidthUsed int64 `bson:"total_bandwidth_used" json:"total_bandwidth_used"` // in bytes
	PeakBandwidth      int64 `bson:"peak_bandwidth" json:"peak_bandwidth"`             // in bytes per second

	// Error Information
	Errors []TechnicalError `bson:"errors,omitempty" json:"errors,omitempty"`
}

type TechnicalError struct {
	Code      string    `bson:"code" json:"code"`
	Message   string    `bson:"message" json:"message"`
	Timestamp time.Time `bson:"timestamp" json:"timestamp"`
	Severity  string    `bson:"severity" json:"severity"`
	Component string    `bson:"component" json:"component"` // "webrtc", "signaling", "media"
}

type CallFeatures struct {
	VideoCall         bool `bson:"video_call" json:"video_call"`
	ScreenShare       bool `bson:"screen_share" json:"screen_share"`
	Recording         bool `bson:"recording" json:"recording"`
	ChatDuringCall    bool `bson:"chat_during_call" json:"chat_during_call"`
	FileSharing       bool `bson:"file_sharing" json:"file_sharing"`
	BackgroundEffects bool `bson:"background_effects" json:"background_effects"`
	NoiseReduction    bool `bson:"noise_reduction" json:"noise_reduction"`
	EchoCancellation  bool `bson:"echo_cancellation" json:"echo_cancellation"`
}

type CallSettings struct {
	MaxParticipants   int  `bson:"max_participants" json:"max_participants"`
	RequirePermission bool `bson:"require_permission" json:"require_permission"`
	AutoAccept        bool `bson:"auto_accept" json:"auto_accept"`
	RecordingEnabled  bool `bson:"recording_enabled" json:"recording_enabled"`

	// Quality Settings
	MaxVideoBitrate int  `bson:"max_video_bitrate" json:"max_video_bitrate"`
	MaxAudioBitrate int  `bson:"max_audio_bitrate" json:"max_audio_bitrate"`
	AdaptiveQuality bool `bson:"adaptive_quality" json:"adaptive_quality"`

	// Security Settings
	EndToEndEncryption bool   `bson:"end_to_end_encryption" json:"end_to_end_encryption"`
	RequirePassword    bool   `bson:"require_password" json:"require_password"`
	Password           string `bson:"password,omitempty" json:"-"`
}

type RecordingInfo struct {
	IsRecording bool       `bson:"is_recording" json:"is_recording"`
	StartedAt   *time.Time `bson:"started_at,omitempty" json:"started_at,omitempty"`
	EndedAt     *time.Time `bson:"ended_at,omitempty" json:"ended_at,omitempty"`
	Duration    int64      `bson:"duration" json:"duration"` // seconds

	// Recording Files
	VideoURL string `bson:"video_url,omitempty" json:"video_url,omitempty"`
	AudioURL string `bson:"audio_url,omitempty" json:"audio_url,omitempty"`

	// Recording Settings
	Quality  VideoQuality `bson:"quality" json:"quality"`
	Format   string       `bson:"format" json:"format"`       // "mp4", "webm", "audio/mp3"
	FileSize int64        `bson:"file_size" json:"file_size"` // bytes

	// Access Control
	IsPublic  bool       `bson:"is_public" json:"is_public"`
	AccessKey string     `bson:"access_key,omitempty" json:"access_key,omitempty"`
	ExpiresAt *time.Time `bson:"expires_at,omitempty" json:"expires_at,omitempty"`

	// Recording by participant
	RecordedBy            primitive.ObjectID     `bson:"recorded_by" json:"recorded_by"`
	ParticipantRecordings []ParticipantRecording `bson:"participant_recordings,omitempty" json:"participant_recordings,omitempty"`
}

type ParticipantRecording struct {
	UserID    primitive.ObjectID `bson:"user_id" json:"user_id"`
	StartedAt time.Time          `bson:"started_at" json:"started_at"`
	EndedAt   *time.Time         `bson:"ended_at,omitempty" json:"ended_at,omitempty"`
	VideoURL  string             `bson:"video_url,omitempty" json:"video_url,omitempty"`
	AudioURL  string             `bson:"audio_url,omitempty" json:"audio_url,omitempty"`
}

type BillingInfo struct {
	Cost            float64 `bson:"cost" json:"cost"`
	Currency        string  `bson:"currency" json:"currency"`
	BillingMethod   string  `bson:"billing_method" json:"billing_method"` // "duration", "participant_minutes"
	FreeMinutesUsed int     `bson:"free_minutes_used" json:"free_minutes_used"`
	PremiumMinutes  int     `bson:"premium_minutes" json:"premium_minutes"`
}

type CallAnalytics struct {
	// Connection Analytics
	ConnectionAttempts    int `bson:"connection_attempts" json:"connection_attempts"`
	SuccessfulConnections int `bson:"successful_connections" json:"successful_connections"`
	FailedConnections     int `bson:"failed_connections" json:"failed_connections"`

	// Quality Analytics
	AverageQuality      float64        `bson:"average_quality" json:"average_quality"`
	QualityDistribution map[string]int `bson:"quality_distribution" json:"quality_distribution"`

	// Usage Analytics
	PeakParticipants        int `bson:"peak_participants" json:"peak_participants"`
	TotalParticipantMinutes int `bson:"total_participant_minutes" json:"total_participant_minutes"`

	// Feature Usage
	FeaturesUsed        []string `bson:"features_used" json:"features_used"`
	ScreenShareDuration int64    `bson:"screen_share_duration" json:"screen_share_duration"`
	RecordingDuration   int64    `bson:"recording_duration" json:"recording_duration"`

	// Geographic Data
	ParticipantLocations []string `bson:"participant_locations,omitempty" json:"participant_locations,omitempty"`
}

type CallReport struct {
	ReporterID primitive.ObjectID  `bson:"reporter_id" json:"reporter_id"`
	Reason     string              `bson:"reason" json:"reason"`
	Details    string              `bson:"details,omitempty" json:"details,omitempty"`
	ReportedAt time.Time           `bson:"reported_at" json:"reported_at"`
	Status     string              `bson:"status" json:"status"` // "pending", "reviewed", "dismissed"
	ReviewedBy *primitive.ObjectID `bson:"reviewed_by,omitempty" json:"reviewed_by,omitempty"`
	ReviewedAt *time.Time          `bson:"reviewed_at,omitempty" json:"reviewed_at,omitempty"`
}

// Enums and constants
type EndReason string

const (
	EndReasonNormal       EndReason = "normal"
	EndReasonBusy         EndReason = "busy"
	EndReasonNoAnswer     EndReason = "no_answer"
	EndReasonRejected     EndReason = "rejected"
	EndReasonCancelled    EndReason = "cancelled"
	EndReasonNetworkError EndReason = "network_error"
	EndReasonServerError  EndReason = "server_error"
	EndReasonTimeout      EndReason = "timeout"
)

type RejectReason string

const (
	RejectReasonBusy       RejectReason = "busy"
	RejectReasonDeclined   RejectReason = "declined"
	RejectReasonNoDevice   RejectReason = "no_device"
	RejectReasonPermission RejectReason = "permission"
)

type VideoQuality string

const (
	VideoQualityLow    VideoQuality = "low"    // 360p
	VideoQualityMedium VideoQuality = "medium" // 720p
	VideoQualityHigh   VideoQuality = "high"   // 1080p
	VideoQualityHD     VideoQuality = "hd"     // 1440p
	VideoQuality4K     VideoQuality = "4k"     // 2160p
)

type AudioQuality string

const (
	AudioQualityLow    AudioQuality = "low"    // 64 kbps
	AudioQualityMedium AudioQuality = "medium" // 128 kbps
	AudioQualityHigh   AudioQuality = "high"   // 256 kbps
)

type Resolution struct {
	Width  int `bson:"width" json:"width"`
	Height int `bson:"height" json:"height"`
}

// Request/Response structs

type InitiateCallRequest struct {
	Type         CallType             `json:"type" validate:"required"`
	Participants []primitive.ObjectID `json:"participants" validate:"required"`
	ChatID       primitive.ObjectID   `json:"chat_id" validate:"required"`
	VideoEnabled bool                 `json:"video_enabled"`
	Settings     *CallSettings        `json:"settings,omitempty"`
}

type AnswerCallRequest struct {
	Accept       bool          `json:"accept" validate:"required"`
	VideoEnabled bool          `json:"video_enabled"`
	RejectReason *RejectReason `json:"reject_reason,omitempty"`
}

type CallControlRequest struct {
	Action string      `json:"action" validate:"required"` // "mute", "unmute", "video_on", "video_off", "screen_share_start", "screen_share_stop"
	Data   interface{} `json:"data,omitempty"`
}

type CallResponse struct {
	Call
	MyParticipant *CallParticipant `json:"my_participant,omitempty"`
	CanControl    bool             `json:"can_control"`
	CanRecord     bool             `json:"can_record"`
	CanInvite     bool             `json:"can_invite"`
	TURNServers   []TURNServer     `json:"turn_servers"`
}

type TURNServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

type CallHistoryResponse struct {
	Calls      []CallResponse `json:"calls"`
	TotalCount int64          `json:"total_count"`
	HasMore    bool           `json:"has_more"`
}

// Helper methods

// BeforeCreate sets default values before creating call
func (c *Call) BeforeCreate() {
	now := time.Now()
	c.CreatedAt = now
	c.UpdatedAt = now
	c.InitiatedAt = now
	c.SessionID = primitive.NewObjectID().Hex()

	// Set default features
	c.Features = CallFeatures{
		VideoCall:         c.Type == CallTypeVideo,
		ScreenShare:       true,
		Recording:         false,
		ChatDuringCall:    true,
		FileSharing:       true,
		BackgroundEffects: true,
		NoiseReduction:    true,
		EchoCancellation:  true,
	}

	// Set default settings
	c.Settings = CallSettings{
		MaxParticipants:    10,
		RequirePermission:  false,
		AutoAccept:         false,
		RecordingEnabled:   false,
		MaxVideoBitrate:    2000000, // 2 Mbps
		MaxAudioBitrate:    128000,  // 128 kbps
		AdaptiveQuality:    true,
		EndToEndEncryption: true,
		RequirePassword:    false,
	}

	// Initialize analytics
	c.Analytics = CallAnalytics{
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

	// Initialize arrays
	c.Reports = []CallReport{}
}

// BeforeUpdate sets updated timestamp
func (c *Call) BeforeUpdate() {
	c.UpdatedAt = time.Now()
}

// AddParticipant adds a participant to the call
func (c *Call) AddParticipant(userID primitive.ObjectID, role ParticipantRole) *CallParticipant {
	participant := CallParticipant{
		UserID: userID,
		Status: ParticipantStatusInvited,
		Role:   role,
		MediaState: MediaState{
			VideoEnabled:      c.Type == CallTypeVideo,
			AudioEnabled:      true,
			ScreenSharing:     false,
			RecordingLocal:    false,
			VideoQuality:      VideoQualityMedium,
			VideoResolution:   Resolution{Width: 1280, Height: 720},
			VideoFrameRate:    30,
			AudioQuality:      AudioQualityMedium,
			AudioCodec:        "opus",
			NetworkAdaptation: true,
		},
		DeviceInfo: DeviceInfo{
			SupportsVideo:       true,
			SupportsAudio:       true,
			SupportsScreenShare: true,
		},
		Settings: ParticipantSettings{
			AutoMute:         false,
			AutoVideo:        c.Type == CallTypeVideo,
			NoiseReduction:   true,
			EchoCancellation: true,
			BackgroundBlur:   false,
		},
	}

	c.Participants = append(c.Participants, participant)
	c.BeforeUpdate()
	return &participant
}

// GetParticipant returns a participant by user ID
func (c *Call) GetParticipant(userID primitive.ObjectID) *CallParticipant {
	for i := range c.Participants {
		if c.Participants[i].UserID == userID {
			return &c.Participants[i]
		}
	}
	return nil
}

// UpdateParticipantStatus updates a participant's status
func (c *Call) UpdateParticipantStatus(userID primitive.ObjectID, status ParticipantStatus) {
	for i := range c.Participants {
		if c.Participants[i].UserID == userID {
			c.Participants[i].Status = status

			if status == ParticipantStatusConnected && c.Participants[i].JoinedAt == nil {
				now := time.Now()
				c.Participants[i].JoinedAt = &now

				// Start call if this is the first connection
				if c.Status == CallStatusRinging {
					c.StartCall()
				}
			} else if status == ParticipantStatusDisconnected && c.Participants[i].LeftAt == nil {
				now := time.Now()
				c.Participants[i].LeftAt = &now

				if c.Participants[i].JoinedAt != nil {
					c.Participants[i].Duration = int64(now.Sub(*c.Participants[i].JoinedAt).Seconds())
				}
			}

			break
		}
	}
	c.BeforeUpdate()
}

// StartCall starts the call
func (c *Call) StartCall() {
	if c.StartedAt == nil {
		now := time.Now()
		c.StartedAt = &now
		c.Status = CallStatusActive
		c.BeforeUpdate()
	}
}

// EndCall ends the call
func (c *Call) EndCall(endedBy primitive.ObjectID, reason EndReason) {
	if c.EndedAt == nil {
		now := time.Now()
		c.EndedAt = &now
		c.EndedBy = &endedBy
		c.EndReason = reason
		c.Status = CallStatusEnded

		// Calculate duration
		if c.StartedAt != nil {
			c.Duration = int64(now.Sub(*c.StartedAt).Seconds())
		} else {
			c.Duration = int64(now.Sub(c.InitiatedAt).Seconds())
		}

		// Update all connected participants
		for i := range c.Participants {
			if c.Participants[i].Status == ParticipantStatusConnected {
				c.Participants[i].Status = ParticipantStatusDisconnected
				c.Participants[i].LeftAt = &now

				if c.Participants[i].JoinedAt != nil {
					c.Participants[i].Duration = int64(now.Sub(*c.Participants[i].JoinedAt).Seconds())
				}
			}
		}

		c.BeforeUpdate()
	}
}

// GetConnectedParticipants returns list of connected participants
func (c *Call) GetConnectedParticipants() []CallParticipant {
	var connected []CallParticipant
	for _, participant := range c.Participants {
		if participant.Status == ParticipantStatusConnected {
			connected = append(connected, participant)
		}
	}
	return connected
}

// IsActive checks if the call is currently active
func (c *Call) IsActive() bool {
	return c.Status == CallStatusActive || c.Status == CallStatusRinging || c.Status == CallStatusConnecting
}

// CanUserJoin checks if a user can join the call
func (c *Call) CanUserJoin(userID primitive.ObjectID) bool {
	if !c.IsActive() {
		return false
	}

	// Check if user is already a participant
	participant := c.GetParticipant(userID)
	if participant == nil {
		return false
	}

	// Check if max participants reached
	connectedCount := len(c.GetConnectedParticipants())
	if connectedCount >= c.Settings.MaxParticipants {
		return false
	}

	return true
}

// StartRecording starts call recording
func (c *Call) StartRecording(recordedBy primitive.ObjectID) {
	if c.Recording == nil {
		now := time.Now()
		c.Recording = &RecordingInfo{
			IsRecording: true,
			StartedAt:   &now,
			Quality:     VideoQualityHigh,
			Format:      "mp4",
			IsPublic:    false,
			RecordedBy:  recordedBy,
		}
		c.BeforeUpdate()
	}
}

// StopRecording stops call recording
func (c *Call) StopRecording() {
	if c.Recording != nil && c.Recording.IsRecording {
		now := time.Now()
		c.Recording.IsRecording = false
		c.Recording.EndedAt = &now

		if c.Recording.StartedAt != nil {
			c.Recording.Duration = int64(now.Sub(*c.Recording.StartedAt).Seconds())
		}

		c.BeforeUpdate()
	}
}

// UpdateQuality updates call quality metrics
func (c *Call) UpdateQuality(participantID primitive.ObjectID, metrics QualityMetrics) {
	participant := c.GetParticipant(participantID)
	if participant != nil {
		participant.QualityMetrics = metrics
		c.BeforeUpdate()
	}
}

// AddQualityIssue adds a quality issue
func (c *Call) AddQualityIssue(issueType, severity, description string) {
	issue := QualityIssue{
		Type:        issueType,
		Severity:    severity,
		Timestamp:   time.Now(),
		Description: description,
	}

	c.Quality.Issues = append(c.Quality.Issues, issue)
	c.BeforeUpdate()
}

// GetCallSummary returns a summary of the call
func (c *Call) GetCallSummary() map[string]interface{} {
	return map[string]interface{}{
		"id":                c.ID,
		"type":              c.Type,
		"status":            c.Status,
		"duration":          c.Duration,
		"participant_count": len(c.Participants),
		"quality_rating":    c.Quality.OverallRating,
		"features_used":     c.Analytics.FeaturesUsed,
	}
}
