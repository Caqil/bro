package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Message struct {
	ID       primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ChatID   primitive.ObjectID `bson:"chat_id" json:"chat_id" validate:"required"`
	SenderID primitive.ObjectID `bson:"sender_id" json:"sender_id" validate:"required"`

	// Message Content
	Type         MessageType `bson:"type" json:"type"`
	Content      string      `bson:"content" json:"content"`
	MediaURL     string      `bson:"media_url,omitempty" json:"media_url,omitempty"`
	ThumbnailURL string      `bson:"thumbnail_url,omitempty" json:"thumbnail_url,omitempty"`
	FileSize     int64       `bson:"file_size,omitempty" json:"file_size,omitempty"`
	FileName     string      `bson:"file_name,omitempty" json:"file_name,omitempty"`
	MimeType     string      `bson:"mime_type,omitempty" json:"mime_type,omitempty"`
	Duration     int         `bson:"duration,omitempty" json:"duration,omitempty"` // For audio/video in seconds

	// Message Metadata
	IsEncrypted bool            `bson:"is_encrypted" json:"is_encrypted"`
	Metadata    MessageMetadata `bson:"metadata" json:"metadata"`

	// Reply & Quote
	ReplyToID     *primitive.ObjectID `bson:"reply_to_id,omitempty" json:"reply_to_id,omitempty"`
	QuotedMessage *Message            `bson:"quoted_message,omitempty" json:"quoted_message,omitempty"`

	// Forward Information
	ForwardedFrom *ForwardInfo `bson:"forwarded_from,omitempty" json:"forwarded_from,omitempty"`

	// Message Status
	Status      MessageStatus     `bson:"status" json:"status"`
	ReadBy      []ReadReceipt     `bson:"read_by" json:"read_by"`
	DeliveredTo []DeliveryReceipt `bson:"delivered_to" json:"delivered_to"`

	// Edit Information
	IsEdited    bool          `bson:"is_edited" json:"is_edited"`
	EditHistory []EditHistory `bson:"edit_history,omitempty" json:"edit_history,omitempty"`

	// Deletion
	IsDeleted  bool                 `bson:"is_deleted" json:"is_deleted"`
	DeletedFor []primitive.ObjectID `bson:"deleted_for,omitempty" json:"deleted_for,omitempty"`
	DeletedAt  *time.Time           `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`

	// Reactions
	Reactions []Reaction `bson:"reactions" json:"reactions"`

	// Mentions
	Mentions []primitive.ObjectID `bson:"mentions,omitempty" json:"mentions,omitempty"`

	// Timestamps
	CreatedAt   time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `bson:"updated_at" json:"updated_at"`
	ScheduledAt *time.Time `bson:"scheduled_at,omitempty" json:"scheduled_at,omitempty"`

	// Admin/Moderation
	IsReported  bool                `bson:"is_reported" json:"is_reported"`
	Reports     []MessageReport     `bson:"reports,omitempty" json:"reports,omitempty"`
	ModeratedBy *primitive.ObjectID `bson:"moderated_by,omitempty" json:"moderated_by,omitempty"`
}

type MessageType string

const (
	MessageTypeText         MessageType = "text"
	MessageTypeImage        MessageType = "image"
	MessageTypeVideo        MessageType = "video"
	MessageTypeAudio        MessageType = "audio"
	MessageTypeDocument     MessageType = "document"
	MessageTypeVoiceNote    MessageType = "voice_note"
	MessageTypeLocation     MessageType = "location"
	MessageTypeContact      MessageType = "contact"
	MessageTypeSticker      MessageType = "sticker"
	MessageTypeGIF          MessageType = "gif"
	MessageTypeCall         MessageType = "call"
	MessageTypeSystem       MessageType = "system"
	MessageTypeDeleted      MessageType = "deleted"
	MessageTypeJoinGroup    MessageType = "join_group"
	MessageTypeLeaveGroup   MessageType = "leave_group"
	MessageTypeGroupCreated MessageType = "group_created"
	MessageTypeGroupRenamed MessageType = "group_renamed"
	MessageTypePhotoChanged MessageType = "photo_changed"
)

type MessageStatus string

const (
	MessageStatusSent      MessageStatus = "sent"
	MessageStatusDelivered MessageStatus = "delivered"
	MessageStatusRead      MessageStatus = "read"
	MessageStatusFailed    MessageStatus = "failed"
	MessageStatusPending   MessageStatus = "pending"
)

type MessageMetadata struct {
	// Location data for location messages
	Location *LocationData `bson:"location,omitempty" json:"location,omitempty"`

	// Contact data for contact messages
	Contact *ContactData `bson:"contact,omitempty" json:"contact,omitempty"`

	// Call data for call messages
	CallData *CallMessageData `bson:"call_data,omitempty" json:"call_data,omitempty"`

	// System message data
	SystemData *SystemMessageData `bson:"system_data,omitempty" json:"system_data,omitempty"`

	// Rich content preview
	LinkPreview *LinkPreview `bson:"link_preview,omitempty" json:"link_preview,omitempty"`
}

type LocationData struct {
	Latitude  float64 `bson:"latitude" json:"latitude"`
	Longitude float64 `bson:"longitude" json:"longitude"`
	Address   string  `bson:"address,omitempty" json:"address,omitempty"`
	Name      string  `bson:"name,omitempty" json:"name,omitempty"`
}

type ContactData struct {
	Name        string `bson:"name" json:"name"`
	PhoneNumber string `bson:"phone_number" json:"phone_number"`
	Email       string `bson:"email,omitempty" json:"email,omitempty"`
	Avatar      string `bson:"avatar,omitempty" json:"avatar,omitempty"`
}

type CallMessageData struct {
	CallType     string               `bson:"call_type" json:"call_type"` // "voice", "video"
	Duration     int                  `bson:"duration" json:"duration"`   // in seconds
	Status       string               `bson:"status" json:"status"`       // "completed", "missed", "declined"
	Participants []primitive.ObjectID `bson:"participants" json:"participants"`
}

type SystemMessageData struct {
	Action    string                 `bson:"action" json:"action"`
	ActorID   primitive.ObjectID     `bson:"actor_id" json:"actor_id"`
	TargetID  *primitive.ObjectID    `bson:"target_id,omitempty" json:"target_id,omitempty"`
	OldValue  string                 `bson:"old_value,omitempty" json:"old_value,omitempty"`
	NewValue  string                 `bson:"new_value,omitempty" json:"new_value,omitempty"`
	ExtraData map[string]interface{} `bson:"extra_data,omitempty" json:"extra_data,omitempty"`
}

type LinkPreview struct {
	URL         string `bson:"url" json:"url"`
	Title       string `bson:"title" json:"title"`
	Description string `bson:"description" json:"description"`
	Image       string `bson:"image,omitempty" json:"image,omitempty"`
	SiteName    string `bson:"site_name,omitempty" json:"site_name,omitempty"`
	Type        string `bson:"type,omitempty" json:"type,omitempty"`
}

type ForwardInfo struct {
	OriginalSenderID  primitive.ObjectID `bson:"original_sender_id" json:"original_sender_id"`
	OriginalChatID    primitive.ObjectID `bson:"original_chat_id" json:"original_chat_id"`
	OriginalMessageID primitive.ObjectID `bson:"original_message_id" json:"original_message_id"`
	ForwardedAt       time.Time          `bson:"forwarded_at" json:"forwarded_at"`
	ForwardCount      int                `bson:"forward_count" json:"forward_count"`
}

type ReadReceipt struct {
	UserID primitive.ObjectID `bson:"user_id" json:"user_id"`
	ReadAt time.Time          `bson:"read_at" json:"read_at"`
}

type DeliveryReceipt struct {
	UserID      primitive.ObjectID `bson:"user_id" json:"user_id"`
	DeliveredAt time.Time          `bson:"delivered_at" json:"delivered_at"`
}

type EditHistory struct {
	OldContent string    `bson:"old_content" json:"old_content"`
	EditedAt   time.Time `bson:"edited_at" json:"edited_at"`
	EditReason string    `bson:"edit_reason,omitempty" json:"edit_reason,omitempty"`
}

type Reaction struct {
	UserID  primitive.ObjectID `bson:"user_id" json:"user_id"`
	Emoji   string             `bson:"emoji" json:"emoji"`
	AddedAt time.Time          `bson:"added_at" json:"added_at"`
}

type MessageReport struct {
	ReporterID primitive.ObjectID `bson:"reporter_id" json:"reporter_id"`
	Reason     string             `bson:"reason" json:"reason"`
	Details    string             `bson:"details,omitempty" json:"details,omitempty"`
	ReportedAt time.Time          `bson:"reported_at" json:"reported_at"`
	Status     string             `bson:"status" json:"status"` // "pending", "reviewed", "dismissed"
}

// Request/Response structs

type SendMessageRequest struct {
	ChatID      primitive.ObjectID   `json:"chat_id" validate:"required"`
	Type        MessageType          `json:"type" validate:"required"`
	Content     string               `json:"content,omitempty"`
	MediaURL    string               `json:"media_url,omitempty"`
	ReplyToID   *primitive.ObjectID  `json:"reply_to_id,omitempty"`
	Mentions    []primitive.ObjectID `json:"mentions,omitempty"`
	ScheduledAt *time.Time           `json:"scheduled_at,omitempty"`
	Metadata    MessageMetadata      `json:"metadata,omitempty"`
}

type UpdateMessageRequest struct {
	Content string `json:"content" validate:"required"`
	Reason  string `json:"reason,omitempty"`
}

type MessageStatusUpdate struct {
	MessageID primitive.ObjectID `json:"message_id" validate:"required"`
	Status    MessageStatus      `json:"status" validate:"required"`
}

type AddReactionRequest struct {
	Emoji string `json:"emoji" validate:"required"`
}

type MessageResponse struct {
	Message
	Sender         UserPublicInfo `json:"sender"`
	ReplyTo        *Message       `json:"reply_to,omitempty"`
	CanEdit        bool           `json:"can_edit"`
	CanDelete      bool           `json:"can_delete"`
	IsDeletedForMe bool           `json:"is_deleted_for_me"`
}

type MessagesResponse struct {
	Messages      []MessageResponse   `json:"messages"`
	HasMore       bool                `json:"has_more"`
	TotalCount    int64               `json:"total_count"`
	LastMessageID *primitive.ObjectID `json:"last_message_id,omitempty"`
}

// Helper methods

// BeforeCreate sets default values before creating message
func (m *Message) BeforeCreate() {
	now := time.Now()
	m.CreatedAt = now
	m.UpdatedAt = now
	m.Status = MessageStatusSent
	m.IsEncrypted = true // Default to encrypted
	m.Reactions = []Reaction{}
	m.ReadBy = []ReadReceipt{}
	m.DeliveredTo = []DeliveryReceipt{}
}

// BeforeUpdate sets updated timestamp
func (m *Message) BeforeUpdate() {
	m.UpdatedAt = time.Now()
}

// MarkAsRead marks message as read by a user
func (m *Message) MarkAsRead(userID primitive.ObjectID) {
	// Check if already read
	for _, receipt := range m.ReadBy {
		if receipt.UserID == userID {
			return
		}
	}

	// Add read receipt
	m.ReadBy = append(m.ReadBy, ReadReceipt{
		UserID: userID,
		ReadAt: time.Now(),
	})

	// Update status if this is the sender's first read
	if m.Status == MessageStatusDelivered {
		m.Status = MessageStatusRead
	}
}

// MarkAsDelivered marks message as delivered to a user
func (m *Message) MarkAsDelivered(userID primitive.ObjectID) {
	// Check if already delivered
	for _, receipt := range m.DeliveredTo {
		if receipt.UserID == userID {
			return
		}
	}

	// Add delivery receipt
	m.DeliveredTo = append(m.DeliveredTo, DeliveryReceipt{
		UserID:      userID,
		DeliveredAt: time.Now(),
	})

	// Update status if this is the first delivery
	if m.Status == MessageStatusSent {
		m.Status = MessageStatusDelivered
	}
}

// AddReaction adds a reaction to the message
func (m *Message) AddReaction(userID primitive.ObjectID, emoji string) {
	// Remove existing reaction from this user
	m.RemoveReaction(userID)

	// Add new reaction
	m.Reactions = append(m.Reactions, Reaction{
		UserID:  userID,
		Emoji:   emoji,
		AddedAt: time.Now(),
	})
}

// RemoveReaction removes a reaction from the message
func (m *Message) RemoveReaction(userID primitive.ObjectID) {
	for i, reaction := range m.Reactions {
		if reaction.UserID == userID {
			m.Reactions = append(m.Reactions[:i], m.Reactions[i+1:]...)
			break
		}
	}
}

// IsDeletedForUser checks if message is deleted for a specific user
func (m *Message) IsDeletedForUser(userID primitive.ObjectID) bool {
	if m.IsDeleted {
		return true
	}

	for _, deletedFor := range m.DeletedFor {
		if deletedFor == userID {
			return true
		}
	}

	return false
}

// DeleteForUser marks message as deleted for a specific user
func (m *Message) DeleteForUser(userID primitive.ObjectID) {
	if m.IsDeletedForUser(userID) {
		return
	}

	m.DeletedFor = append(m.DeletedFor, userID)
}

// DeleteForEveryone marks message as deleted for everyone
func (m *Message) DeleteForEveryone() {
	m.IsDeleted = true
	now := time.Now()
	m.DeletedAt = &now
	m.Type = MessageTypeDeleted
	m.Content = "This message was deleted"
	m.MediaURL = ""
}

// CanBeEditedBy checks if message can be edited by a user
func (m *Message) CanBeEditedBy(userID primitive.ObjectID) bool {
	// Only sender can edit
	if m.SenderID != userID {
		return false
	}

	// Can't edit deleted messages
	if m.IsDeleted || m.IsDeletedForUser(userID) {
		return false
	}

	// Can only edit text messages
	if m.Type != MessageTypeText {
		return false
	}

	// Can only edit within 15 minutes
	return time.Since(m.CreatedAt) <= 15*time.Minute
}

// CanBeDeletedBy checks if message can be deleted by a user
func (m *Message) CanBeDeletedBy(userID primitive.ObjectID) bool {
	// Sender can always delete (for themselves or everyone within time limit)
	if m.SenderID == userID {
		return true
	}

	// Already deleted
	if m.IsDeleted || m.IsDeletedForUser(userID) {
		return false
	}

	return true
}

// CanBeDeletedForEveryone checks if message can be deleted for everyone
func (m *Message) CanBeDeletedForEveryone(userID primitive.ObjectID) bool {
	// Only sender can delete for everyone
	if m.SenderID != userID {
		return false
	}

	// Already deleted
	if m.IsDeleted {
		return false
	}

	// Can delete for everyone within 1 hour
	return time.Since(m.CreatedAt) <= 1*time.Hour
}

// Edit edits the message content
func (m *Message) Edit(newContent, reason string) {
	// Save edit history
	if !m.IsEdited {
		m.EditHistory = []EditHistory{}
	}

	m.EditHistory = append(m.EditHistory, EditHistory{
		OldContent: m.Content,
		EditedAt:   time.Now(),
		EditReason: reason,
	})

	m.Content = newContent
	m.IsEdited = true
	m.BeforeUpdate()
}

// GetPreviewText returns a preview text for the message
func (m *Message) GetPreviewText() string {
	if m.IsDeleted {
		return "This message was deleted"
	}

	switch m.Type {
	case MessageTypeText:
		if len(m.Content) > 100 {
			return m.Content[:100] + "..."
		}
		return m.Content
	case MessageTypeImage:
		return "📷 Photo"
	case MessageTypeVideo:
		return "🎥 Video"
	case MessageTypeAudio:
		return "🎵 Audio"
	case MessageTypeVoiceNote:
		return "🎤 Voice message"
	case MessageTypeDocument:
		return "📄 " + m.FileName
	case MessageTypeLocation:
		return "📍 Location"
	case MessageTypeContact:
		return "👤 Contact"
	case MessageTypeSticker:
		return "😊 Sticker"
	case MessageTypeGIF:
		return "🎞️ GIF"
	case MessageTypeCall:
		if m.Metadata.CallData != nil {
			return "📞 " + m.Metadata.CallData.CallType + " call"
		}
		return "📞 Call"
	default:
		return "Message"
	}
}
