package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/scrypt"
)

// EncryptionService provides encryption/decryption utilities
type EncryptionService struct {
	key []byte
}

// NewEncryptionService creates a new encryption service
func NewEncryptionService(key string) (*EncryptionService, error) {
	if len(key) != 32 {
		return nil, errors.New("encryption key must be exactly 32 characters")
	}

	return &EncryptionService{
		key: []byte(key),
	}, nil
}

// JWTClaims represents JWT token claims
type JWTClaims struct {
	UserID      string    `json:"user_id"`
	PhoneNumber string    `json:"phone_number"`
	Role        string    `json:"role"`
	DeviceID    string    `json:"device_id,omitempty"`
	IssuedAt    time.Time `json:"iat"`
	ExpiresAt   time.Time `json:"exp"`
	jwt.RegisteredClaims
}

// TokenPair represents access and refresh tokens
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
}

// EncryptedMessage represents an encrypted message
type EncryptedMessage struct {
	Content   string `json:"content"`
	Nonce     string `json:"nonce"`
	Timestamp int64  `json:"timestamp"`
}

// Password Hashing

// HashPassword creates a bcrypt hash of the password
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("password cannot be empty")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	return string(hash), nil
}

// VerifyPassword verifies a password against its hash
func VerifyPassword(password, hash string) bool {
	if password == "" || hash == "" {
		return false
	}

	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// CheckPasswordStrength validates password strength
func CheckPasswordStrength(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters long")
	}

	if len(password) > 128 {
		return errors.New("password must be less than 128 characters long")
	}

	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false

	for _, char := range password {
		switch {
		case char >= 'A' && char <= 'Z':
			hasUpper = true
		case char >= 'a' && char <= 'z':
			hasLower = true
		case char >= '0' && char <= '9':
			hasDigit = true
		case char >= '!' && char <= '/' ||
			char >= ':' && char <= '@' ||
			char >= '[' && char <= '`' ||
			char >= '{' && char <= '~':
			hasSpecial = true
		}
	}

	if !hasUpper {
		return errors.New("password must contain at least one uppercase letter")
	}

	if !hasLower {
		return errors.New("password must contain at least one lowercase letter")
	}

	if !hasDigit {
		return errors.New("password must contain at least one digit")
	}

	if !hasSpecial {
		return errors.New("password must contain at least one special character")
	}

	return nil
}

// JWT Token Operations

// GenerateTokenPair generates access and refresh tokens
func GenerateTokenPair(userID, phoneNumber, role, deviceID, jwtSecret string) (*TokenPair, error) {
	if jwtSecret == "" {
		return nil, errors.New("JWT secret cannot be empty")
	}

	now := time.Now()
	accessExpiry := now.Add(24 * time.Hour)       // 24 hours
	refreshExpiry := now.Add(30 * 24 * time.Hour) // 30 days

	// Access token claims
	accessClaims := &JWTClaims{
		UserID:      userID,
		PhoneNumber: phoneNumber,
		Role:        role,
		DeviceID:    deviceID,
		IssuedAt:    now,
		ExpiresAt:   accessExpiry,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(accessExpiry),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "chatapp",
			Subject:   userID,
			ID:        generateRandomString(16),
		},
	}

	// Refresh token claims
	refreshClaims := &JWTClaims{
		UserID:      userID,
		PhoneNumber: phoneNumber,
		Role:        role,
		DeviceID:    deviceID,
		IssuedAt:    now,
		ExpiresAt:   refreshExpiry,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(refreshExpiry),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "chatapp",
			Subject:   userID,
			ID:        generateRandomString(16),
		},
	}

	// Generate tokens
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString([]byte(jwtSecret))
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString([]byte(jwtSecret))
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: refreshTokenString,
		ExpiresAt:    accessExpiry,
		TokenType:    "Bearer",
	}, nil
}

// ValidateToken validates and parses JWT token
func ValidateToken(tokenString, jwtSecret string) (*JWTClaims, error) {
	if tokenString == "" {
		return nil, errors.New("token cannot be empty")
	}

	// Remove "Bearer " prefix if present
	if strings.HasPrefix(tokenString, "Bearer ") {
		tokenString = strings.TrimPrefix(tokenString, "Bearer ")
	}

	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(jwtSecret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	// Check if token is expired
	if time.Now().After(claims.ExpiresAt) {
		return nil, errors.New("token has expired")
	}

	return claims, nil
}

// RefreshToken generates new access token from refresh token
func RefreshToken(refreshTokenString, jwtSecret string) (*TokenPair, error) {
	claims, err := ValidateToken(refreshTokenString, jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	// Generate new token pair
	return GenerateTokenPair(claims.UserID, claims.PhoneNumber, claims.Role, claims.DeviceID, jwtSecret)
}

// Message Encryption

// EncryptMessage encrypts a message using AES-GCM
func (e *EncryptionService) EncryptMessage(plaintext string) (*EncryptedMessage, error) {
	if plaintext == "" {
		return nil, errors.New("message cannot be empty")
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	return &EncryptedMessage{
		Content:   base64.StdEncoding.EncodeToString(ciphertext),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Timestamp: time.Now().Unix(),
	}, nil
}

// DecryptMessage decrypts a message using AES-GCM
func (e *EncryptionService) DecryptMessage(encMsg *EncryptedMessage) (string, error) {
	if encMsg == nil {
		return "", errors.New("encrypted message cannot be nil")
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encMsg.Content)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	nonce, err := base64.StdEncoding.DecodeString(encMsg.Nonce)
	if err != nil {
		return "", fmt.Errorf("failed to decode nonce: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

// File Encryption

// EncryptFile encrypts file data
func (e *EncryptionService) EncryptFile(data []byte) ([]byte, string, error) {
	if len(data) == 0 {
		return nil, "", errors.New("file data cannot be empty")
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	nonceHex := hex.EncodeToString(nonce)

	return ciphertext, nonceHex, nil
}

// DecryptFile decrypts file data
func (e *EncryptionService) DecryptFile(encData []byte, nonceHex string) ([]byte, error) {
	if len(encData) == 0 {
		return nil, errors.New("encrypted data cannot be empty")
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce, err := hex.DecodeString(nonceHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode nonce: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(encData) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	ciphertext := encData[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// Utility Functions

// GenerateSecureKey generates a secure random key
func GenerateSecureKey(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure key: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// GenerateRandomString generates a random string of specified length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based if crypto/rand fails
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}

	for i, b := range bytes {
		bytes[i] = charset[b%byte(len(charset))]
	}
	return string(bytes)
}

// GenerateAPIKey generates a secure API key
func GenerateAPIKey() (string, error) {
	key, err := GenerateSecureKey(32)
	if err != nil {
		return "", err
	}
	return "ca_" + key, nil
}

// HashAPIKey hashes an API key for storage
func HashAPIKey(apiKey string) (string, error) {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:]), nil
}

// VerifyAPIKey verifies an API key against its hash
func VerifyAPIKey(apiKey, hash string) bool {
	computed, err := HashAPIKey(apiKey)
	if err != nil {
		return false
	}
	return computed == hash
}

// DeriveKey derives a key from password and salt using scrypt
func DeriveKey(password, salt []byte) ([]byte, error) {
	return scrypt.Key(password, salt, 32768, 8, 1, 32)
}

// GenerateSalt generates a random salt
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	return salt, nil
}

// End-to-End Encryption Helpers

// GenerateKeyPair generates a new encryption key pair for E2E encryption
func GenerateKeyPair() (privateKey, publicKey string, err error) {
	// For simplicity, using symmetric key approach
	// In production, you might want to use asymmetric encryption like RSA or ECDH
	key, err := GenerateSecureKey(32)
	if err != nil {
		return "", "", err
	}

	// For demo purposes, using same key as both
	// In reality, you'd generate actual public/private key pairs
	return key, key, nil
}

// CreateChatEncryptionKey creates a unique encryption key for a chat
func CreateChatEncryptionKey(participants []string) (string, error) {
	// Create a deterministic but unique key based on participants
	// In production, you might use key exchange protocols
	data := strings.Join(participants, "|")
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:]), nil
}

// EncryptForParticipants encrypts data for specific participants
func (e *EncryptionService) EncryptForParticipants(data string, participantKeys []string) (map[string]*EncryptedMessage, error) {
	result := make(map[string]*EncryptedMessage)

	for _, key := range participantKeys {
		if len(key) == 32 {
			tempService := &EncryptionService{key: []byte(key)}
			encrypted, err := tempService.EncryptMessage(data)
			if err != nil {
				return nil, fmt.Errorf("failed to encrypt for participant: %w", err)
			}
			result[key] = encrypted
		}
	}

	return result, nil
}

// Two-Factor Authentication

// GenerateTOTPSecret generates a new TOTP secret for 2FA
func GenerateTOTPSecret() (string, error) {
	return GenerateSecureKey(20)
}

// Session Management

// GenerateSessionToken generates a secure session token
func GenerateSessionToken() (string, error) {
	return GenerateSecureKey(32)
}

// HashSessionToken hashes a session token for storage
func HashSessionToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// OTP Generation and Verification

// GenerateOTP generates a 6-digit OTP
func GenerateOTP() string {
	bytes := make([]byte, 3)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based if crypto/rand fails
		return fmt.Sprintf("%06d", time.Now().Unix()%1000000)
	}

	// Convert to 6-digit number
	num := int(bytes[0])<<16 | int(bytes[1])<<8 | int(bytes[2])
	return fmt.Sprintf("%06d", num%1000000)
}

// GenerateSecureOTP generates a cryptographically secure OTP
func GenerateSecureOTP() (string, error) {
	bytes := make([]byte, 3)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure OTP: %w", err)
	}

	// Convert to 6-digit number
	num := int(bytes[0])<<16 | int(bytes[1])<<8 | int(bytes[2])
	return fmt.Sprintf("%06d", num%1000000), nil
}

// Backup Encryption

// EncryptBackupData encrypts backup data with additional metadata
func (e *EncryptionService) EncryptBackupData(data []byte, metadata map[string]string) (*EncryptedMessage, error) {
	// Combine data and metadata
	backupData := struct {
		Data     []byte            `json:"data"`
		Metadata map[string]string `json:"metadata"`
		Version  string            `json:"version"`
	}{
		Data:     data,
		Metadata: metadata,
		Version:  "1.0",
	}

	jsonData, err := json.Marshal(backupData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal backup data: %w", err)
	}

	return e.EncryptMessage(string(jsonData))
}

// DecryptBackupData decrypts backup data and extracts metadata
func (e *EncryptionService) DecryptBackupData(encMsg *EncryptedMessage) ([]byte, map[string]string, error) {
	decrypted, err := e.DecryptMessage(encMsg)
	if err != nil {
		return nil, nil, err
	}

	var backupData struct {
		Data     []byte            `json:"data"`
		Metadata map[string]string `json:"metadata"`
		Version  string            `json:"version"`
	}

	if err := json.Unmarshal([]byte(decrypted), &backupData); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal backup data: %w", err)
	}

	return backupData.Data, backupData.Metadata, nil
}
