package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Chat struct {
	ID   primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Type ChatType           `bson:"type" json:"type"`

	// Participants
	Participants []primitive.ObjectID `bson:"participants" json:"participants"`
	CreatedBy    primitive.ObjectID   `bson:"created_by" json:"created_by"`

	// Chat Information
	Name        string `bson:"name,omitempty" json:"name,omitempty"`
	Description string `bson:"description,omitempty" json:"description,omitempty"`
	Avatar      string `bson:"avatar,omitempty" json:"avatar,omitempty"`

	// Settings
	Settings ChatSettings `bson:"settings" json:"settings"`

	// Last Message Info
	LastMessage  *LastMessageInfo `bson:"last_message,omitempty" json:"last_message,omitempty"`
	LastActivity time.Time        `bson:"last_activity" json:"last_activity"`

	// Message Counts
	MessageCount int64         `bson:"message_count" json:"message_count"`
	UnreadCounts []UnreadCount `bson:"unread_counts" json:"unread_counts"`

	// Status
	IsActive   bool          `bson:"is_active" json:"is_active"`
	IsArchived []ArchivedFor `bson:"is_archived" json:"is_archived"`
	IsMuted    []MutedFor    `bson:"is_muted" json:"is_muted"`
	IsPinned   []PinnedFor   `bson:"is_pinned" json:"is_pinned"`
	IsBlocked  []BlockedFor  `bson:"is_blocked" json:"is_blocked"`

	// Privacy & Security
	IsEncrypted   bool   `bson:"is_encrypted" json:"is_encrypted"`
	EncryptionKey string `bson:"encryption_key,omitempty" json:"-"`

	// Timestamps
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`

	// Draft Messages (per user)
	DraftMessages []DraftMessage `bson:"draft_messages,omitempty" json:"draft_messages,omitempty"`

	// Admin & Moderation
	IsReported  bool                `bson:"is_reported" json:"is_reported"`
	Reports     []ChatReport        `bson:"reports,omitempty" json:"reports,omitempty"`
	ModeratedBy *primitive.ObjectID `bson:"moderated_by,omitempty" json:"moderated_by,omitempty"`

	// Metadata
	Metadata map[string]interface{} `bson:"metadata,omitempty" json:"metadata,omitempty"`
}

type ChatType string

const (
	ChatTypePrivate   ChatType = "private"   // 1-on-1 chat
	ChatTypeGroup     ChatType = "group"     // Group chat
	ChatTypeBroadcast ChatType = "broadcast" // Broadcast channel
	ChatTypeBot       ChatType = "bot"       // Bot chat
	ChatTypeSupport   ChatType = "support"   // Support chat
)

type ChatSettings struct {
	// Message Settings
	AllowMessages      bool `bson:"allow_messages" json:"allow_messages"`
	AllowMediaSharing  bool `bson:"allow_media_sharing" json:"allow_media_sharing"`
	AllowVoiceMessages bool `bson:"allow_voice_messages" json:"allow_voice_messages"`
	AllowDocuments     bool `bson:"allow_documents" json:"allow_documents"`

	// Call Settings
	AllowVoiceCalls bool `bson:"allow_voice_calls" json:"allow_voice_calls"`
	AllowVideoCalls bool `bson:"allow_video_calls" json:"allow_video_calls"`

	// Group Settings (for group chats)
	OnlyAdminsCanMessage    bool `bson:"only_admins_can_message" json:"only_admins_can_message"`
	OnlyAdminsCanAddMembers bool `bson:"only_admins_can_add_members" json:"only_admins_can_add_members"`
	OnlyAdminsCanEditInfo   bool `bson:"only_admins_can_edit_info" json:"only_admins_can_edit_info"`
	ApprovalRequired        bool `bson:"approval_required" json:"approval_required"`

	// Auto-delete settings
	MessageAutoDelete AutoDeleteSetting `bson:"message_auto_delete" json:"message_auto_delete"`

	// Notifications
	MentionSettings MentionSettings `bson:"mention_settings" json:"mention_settings"`

	// Privacy
	ReadReceipts     bool `bson:"read_receipts" json:"read_receipts"`
	TypingIndicators bool `bson:"typing_indicators" json:"typing_indicators"`
	OnlineStatus     bool `bson:"online_status" json:"online_status"`

	// Backup & Export
	AllowBackup bool `bson:"allow_backup" json:"allow_backup"`
	AllowExport bool `bson:"allow_export" json:"allow_export"`
}

type LastMessageInfo struct {
	MessageID primitive.ObjectID `bson:"message_id" json:"message_id"`
	SenderID  primitive.ObjectID `bson:"sender_id" json:"sender_id"`
	Content   string             `bson:"content" json:"content"`
	Type      MessageType        `bson:"type" json:"type"`
	Timestamp time.Time          `bson:"timestamp" json:"timestamp"`
	IsDeleted bool               `bson:"is_deleted" json:"is_deleted"`
}

type UnreadCount struct {
	UserID       primitive.ObjectID `bson:"user_id" json:"user_id"`
	Count        int64              `bson:"count" json:"count"`
	LastReadAt   time.Time          `bson:"last_read_at" json:"last_read_at"`
	MentionCount int64              `bson:"mention_count" json:"mention_count"`
}

type ArchivedFor struct {
	UserID     primitive.ObjectID `bson:"user_id" json:"user_id"`
	ArchivedAt time.Time          `bson:"archived_at" json:"archived_at"`
}

type MutedFor struct {
	UserID     primitive.ObjectID `bson:"user_id" json:"user_id"`
	MutedAt    time.Time          `bson:"muted_at" json:"muted_at"`
	MutedUntil *time.Time         `bson:"muted_until,omitempty" json:"muted_until,omitempty"`
}

type PinnedFor struct {
	UserID   primitive.ObjectID `bson:"user_id" json:"user_id"`
	PinnedAt time.Time          `bson:"pinned_at" json:"pinned_at"`
}

type BlockedFor struct {
	UserID    primitive.ObjectID `bson:"user_id" json:"user_id"`
	BlockedAt time.Time          `bson:"blocked_at" json:"blocked_at"`
}

type DraftMessage struct {
	UserID    primitive.ObjectID `bson:"user_id" json:"user_id"`
	Content   string             `bson:"content" json:"content"`
	Type      MessageType        `bson:"type" json:"type"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`
}

type AutoDeleteSetting struct {
	Enabled  bool          `bson:"enabled" json:"enabled"`
	Duration time.Duration `bson:"duration" json:"duration"` // Auto-delete after this duration
}

type MentionSettings struct {
	AllowMentions    bool `bson:"allow_mentions" json:"allow_mentions"`
	OnlyFromContacts bool `bson:"only_from_contacts" json:"only_from_contacts"`
	MentionEveryone  bool `bson:"mention_everyone" json:"mention_everyone"`
}

type ChatReport struct {
	ReporterID primitive.ObjectID `bson:"reporter_id" json:"reporter_id"`
	Reason     string             `bson:"reason" json:"reason"`
	Details    string             `bson:"details,omitempty" json:"details,omitempty"`
	ReportedAt time.Time          `bson:"reported_at" json:"reported_at"`
	Status     string             `bson:"status" json:"status"` // "pending", "reviewed", "dismissed"
}

// Request/Response structs

type CreateChatRequest struct {
	Type         ChatType             `json:"type" validate:"required"`
	Participants []primitive.ObjectID `json:"participants" validate:"required"`
	Name         string               `json:"name,omitempty"`
	Description  string               `json:"description,omitempty"`
	Settings     *ChatSettings        `json:"settings,omitempty"`
}

type UpdateChatRequest struct {
	Name        *string       `json:"name,omitempty"`
	Description *string       `json:"description,omitempty"`
	Avatar      *string       `json:"avatar,omitempty"`
	Settings    *ChatSettings `json:"settings,omitempty"`
}

type ChatMemberAction struct {
	Action string             `json:"action" validate:"required"` // "add", "remove", "promote", "demote"
	UserID primitive.ObjectID `json:"user_id" validate:"required"`
}

type ChatResponse struct {
	Chat
	UnreadCount     int64            `json:"unread_count"`
	MentionCount    int64            `json:"mention_count"`
	IsMutedByMe     bool             `json:"is_muted_by_me"`
	IsArchivedByMe  bool             `json:"is_archived_by_me"`
	IsPinnedByMe    bool             `json:"is_pinned_by_me"`
	IsBlockedByMe   bool             `json:"is_blocked_by_me"`
	MyDraft         *DraftMessage    `json:"my_draft,omitempty"`
	ParticipantInfo []UserPublicInfo `json:"participant_info"`
	CanMessage      bool             `json:"can_message"`
	CanCall         bool             `json:"can_call"`
	CanAddMembers   bool             `json:"can_add_members"`
	CanEditInfo     bool             `json:"can_edit_info"`
}

type ChatsListResponse struct {
	Chats      []ChatResponse `json:"chats"`
	TotalCount int64          `json:"total_count"`
	HasMore    bool           `json:"has_more"`
}

// Helper methods

// BeforeCreate sets default values before creating chat
func (c *Chat) BeforeCreate() {
	now := time.Now()
	c.CreatedAt = now
	c.UpdatedAt = now
	c.LastActivity = now
	c.IsActive = true
	c.IsEncrypted = true
	c.MessageCount = 0

	// Set default settings
	c.Settings = ChatSettings{
		AllowMessages:           true,
		AllowMediaSharing:       true,
		AllowVoiceMessages:      true,
		AllowDocuments:          true,
		AllowVoiceCalls:         true,
		AllowVideoCalls:         true,
		OnlyAdminsCanMessage:    false,
		OnlyAdminsCanAddMembers: false,
		OnlyAdminsCanEditInfo:   false,
		ApprovalRequired:        false,
		MessageAutoDelete: AutoDeleteSetting{
			Enabled:  false,
			Duration: 0,
		},
		MentionSettings: MentionSettings{
			AllowMentions:    true,
			OnlyFromContacts: false,
			MentionEveryone:  true,
		},
		ReadReceipts:     true,
		TypingIndicators: true,
		OnlineStatus:     true,
		AllowBackup:      true,
		AllowExport:      true,
	}

	// Initialize arrays
	c.UnreadCounts = []UnreadCount{}
	c.IsArchived = []ArchivedFor{}
	c.IsMuted = []MutedFor{}
	c.IsPinned = []PinnedFor{}
	c.IsBlocked = []BlockedFor{}
	c.DraftMessages = []DraftMessage{}
	c.Reports = []ChatReport{}

	// Initialize unread counts for all participants
	for _, participantID := range c.Participants {
		c.UnreadCounts = append(c.UnreadCounts, UnreadCount{
			UserID:       participantID,
			Count:        0,
			LastReadAt:   now,
			MentionCount: 0,
		})
	}
}

// BeforeUpdate sets updated timestamp
func (c *Chat) BeforeUpdate() {
	c.UpdatedAt = time.Now()
}

// AddParticipant adds a new participant to the chat
func (c *Chat) AddParticipant(userID primitive.ObjectID) {
	// Check if user is already a participant
	for _, participantID := range c.Participants {
		if participantID == userID {
			return
		}
	}

	// Add to participants
	c.Participants = append(c.Participants, userID)

	// Initialize unread count for new participant
	c.UnreadCounts = append(c.UnreadCounts, UnreadCount{
		UserID:       userID,
		Count:        0,
		LastReadAt:   time.Now(),
		MentionCount: 0,
	})

	c.BeforeUpdate()
}

// RemoveParticipant removes a participant from the chat
func (c *Chat) RemoveParticipant(userID primitive.ObjectID) {
	// Remove from participants
	for i, participantID := range c.Participants {
		if participantID == userID {
			c.Participants = append(c.Participants[:i], c.Participants[i+1:]...)
			break
		}
	}

	// Remove unread count
	for i, unread := range c.UnreadCounts {
		if unread.UserID == userID {
			c.UnreadCounts = append(c.UnreadCounts[:i], c.UnreadCounts[i+1:]...)
			break
		}
	}

	// Remove from archived, muted, pinned, blocked lists
	c.removeFromUserLists(userID)

	c.BeforeUpdate()
}

// IsParticipant checks if a user is a participant
func (c *Chat) IsParticipant(userID primitive.ObjectID) bool {
	for _, participantID := range c.Participants {
		if participantID == userID {
			return true
		}
	}
	return false
}

// GetUnreadCount returns unread count for a user
func (c *Chat) GetUnreadCount(userID primitive.ObjectID) *UnreadCount {
	for i := range c.UnreadCounts {
		if c.UnreadCounts[i].UserID == userID {
			return &c.UnreadCounts[i]
		}
	}
	return nil
}

// IncrementUnreadCount increments unread count for all participants except sender
func (c *Chat) IncrementUnreadCount(senderID primitive.ObjectID, isMention bool) {
	for i := range c.UnreadCounts {
		if c.UnreadCounts[i].UserID != senderID {
			c.UnreadCounts[i].Count++
			if isMention {
				c.UnreadCounts[i].MentionCount++
			}
		}
	}
	c.MessageCount++
	c.LastActivity = time.Now()
	c.BeforeUpdate()
}

// MarkAsRead marks chat as read for a user
func (c *Chat) MarkAsRead(userID primitive.ObjectID) {
	for i := range c.UnreadCounts {
		if c.UnreadCounts[i].UserID == userID {
			c.UnreadCounts[i].Count = 0
			c.UnreadCounts[i].MentionCount = 0
			c.UnreadCounts[i].LastReadAt = time.Now()
			break
		}
	}
	c.BeforeUpdate()
}

// UpdateLastMessage updates the last message info
func (c *Chat) UpdateLastMessage(messageID, senderID primitive.ObjectID, content string, msgType MessageType) {
	c.LastMessage = &LastMessageInfo{
		MessageID: messageID,
		SenderID:  senderID,
		Content:   content,
		Type:      msgType,
		Timestamp: time.Now(),
		IsDeleted: false,
	}
	c.LastActivity = time.Now()
	c.BeforeUpdate()
}

// Archive archives the chat for a user
func (c *Chat) Archive(userID primitive.ObjectID) {
	// Check if already archived
	for _, archived := range c.IsArchived {
		if archived.UserID == userID {
			return
		}
	}

	c.IsArchived = append(c.IsArchived, ArchivedFor{
		UserID:     userID,
		ArchivedAt: time.Now(),
	})
	c.BeforeUpdate()
}

// Unarchive unarchives the chat for a user
func (c *Chat) Unarchive(userID primitive.ObjectID) {
	for i, archived := range c.IsArchived {
		if archived.UserID == userID {
			c.IsArchived = append(c.IsArchived[:i], c.IsArchived[i+1:]...)
			break
		}
	}
	c.BeforeUpdate()
}

// Mute mutes the chat for a user
func (c *Chat) Mute(userID primitive.ObjectID, mutedUntil *time.Time) {
	// Remove existing mute first
	c.Unmute(userID)

	c.IsMuted = append(c.IsMuted, MutedFor{
		UserID:     userID,
		MutedAt:    time.Now(),
		MutedUntil: mutedUntil,
	})
	c.BeforeUpdate()
}

// Unmute unmutes the chat for a user
func (c *Chat) Unmute(userID primitive.ObjectID) {
	for i, muted := range c.IsMuted {
		if muted.UserID == userID {
			c.IsMuted = append(c.IsMuted[:i], c.IsMuted[i+1:]...)
			break
		}
	}
	c.BeforeUpdate()
}

// Pin pins the chat for a user
func (c *Chat) Pin(userID primitive.ObjectID) {
	// Check if already pinned
	for _, pinned := range c.IsPinned {
		if pinned.UserID == userID {
			return
		}
	}

	c.IsPinned = append(c.IsPinned, PinnedFor{
		UserID:   userID,
		PinnedAt: time.Now(),
	})
	c.BeforeUpdate()
}

// Unpin unpins the chat for a user
func (c *Chat) Unpin(userID primitive.ObjectID) {
	for i, pinned := range c.IsPinned {
		if pinned.UserID == userID {
			c.IsPinned = append(c.IsPinned[:i], c.IsPinned[i+1:]...)
			break
		}
	}
	c.BeforeUpdate()
}

// Block blocks the chat for a user
func (c *Chat) Block(userID primitive.ObjectID) {
	// Check if already blocked
	for _, blocked := range c.IsBlocked {
		if blocked.UserID == userID {
			return
		}
	}

	c.IsBlocked = append(c.IsBlocked, BlockedFor{
		UserID:    userID,
		BlockedAt: time.Now(),
	})
	c.BeforeUpdate()
}

// Unblock unblocks the chat for a user
func (c *Chat) Unblock(userID primitive.ObjectID) {
	for i, blocked := range c.IsBlocked {
		if blocked.UserID == userID {
			c.IsBlocked = append(c.IsBlocked[:i], c.IsBlocked[i+1:]...)
			break
		}
	}
	c.BeforeUpdate()
}

// SetDraft sets draft message for a user
func (c *Chat) SetDraft(userID primitive.ObjectID, content string, msgType MessageType) {
	// Remove existing draft
	c.ClearDraft(userID)

	if content != "" {
		c.DraftMessages = append(c.DraftMessages, DraftMessage{
			UserID:    userID,
			Content:   content,
			Type:      msgType,
			UpdatedAt: time.Now(),
		})
	}
	c.BeforeUpdate()
}

// ClearDraft clears draft message for a user
func (c *Chat) ClearDraft(userID primitive.ObjectID) {
	for i, draft := range c.DraftMessages {
		if draft.UserID == userID {
			c.DraftMessages = append(c.DraftMessages[:i], c.DraftMessages[i+1:]...)
			break
		}
	}
}

// GetDraft returns draft message for a user
func (c *Chat) GetDraft(userID primitive.ObjectID) *DraftMessage {
	for i := range c.DraftMessages {
		if c.DraftMessages[i].UserID == userID {
			return &c.DraftMessages[i]
		}
	}
	return nil
}

// IsArchivedFor checks if chat is archived for a user
func (c *Chat) IsArchivedFor(userID primitive.ObjectID) bool {
	for _, archived := range c.IsArchived {
		if archived.UserID == userID {
			return true
		}
	}
	return false
}

// IsMutedFor checks if chat is muted for a user
func (c *Chat) IsMutedFor(userID primitive.ObjectID) bool {
	for _, muted := range c.IsMuted {
		if muted.UserID == userID {
			// Check if mute has expired
			if muted.MutedUntil != nil && time.Now().After(*muted.MutedUntil) {
				return false
			}
			return true
		}
	}
	return false
}

// IsPinnedFor checks if chat is pinned for a user
func (c *Chat) IsPinnedFor(userID primitive.ObjectID) bool {
	for _, pinned := range c.IsPinned {
		if pinned.UserID == userID {
			return true
		}
	}
	return false
}

// IsBlockedFor checks if chat is blocked for a user
func (c *Chat) IsBlockedFor(userID primitive.ObjectID) bool {
	for _, blocked := range c.IsBlocked {
		if blocked.UserID == userID {
			return true
		}
	}
	return false
}

// CanUserMessage checks if a user can send messages
func (c *Chat) CanUserMessage(userID primitive.ObjectID) bool {
	if !c.IsActive || c.IsBlockedFor(userID) {
		return false
	}

	if !c.Settings.AllowMessages {
		return false
	}

	// For group chats, check admin restrictions
	if c.Type == ChatTypeGroup && c.Settings.OnlyAdminsCanMessage {
		// This would need to check if user is admin in group
		// Implementation depends on Group model
		return false
	}

	return true
}

// GetChatName returns appropriate chat name for a user
func (c *Chat) GetChatName(forUserID primitive.ObjectID) string {
	if c.Name != "" {
		return c.Name
	}

	// For private chats, return the other participant's name
	if c.Type == ChatTypePrivate && len(c.Participants) == 2 {
		// This would need to fetch the other user's name
		// Implementation depends on your service layer
		return "Private Chat"
	}

	return "Chat"
}

// Helper function to remove user from all user-specific lists
func (c *Chat) removeFromUserLists(userID primitive.ObjectID) {
	// Remove from archived
	for i, archived := range c.IsArchived {
		if archived.UserID == userID {
			c.IsArchived = append(c.IsArchived[:i], c.IsArchived[i+1:]...)
			break
		}
	}

	// Remove from muted
	for i, muted := range c.IsMuted {
		if muted.UserID == userID {
			c.IsMuted = append(c.IsMuted[:i], c.IsMuted[i+1:]...)
			break
		}
	}

	// Remove from pinned
	for i, pinned := range c.IsPinned {
		if pinned.UserID == userID {
			c.IsPinned = append(c.IsPinned[:i], c.IsPinned[i+1:]...)
			break
		}
	}

	// Remove from blocked
	for i, blocked := range c.IsBlocked {
		if blocked.UserID == userID {
			c.IsBlocked = append(c.IsBlocked[:i], c.IsBlocked[i+1:]...)
			break
		}
	}

	// Remove drafts
	c.ClearDraft(userID)
}
