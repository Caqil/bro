package services

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"image"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bro/internal/config"
	"bro/internal/utils"
	"bro/pkg/database"
	"bro/pkg/logger"
	"bro/pkg/redis"
)

// FileService handles file operations
type FileService struct {
	config      *config.Config
	db          *mongo.Database
	collections *database.Collections
	redisClient *redis.Client
	encryption  *utils.EncryptionService
	storage     StorageProvider
}

// StorageProvider interface for different storage providers
type StorageProvider interface {
	Upload(file *FileUpload) (*FileInfo, error)
	Download(fileID string) (io.ReadCloser, error)
	Delete(fileID string) error
	GetURL(fileID string) string
	GetProviderName() string
}

// FileUpload represents file upload request
type FileUpload struct {
	ID          string                 `json:"id"`
	UserID      primitive.ObjectID     `json:"user_id"`
	FileName    string                 `json:"file_name"`
	ContentType string                 `json:"content_type"`
	Size        int64                  `json:"size"`
	Data        io.Reader              `json:"-"`
	Purpose     string                 `json:"purpose"` // "message", "avatar", "document", "media"
	ChatID      *primitive.ObjectID    `json:"chat_id,omitempty"`
	MessageID   *primitive.ObjectID    `json:"message_id,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	IsPublic    bool                   `json:"is_public"`
	ExpiresAt   *time.Time             `json:"expires_at,omitempty"`
}

// FileInfo represents file information
type FileInfo struct {
	ID            primitive.ObjectID     `bson:"_id,omitempty" json:"id"`
	FileName      string                 `bson:"file_name" json:"file_name"`
	OriginalName  string                 `bson:"original_name" json:"original_name"`
	ContentType   string                 `bson:"content_type" json:"content_type"`
	Size          int64                  `bson:"size" json:"size"`
	Hash          string                 `bson:"hash" json:"hash"`
	StoragePath   string                 `bson:"storage_path" json:"storage_path"`
	URL           string                 `bson:"url" json:"url"`
	ThumbnailURL  string                 `bson:"thumbnail_url,omitempty" json:"thumbnail_url,omitempty"`
	Purpose       string                 `bson:"purpose" json:"purpose"`
	UserID        primitive.ObjectID     `bson:"user_id" json:"user_id"`
	ChatID        *primitive.ObjectID    `bson:"chat_id,omitempty" json:"chat_id,omitempty"`
	MessageID     *primitive.ObjectID    `bson:"message_id,omitempty" json:"message_id,omitempty"`
	IsPublic      bool                   `bson:"is_public" json:"is_public"`
	IsEncrypted   bool                   `bson:"is_encrypted" json:"is_encrypted"`
	Metadata      map[string]interface{} `bson:"metadata,omitempty" json:"metadata,omitempty"`
	MediaInfo     *MediaInfo             `bson:"media_info,omitempty" json:"media_info,omitempty"`
	UploadedAt    time.Time              `bson:"uploaded_at" json:"uploaded_at"`
	ExpiresAt     *time.Time             `bson:"expires_at,omitempty" json:"expires_at,omitempty"`
	DownloadCount int64                  `bson:"download_count" json:"download_count"`
	LastAccessed  *time.Time             `bson:"last_accessed,omitempty" json:"last_accessed,omitempty"`
}

// MediaInfo represents media file information
type MediaInfo struct {
	Type      string    `bson:"type" json:"type"`                             // "image", "video", "audio"
	Duration  int       `bson:"duration,omitempty" json:"duration,omitempty"` // seconds for video/audio
	Width     int       `bson:"width,omitempty" json:"width,omitempty"`
	Height    int       `bson:"height,omitempty" json:"height,omitempty"`
	Bitrate   int       `bson:"bitrate,omitempty" json:"bitrate,omitempty"`
	Codec     string    `bson:"codec,omitempty" json:"codec,omitempty"`
	Thumbnail string    `bson:"thumbnail,omitempty" json:"thumbnail,omitempty"`
	Processed bool      `bson:"processed" json:"processed"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
}

// ThumbnailConfig represents thumbnail configuration
type ThumbnailConfig struct {
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Quality int    `json:"quality"`
	Format  string `json:"format"` // "jpeg", "png", "webp"
}

// FileValidationRules represents file validation rules
type FileValidationRules struct {
	MaxFileSize    int64    `json:"max_file_size"`
	AllowedTypes   []string `json:"allowed_types"`
	RequiredFields []string `json:"required_fields"`
}

// LocalStorageProvider implements local file storage
type LocalStorageProvider struct {
	config    *config.LocalStorageConfig
	baseURL   string
	uploadDir string
}

// AWSStorageProvider implements AWS S3 storage (placeholder)
type AWSStorageProvider struct {
	config *config.AWSConfig
}

// Default validation rules
var DefaultValidationRules = map[string]FileValidationRules{
	"message": {
		MaxFileSize:  50 * 1024 * 1024, // 50MB
		AllowedTypes: []string{"image/jpeg", "image/png", "image/gif", "image/webp", "video/mp4", "video/quicktime", "audio/mpeg", "audio/wav", "application/pdf", "text/plain"},
	},
	"avatar": {
		MaxFileSize:  5 * 1024 * 1024, // 5MB
		AllowedTypes: []string{"image/jpeg", "image/png", "image/webp"},
	},
	"document": {
		MaxFileSize:  100 * 1024 * 1024, // 100MB
		AllowedTypes: []string{"application/pdf", "application/msword", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "text/plain"},
	},
}

// Thumbnail configurations
var ThumbnailConfigs = map[string]ThumbnailConfig{
	"small": {
		Width:   150,
		Height:  150,
		Quality: 80,
		Format:  "jpeg",
	},
	"medium": {
		Width:   300,
		Height:  300,
		Quality: 85,
		Format:  "jpeg",
	},
	"large": {
		Width:   800,
		Height:  600,
		Quality: 90,
		Format:  "jpeg",
	},
}

// NewFileService creates a new file service
func NewFileService(config *config.Config) *FileService {
	db := database.GetDB()
	collections := database.GetCollections()
	redisClient := redis.GetClient()

	encryption, err := utils.NewEncryptionService(config.EncryptionKey)
	if err != nil {
		logger.Fatal("Failed to initialize encryption service:", err)
	}

	service := &FileService{
		config:      config,
		db:          db,
		collections: collections,
		redisClient: redisClient,
		encryption:  encryption,
	}

	// Initialize storage provider
	switch strings.ToLower(config.StorageProvider) {
	case "local":
		service.storage = &LocalStorageProvider{
			config:    &config.LocalStorage,
			baseURL:   "http://localhost:" + config.Port,
			uploadDir: config.LocalStorage.UploadPath,
		}
	case "aws", "s3":
		service.storage = &AWSStorageProvider{
			config: &config.AWSConfig,
		}
	default:
		logger.Warn("No storage provider configured, using local storage")
		service.storage = &LocalStorageProvider{
			config:    &config.LocalStorage,
			baseURL:   "http://localhost:" + config.Port,
			uploadDir: "./uploads",
		}
	}

	// Ensure upload directory exists
	if localProvider, ok := service.storage.(*LocalStorageProvider); ok {
		if err := os.MkdirAll(localProvider.uploadDir, 0755); err != nil {
			logger.Fatal("Failed to create upload directory:", err)
		}
	}

	// Start background tasks
	go service.cleanupExpiredFiles()
	go service.processMediaFiles()

	logger.Infof("File service initialized with storage: %s", service.storage.GetProviderName())
	return service
}

// Public File Service Methods

// UploadFile uploads a file
func (s *FileService) UploadFile(upload *FileUpload) (*FileInfo, error) {
	startTime := time.Now()

	// Validate upload
	if err := s.validateUpload(upload); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check file size quota
	if err := s.checkQuota(upload.UserID, upload.Size); err != nil {
		return nil, fmt.Errorf("quota exceeded: %w", err)
	}

	// Generate file ID and hash
	upload.ID = primitive.NewObjectID().Hex()
	hash, err := s.calculateHash(upload.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate hash: %w", err)
	}

	// Check for duplicate files
	if existing, err := s.findFileByHash(hash); err == nil && existing != nil {
		// Duplicate file found, create reference instead of uploading again
		return s.createFileReference(existing, upload)
	}

	// Reset reader (hash calculation consumed it)
	if seeker, ok := upload.Data.(io.Seeker); ok {
		seeker.Seek(0, 0)
	}

	// Upload to storage
	fileInfo, err := s.storage.Upload(upload)
	if err != nil {
		return nil, fmt.Errorf("storage upload failed: %w", err)
	}

	// Set additional fields
	fileInfo.Hash = hash
	fileInfo.OriginalName = upload.FileName
	fileInfo.UserID = upload.UserID
	fileInfo.ChatID = upload.ChatID
	fileInfo.MessageID = upload.MessageID
	fileInfo.Purpose = upload.Purpose
	fileInfo.IsPublic = upload.IsPublic
	fileInfo.ExpiresAt = upload.ExpiresAt
	fileInfo.UploadedAt = time.Now()
	fileInfo.Metadata = upload.Metadata

	// Encrypt file if configured
	if s.shouldEncryptFile(upload.Purpose) {
		fileInfo.IsEncrypted = true
	}

	// Extract media info for supported types
	if s.isMediaFile(fileInfo.ContentType) {
		go s.extractMediaInfo(fileInfo)
		go s.generateThumbnails(fileInfo)
	}

	// Save to database
	if err := s.saveFileInfo(fileInfo); err != nil {
		// Try to delete uploaded file if database save fails
		s.storage.Delete(fileInfo.ID.Hex())
		return nil, fmt.Errorf("failed to save file info: %w", err)
	}

	// Update user quota
	s.updateUserQuota(upload.UserID, upload.Size)

	// Log successful upload
	duration := time.Since(startTime)
	s.logFileEvent("file_uploaded", fileInfo, map[string]interface{}{
		"duration_ms": duration.Milliseconds(),
		"size":        fileInfo.Size,
		"type":        fileInfo.ContentType,
	})

	logger.Infof("File uploaded successfully: %s (%s, %s)",
		fileInfo.FileName, utils.FormatFileSize(fileInfo.Size), fileInfo.ContentType)

	return fileInfo, nil
}

// DownloadFile downloads a file
func (s *FileService) DownloadFile(fileID string, userID primitive.ObjectID) (io.ReadCloser, *FileInfo, error) {
	startTime := time.Now()

	// Get file info
	fileInfo, err := s.getFileInfo(fileID)
	if err != nil {
		return nil, nil, fmt.Errorf("file not found: %w", err)
	}

	// Check permissions
	if err := s.checkDownloadPermission(fileInfo, userID); err != nil {
		return nil, nil, fmt.Errorf("access denied: %w", err)
	}

	// Check if file is expired
	if fileInfo.ExpiresAt != nil && time.Now().After(*fileInfo.ExpiresAt) {
		return nil, nil, fmt.Errorf("file has expired")
	}

	// Download from storage
	reader, err := s.storage.Download(fileID)
	if err != nil {
		return nil, nil, fmt.Errorf("storage download failed: %w", err)
	}

	// Decrypt if necessary
	if fileInfo.IsEncrypted {
		// For encrypted files, we would need to decrypt the stream
		// This is a placeholder - actual implementation would depend on encryption method
		logger.Debug("File is encrypted, decryption required")
	}

	// Update download statistics
	go s.updateDownloadStats(fileInfo.ID)

	// Log download
	duration := time.Since(startTime)
	s.logFileEvent("file_downloaded", fileInfo, map[string]interface{}{
		"user_id":     userID.Hex(),
		"duration_ms": duration.Milliseconds(),
	})

	return reader, fileInfo, nil
}

// DeleteFile deletes a file
func (s *FileService) DeleteFile(fileID string, userID primitive.ObjectID) error {
	// Get file info
	fileInfo, err := s.getFileInfo(fileID)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	// Check permissions
	if err := s.checkDeletePermission(fileInfo, userID); err != nil {
		return fmt.Errorf("access denied: %w", err)
	}

	// Delete from storage
	if err := s.storage.Delete(fileID); err != nil {
		logger.Errorf("Failed to delete file from storage: %v", err)
		// Continue with database deletion even if storage deletion fails
	}

	// Delete thumbnails
	if fileInfo.ThumbnailURL != "" {
		go s.deleteThumbnails(fileInfo)
	}

	// Delete from database
	if err := s.deleteFileInfo(fileInfo.ID); err != nil {
		return fmt.Errorf("failed to delete file info: %w", err)
	}

	// Update user quota
	s.updateUserQuota(userID, -fileInfo.Size)

	// Log deletion
	s.logFileEvent("file_deleted", fileInfo, map[string]interface{}{
		"deleted_by": userID.Hex(),
	})

	logger.Infof("File deleted: %s", fileInfo.FileName)
	return nil
}

// GetFileInfo gets file information
func (s *FileService) GetFileInfo(fileID string, userID primitive.ObjectID) (*FileInfo, error) {
	fileInfo, err := s.getFileInfo(fileID)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}

	// Check permissions
	if err := s.checkViewPermission(fileInfo, userID); err != nil {
		return nil, fmt.Errorf("access denied: %w", err)
	}

	return fileInfo, nil
}

// GetThumbnail gets file thumbnail
func (s *FileService) GetThumbnail(fileID, size string, userID primitive.ObjectID) (io.ReadCloser, *FileInfo, error) {
	// Get file info
	fileInfo, err := s.getFileInfo(fileID)
	if err != nil {
		return nil, nil, fmt.Errorf("file not found: %w", err)
	}

	// Check permissions
	if err := s.checkViewPermission(fileInfo, userID); err != nil {
		return nil, nil, fmt.Errorf("access denied: %w", err)
	}

	// Check if file has thumbnails
	if fileInfo.ThumbnailURL == "" {
		return nil, nil, fmt.Errorf("no thumbnail available")
	}

	// Generate thumbnail path
	thumbnailPath := s.getThumbnailPath(fileInfo.ID.Hex(), size)

	// Try to read thumbnail file
	reader, err := os.Open(thumbnailPath)
	if err != nil {
		// Generate thumbnail if it doesn't exist
		if err := s.generateSingleThumbnail(fileInfo, size); err != nil {
			return nil, nil, fmt.Errorf("failed to generate thumbnail: %w", err)
		}

		// Try again
		reader, err = os.Open(thumbnailPath)
		if err != nil {
			return nil, nil, fmt.Errorf("thumbnail not available: %w", err)
		}
	}

	return reader, fileInfo, nil
}

// GetUserFiles gets files uploaded by user
func (s *FileService) GetUserFiles(userID primitive.ObjectID, purpose string, limit, offset int) ([]*FileInfo, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{
		"user_id": userID,
	}

	if purpose != "" {
		filter["purpose"] = purpose
	}

	// Get total count
	total, err := s.collections.Files.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count files: %w", err)
	}

	// Get files
	opts := options.Find().
		SetSort(bson.D{{Key: "uploaded_at", Value: -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(offset))

	cursor, err := s.collections.Files.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to find files: %w", err)
	}
	defer cursor.Close(ctx)

	var files []*FileInfo
	if err := cursor.All(ctx, &files); err != nil {
		return nil, 0, fmt.Errorf("failed to decode files: %w", err)
	}

	return files, total, nil
}

// GetChatFiles gets files from a specific chat
func (s *FileService) GetChatFiles(chatID primitive.ObjectID, userID primitive.ObjectID, fileType string, limit, offset int) ([]*FileInfo, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{
		"chat_id": chatID,
	}

	if fileType != "" {
		filter["content_type"] = bson.M{"$regex": "^" + fileType}
	}

	// Get total count
	total, err := s.collections.Files.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count files: %w", err)
	}

	// Get files
	opts := options.Find().
		SetSort(bson.D{{Key: "uploaded_at", Value: -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(offset))

	cursor, err := s.collections.Files.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to find files: %w", err)
	}
	defer cursor.Close(ctx)

	var files []*FileInfo
	if err := cursor.All(ctx, &files); err != nil {
		return nil, 0, fmt.Errorf("failed to decode files: %w", err)
	}

	return files, total, nil
}

// SearchFiles searches files by name or content type
func (s *FileService) SearchFiles(userID primitive.ObjectID, query, contentType string, limit, offset int) ([]*FileInfo, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{
		"user_id": userID,
	}

	if query != "" {
		filter["$or"] = []bson.M{
			{"file_name": bson.M{"$regex": query, "$options": "i"}},
			{"original_name": bson.M{"$regex": query, "$options": "i"}},
		}
	}

	if contentType != "" {
		filter["content_type"] = bson.M{"$regex": "^" + contentType}
	}

	// Get total count
	total, err := s.collections.Files.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count files: %w", err)
	}

	// Get files
	opts := options.Find().
		SetSort(bson.D{{Key: "uploaded_at", Value: -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(offset))

	cursor, err := s.collections.Files.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to find files: %w", err)
	}
	defer cursor.Close(ctx)

	var files []*FileInfo
	if err := cursor.All(ctx, &files); err != nil {
		return nil, 0, fmt.Errorf("failed to decode files: %w", err)
	}

	return files, total, nil
}

// Helper Methods

// validateUpload validates file upload
func (s *FileService) validateUpload(upload *FileUpload) error {
	if upload.FileName == "" {
		return fmt.Errorf("file name is required")
	}

	if upload.Size <= 0 {
		return fmt.Errorf("invalid file size")
	}

	if upload.Data == nil {
		return fmt.Errorf("file data is required")
	}

	// Get validation rules for purpose
	rules, exists := DefaultValidationRules[upload.Purpose]
	if !exists {
		rules = DefaultValidationRules["message"] // Default rules
	}

	// Check file size
	if upload.Size > rules.MaxFileSize {
		return fmt.Errorf("file size exceeds limit of %s", utils.FormatFileSize(rules.MaxFileSize))
	}

	// Check content type
	if len(rules.AllowedTypes) > 0 {
		allowed := false
		for _, allowedType := range rules.AllowedTypes {
			if upload.ContentType == allowedType {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("file type not allowed: %s", upload.ContentType)
		}
	}

	// Validate file extension matches content type
	if err := s.validateContentType(upload.FileName, upload.ContentType); err != nil {
		return err
	}

	return nil
}

// validateContentType validates content type matches file extension
func (s *FileService) validateContentType(fileName, contentType string) error {
	ext := strings.ToLower(filepath.Ext(fileName))
	expectedType := mime.TypeByExtension(ext)

	if expectedType != "" && expectedType != contentType {
		logger.Warnf("Content type mismatch: expected %s, got %s for file %s", expectedType, contentType, fileName)
		// Don't fail validation, just log warning
	}

	return nil
}

// calculateHash calculates MD5 hash of file content
func (s *FileService) calculateHash(reader io.Reader) (string, error) {
	hash := md5.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// findFileByHash finds existing file by hash
func (s *FileService) findFileByHash(hash string) (*FileInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var fileInfo FileInfo
	err := s.collections.Files.FindOne(ctx, bson.M{"hash": hash}).Decode(&fileInfo)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return &fileInfo, nil
}

// createFileReference creates a reference to existing file
func (s *FileService) createFileReference(existing *FileInfo, upload *FileUpload) (*FileInfo, error) {
	// Create new file info referencing the same storage location
	newFileInfo := &FileInfo{
		ID:           primitive.NewObjectID(),
		FileName:     existing.FileName,
		OriginalName: upload.FileName,
		ContentType:  existing.ContentType,
		Size:         existing.Size,
		Hash:         existing.Hash,
		StoragePath:  existing.StoragePath, // Same storage path
		URL:          existing.URL,
		ThumbnailURL: existing.ThumbnailURL,
		Purpose:      upload.Purpose,
		UserID:       upload.UserID,
		ChatID:       upload.ChatID,
		MessageID:    upload.MessageID,
		IsPublic:     upload.IsPublic,
		IsEncrypted:  existing.IsEncrypted,
		Metadata:     upload.Metadata,
		MediaInfo:    existing.MediaInfo,
		UploadedAt:   time.Now(),
		ExpiresAt:    upload.ExpiresAt,
	}

	// Save to database
	if err := s.saveFileInfo(newFileInfo); err != nil {
		return nil, fmt.Errorf("failed to save file reference: %w", err)
	}

	logger.Infof("Created file reference instead of duplicate upload: %s", newFileInfo.FileName)
	return newFileInfo, nil
}

// checkQuota checks if user has enough quota for upload
func (s *FileService) checkQuota(userID primitive.ObjectID, fileSize int64) error {
	// Get user's current usage
	currentUsage, err := s.getUserStorageUsage(userID)
	if err != nil {
		logger.Errorf("Failed to get user storage usage: %v", err)
		return nil // Allow upload if we can't check quota
	}

	// Get user's quota limit (this would come from user subscription/plan)
	quotaLimit := int64(1024 * 1024 * 1024) // 1GB default

	if currentUsage+fileSize > quotaLimit {
		return fmt.Errorf("storage quota exceeded. Current: %s, Limit: %s",
			utils.FormatFileSize(currentUsage), utils.FormatFileSize(quotaLimit))
	}

	return nil
}

// getUserStorageUsage gets user's current storage usage
func (s *FileService) getUserStorageUsage(userID primitive.ObjectID) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		{
			"$match": bson.M{
				"user_id": userID,
			},
		},
		{
			"$group": bson.M{
				"_id": nil,
				"total_size": bson.M{
					"$sum": "$size",
				},
			},
		},
	}

	cursor, err := s.collections.Files.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	var result struct {
		TotalSize int64 `bson:"total_size"`
	}

	if cursor.Next(ctx) {
		cursor.Decode(&result)
	}

	return result.TotalSize, nil
}

// updateUserQuota updates user quota usage
func (s *FileService) updateUserQuota(userID primitive.ObjectID, sizeChange int64) {
	if s.redisClient == nil {
		return
	}

	key := fmt.Sprintf("user_quota:%s", userID.Hex())
	if sizeChange > 0 {
		s.redisClient.IncrementBy(key, sizeChange)
	} else {
		s.redisClient.DecrementBy(key, -sizeChange)
	}
}

// shouldEncryptFile determines if file should be encrypted
func (s *FileService) shouldEncryptFile(purpose string) bool {
	// Encrypt message files and documents by default
	return purpose == "message" || purpose == "document"
}

// isMediaFile checks if file is a media file
func (s *FileService) isMediaFile(contentType string) bool {
	return strings.HasPrefix(contentType, "image/") ||
		strings.HasPrefix(contentType, "video/") ||
		strings.HasPrefix(contentType, "audio/")
}

// Permission checks

// checkDownloadPermission checks if user can download file
func (s *FileService) checkDownloadPermission(fileInfo *FileInfo, userID primitive.ObjectID) error {
	// Owner can always download
	if fileInfo.UserID == userID {
		return nil
	}

	// Public files can be downloaded by anyone
	if fileInfo.IsPublic {
		return nil
	}

	// Check if user has access to the chat
	if fileInfo.ChatID != nil {
		return s.checkChatAccess(*fileInfo.ChatID, userID)
	}

	return fmt.Errorf("access denied")
}

// checkDeletePermission checks if user can delete file
func (s *FileService) checkDeletePermission(fileInfo *FileInfo, userID primitive.ObjectID) error {
	// Only owner can delete
	if fileInfo.UserID != userID {
		return fmt.Errorf("only file owner can delete")
	}

	return nil
}

// checkViewPermission checks if user can view file info
func (s *FileService) checkViewPermission(fileInfo *FileInfo, userID primitive.ObjectID) error {
	// Same as download permission
	return s.checkDownloadPermission(fileInfo, userID)
}

// checkChatAccess checks if user has access to chat
func (s *FileService) checkChatAccess(chatID, userID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := s.collections.Chats.CountDocuments(ctx, bson.M{
		"_id":          chatID,
		"participants": userID,
		"is_active":    true,
	})
	if err != nil {
		return fmt.Errorf("failed to check chat access: %w", err)
	}

	if count == 0 {
		return fmt.Errorf("no access to chat")
	}

	return nil
}

// Media Processing

// extractMediaInfo extracts media information from file
func (s *FileService) extractMediaInfo(fileInfo *FileInfo) {
	filePath := s.getLocalFilePath(fileInfo)

	mediaInfo := &MediaInfo{
		CreatedAt: time.Now(),
	}

	if strings.HasPrefix(fileInfo.ContentType, "image/") {
		s.extractImageInfo(filePath, mediaInfo)
	} else if strings.HasPrefix(fileInfo.ContentType, "video/") {
		s.extractVideoInfo(filePath, mediaInfo)
	} else if strings.HasPrefix(fileInfo.ContentType, "audio/") {
		s.extractAudioInfo(filePath, mediaInfo)
	}

	mediaInfo.Processed = true

	// Update file info in database
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.collections.Files.UpdateOne(ctx,
		bson.M{"_id": fileInfo.ID},
		bson.M{"$set": bson.M{"media_info": mediaInfo}},
	)
	if err != nil {
		logger.Errorf("Failed to update media info: %v", err)
	}
}

// extractImageInfo extracts image information
func (s *FileService) extractImageInfo(filePath string, mediaInfo *MediaInfo) {
	file, err := os.Open(filePath)
	if err != nil {
		logger.Errorf("Failed to open image file: %v", err)
		return
	}
	defer file.Close()

	config, _, err := image.DecodeConfig(file)
	if err != nil {
		logger.Errorf("Failed to decode image config: %v", err)
		return
	}

	mediaInfo.Type = "image"
	mediaInfo.Width = config.Width
	mediaInfo.Height = config.Height
}

// extractVideoInfo extracts video information using ffprobe
func (s *FileService) extractVideoInfo(filePath string, mediaInfo *MediaInfo) {
	// Use ffprobe to extract video information
	cmd := exec.Command("ffprobe", "-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", filePath)
	output, err := cmd.Output()
	if err != nil {
		logger.Errorf("Failed to extract video info: %v", err)
		return
	}

	// Parse ffprobe output (simplified)
	// In production, you would properly parse the JSON output
	mediaInfo.Type = "video"
	logger.Debugf("Video info extracted: %s", string(output))
}

// extractAudioInfo extracts audio information
func (s *FileService) extractAudioInfo(filePath string, mediaInfo *MediaInfo) {
	// Use ffprobe to extract audio information
	cmd := exec.Command("ffprobe", "-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", filePath)
	output, err := cmd.Output()
	if err != nil {
		logger.Errorf("Failed to extract audio info: %v", err)
		return
	}

	mediaInfo.Type = "audio"
	logger.Debugf("Audio info extracted: %s", string(output))
}

// Thumbnail Generation

// generateThumbnails generates thumbnails for media files
func (s *FileService) generateThumbnails(fileInfo *FileInfo) {
	if !strings.HasPrefix(fileInfo.ContentType, "image/") {
		return // Only generate thumbnails for images for now
	}

	s.getLocalFilePath(fileInfo)

	// Generate thumbnails for all configured sizes
	for size := range ThumbnailConfigs {
		if err := s.generateSingleThumbnail(fileInfo, size); err != nil {
			logger.Errorf("Failed to generate %s thumbnail for %s: %v", size, fileInfo.FileName, err)
		}
	}

	// Update file info with thumbnail URL
	thumbnailURL := s.getThumbnailURL(fileInfo.ID.Hex(), "medium")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.collections.Files.UpdateOne(ctx,
		bson.M{"_id": fileInfo.ID},
		bson.M{"$set": bson.M{"thumbnail_url": thumbnailURL}},
	)
	if err != nil {
		logger.Errorf("Failed to update thumbnail URL: %v", err)
	}
}

// generateSingleThumbnail generates a single thumbnail
func (s *FileService) generateSingleThumbnail(fileInfo *FileInfo, size string) error {
	config, exists := ThumbnailConfigs[size]
	if !exists {
		return fmt.Errorf("thumbnail config not found: %s", size)
	}

	filePath := s.getLocalFilePath(fileInfo)
	thumbnailPath := s.getThumbnailPath(fileInfo.ID.Hex(), size)

	// Ensure thumbnail directory exists
	if err := os.MkdirAll(filepath.Dir(thumbnailPath), 0755); err != nil {
		return fmt.Errorf("failed to create thumbnail directory: %w", err)
	}

	// Open source image
	sourceFile, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	// Decode image
	sourceImage, _, err := image.Decode(sourceFile)
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	// Calculate new dimensions maintaining aspect ratio
	srcBounds := sourceImage.Bounds()
	srcWidth := srcBounds.Dx()
	srcHeight := srcBounds.Dy()

	var newWidth, newHeight int
	if srcWidth > srcHeight {
		newWidth = config.Width
		newHeight = int(float64(srcHeight) * float64(config.Width) / float64(srcWidth))
	} else {
		newHeight = config.Height
		newWidth = int(float64(srcWidth) * float64(config.Height) / float64(srcHeight))
	}

	// Resize the image using the imaging library
	thumbnailImage := imaging.Resize(sourceImage, newWidth, newHeight, imaging.Lanczos)

	// Create thumbnail file
	thumbnailFile, err := os.Create(thumbnailPath)
	if err != nil {
		return fmt.Errorf("failed to create thumbnail file: %w", err)
	}
	defer thumbnailFile.Close()

	// Save the resized image as JPEG
	err = imaging.Save(thumbnailImage, thumbnailPath, imaging.JPEGQuality(90))
	if err != nil {
		return fmt.Errorf("failed to save thumbnail: %w", err)
	}

	return nil
}

// getThumbnailPath gets thumbnail file path
func (s *FileService) getThumbnailPath(fileID, size string) string {
	if localProvider, ok := s.storage.(*LocalStorageProvider); ok {
		return filepath.Join(localProvider.uploadDir, "thumbnails", size, fileID+".jpg")
	}
	return filepath.Join("./uploads/thumbnails", size, fileID+".jpg")
}

// getThumbnailURL gets thumbnail URL
func (s *FileService) getThumbnailURL(fileID, size string) string {
	return fmt.Sprintf("/api/files/%s/thumbnail?size=%s", fileID, size)
}

// deleteThumbnails deletes all thumbnails for a file
func (s *FileService) deleteThumbnails(fileInfo *FileInfo) {
	for size := range ThumbnailConfigs {
		thumbnailPath := s.getThumbnailPath(fileInfo.ID.Hex(), size)
		if err := os.Remove(thumbnailPath); err != nil && !os.IsNotExist(err) {
			logger.Errorf("Failed to delete thumbnail %s: %v", thumbnailPath, err)
		}
	}
}

// getLocalFilePath gets local file path
func (s *FileService) getLocalFilePath(fileInfo *FileInfo) string {
	if localProvider, ok := s.storage.(*LocalStorageProvider); ok {
		return filepath.Join(localProvider.uploadDir, fileInfo.StoragePath)
	}
	return fileInfo.StoragePath
}

// Database Operations

// saveFileInfo saves file info to database
func (s *FileService) saveFileInfo(fileInfo *FileInfo) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := s.collections.Files.InsertOne(ctx, fileInfo)
	if err != nil {
		return err
	}

	fileInfo.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

// getFileInfo gets file info from database
func (s *FileService) getFileInfo(fileID string) (*FileInfo, error) {
	objID, err := primitive.ObjectIDFromHex(fileID)
	if err != nil {
		return nil, fmt.Errorf("invalid file ID: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var fileInfo FileInfo
	err = s.collections.Files.FindOne(ctx, bson.M{"_id": objID}).Decode(&fileInfo)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("file not found")
		}
		return nil, err
	}

	return &fileInfo, nil
}

// deleteFileInfo deletes file info from database
func (s *FileService) deleteFileInfo(fileID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.collections.Files.DeleteOne(ctx, bson.M{"_id": fileID})
	return err
}

// updateDownloadStats updates download statistics
func (s *FileService) updateDownloadStats(fileID primitive.ObjectID) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"$inc": bson.M{"download_count": 1},
		"$set": bson.M{"last_accessed": time.Now()},
	}

	_, err := s.collections.Files.UpdateOne(ctx, bson.M{"_id": fileID}, update)
	if err != nil {
		logger.Errorf("Failed to update download stats: %v", err)
	}
}

// Background Tasks

// cleanupExpiredFiles cleans up expired files
func (s *FileService) cleanupExpiredFiles() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		logger.Debug("Running expired files cleanup")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

		// Find expired files
		cursor, err := s.collections.Files.Find(ctx, bson.M{
			"expires_at": bson.M{"$lt": time.Now()},
		})
		if err != nil {
			cancel()
			logger.Errorf("Failed to find expired files: %v", err)
			continue
		}

		var expiredFiles []FileInfo
		if err := cursor.All(ctx, &expiredFiles); err != nil {
			cursor.Close(ctx)
			cancel()
			logger.Errorf("Failed to decode expired files: %v", err)
			continue
		}
		cursor.Close(ctx)

		// Delete expired files
		for _, file := range expiredFiles {
			if err := s.storage.Delete(file.ID.Hex()); err != nil {
				logger.Errorf("Failed to delete expired file from storage: %v", err)
			}

			if err := s.deleteFileInfo(file.ID); err != nil {
				logger.Errorf("Failed to delete expired file from database: %v", err)
			}
		}

		cancel()

		if len(expiredFiles) > 0 {
			logger.Infof("Cleaned up %d expired files", len(expiredFiles))
		}
	}
}

// processMediaFiles processes pending media files
func (s *FileService) processMediaFiles() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		logger.Debug("Processing pending media files")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

		// Find unprocessed media files
		filter := bson.M{
			"$and": []bson.M{
				{
					"content_type": bson.M{
						"$regex": "^(image|video|audio)/",
					},
				},
				{
					"$or": []bson.M{
						{"media_info": bson.M{"$exists": false}},
						{"media_info.processed": false},
					},
				},
			},
		}

		cursor, err := s.collections.Files.Find(ctx, filter, options.Find().SetLimit(10))
		if err != nil {
			cancel()
			logger.Errorf("Failed to find unprocessed media files: %v", err)
			continue
		}

		var mediaFiles []FileInfo
		if err := cursor.All(ctx, &mediaFiles); err != nil {
			cursor.Close(ctx)
			cancel()
			logger.Errorf("Failed to decode media files: %v", err)
			continue
		}
		cursor.Close(ctx)
		cancel()

		// Process media files
		for _, file := range mediaFiles {
			s.extractMediaInfo(&file)
			if strings.HasPrefix(file.ContentType, "image/") {
				s.generateThumbnails(&file)
			}
		}

		if len(mediaFiles) > 0 {
			logger.Infof("Processed %d media files", len(mediaFiles))
		}
	}
}

// Logging

// logFileEvent logs file-related events
func (s *FileService) logFileEvent(event string, fileInfo *FileInfo, metadata map[string]interface{}) {
	fields := map[string]interface{}{
		"event":     event,
		"file_id":   fileInfo.ID.Hex(),
		"file_name": fileInfo.FileName,
		"user_id":   fileInfo.UserID.Hex(),
		"purpose":   fileInfo.Purpose,
		"storage":   s.storage.GetProviderName(),
		"type":      "file_event",
	}

	if fileInfo.ChatID != nil {
		fields["chat_id"] = fileInfo.ChatID.Hex()
	}

	for k, v := range metadata {
		fields[k] = v
	}

	logger.WithFields(fields).Info("File event")
}

// Storage Provider Implementations

// Local Storage Provider

func (l *LocalStorageProvider) Upload(upload *FileUpload) (*FileInfo, error) {
	// Generate unique filename
	fileName := fmt.Sprintf("%s_%s", upload.ID, upload.FileName)
	filePath := filepath.Join(l.uploadDir, fileName)

	// Create file
	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Copy data
	_, err = io.Copy(file, upload.Data)
	if err != nil {
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// Generate URL
	url := fmt.Sprintf("%s/uploads/%s", l.baseURL, fileName)

	fileInfo := &FileInfo{
		FileName:    fileName,
		ContentType: upload.ContentType,
		Size:        upload.Size,
		StoragePath: fileName,
		URL:         url,
	}

	return fileInfo, nil
}

func (l *LocalStorageProvider) Download(fileID string) (io.ReadCloser, error) {
	// Find file by ID (simplified - in production you'd query database for path)
	files, err := filepath.Glob(filepath.Join(l.uploadDir, fileID+"_*"))
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("file not found")
	}

	return os.Open(files[0])
}

func (l *LocalStorageProvider) Delete(fileID string) error {
	files, err := filepath.Glob(filepath.Join(l.uploadDir, fileID+"_*"))
	if err != nil {
		return err
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			logger.Errorf("Failed to delete file %s: %v", file, err)
		}
	}

	return nil
}

func (l *LocalStorageProvider) GetURL(fileID string) string {
	return fmt.Sprintf("%s/uploads/%s", l.baseURL, fileID)
}

func (l *LocalStorageProvider) GetProviderName() string {
	return "local"
}

// AWS Storage Provider (placeholder)

func (a *AWSStorageProvider) Upload(upload *FileUpload) (*FileInfo, error) {
	return nil, fmt.Errorf("AWS S3 storage not implemented yet")
}

func (a *AWSStorageProvider) Download(fileID string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("AWS S3 storage not implemented yet")
}

func (a *AWSStorageProvider) Delete(fileID string) error {
	return fmt.Errorf("AWS S3 storage not implemented yet")
}

func (a *AWSStorageProvider) GetURL(fileID string) string {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s",
		a.config.BucketName, a.config.Region, fileID)
}

func (a *AWSStorageProvider) GetProviderName() string {
	return "aws_s3"
}

// Utility Functions

// CreateFileUploadFromMultipart creates FileUpload from multipart file
func CreateFileUploadFromMultipart(fileHeader *multipart.FileHeader, userID primitive.ObjectID, purpose string) (*FileUpload, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open uploaded file: %w", err)
	}

	// Detect content type
	buffer := make([]byte, 512)
	_, err = file.Read(buffer)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to read file header: %w", err)
	}

	contentType := http.DetectContentType(buffer)

	// Reset file pointer
	file.Seek(0, 0)

	upload := &FileUpload{
		UserID:      userID,
		FileName:    fileHeader.Filename,
		ContentType: contentType,
		Size:        fileHeader.Size,
		Data:        file,
		Purpose:     purpose,
		IsPublic:    false,
	}

	return upload, nil
}

// Global service instance
var globalFileService *FileService

func GetFileService() *FileService {
	return globalFileService
}

func SetFileService(service *FileService) {
	globalFileService = service
}
