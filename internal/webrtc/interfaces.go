package webrtc

import (
	"bro/internal/models"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ChatServiceInterface defines the interface for chat service operations needed by WebRTC
type ChatServiceInterface interface {
	// Add only the methods that WebRTC actually needs from ChatService
	GetChat(chatID primitive.ObjectID, userID primitive.ObjectID) (*models.ChatResponse, error)
	// Add other methods as needed
}

// PushServiceInterface defines the interface for push service operations needed by WebRTC
type PushServiceInterface interface {
	// Add only the methods that WebRTC actually needs from PushService
	SendNotification(userID primitive.ObjectID, title, body string, data map[string]string) error
	SendCallNotification(call *models.Call, caller *models.User, recipient primitive.ObjectID) error
	// Add other methods as needed
}
