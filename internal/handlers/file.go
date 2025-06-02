package handlers

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"bro/internal/middleware"
	"bro/internal/services"
	"bro/internal/utils"
	"bro/pkg/logger"
)

// FileHandler handles file-related HTTP requests
type FileHandler struct {
	fileService *services.FileService
}

// NewFileHandler creates a new file handler
func NewFileHandler(fileService *services.FileService) *FileHandler {
	return &FileHandler{
		fileService: fileService,
	}
}

// RegisterRoutes registers file routes
func (h *FileHandler) RegisterRoutes(r *gin.RouterGroup, jwtSecret string) {
	auth := middleware.AuthMiddleware(jwtSecret)

	files := r.Group("/files")
	{
		// File upload
		files.POST("/upload", auth, h.UploadFile)

		// File download
		files.GET("/:fileId", auth, h.DownloadFile)
		files.GET("/:fileId/download", auth, h.DownloadFile)

		// File information
		files.GET("/:fileId/info", auth, h.GetFileInfo)

		// File thumbnail
		files.GET("/:fileId/thumbnail", auth, h.GetThumbnail)

		// File management
		files.DELETE("/:fileId", auth, h.DeleteFile)

		// User files
		files.GET("/", auth, h.GetUserFiles)
		files.GET("/search", auth, h.SearchFiles)

		// Chat files
		files.GET("/chat/:chatId", auth, h.GetChatFiles)

		// File statistics
		files.GET("/stats", auth, h.GetFileStats)
	}
}

// UploadFile handles file upload
func (h *FileHandler) UploadFile(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	// Get form values
	purpose := c.DefaultPostForm("purpose", "message")
	chatID := c.PostForm("chat_id")
	messageID := c.PostForm("message_id")
	isPublic := c.DefaultPostForm("public", "false") == "true"
	expiryStr := c.PostForm("expires_at")

	// Get uploaded file
	fileHeader, err := c.FormFile("file")
	if err != nil {
		utils.BadRequest(c, "No file provided")
		return
	}

	// Validate file size
	maxSize := int64(50 * 1024 * 1024) // 50MB default
	if purpose == "avatar" {
		maxSize = int64(5 * 1024 * 1024) // 5MB for avatars
	}
	if fileHeader.Size > maxSize {
		utils.FileTooBig(c, utils.FormatFileSize(maxSize))
		return
	}

	// Create file upload from multipart
	upload, err := services.CreateFileUploadFromMultipart(fileHeader, userID, purpose)
	if err != nil {
		logger.Errorf("Failed to create file upload: %v", err)
		utils.InternalServerError(c, "Failed to process file")
		return
	}
	defer upload.Data.(io.Closer).Close()

	// Set additional properties
	upload.IsPublic = isPublic

	// Parse chat ID if provided
	if chatID != "" {
		if chatObjID, err := primitive.ObjectIDFromHex(chatID); err == nil {
			upload.ChatID = &chatObjID
		}
	}

	// Parse message ID if provided
	if messageID != "" {
		if msgObjID, err := primitive.ObjectIDFromHex(messageID); err == nil {
			upload.MessageID = &msgObjID
		}
	}

	// Parse expiry if provided
	if expiryStr != "" {
		if expiry, err := utils.ParseISO8601(expiryStr); err == nil {
			upload.ExpiresAt = &expiry
		}
	}

	// Upload file
	fileInfo, err := h.fileService.UploadFile(upload)
	if err != nil {
		logger.Errorf("File upload failed: %v", err)
		if strings.Contains(err.Error(), "validation") {
			utils.BadRequest(c, err.Error())
		} else if strings.Contains(err.Error(), "quota") {
			utils.QuotaExceeded(c, "storage")
		} else {
			utils.InternalServerError(c, "File upload failed")
		}
		return
	}

	// Log successful upload
	logger.LogUserAction(userID.Hex(), "file_uploaded", "file_handler", map[string]interface{}{
		"file_id":   fileInfo.ID.Hex(),
		"file_name": fileInfo.FileName,
		"file_size": fileInfo.Size,
		"purpose":   purpose,
	})

	utils.Created(c, fileInfo)
}

// DownloadFile handles file download
func (h *FileHandler) DownloadFile(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	fileID := c.Param("fileId")
	if fileID == "" {
		utils.BadRequest(c, "File ID is required")
		return
	}

	// Download file
	reader, fileInfo, err := h.fileService.DownloadFile(fileID, userID)
	if err != nil {
		logger.Errorf("File download failed: %v", err)
		if strings.Contains(err.Error(), "not found") {
			utils.NotFound(c, "File not found")
		} else if strings.Contains(err.Error(), "access denied") {
			utils.Forbidden(c, "Access denied")
		} else if strings.Contains(err.Error(), "expired") {
			utils.BadRequest(c, "File has expired")
		} else {
			utils.InternalServerError(c, "File download failed")
		}
		return
	}
	defer reader.Close()

	// Set headers
	c.Header("Content-Type", fileInfo.ContentType)
	c.Header("Content-Length", strconv.FormatInt(fileInfo.Size, 10))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileInfo.OriginalName))
	c.Header("Cache-Control", "private, max-age=3600")

	// Stream file
	_, err = io.Copy(c.Writer, reader)
	if err != nil {
		logger.Errorf("Failed to stream file: %v", err)
		return
	}

	// Log download
	logger.LogUserAction(userID.Hex(), "file_downloaded", "file_handler", map[string]interface{}{
		"file_id":   fileInfo.ID.Hex(),
		"file_name": fileInfo.FileName,
	})
}

// GetFileInfo returns file information
func (h *FileHandler) GetFileInfo(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	fileID := c.Param("fileId")
	if fileID == "" {
		utils.BadRequest(c, "File ID is required")
		return
	}

	fileInfo, err := h.fileService.GetFileInfo(fileID, userID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			utils.NotFound(c, "File not found")
		} else if strings.Contains(err.Error(), "access denied") {
			utils.Forbidden(c, "Access denied")
		} else {
			utils.InternalServerError(c, "Failed to get file info")
		}
		return
	}

	utils.Success(c, fileInfo)
}

// GetThumbnail returns file thumbnail
func (h *FileHandler) GetThumbnail(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	fileID := c.Param("fileId")
	if fileID == "" {
		utils.BadRequest(c, "File ID is required")
		return
	}

	size := c.DefaultQuery("size", "medium")
	if size != "small" && size != "medium" && size != "large" {
		utils.BadRequest(c, "Invalid thumbnail size")
		return
	}

	reader, _, err := h.fileService.GetThumbnail(fileID, size, userID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			utils.NotFound(c, "File not found")
		} else if strings.Contains(err.Error(), "access denied") {
			utils.Forbidden(c, "Access denied")
		} else if strings.Contains(err.Error(), "no thumbnail") {
			utils.NotFound(c, "Thumbnail not available")
		} else {
			utils.InternalServerError(c, "Failed to get thumbnail")
		}
		return
	}
	defer reader.Close()

	// Set headers
	c.Header("Content-Type", "image/jpeg")
	c.Header("Cache-Control", "public, max-age=86400") // 24 hours

	// Stream thumbnail
	_, err = io.Copy(c.Writer, reader)
	if err != nil {
		logger.Errorf("Failed to stream thumbnail: %v", err)
	}
}

// DeleteFile handles file deletion
func (h *FileHandler) DeleteFile(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	fileID := c.Param("fileId")
	if fileID == "" {
		utils.BadRequest(c, "File ID is required")
		return
	}

	err = h.fileService.DeleteFile(fileID, userID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			utils.NotFound(c, "File not found")
		} else if strings.Contains(err.Error(), "access denied") {
			utils.Forbidden(c, "Access denied")
		} else {
			utils.InternalServerError(c, "Failed to delete file")
		}
		return
	}

	// Log deletion
	logger.LogUserAction(userID.Hex(), "file_deleted", "file_handler", map[string]interface{}{
		"file_id": fileID,
	})

	utils.SuccessWithMessage(c, "File deleted successfully", nil)
}

// GetUserFiles returns user's files
func (h *FileHandler) GetUserFiles(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	// Parse pagination parameters
	params := utils.GetPaginationParams(c)
	purpose := c.Query("purpose")

	files, total, err := h.fileService.GetUserFiles(userID, purpose, params.Limit, (params.Page-1)*params.Limit)
	if err != nil {
		logger.Errorf("Failed to get user files: %v", err)
		utils.InternalServerError(c, "Failed to get files")
		return
	}

	utils.PaginatedResponse(c, files, params.Page, params.Limit, total)
}

// SearchFiles searches user's files
func (h *FileHandler) SearchFiles(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	// Parse parameters
	params := utils.GetPaginationParams(c)
	query := c.Query("q")
	contentType := c.Query("type")

	if query == "" {
		utils.BadRequest(c, "Search query is required")
		return
	}

	files, total, err := h.fileService.SearchFiles(userID, query, contentType, params.Limit, (params.Page-1)*params.Limit)
	if err != nil {
		logger.Errorf("Failed to search files: %v", err)
		utils.InternalServerError(c, "Failed to search files")
		return
	}

	utils.PaginatedResponse(c, files, params.Page, params.Limit, total)
}

// GetChatFiles returns files from a specific chat
func (h *FileHandler) GetChatFiles(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	chatIDStr := c.Param("chatId")
	chatID, err := primitive.ObjectIDFromHex(chatIDStr)
	if err != nil {
		utils.BadRequest(c, "Invalid chat ID")
		return
	}

	// Parse parameters
	params := utils.GetPaginationParams(c)
	fileType := c.Query("type")

	files, total, err := h.fileService.GetChatFiles(chatID, userID, fileType, params.Limit, (params.Page-1)*params.Limit)
	if err != nil {
		logger.Errorf("Failed to get chat files: %v", err)
		utils.InternalServerError(c, "Failed to get chat files")
		return
	}

	utils.PaginatedResponse(c, files, params.Page, params.Limit, total)
}

// GetFileStats returns file statistics for the user
func (h *FileHandler) GetFileStats(c *gin.Context) {
	userID, err := utils.GetUserIDFromContext(c)
	if err != nil {
		utils.Unauthorized(c, "User not authenticated")
		return
	}

	// Get user files grouped by purpose and type
	stats := map[string]interface{}{
		"total_files": 0,
		"total_size":  0,
		"by_purpose":  map[string]interface{}{},
		"by_type":     map[string]interface{}{},
		"quota_used":  0,
		"quota_limit": 1024 * 1024 * 1024, // 1GB default
	}

	// Get all user files for stats calculation
	allFiles, _, err := h.fileService.GetUserFiles(userID, "", 1000, 0)
	if err != nil {
		logger.Errorf("Failed to get files for stats: %v", err)
		utils.InternalServerError(c, "Failed to get file statistics")
		return
	}

	// Calculate stats
	totalSize := int64(0)
	purposeCounts := make(map[string]int)
	purposeSizes := make(map[string]int64)
	typeCounts := make(map[string]int)
	typeSizes := make(map[string]int64)

	for _, file := range allFiles {
		totalSize += file.Size

		// Count by purpose
		purposeCounts[file.Purpose]++
		purposeSizes[file.Purpose] += file.Size

		// Count by content type (simplified)
		var fileType string
		if strings.HasPrefix(file.ContentType, "image/") {
			fileType = "images"
		} else if strings.HasPrefix(file.ContentType, "video/") {
			fileType = "videos"
		} else if strings.HasPrefix(file.ContentType, "audio/") {
			fileType = "audio"
		} else {
			fileType = "documents"
		}

		typeCounts[fileType]++
		typeSizes[fileType] += file.Size
	}

	// Build response
	stats["total_files"] = len(allFiles)
	stats["total_size"] = totalSize
	stats["quota_used"] = totalSize

	// Purpose breakdown
	purposeStats := make(map[string]interface{})
	for purpose, count := range purposeCounts {
		purposeStats[purpose] = map[string]interface{}{
			"count": count,
			"size":  purposeSizes[purpose],
		}
	}
	stats["by_purpose"] = purposeStats

	// Type breakdown
	typeStats := make(map[string]interface{})
	for fileType, count := range typeCounts {
		typeStats[fileType] = map[string]interface{}{
			"count": count,
			"size":  typeSizes[fileType],
		}
	}
	stats["by_type"] = typeStats

	utils.Success(c, stats)
}
