package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	// Server Configuration
	Port       string
	Production bool
	Debug      bool
	JWTSecret  string

	// Database Configuration
	MongoURI string
	RedisURL string

	// SMS Service Configuration
	SMSProvider     string // "twilio", "firebase", "onesignal"
	TwilioConfig    TwilioConfig
	FirebaseConfig  FirebaseConfig
	OneSignalConfig OneSignalConfig

	// File Storage Configuration
	StorageProvider string // "local", "aws", "gcp"
	LocalStorage    LocalStorageConfig
	AWSConfig       AWSConfig

	// Push Notification Configuration
	PushProvider string // "firebase", "onesignal"

	// COTURN Configuration
	COTURNConfig COTURNConfig

	// Security Configuration
	EncryptionKey string

	// Admin Configuration
	AdminEmail    string
	AdminPassword string

	// Feature Flags
	Features FeatureFlags

	// Rate Limiting
	RateLimit RateLimitConfig
}

type TwilioConfig struct {
	AccountSID  string
	AuthToken   string
	PhoneNumber string
}

type FirebaseConfig struct {
	ProjectID     string
	PrivateKeyID  string
	PrivateKey    string
	ClientEmail   string
	ClientID      string
	AuthURI       string
	TokenURI      string
	CertURL       string
	DatabaseURL   string
	StorageBucket string
}

type OneSignalConfig struct {
	AppID      string
	APIKey     string
	RestAPIKey string
}

type LocalStorageConfig struct {
	UploadPath   string
	MaxFileSize  int64
	AllowedTypes []string
}

type AWSConfig struct {
	AccessKeyID     string
	SecretAccessKey string
	Region          string
	BucketName      string
	CloudFrontURL   string
}

type COTURNConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	Realm    string
	Secret   string
}

type FeatureFlags struct {
	VoiceCalls        bool
	VideoCalls        bool
	GroupCalls        bool
	FileSharing       bool
	ScreenSharing     bool
	CallRecording     bool
	MessageEncryption bool
	GroupMessages     bool
	BroadcastMessages bool
	Stories           bool
	StatusMessages    bool
}

type RateLimitConfig struct {
	Enabled        bool
	RequestsPerMin int
	BurstSize      int
}

func Load() *Config {
	return &Config{
		Port:       getEnv("PORT", "8080"),
		Production: getEnvBool("PRODUCTION", false),
		Debug:      getEnvBool("DEBUG", true),
		JWTSecret:  getEnv("JWT_SECRET", "your-super-secret-jwt-key"),

		MongoURI: getEnv("MONGODB_URI", "mongodb://localhost:27017/chatapp"),
		RedisURL: getEnv("REDIS_URL", "redis://localhost:6379"),

		SMSProvider: getEnv("SMS_PROVIDER", "twilio"),
		TwilioConfig: TwilioConfig{
			AccountSID:  getEnv("TWILIO_ACCOUNT_SID", ""),
			AuthToken:   getEnv("TWILIO_AUTH_TOKEN", ""),
			PhoneNumber: getEnv("TWILIO_PHONE_NUMBER", ""),
		},

		FirebaseConfig: FirebaseConfig{
			ProjectID:     getEnv("FIREBASE_PROJECT_ID", ""),
			PrivateKeyID:  getEnv("FIREBASE_PRIVATE_KEY_ID", ""),
			PrivateKey:    getEnv("FIREBASE_PRIVATE_KEY", ""),
			ClientEmail:   getEnv("FIREBASE_CLIENT_EMAIL", ""),
			ClientID:      getEnv("FIREBASE_CLIENT_ID", ""),
			AuthURI:       getEnv("FIREBASE_AUTH_URI", "https://accounts.google.com/o/oauth2/auth"),
			TokenURI:      getEnv("FIREBASE_TOKEN_URI", "https://oauth2.googleapis.com/token"),
			CertURL:       getEnv("FIREBASE_CERT_URL", ""),
			DatabaseURL:   getEnv("FIREBASE_DATABASE_URL", ""),
			StorageBucket: getEnv("FIREBASE_STORAGE_BUCKET", ""),
		},

		OneSignalConfig: OneSignalConfig{
			AppID:      getEnv("ONESIGNAL_APP_ID", ""),
			APIKey:     getEnv("ONESIGNAL_API_KEY", ""),
			RestAPIKey: getEnv("ONESIGNAL_REST_API_KEY", ""),
		},

		StorageProvider: getEnv("STORAGE_PROVIDER", "local"),
		LocalStorage: LocalStorageConfig{
			UploadPath:   getEnv("UPLOAD_PATH", "./web/static/uploads"),
			MaxFileSize:  getEnvInt64("MAX_FILE_SIZE", 50*1024*1024), // 50MB
			AllowedTypes: []string{"image/jpeg", "image/png", "image/gif", "video/mp4", "audio/mpeg", "application/pdf"},
		},

		AWSConfig: AWSConfig{
			AccessKeyID:     getEnv("AWS_ACCESS_KEY_ID", ""),
			SecretAccessKey: getEnv("AWS_SECRET_ACCESS_KEY", ""),
			Region:          getEnv("AWS_REGION", "us-west-2"),
			BucketName:      getEnv("AWS_BUCKET_NAME", ""),
			CloudFrontURL:   getEnv("AWS_CLOUDFRONT_URL", ""),
		},

		PushProvider: getEnv("PUSH_PROVIDER", "firebase"),

		COTURNConfig: COTURNConfig{
			Host:     getEnv("COTURN_HOST", "localhost"),
			Port:     getEnvInt("COTURN_PORT", 3478),
			Username: getEnv("COTURN_USERNAME", "chatapp"),
			Password: getEnv("COTURN_PASSWORD", "chatapp123"),
			Realm:    getEnv("COTURN_REALM", "chatapp.com"),
			Secret:   getEnv("COTURN_SECRET", "coturn-secret"),
		},

		EncryptionKey: getEnv("ENCRYPTION_KEY", "your-encryption-key-must-be-32-chars"),

		AdminEmail:    getEnv("ADMIN_EMAIL", "admin@chatapp.com"),
		AdminPassword: getEnv("ADMIN_PASSWORD", "admin123"),

		Features: FeatureFlags{
			VoiceCalls:        getEnvBool("FEATURE_VOICE_CALLS", true),
			VideoCalls:        getEnvBool("FEATURE_VIDEO_CALLS", true),
			GroupCalls:        getEnvBool("FEATURE_GROUP_CALLS", true),
			FileSharing:       getEnvBool("FEATURE_FILE_SHARING", true),
			ScreenSharing:     getEnvBool("FEATURE_SCREEN_SHARING", true),
			CallRecording:     getEnvBool("FEATURE_CALL_RECORDING", false),
			MessageEncryption: getEnvBool("FEATURE_MESSAGE_ENCRYPTION", true),
			GroupMessages:     getEnvBool("FEATURE_GROUP_MESSAGES", true),
			BroadcastMessages: getEnvBool("FEATURE_BROADCAST_MESSAGES", true),
			Stories:           getEnvBool("FEATURE_STORIES", false),
			StatusMessages:    getEnvBool("FEATURE_STATUS_MESSAGES", true),
		},

		RateLimit: RateLimitConfig{
			Enabled:        getEnvBool("RATE_LIMIT_ENABLED", true),
			RequestsPerMin: getEnvInt("RATE_LIMIT_REQUESTS_PER_MIN", 100),
			BurstSize:      getEnvInt("RATE_LIMIT_BURST_SIZE", 20),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if result, err := strconv.ParseBool(value); err == nil {
			return result
		}
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if result, err := strconv.Atoi(value); err == nil {
			return result
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if result, err := strconv.ParseInt(value, 10, 64); err == nil {
			return result
		}
	}
	return defaultValue
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.JWTSecret == "" || c.JWTSecret == "your-super-secret-jwt-key" {
		return fmt.Errorf("JWT_SECRET must be set")
	}

	if c.EncryptionKey == "" || len(c.EncryptionKey) != 32 {
		return fmt.Errorf("ENCRYPTION_KEY must be exactly 32 characters")
	}

	if c.SMSProvider == "twilio" {
		if c.TwilioConfig.AccountSID == "" || c.TwilioConfig.AuthToken == "" {
			return fmt.Errorf("Twilio configuration is incomplete")
		}
	}

	if c.StorageProvider == "aws" {
		if c.AWSConfig.AccessKeyID == "" || c.AWSConfig.SecretAccessKey == "" || c.AWSConfig.BucketName == "" {
			return fmt.Errorf("AWS configuration is incomplete")
		}
	}

	return nil
}

// GetTURNServers returns TURN server configuration for WebRTC
func (c *Config) GetTURNServers() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"urls":       []string{fmt.Sprintf("turn:%s:%d", c.COTURNConfig.Host, c.COTURNConfig.Port)},
			"username":   c.COTURNConfig.Username,
			"credential": c.COTURNConfig.Password,
		},
		{
			"urls": []string{fmt.Sprintf("stun:%s:%d", c.COTURNConfig.Host, c.COTURNConfig.Port)},
		},
	}
}
