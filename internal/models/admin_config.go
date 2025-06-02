package models

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type AdminConfig struct {
	ID primitive.ObjectID `bson:"_id,omitempty" json:"id"`

	// Application Settings
	AppSettings AppSettings `bson:"app_settings" json:"app_settings"`

	// Server Configuration
	ServerConfig ServerConfig `bson:"server_config" json:"server_config"`

	// Database Settings
	DatabaseConfig DatabaseConfig `bson:"database_config" json:"database_config"`

	// Authentication & Security
	AuthConfig     AuthConfig     `bson:"auth_config" json:"auth_config"`
	SecurityConfig SecurityConfig `bson:"security_config" json:"security_config"`

	// Communication Services
	SMSConfig   SMSServiceConfig   `bson:"sms_config" json:"sms_config"`
	EmailConfig EmailServiceConfig `bson:"email_config" json:"email_config"`
	PushConfig  PushServiceConfig  `bson:"push_config" json:"push_config"`

	// Media & File Storage
	StorageConfig StorageServiceConfig `bson:"storage_config" json:"storage_config"`
	MediaConfig   MediaServiceConfig   `bson:"media_config" json:"media_config"`

	// WebRTC & Calling
	WebRTCConfig WebRTCServiceConfig `bson:"webrtc_config" json:"webrtc_config"`
	CallConfig   CallServiceConfig   `bson:"call_config" json:"call_config"`

	// Feature Flags
	FeatureFlags SystemFeatureFlags `bson:"feature_flags" json:"feature_flags"`

	// User Management
	UserSettings UserManagementSettings `bson:"user_settings" json:"user_settings"`

	// Content Moderation
	ModerationConfig ModerationConfig `bson:"moderation_config" json:"moderation_config"`

	// Analytics & Monitoring
	AnalyticsConfig  AnalyticsConfig  `bson:"analytics_config" json:"analytics_config"`
	MonitoringConfig MonitoringConfig `bson:"monitoring_config" json:"monitoring_config"`

	// Business & Billing
	BusinessConfig BusinessConfig `bson:"business_config" json:"business_config"`
	BillingConfig  BillingConfig  `bson:"billing_config" json:"billing_config"`

	// Notifications & Alerts
	NotificationConfig NotificationConfig `bson:"notification_config" json:"notification_config"`

	// Backup & Recovery
	BackupConfig BackupConfig `bson:"backup_config" json:"backup_config"`

	// Compliance & Legal
	ComplianceConfig ComplianceConfig `bson:"compliance_config" json:"compliance_config"`

	// UI/UX Customization
	UIConfig UICustomizationConfig `bson:"ui_config" json:"ui_config"`

	// API & Integration
	APIConfig         APIServiceConfig  `bson:"api_config" json:"api_config"`
	IntegrationConfig IntegrationConfig `bson:"integration_config" json:"integration_config"`

	// Maintenance & Updates
	MaintenanceConfig MaintenanceConfig `bson:"maintenance_config" json:"maintenance_config"`

	// Version and Metadata
	ConfigVersion string             `bson:"config_version" json:"config_version"`
	LastUpdatedBy primitive.ObjectID `bson:"last_updated_by" json:"last_updated_by"`
	LastUpdatedAt time.Time          `bson:"last_updated_at" json:"last_updated_at"`
	CreatedAt     time.Time          `bson:"created_at" json:"created_at"`

	// Environment
	Environment string `bson:"environment" json:"environment"` // "development", "staging", "production"

	// Custom Fields
	CustomSettings map[string]interface{} `bson:"custom_settings,omitempty" json:"custom_settings,omitempty"`
}

type AppSettings struct {
	AppName            string   `bson:"app_name" json:"app_name"`
	AppVersion         string   `bson:"app_version" json:"app_version"`
	AppDescription     string   `bson:"app_description" json:"app_description"`
	AppLogo            string   `bson:"app_logo" json:"app_logo"`
	AppIcon            string   `bson:"app_icon" json:"app_icon"`
	WebsiteURL         string   `bson:"website_url" json:"website_url"`
	SupportEmail       string   `bson:"support_email" json:"support_email"`
	SupportPhone       string   `bson:"support_phone" json:"support_phone"`
	TermsURL           string   `bson:"terms_url" json:"terms_url"`
	PrivacyURL         string   `bson:"privacy_url" json:"privacy_url"`
	DefaultLanguage    string   `bson:"default_language" json:"default_language"`
	SupportedLanguages []string `bson:"supported_languages" json:"supported_languages"`
	Timezone           string   `bson:"timezone" json:"timezone"`
	MaintenanceMode    bool     `bson:"maintenance_mode" json:"maintenance_mode"`
	MaintenanceMessage string   `bson:"maintenance_message" json:"maintenance_message"`
}

type ServerConfig struct {
	ServerName       string        `bson:"server_name" json:"server_name"`
	MaxConnections   int           `bson:"max_connections" json:"max_connections"`
	RequestTimeout   time.Duration `bson:"request_timeout" json:"request_timeout"`
	KeepAliveTimeout time.Duration `bson:"keep_alive_timeout" json:"keep_alive_timeout"`
	ReadTimeout      time.Duration `bson:"read_timeout" json:"read_timeout"`
	WriteTimeout     time.Duration `bson:"write_timeout" json:"write_timeout"`
	MaxRequestSize   int64         `bson:"max_request_size" json:"max_request_size"`
	EnableHTTPS      bool          `bson:"enable_https" json:"enable_https"`
	SSLCertPath      string        `bson:"ssl_cert_path" json:"ssl_cert_path"`
	SSLKeyPath       string        `bson:"ssl_key_path" json:"ssl_key_path"`
	CORSOrigins      []string      `bson:"cors_origins" json:"cors_origins"`
	CORSMethods      []string      `bson:"cors_methods" json:"cors_methods"`
	CORSHeaders      []string      `bson:"cors_headers" json:"cors_headers"`
}

type DatabaseConfig struct {
	MaxPoolSize        int           `bson:"max_pool_size" json:"max_pool_size"`
	MinPoolSize        int           `bson:"min_pool_size" json:"min_pool_size"`
	MaxIdleTime        time.Duration `bson:"max_idle_time" json:"max_idle_time"`
	ConnectTimeout     time.Duration `bson:"connect_timeout" json:"connect_timeout"`
	HeartbeatInterval  time.Duration `bson:"heartbeat_interval" json:"heartbeat_interval"`
	EnableLogging      bool          `bson:"enable_logging" json:"enable_logging"`
	SlowQueryThreshold time.Duration `bson:"slow_query_threshold" json:"slow_query_threshold"`
	BackupSchedule     string        `bson:"backup_schedule" json:"backup_schedule"`
	RetentionPeriod    time.Duration `bson:"retention_period" json:"retention_period"`
	IndexOptimization  bool          `bson:"index_optimization" json:"index_optimization"`
}

type AuthConfig struct {
	JWTExpiration            time.Duration `bson:"jwt_expiration" json:"jwt_expiration"`
	RefreshTokenExpiration   time.Duration `bson:"refresh_token_expiration" json:"refresh_token_expiration"`
	OTPExpiration            time.Duration `bson:"otp_expiration" json:"otp_expiration"`
	MaxOTPAttempts           int           `bson:"max_otp_attempts" json:"max_otp_attempts"`
	PasswordMinLength        int           `bson:"password_min_length" json:"password_min_length"`
	PasswordRequireUppercase bool          `bson:"password_require_uppercase" json:"password_require_uppercase"`
	PasswordRequireLowercase bool          `bson:"password_require_lowercase" json:"password_require_lowercase"`
	PasswordRequireNumbers   bool          `bson:"password_require_numbers" json:"password_require_numbers"`
	PasswordRequireSymbols   bool          `bson:"password_require_symbols" json:"password_require_symbols"`
	SessionTimeout           time.Duration `bson:"session_timeout" json:"session_timeout"`
	MaxActiveSessions        int           `bson:"max_active_sessions" json:"max_active_sessions"`
	RequirePhoneVerification bool          `bson:"require_phone_verification" json:"require_phone_verification"`
	RequireEmailVerification bool          `bson:"require_email_verification" json:"require_email_verification"`
	EnableTwoFactor          bool          `bson:"enable_two_factor" json:"enable_two_factor"`
	RequireTwoFactor         bool          `bson:"require_two_factor" json:"require_two_factor"`
}

type SecurityConfig struct {
	EnableRateLimit      bool           `bson:"enable_rate_limit" json:"enable_rate_limit"`
	RateLimitRequests    int            `bson:"rate_limit_requests" json:"rate_limit_requests"`
	RateLimitWindow      time.Duration  `bson:"rate_limit_window" json:"rate_limit_window"`
	EnableIPWhitelist    bool           `bson:"enable_ip_whitelist" json:"enable_ip_whitelist"`
	IPWhitelist          []string       `bson:"ip_whitelist" json:"ip_whitelist"`
	EnableIPBlacklist    bool           `bson:"enable_ip_blacklist" json:"enable_ip_blacklist"`
	IPBlacklist          []string       `bson:"ip_blacklist" json:"ip_blacklist"`
	MaxLoginAttempts     int            `bson:"max_login_attempts" json:"max_login_attempts"`
	LoginLockoutDuration time.Duration  `bson:"login_lockout_duration" json:"login_lockout_duration"`
	EnableEncryption     bool           `bson:"enable_encryption" json:"enable_encryption"`
	EncryptionAlgorithm  string         `bson:"encryption_algorithm" json:"encryption_algorithm"`
	EnableAuditLog       bool           `bson:"enable_audit_log" json:"enable_audit_log"`
	AuditLogRetention    time.Duration  `bson:"audit_log_retention" json:"audit_log_retention"`
	EnableFirewall       bool           `bson:"enable_firewall" json:"enable_firewall"`
	FirewallRules        []FirewallRule `bson:"firewall_rules" json:"firewall_rules"`
}

type FirewallRule struct {
	Name      string   `bson:"name" json:"name"`
	Action    string   `bson:"action" json:"action"`     // "allow", "block"
	Protocol  string   `bson:"protocol" json:"protocol"` // "tcp", "udp", "http", "https"
	SourceIPs []string `bson:"source_ips" json:"source_ips"`
	Ports     []int    `bson:"ports" json:"ports"`
	Enabled   bool     `bson:"enabled" json:"enabled"`
	Priority  int      `bson:"priority" json:"priority"`
}

type SMSServiceConfig struct {
	Provider              string         `bson:"provider" json:"provider"` // "twilio", "nexmo", "aws_sns"
	TwilioConfig          TwilioSettings `bson:"twilio_config" json:"twilio_config"`
	NexmoConfig           NexmoSettings  `bson:"nexmo_config" json:"nexmo_config"`
	AWSConfig             AWSSNSSettings `bson:"aws_config" json:"aws_config"`
	DefaultCountryCode    string         `bson:"default_country_code" json:"default_country_code"`
	EnableDeliveryReports bool           `bson:"enable_delivery_reports" json:"enable_delivery_reports"`
	MaxRetryAttempts      int            `bson:"max_retry_attempts" json:"max_retry_attempts"`
	RetryDelay            time.Duration  `bson:"retry_delay" json:"retry_delay"`
}

type TwilioSettings struct {
	AccountSID          string `bson:"account_sid" json:"account_sid"`
	AuthToken           string `bson:"auth_token" json:"-"`
	FromNumber          string `bson:"from_number" json:"from_number"`
	MessagingServiceSID string `bson:"messaging_service_sid" json:"messaging_service_sid"`
	WebhookURL          string `bson:"webhook_url" json:"webhook_url"`
}

type NexmoSettings struct {
	APIKey     string `bson:"api_key" json:"api_key"`
	APISecret  string `bson:"api_secret" json:"-"`
	FromNumber string `bson:"from_number" json:"from_number"`
	WebhookURL string `bson:"webhook_url" json:"webhook_url"`
}

type AWSSNSSettings struct {
	AccessKeyID     string `bson:"access_key_id" json:"access_key_id"`
	SecretAccessKey string `bson:"secret_access_key" json:"-"`
	Region          string `bson:"region" json:"region"`
}

type EmailServiceConfig struct {
	Provider       string           `bson:"provider" json:"provider"` // "smtp", "sendgrid", "mailgun", "aws_ses"
	SMTPConfig     SMTPSettings     `bson:"smtp_config" json:"smtp_config"`
	SendGridConfig SendGridSettings `bson:"sendgrid_config" json:"sendgrid_config"`
	MailgunConfig  MailgunSettings  `bson:"mailgun_config" json:"mailgun_config"`
	AWSSESConfig   AWSSESSettings   `bson:"aws_ses_config" json:"aws_ses_config"`
	FromEmail      string           `bson:"from_email" json:"from_email"`
	FromName       string           `bson:"from_name" json:"from_name"`
	ReplyToEmail   string           `bson:"reply_to_email" json:"reply_to_email"`
	EnableHTML     bool             `bson:"enable_html" json:"enable_html"`
	EnableTracking bool             `bson:"enable_tracking" json:"enable_tracking"`
}

type SMTPSettings struct {
	Host      string `bson:"host" json:"host"`
	Port      int    `bson:"port" json:"port"`
	Username  string `bson:"username" json:"username"`
	Password  string `bson:"password" json:"-"`
	EnableTLS bool   `bson:"enable_tls" json:"enable_tls"`
	EnableSSL bool   `bson:"enable_ssl" json:"enable_ssl"`
}

type SendGridSettings struct {
	APIKey     string `bson:"api_key" json:"-"`
	TemplateID string `bson:"template_id" json:"template_id"`
	WebhookURL string `bson:"webhook_url" json:"webhook_url"`
}

type MailgunSettings struct {
	APIKey     string `bson:"api_key" json:"-"`
	Domain     string `bson:"domain" json:"domain"`
	WebhookURL string `bson:"webhook_url" json:"webhook_url"`
}

type AWSSESSettings struct {
	AccessKeyID     string `bson:"access_key_id" json:"access_key_id"`
	SecretAccessKey string `bson:"secret_access_key" json:"-"`
	Region          string `bson:"region" json:"region"`
}

type PushServiceConfig struct {
	Provider         string                `bson:"provider" json:"provider"` // "firebase", "onesignal", "apns"
	FirebaseConfig   FirebasePushSettings  `bson:"firebase_config" json:"firebase_config"`
	OneSignalConfig  OneSignalPushSettings `bson:"onesignal_config" json:"onesignal_config"`
	APNSConfig       APNSSettings          `bson:"apns_config" json:"apns_config"`
	EnableBadgeCount bool                  `bson:"enable_badge_count" json:"enable_badge_count"`
	EnableSound      bool                  `bson:"enable_sound" json:"enable_sound"`
	DefaultSound     string                `bson:"default_sound" json:"default_sound"`
	EnableVibration  bool                  `bson:"enable_vibration" json:"enable_vibration"`
	MaxRetryAttempts int                   `bson:"max_retry_attempts" json:"max_retry_attempts"`
	EnableAnalytics  bool                  `bson:"enable_analytics" json:"enable_analytics"`
}

type FirebasePushSettings struct {
	ProjectID    string `bson:"project_id" json:"project_id"`
	PrivateKeyID string `bson:"private_key_id" json:"private_key_id"`
	PrivateKey   string `bson:"private_key" json:"-"`
	ClientEmail  string `bson:"client_email" json:"client_email"`
	ClientID     string `bson:"client_id" json:"client_id"`
	AuthURI      string `bson:"auth_uri" json:"auth_uri"`
	TokenURI     string `bson:"token_uri" json:"token_uri"`
	DatabaseURL  string `bson:"database_url" json:"database_url"`
}

type OneSignalPushSettings struct {
	AppID      string `bson:"app_id" json:"app_id"`
	APIKey     string `bson:"api_key" json:"-"`
	RestAPIKey string `bson:"rest_api_key" json:"-"`
	WebhookURL string `bson:"webhook_url" json:"webhook_url"`
}

type APNSSettings struct {
	KeyID       string `bson:"key_id" json:"key_id"`
	TeamID      string `bson:"team_id" json:"team_id"`
	BundleID    string `bson:"bundle_id" json:"bundle_id"`
	PrivateKey  string `bson:"private_key" json:"-"`
	Environment string `bson:"environment" json:"environment"` // "development", "production"
}

type StorageServiceConfig struct {
	Provider          string               `bson:"provider" json:"provider"` // "local", "aws_s3", "gcp", "azure"
	LocalConfig       LocalStorageSettings `bson:"local_config" json:"local_config"`
	AWSS3Config       AWSS3Settings        `bson:"aws_s3_config" json:"aws_s3_config"`
	GCPConfig         GCPStorageSettings   `bson:"gcp_config" json:"gcp_config"`
	AzureConfig       AzureStorageSettings `bson:"azure_config" json:"azure_config"`
	MaxFileSize       int64                `bson:"max_file_size" json:"max_file_size"`
	AllowedTypes      []string             `bson:"allowed_types" json:"allowed_types"`
	EnableCompression bool                 `bson:"enable_compression" json:"enable_compression"`
	EnableEncryption  bool                 `bson:"enable_encryption" json:"enable_encryption"`
	CDNEnabled        bool                 `bson:"cdn_enabled" json:"cdn_enabled"`
	CDNBaseURL        string               `bson:"cdn_base_url" json:"cdn_base_url"`
}

type LocalStorageSettings struct {
	BasePath        string        `bson:"base_path" json:"base_path"`
	TempPath        string        `bson:"temp_path" json:"temp_path"`
	BackupPath      string        `bson:"backup_path" json:"backup_path"`
	MaxDiskUsage    int64         `bson:"max_disk_usage" json:"max_disk_usage"`
	CleanupInterval time.Duration `bson:"cleanup_interval" json:"cleanup_interval"`
}

type AWSS3Settings struct {
	AccessKeyID          string `bson:"access_key_id" json:"access_key_id"`
	SecretAccessKey      string `bson:"secret_access_key" json:"-"`
	Region               string `bson:"region" json:"region"`
	BucketName           string `bson:"bucket_name" json:"bucket_name"`
	CloudFrontURL        string `bson:"cloudfront_url" json:"cloudfront_url"`
	ServerSideEncryption bool   `bson:"server_side_encryption" json:"server_side_encryption"`
}

type GCPStorageSettings struct {
	ProjectID       string `bson:"project_id" json:"project_id"`
	CredentialsPath string `bson:"credentials_path" json:"credentials_path"`
	BucketName      string `bson:"bucket_name" json:"bucket_name"`
	CDNBaseURL      string `bson:"cdn_base_url" json:"cdn_base_url"`
}

type AzureStorageSettings struct {
	AccountName   string `bson:"account_name" json:"account_name"`
	AccountKey    string `bson:"account_key" json:"-"`
	ContainerName string `bson:"container_name" json:"container_name"`
	CDNBaseURL    string `bson:"cdn_base_url" json:"cdn_base_url"`
}

type MediaServiceConfig struct {
	EnableImageProcessing bool            `bson:"enable_image_processing" json:"enable_image_processing"`
	ImageProcessor        string          `bson:"image_processor" json:"image_processor"` // "ffmpeg", "imagemagick"
	MaxImageWidth         int             `bson:"max_image_width" json:"max_image_width"`
	MaxImageHeight        int             `bson:"max_image_height" json:"max_image_height"`
	ImageQuality          int             `bson:"image_quality" json:"image_quality"`
	EnableThumbnails      bool            `bson:"enable_thumbnails" json:"enable_thumbnails"`
	ThumbnailSizes        []ThumbnailSize `bson:"thumbnail_sizes" json:"thumbnail_sizes"`
	EnableVideoProcessing bool            `bson:"enable_video_processing" json:"enable_video_processing"`
	VideoProcessor        string          `bson:"video_processor" json:"video_processor"`
	MaxVideoDuration      time.Duration   `bson:"max_video_duration" json:"max_video_duration"`
	VideoCodec            string          `bson:"video_codec" json:"video_codec"`
	AudioCodec            string          `bson:"audio_codec" json:"audio_codec"`
	EnableAudioProcessing bool            `bson:"enable_audio_processing" json:"enable_audio_processing"`
	MaxAudioDuration      time.Duration   `bson:"max_audio_duration" json:"max_audio_duration"`
}

type ThumbnailSize struct {
	Name   string `bson:"name" json:"name"`
	Width  int    `bson:"width" json:"width"`
	Height int    `bson:"height" json:"height"`
}

type WebRTCServiceConfig struct {
	STUNServers          []STUNServer         `bson:"stun_servers" json:"stun_servers"`
	TURNServers          []TURNServerConfig   `bson:"turn_servers" json:"turn_servers"`
	COTURNConfig         COTURNConfigSettings `bson:"coturn_config" json:"coturn_config"`
	ICEConnectionTimeout time.Duration        `bson:"ice_connection_timeout" json:"ice_connection_timeout"`
	DTLSTimeout          time.Duration        `bson:"dtls_timeout" json:"dtls_timeout"`
	EnableIPv6           bool                 `bson:"enable_ipv6" json:"enable_ipv6"`
	MaxBitrate           int                  `bson:"max_bitrate" json:"max_bitrate"`
	MinBitrate           int                  `bson:"min_bitrate" json:"min_bitrate"`
	AdaptiveBitrate      bool                 `bson:"adaptive_bitrate" json:"adaptive_bitrate"`
	EnableRecording      bool                 `bson:"enable_recording" json:"enable_recording"`
	RecordingFormat      string               `bson:"recording_format" json:"recording_format"`
}

type STUNServer struct {
	URL string `bson:"url" json:"url"`
}

type TURNServerConfig struct {
	URLs       []string `bson:"urls" json:"urls"`
	Username   string   `bson:"username" json:"username"`
	Credential string   `bson:"credential" json:"-"`
}

type COTURNConfigSettings struct {
	Host            string        `bson:"host" json:"host"`
	Port            int           `bson:"port" json:"port"`
	AuthSecret      string        `bson:"auth_secret" json:"-"`
	Realm           string        `bson:"realm" json:"realm"`
	MinPort         int           `bson:"min_port" json:"min_port"`
	MaxPort         int           `bson:"max_port" json:"max_port"`
	Verbosity       int           `bson:"verbosity" json:"verbosity"`
	LogFile         string        `bson:"log_file" json:"log_file"`
	EnableLogging   bool          `bson:"enable_logging" json:"enable_logging"`
	SessionLifetime time.Duration `bson:"session_lifetime" json:"session_lifetime"`
}

type CallServiceConfig struct {
	MaxCallDuration      time.Duration       `bson:"max_call_duration" json:"max_call_duration"`
	MaxParticipants      int                 `bson:"max_participants" json:"max_participants"`
	EnableGroupCalls     bool                `bson:"enable_group_calls" json:"enable_group_calls"`
	EnableRecording      bool                `bson:"enable_recording" json:"enable_recording"`
	MaxRecordingDuration time.Duration       `bson:"max_recording_duration" json:"max_recording_duration"`
	CallTimeout          time.Duration       `bson:"call_timeout" json:"call_timeout"`
	RingingTimeout       time.Duration       `bson:"ringing_timeout" json:"ringing_timeout"`
	EnableCallWaiting    bool                `bson:"enable_call_waiting" json:"enable_call_waiting"`
	EnableCallForwarding bool                `bson:"enable_call_forwarding" json:"enable_call_forwarding"`
	QualitySettings      CallQualitySettings `bson:"quality_settings" json:"quality_settings"`
	EnableAnalytics      bool                `bson:"enable_analytics" json:"enable_analytics"`
	AnalyticsRetention   time.Duration       `bson:"analytics_retention" json:"analytics_retention"`
}

type CallQualitySettings struct {
	VideoMinBitrate  int  `bson:"video_min_bitrate" json:"video_min_bitrate"`
	VideoMaxBitrate  int  `bson:"video_max_bitrate" json:"video_max_bitrate"`
	AudioMinBitrate  int  `bson:"audio_min_bitrate" json:"audio_min_bitrate"`
	AudioMaxBitrate  int  `bson:"audio_max_bitrate" json:"audio_max_bitrate"`
	MinFrameRate     int  `bson:"min_frame_rate" json:"min_frame_rate"`
	MaxFrameRate     int  `bson:"max_frame_rate" json:"max_frame_rate"`
	AdaptiveQuality  bool `bson:"adaptive_quality" json:"adaptive_quality"`
	NoiseReduction   bool `bson:"noise_reduction" json:"noise_reduction"`
	EchoCancellation bool `bson:"echo_cancellation" json:"echo_cancellation"`
}

type SystemFeatureFlags struct {
	EnableRegistration      bool `bson:"enable_registration" json:"enable_registration"`
	EnableGuestMode         bool `bson:"enable_guest_mode" json:"enable_guest_mode"`
	EnableGroupChats        bool `bson:"enable_group_chats" json:"enable_group_chats"`
	EnableVoiceCalls        bool `bson:"enable_voice_calls" json:"enable_voice_calls"`
	EnableVideoCalls        bool `bson:"enable_video_calls" json:"enable_video_calls"`
	EnableScreenSharing     bool `bson:"enable_screen_sharing" json:"enable_screen_sharing"`
	EnableFileSharing       bool `bson:"enable_file_sharing" json:"enable_file_sharing"`
	EnableVoiceMessages     bool `bson:"enable_voice_messages" json:"enable_voice_messages"`
	EnableStickers          bool `bson:"enable_stickers" json:"enable_stickers"`
	EnableGifs              bool `bson:"enable_gifs" json:"enable_gifs"`
	EnableEmojis            bool `bson:"enable_emojis" json:"enable_emojis"`
	EnableReactions         bool `bson:"enable_reactions" json:"enable_reactions"`
	EnableStories           bool `bson:"enable_stories" json:"enable_stories"`
	EnableStatus            bool `bson:"enable_status" json:"enable_status"`
	EnableBroadcast         bool `bson:"enable_broadcast" json:"enable_broadcast"`
	EnableChannels          bool `bson:"enable_channels" json:"enable_channels"`
	EnableBots              bool `bson:"enable_bots" json:"enable_bots"`
	EnableThemes            bool `bson:"enable_themes" json:"enable_themes"`
	EnableDarkMode          bool `bson:"enable_dark_mode" json:"enable_dark_mode"`
	EnableMessageEncryption bool `bson:"enable_message_encryption" json:"enable_message_encryption"`
	EnableDeleteForEveryone bool `bson:"enable_delete_for_everyone" json:"enable_delete_for_everyone"`
	EnableMessageEditing    bool `bson:"enable_message_editing" json:"enable_message_editing"`
	EnableForwarding        bool `bson:"enable_forwarding" json:"enable_forwarding"`
	EnableBackup            bool `bson:"enable_backup" json:"enable_backup"`
	EnableExport            bool `bson:"enable_export" json:"enable_export"`
}

type UserManagementSettings struct {
	AutoDeleteInactiveUsers bool          `bson:"auto_delete_inactive_users" json:"auto_delete_inactive_users"`
	InactiveUserThreshold   time.Duration `bson:"inactive_user_threshold" json:"inactive_user_threshold"`
	MaxUsersPerGroup        int           `bson:"max_users_per_group" json:"max_users_per_group"`
	MaxGroupsPerUser        int           `bson:"max_groups_per_user" json:"max_groups_per_user"`
	MaxContactsPerUser      int           `bson:"max_contacts_per_user" json:"max_contacts_per_user"`
	RequireProfilePicture   bool          `bson:"require_profile_picture" json:"require_profile_picture"`
	AllowUsernameChange     bool          `bson:"allow_username_change" json:"allow_username_change"`
	UsernameChangeInterval  time.Duration `bson:"username_change_interval" json:"username_change_interval"`
	EnableUserSearch        bool          `bson:"enable_user_search" json:"enable_user_search"`
	EnableNearbyUsers       bool          `bson:"enable_nearby_users" json:"enable_nearby_users"`
	EnableUserReports       bool          `bson:"enable_user_reports" json:"enable_user_reports"`
	EnableUserBlocking      bool          `bson:"enable_user_blocking" json:"enable_user_blocking"`
}

type ModerationConfig struct {
	EnableAutoModeration    bool              `bson:"enable_auto_moderation" json:"enable_auto_moderation"`
	EnableProfanityFilter   bool              `bson:"enable_profanity_filter" json:"enable_profanity_filter"`
	ProfanityWordList       []string          `bson:"profanity_word_list" json:"profanity_word_list"`
	EnableSpamDetection     bool              `bson:"enable_spam_detection" json:"enable_spam_detection"`
	SpamDetectionRules      []SpamRule        `bson:"spam_detection_rules" json:"spam_detection_rules"`
	EnableImageModeration   bool              `bson:"enable_image_moderation" json:"enable_image_moderation"`
	ImageModerationProvider string            `bson:"image_moderation_provider" json:"image_moderation_provider"`
	EnableReportSystem      bool              `bson:"enable_report_system" json:"enable_report_system"`
	AutoActionOnReports     AutoActionSetting `bson:"auto_action_on_reports" json:"auto_action_on_reports"`
	ModerationLogRetention  time.Duration     `bson:"moderation_log_retention" json:"moderation_log_retention"`
}

type SpamRule struct {
	Name     string `bson:"name" json:"name"`
	Pattern  string `bson:"pattern" json:"pattern"`
	Action   string `bson:"action" json:"action"` // "warn", "mute", "ban"
	Severity int    `bson:"severity" json:"severity"`
	Enabled  bool   `bson:"enabled" json:"enabled"`
}

type AutoActionSetting struct {
	ReportThreshold int           `bson:"report_threshold" json:"report_threshold"`
	Action          string        `bson:"action" json:"action"` // "warn", "mute", "suspend", "ban"
	Duration        time.Duration `bson:"duration" json:"duration"`
}

type AnalyticsConfig struct {
	EnableAnalytics         bool          `bson:"enable_analytics" json:"enable_analytics"`
	AnalyticsProvider       string        `bson:"analytics_provider" json:"analytics_provider"` // "internal", "google", "mixpanel"
	TrackUserActivity       bool          `bson:"track_user_activity" json:"track_user_activity"`
	TrackMessageMetrics     bool          `bson:"track_message_metrics" json:"track_message_metrics"`
	TrackCallMetrics        bool          `bson:"track_call_metrics" json:"track_call_metrics"`
	TrackPerformanceMetrics bool          `bson:"track_performance_metrics" json:"track_performance_metrics"`
	DataRetentionPeriod     time.Duration `bson:"data_retention_period" json:"data_retention_period"`
	EnableDataExport        bool          `bson:"enable_data_export" json:"enable_data_export"`
	ExportSchedule          string        `bson:"export_schedule" json:"export_schedule"`
	PrivacyCompliant        bool          `bson:"privacy_compliant" json:"privacy_compliant"`
}

type MonitoringConfig struct {
	EnableLogging       bool          `bson:"enable_logging" json:"enable_logging"`
	LogLevel            string        `bson:"log_level" json:"log_level"`   // "debug", "info", "warn", "error"
	LogFormat           string        `bson:"log_format" json:"log_format"` // "json", "text"
	LogRotation         bool          `bson:"log_rotation" json:"log_rotation"`
	LogRetentionPeriod  time.Duration `bson:"log_retention_period" json:"log_retention_period"`
	EnableHealthChecks  bool          `bson:"enable_health_checks" json:"enable_health_checks"`
	HealthCheckInterval time.Duration `bson:"health_check_interval" json:"health_check_interval"`
	EnableMetrics       bool          `bson:"enable_metrics" json:"enable_metrics"`
	MetricsProvider     string        `bson:"metrics_provider" json:"metrics_provider"` // "prometheus", "datadog"
	EnableAlerting      bool          `bson:"enable_alerting" json:"enable_alerting"`
	AlertingRules       []AlertRule   `bson:"alerting_rules" json:"alerting_rules"`
	EnableUptime        bool          `bson:"enable_uptime" json:"enable_uptime"`
	UptimeCheckInterval time.Duration `bson:"uptime_check_interval" json:"uptime_check_interval"`
}

type AlertRule struct {
	Name      string        `bson:"name" json:"name"`
	Metric    string        `bson:"metric" json:"metric"`
	Condition string        `bson:"condition" json:"condition"` // "greater_than", "less_than", "equals"
	Threshold float64       `bson:"threshold" json:"threshold"`
	Duration  time.Duration `bson:"duration" json:"duration"`
	Severity  string        `bson:"severity" json:"severity"` // "low", "medium", "high", "critical"
	Actions   []AlertAction `bson:"actions" json:"actions"`
	Enabled   bool          `bson:"enabled" json:"enabled"`
}

type AlertAction struct {
	Type    string `bson:"type" json:"type"` // "email", "sms", "webhook", "slack"
	Target  string `bson:"target" json:"target"`
	Message string `bson:"message" json:"message"`
	Enabled bool   `bson:"enabled" json:"enabled"`
}

type BusinessConfig struct {
	CompanyName         string        `bson:"company_name" json:"company_name"`
	CompanyAddress      string        `bson:"company_address" json:"company_address"`
	CompanyPhone        string        `bson:"company_phone" json:"company_phone"`
	CompanyEmail        string        `bson:"company_email" json:"company_email"`
	CompanyWebsite      string        `bson:"company_website" json:"company_website"`
	BusinessModel       string        `bson:"business_model" json:"business_model"` // "freemium", "subscription", "enterprise"
	SupportHours        string        `bson:"support_hours" json:"support_hours"`
	SupportEmail        string        `bson:"support_email" json:"support_email"`
	SupportPhone        string        `bson:"support_phone" json:"support_phone"`
	LegalNotices        []LegalNotice `bson:"legal_notices" json:"legal_notices"`
	EnableGDPR          bool          `bson:"enable_gdpr" json:"enable_gdpr"`
	EnableCCPA          bool          `bson:"enable_ccpa" json:"enable_ccpa"`
	DataProcessingBasis string        `bson:"data_processing_basis" json:"data_processing_basis"`
}

type LegalNotice struct {
	Type          string    `bson:"type" json:"type"` // "terms", "privacy", "cookies", "gdpr"
	Title         string    `bson:"title" json:"title"`
	Content       string    `bson:"content" json:"content"`
	URL           string    `bson:"url" json:"url"`
	Version       string    `bson:"version" json:"version"`
	EffectiveDate time.Time `bson:"effective_date" json:"effective_date"`
	Mandatory     bool      `bson:"mandatory" json:"mandatory"`
}

type BillingConfig struct {
	EnableBilling     bool               `bson:"enable_billing" json:"enable_billing"`
	BillingProvider   string             `bson:"billing_provider" json:"billing_provider"` // "stripe", "paypal", "square"
	Currency          string             `bson:"currency" json:"currency"`
	TaxRate           float64            `bson:"tax_rate" json:"tax_rate"`
	SubscriptionPlans []SubscriptionPlan `bson:"subscription_plans" json:"subscription_plans"`
	EnableFreeTrial   bool               `bson:"enable_free_trial" json:"enable_free_trial"`
	FreeTrialDuration time.Duration      `bson:"free_trial_duration" json:"free_trial_duration"`
	GracePeriod       time.Duration      `bson:"grace_period" json:"grace_period"`
	EnableRefunds     bool               `bson:"enable_refunds" json:"enable_refunds"`
	RefundPeriod      time.Duration      `bson:"refund_period" json:"refund_period"`
	InvoiceRetention  time.Duration      `bson:"invoice_retention" json:"invoice_retention"`
}

type SubscriptionPlan struct {
	ID             string   `bson:"id" json:"id"`
	Name           string   `bson:"name" json:"name"`
	Description    string   `bson:"description" json:"description"`
	Price          float64  `bson:"price" json:"price"`
	Currency       string   `bson:"currency" json:"currency"`
	Interval       string   `bson:"interval" json:"interval"` // "month", "year"
	Features       []string `bson:"features" json:"features"`
	MaxUsers       int      `bson:"max_users" json:"max_users"`
	MaxStorage     int64    `bson:"max_storage" json:"max_storage"`
	MaxCallMinutes int      `bson:"max_call_minutes" json:"max_call_minutes"`
	IsActive       bool     `bson:"is_active" json:"is_active"`
	IsPopular      bool     `bson:"is_popular" json:"is_popular"`
	SortOrder      int      `bson:"sort_order" json:"sort_order"`
}

type NotificationConfig struct {
	EnableEmailNotifications bool                       `bson:"enable_email_notifications" json:"enable_email_notifications"`
	EmailTemplates           []EmailTemplate            `bson:"email_templates" json:"email_templates"`
	EnableSMSNotifications   bool                       `bson:"enable_sms_notifications" json:"enable_sms_notifications"`
	SMSTemplates             []SMSTemplate              `bson:"sms_templates" json:"sms_templates"`
	EnablePushNotifications  bool                       `bson:"enable_push_notifications" json:"enable_push_notifications"`
	PushTemplates            []PushNotificationTemplate `bson:"push_templates" json:"push_templates"`
	EnableInAppNotifications bool                       `bson:"enable_in_app_notifications" json:"enable_in_app_notifications"`
	NotificationRetention    time.Duration              `bson:"notification_retention" json:"notification_retention"`
	EnableNotificationQueue  bool                       `bson:"enable_notification_queue" json:"enable_notification_queue"`
	BatchSize                int                        `bson:"batch_size" json:"batch_size"`
	BatchInterval            time.Duration              `bson:"batch_interval" json:"batch_interval"`
}

type EmailTemplate struct {
	ID          string   `bson:"id" json:"id"`
	Name        string   `bson:"name" json:"name"`
	Subject     string   `bson:"subject" json:"subject"`
	HTMLContent string   `bson:"html_content" json:"html_content"`
	TextContent string   `bson:"text_content" json:"text_content"`
	Variables   []string `bson:"variables" json:"variables"`
	IsActive    bool     `bson:"is_active" json:"is_active"`
}

type SMSTemplate struct {
	ID        string   `bson:"id" json:"id"`
	Name      string   `bson:"name" json:"name"`
	Content   string   `bson:"content" json:"content"`
	Variables []string `bson:"variables" json:"variables"`
	IsActive  bool     `bson:"is_active" json:"is_active"`
}

type PushNotificationTemplate struct {
	ID        string   `bson:"id" json:"id"`
	Name      string   `bson:"name" json:"name"`
	Title     string   `bson:"title" json:"title"`
	Body      string   `bson:"body" json:"body"`
	Icon      string   `bson:"icon" json:"icon"`
	Sound     string   `bson:"sound" json:"sound"`
	Variables []string `bson:"variables" json:"variables"`
	IsActive  bool     `bson:"is_active" json:"is_active"`
}

type BackupConfig struct {
	EnableBackup       bool          `bson:"enable_backup" json:"enable_backup"`
	BackupProvider     string        `bson:"backup_provider" json:"backup_provider"`   // "local", "aws_s3", "gcp", "azure"
	BackupFrequency    string        `bson:"backup_frequency" json:"backup_frequency"` // "daily", "weekly", "monthly"
	BackupTime         string        `bson:"backup_time" json:"backup_time"`           // "02:00" (24h format)
	RetentionPeriod    time.Duration `bson:"retention_period" json:"retention_period"`
	EncryptBackups     bool          `bson:"encrypt_backups" json:"encrypt_backups"`
	CompressBackups    bool          `bson:"compress_backups" json:"compress_backups"`
	BackupLocation     string        `bson:"backup_location" json:"backup_location"`
	EnableReplication  bool          `bson:"enable_replication" json:"enable_replication"`
	ReplicationTargets []string      `bson:"replication_targets" json:"replication_targets"`
	EnableMonitoring   bool          `bson:"enable_monitoring" json:"enable_monitoring"`
	AlertOnFailure     bool          `bson:"alert_on_failure" json:"alert_on_failure"`
}

type ComplianceConfig struct {
	EnableGDPR            bool          `bson:"enable_gdpr" json:"enable_gdpr"`
	EnableCCPA            bool          `bson:"enable_ccpa" json:"enable_ccpa"`
	EnableHIPAA           bool          `bson:"enable_hipaa" json:"enable_hipaa"`
	DataRetentionPeriod   time.Duration `bson:"data_retention_period" json:"data_retention_period"`
	EnableDataDeletion    bool          `bson:"enable_data_deletion" json:"enable_data_deletion"`
	EnableDataExport      bool          `bson:"enable_data_export" json:"enable_data_export"`
	EnableAuditLog        bool          `bson:"enable_audit_log" json:"enable_audit_log"`
	AuditLogRetention     time.Duration `bson:"audit_log_retention" json:"audit_log_retention"`
	RequireConsent        bool          `bson:"require_consent" json:"require_consent"`
	ConsentTypes          []ConsentType `bson:"consent_types" json:"consent_types"`
	EnableRightToErasure  bool          `bson:"enable_right_to_erasure" json:"enable_right_to_erasure"`
	EnableDataPortability bool          `bson:"enable_data_portability" json:"enable_data_portability"`
}

type ConsentType struct {
	ID          string `bson:"id" json:"id"`
	Name        string `bson:"name" json:"name"`
	Description string `bson:"description" json:"description"`
	Required    bool   `bson:"required" json:"required"`
	IsActive    bool   `bson:"is_active" json:"is_active"`
}

type UICustomizationConfig struct {
	BrandName           string `bson:"brand_name" json:"brand_name"`
	BrandLogo           string `bson:"brand_logo" json:"brand_logo"`
	BrandIcon           string `bson:"brand_icon" json:"brand_icon"`
	PrimaryColor        string `bson:"primary_color" json:"primary_color"`
	SecondaryColor      string `bson:"secondary_color" json:"secondary_color"`
	AccentColor         string `bson:"accent_color" json:"accent_color"`
	BackgroundColor     string `bson:"background_color" json:"background_color"`
	TextColor           string `bson:"text_color" json:"text_color"`
	EnableDarkTheme     bool   `bson:"enable_dark_theme" json:"enable_dark_theme"`
	DefaultTheme        string `bson:"default_theme" json:"default_theme"` // "light", "dark", "auto"
	CustomCSS           string `bson:"custom_css" json:"custom_css"`
	CustomJavaScript    string `bson:"custom_javascript" json:"custom_javascript"`
	Favicon             string `bson:"favicon" json:"favicon"`
	LoginPageBackground string `bson:"login_page_background" json:"login_page_background"`
	WelcomeMessage      string `bson:"welcome_message" json:"welcome_message"`
	FooterText          string `bson:"footer_text" json:"footer_text"`
	ShowBranding        bool   `bson:"show_branding" json:"show_branding"`
}

type APIServiceConfig struct {
	EnableAPIAccess   bool          `bson:"enable_api_access" json:"enable_api_access"`
	APIVersion        string        `bson:"api_version" json:"api_version"`
	EnableAPIKeys     bool          `bson:"enable_api_keys" json:"enable_api_keys"`
	APIKeyExpiration  time.Duration `bson:"api_key_expiration" json:"api_key_expiration"`
	EnableRateLimit   bool          `bson:"enable_rate_limit" json:"enable_rate_limit"`
	RateLimitRequests int           `bson:"rate_limit_requests" json:"rate_limit_requests"`
	RateLimitWindow   time.Duration `bson:"rate_limit_window" json:"rate_limit_window"`
	EnableAPILogging  bool          `bson:"enable_api_logging" json:"enable_api_logging"`
	EnableCORS        bool          `bson:"enable_cors" json:"enable_cors"`
	AllowedOrigins    []string      `bson:"allowed_origins" json:"allowed_origins"`
	EnableWebhooks    bool          `bson:"enable_webhooks" json:"enable_webhooks"`
	WebhookEvents     []string      `bson:"webhook_events" json:"webhook_events"`
	MaxRequestSize    int64         `bson:"max_request_size" json:"max_request_size"`
	RequestTimeout    time.Duration `bson:"request_timeout" json:"request_timeout"`
}

type IntegrationConfig struct {
	EnableWebhooks       bool                 `bson:"enable_webhooks" json:"enable_webhooks"`
	WebhookEndpoints     []WebhookEndpoint    `bson:"webhook_endpoints" json:"webhook_endpoints"`
	EnableZapier         bool                 `bson:"enable_zapier" json:"enable_zapier"`
	EnableSlack          bool                 `bson:"enable_slack" json:"enable_slack"`
	SlackConfig          SlackIntegration     `bson:"slack_config" json:"slack_config"`
	EnableMicrosoftTeams bool                 `bson:"enable_microsoft_teams" json:"enable_microsoft_teams"`
	TeamsConfig          TeamsIntegration     `bson:"teams_config" json:"teams_config"`
	EnableGoogle         bool                 `bson:"enable_google" json:"enable_google"`
	GoogleConfig         GoogleIntegration    `bson:"google_config" json:"google_config"`
	EnableOffice365      bool                 `bson:"enable_office365" json:"enable_office365"`
	Office365Config      Office365Integration `bson:"office365_config" json:"office365_config"`
}

type WebhookEndpoint struct {
	ID         string        `bson:"id" json:"id"`
	Name       string        `bson:"name" json:"name"`
	URL        string        `bson:"url" json:"url"`
	Events     []string      `bson:"events" json:"events"`
	Secret     string        `bson:"secret" json:"-"`
	IsActive   bool          `bson:"is_active" json:"is_active"`
	RetryCount int           `bson:"retry_count" json:"retry_count"`
	Timeout    time.Duration `bson:"timeout" json:"timeout"`
}

type SlackIntegration struct {
	ClientID     string `bson:"client_id" json:"client_id"`
	ClientSecret string `bson:"client_secret" json:"-"`
	WebhookURL   string `bson:"webhook_url" json:"webhook_url"`
	BotToken     string `bson:"bot_token" json:"-"`
}

type TeamsIntegration struct {
	AppID      string `bson:"app_id" json:"app_id"`
	AppSecret  string `bson:"app_secret" json:"-"`
	WebhookURL string `bson:"webhook_url" json:"webhook_url"`
}

type GoogleIntegration struct {
	ClientID     string `bson:"client_id" json:"client_id"`
	ClientSecret string `bson:"client_secret" json:"-"`
	ProjectID    string `bson:"project_id" json:"project_id"`
	WebhookURL   string `bson:"webhook_url" json:"webhook_url"`
}

type Office365Integration struct {
	ClientID     string `bson:"client_id" json:"client_id"`
	ClientSecret string `bson:"client_secret" json:"-"`
	TenantID     string `bson:"tenant_id" json:"tenant_id"`
	WebhookURL   string `bson:"webhook_url" json:"webhook_url"`
}

type MaintenanceConfig struct {
	EnableMaintenance  bool       `bson:"enable_maintenance" json:"enable_maintenance"`
	MaintenanceMessage string     `bson:"maintenance_message" json:"maintenance_message"`
	MaintenanceStart   *time.Time `bson:"maintenance_start,omitempty" json:"maintenance_start,omitempty"`
	MaintenanceEnd     *time.Time `bson:"maintenance_end,omitempty" json:"maintenance_end,omitempty"`
	AllowedIPs         []string   `bson:"allowed_ips" json:"allowed_ips"`
	EnableAutoUpdates  bool       `bson:"enable_auto_updates" json:"enable_auto_updates"`
	UpdateSchedule     string     `bson:"update_schedule" json:"update_schedule"`
	UpdateNotification bool       `bson:"update_notification" json:"update_notification"`
	BackupBeforeUpdate bool       `bson:"backup_before_update" json:"backup_before_update"`
	RollbackEnabled    bool       `bson:"rollback_enabled" json:"rollback_enabled"`
}

// Request/Response structs

type UpdateConfigRequest struct {
	Section string      `json:"section" validate:"required"`
	Config  interface{} `json:"config" validate:"required"`
}

type ConfigResponse struct {
	AdminConfig
	LastUpdatedByUser UserPublicInfo `json:"last_updated_by_user,omitempty"`
}

type ConfigSectionResponse struct {
	Section string      `json:"section"`
	Config  interface{} `json:"config"`
}

// Helper methods

// BeforeCreate sets default values before creating config
func (ac *AdminConfig) BeforeCreate() {
	now := time.Now()
	ac.CreatedAt = now
	ac.LastUpdatedAt = now
	ac.ConfigVersion = "1.0.0"
	ac.Environment = "production"

	// Set default values for all configurations
	ac.setDefaults()
}

// BeforeUpdate sets updated timestamp
func (ac *AdminConfig) BeforeUpdate() {
	ac.LastUpdatedAt = time.Now()
}

// setDefaults sets default values for all configuration sections
func (ac *AdminConfig) setDefaults() {
	// App Settings defaults
	ac.AppSettings = AppSettings{
		AppName:            "ChatApp",
		AppVersion:         "1.0.0",
		AppDescription:     "A modern chat application",
		DefaultLanguage:    "en",
		SupportedLanguages: []string{"en", "es", "fr", "de"},
		Timezone:           "UTC",
		MaintenanceMode:    false,
	}

	// Server Config defaults
	ac.ServerConfig = ServerConfig{
		MaxConnections:   10000,
		RequestTimeout:   30 * time.Second,
		KeepAliveTimeout: 60 * time.Second,
		ReadTimeout:      10 * time.Second,
		WriteTimeout:     10 * time.Second,
		MaxRequestSize:   10 * 1024 * 1024, // 10MB
		EnableHTTPS:      true,
		CORSOrigins:      []string{"*"},
		CORSMethods:      []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		CORSHeaders:      []string{"Origin", "Content-Type", "Authorization"},
	}

	// Feature Flags defaults
	ac.FeatureFlags = SystemFeatureFlags{
		EnableRegistration:      true,
		EnableGuestMode:         false,
		EnableGroupChats:        true,
		EnableVoiceCalls:        true,
		EnableVideoCalls:        true,
		EnableScreenSharing:     true,
		EnableFileSharing:       true,
		EnableVoiceMessages:     true,
		EnableStickers:          true,
		EnableGifs:              true,
		EnableEmojis:            true,
		EnableReactions:         true,
		EnableStories:           false,
		EnableStatus:            true,
		EnableBroadcast:         true,
		EnableChannels:          false,
		EnableBots:              false,
		EnableThemes:            true,
		EnableDarkMode:          true,
		EnableMessageEncryption: true,
		EnableDeleteForEveryone: true,
		EnableMessageEditing:    true,
		EnableForwarding:        true,
		EnableBackup:            true,
		EnableExport:            true,
	}

	// Initialize other default configurations
	ac.setDefaultSecurity()
	ac.setDefaultAuth()
	ac.setDefaultStorage()
	ac.setDefaultWebRTC()
	ac.setDefaultModeration()
	ac.setDefaultAnalytics()
	ac.setDefaultMonitoring()
	ac.setDefaultNotifications()
	ac.setDefaultUI()
}

func (ac *AdminConfig) setDefaultSecurity() {
	ac.SecurityConfig = SecurityConfig{
		EnableRateLimit:      true,
		RateLimitRequests:    100,
		RateLimitWindow:      time.Minute,
		EnableIPWhitelist:    false,
		EnableIPBlacklist:    false,
		MaxLoginAttempts:     5,
		LoginLockoutDuration: 15 * time.Minute,
		EnableEncryption:     true,
		EncryptionAlgorithm:  "AES-256-GCM",
		EnableAuditLog:       true,
		AuditLogRetention:    90 * 24 * time.Hour,
		EnableFirewall:       false,
	}
}

func (ac *AdminConfig) setDefaultAuth() {
	ac.AuthConfig = AuthConfig{
		JWTExpiration:            24 * time.Hour,
		RefreshTokenExpiration:   30 * 24 * time.Hour,
		OTPExpiration:            10 * time.Minute,
		MaxOTPAttempts:           3,
		PasswordMinLength:        8,
		PasswordRequireUppercase: true,
		PasswordRequireLowercase: true,
		PasswordRequireNumbers:   true,
		PasswordRequireSymbols:   false,
		SessionTimeout:           24 * time.Hour,
		MaxActiveSessions:        5,
		RequirePhoneVerification: true,
		RequireEmailVerification: false,
		EnableTwoFactor:          true,
		RequireTwoFactor:         false,
	}
}

func (ac *AdminConfig) setDefaultStorage() {
	ac.StorageConfig = StorageServiceConfig{
		Provider:          "local",
		MaxFileSize:       50 * 1024 * 1024, // 50MB
		AllowedTypes:      []string{"image/jpeg", "image/png", "image/gif", "video/mp4", "audio/mpeg", "application/pdf"},
		EnableCompression: true,
		EnableEncryption:  true,
		CDNEnabled:        false,
	}
}

func (ac *AdminConfig) setDefaultWebRTC() {
	ac.WebRTCConfig = WebRTCServiceConfig{
		ICEConnectionTimeout: 30 * time.Second,
		DTLSTimeout:          10 * time.Second,
		EnableIPv6:           true,
		MaxBitrate:           2000000, // 2 Mbps
		MinBitrate:           100000,  // 100 kbps
		AdaptiveBitrate:      true,
		EnableRecording:      true,
		RecordingFormat:      "mp4",
	}
}

func (ac *AdminConfig) setDefaultModeration() {
	ac.ModerationConfig = ModerationConfig{
		EnableAutoModeration:   true,
		EnableProfanityFilter:  true,
		EnableSpamDetection:    true,
		EnableImageModeration:  false,
		EnableReportSystem:     true,
		ModerationLogRetention: 90 * 24 * time.Hour,
	}
}

func (ac *AdminConfig) setDefaultAnalytics() {
	ac.AnalyticsConfig = AnalyticsConfig{
		EnableAnalytics:         true,
		AnalyticsProvider:       "internal",
		TrackUserActivity:       true,
		TrackMessageMetrics:     true,
		TrackCallMetrics:        true,
		TrackPerformanceMetrics: true,
		DataRetentionPeriod:     365 * 24 * time.Hour,
		EnableDataExport:        true,
		ExportSchedule:          "weekly",
		PrivacyCompliant:        true,
	}
}

func (ac *AdminConfig) setDefaultMonitoring() {
	ac.MonitoringConfig = MonitoringConfig{
		EnableLogging:       true,
		LogLevel:            "info",
		LogFormat:           "json",
		LogRotation:         true,
		LogRetentionPeriod:  30 * 24 * time.Hour,
		EnableHealthChecks:  true,
		HealthCheckInterval: 30 * time.Second,
		EnableMetrics:       true,
		MetricsProvider:     "prometheus",
		EnableAlerting:      true,
		EnableUptime:        true,
		UptimeCheckInterval: 60 * time.Second,
	}
}

func (ac *AdminConfig) setDefaultNotifications() {
	ac.NotificationConfig = NotificationConfig{
		EnableEmailNotifications: true,
		EnableSMSNotifications:   true,
		EnablePushNotifications:  true,
		EnableInAppNotifications: true,
		NotificationRetention:    30 * 24 * time.Hour,
		EnableNotificationQueue:  true,
		BatchSize:                100,
		BatchInterval:            5 * time.Minute,
	}
}

func (ac *AdminConfig) setDefaultUI() {
	ac.UIConfig = UICustomizationConfig{
		BrandName:       "ChatApp",
		PrimaryColor:    "#007bff",
		SecondaryColor:  "#6c757d",
		AccentColor:     "#28a745",
		BackgroundColor: "#ffffff",
		TextColor:       "#333333",
		EnableDarkTheme: true,
		DefaultTheme:    "light",
		ShowBranding:    true,
		WelcomeMessage:  "Welcome to ChatApp!",
	}
}

// GetSection returns a specific configuration section
func (ac *AdminConfig) GetSection(section string) interface{} {
	switch section {
	case "app_settings":
		return ac.AppSettings
	case "server_config":
		return ac.ServerConfig
	case "database_config":
		return ac.DatabaseConfig
	case "auth_config":
		return ac.AuthConfig
	case "security_config":
		return ac.SecurityConfig
	case "sms_config":
		return ac.SMSConfig
	case "email_config":
		return ac.EmailConfig
	case "push_config":
		return ac.PushConfig
	case "storage_config":
		return ac.StorageConfig
	case "media_config":
		return ac.MediaConfig
	case "webrtc_config":
		return ac.WebRTCConfig
	case "call_config":
		return ac.CallConfig
	case "feature_flags":
		return ac.FeatureFlags
	case "user_settings":
		return ac.UserSettings
	case "moderation_config":
		return ac.ModerationConfig
	case "analytics_config":
		return ac.AnalyticsConfig
	case "monitoring_config":
		return ac.MonitoringConfig
	case "business_config":
		return ac.BusinessConfig
	case "billing_config":
		return ac.BillingConfig
	case "notification_config":
		return ac.NotificationConfig
	case "backup_config":
		return ac.BackupConfig
	case "compliance_config":
		return ac.ComplianceConfig
	case "ui_config":
		return ac.UIConfig
	case "api_config":
		return ac.APIConfig
	case "integration_config":
		return ac.IntegrationConfig
	case "maintenance_config":
		return ac.MaintenanceConfig
	default:
		return nil
	}
}

// UpdateSection updates a specific configuration section
func (ac *AdminConfig) UpdateSection(section string, config interface{}) error {
	switch section {
	case "app_settings":
		if appSettings, ok := config.(AppSettings); ok {
			ac.AppSettings = appSettings
		} else {
			return fmt.Errorf("invalid config type for app_settings")
		}
	case "server_config":
		if serverConfig, ok := config.(ServerConfig); ok {
			ac.ServerConfig = serverConfig
		} else {
			return fmt.Errorf("invalid config type for server_config")
		}
	case "auth_config":
		if authConfig, ok := config.(AuthConfig); ok {
			ac.AuthConfig = authConfig
		} else {
			return fmt.Errorf("invalid config type for auth_config")
		}
	case "feature_flags":
		if featureFlags, ok := config.(SystemFeatureFlags); ok {
			ac.FeatureFlags = featureFlags
		} else {
			return fmt.Errorf("invalid config type for feature_flags")
		}
	// Add more cases as needed
	default:
		return fmt.Errorf("unknown configuration section: %s", section)
	}

	ac.BeforeUpdate()
	return nil
}

// IsMaintenanceMode checks if the app is in maintenance mode
func (ac *AdminConfig) IsMaintenanceMode() bool {
	return ac.AppSettings.MaintenanceMode || ac.MaintenanceConfig.EnableMaintenance
}

// IsFeatureEnabled checks if a specific feature is enabled
func (ac *AdminConfig) IsFeatureEnabled(feature string) bool {
	switch feature {
	case "registration":
		return ac.FeatureFlags.EnableRegistration
	case "group_chats":
		return ac.FeatureFlags.EnableGroupChats
	case "voice_calls":
		return ac.FeatureFlags.EnableVoiceCalls
	case "video_calls":
		return ac.FeatureFlags.EnableVideoCalls
	case "file_sharing":
		return ac.FeatureFlags.EnableFileSharing
	case "voice_messages":
		return ac.FeatureFlags.EnableVoiceMessages
	case "message_encryption":
		return ac.FeatureFlags.EnableMessageEncryption
	default:
		return false
	}
}
