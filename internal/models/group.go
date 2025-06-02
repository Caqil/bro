package models

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Group struct {
	ID     primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ChatID primitive.ObjectID `bson:"chat_id" json:"chat_id"` // Reference to Chat document

	// Basic Information
	Name        string `bson:"name" json:"name" validate:"required"`
	Description string `bson:"description" json:"description"`
	Avatar      string `bson:"avatar" json:"avatar"`

	// Creator and Admin Information
	CreatedBy primitive.ObjectID   `bson:"created_by" json:"created_by"`
	Admins    []primitive.ObjectID `bson:"admins" json:"admins"`
	Owner     primitive.ObjectID   `bson:"owner" json:"owner"`

	// Members Management
	Members     []GroupMember `bson:"members" json:"members"`
	MaxMembers  int           `bson:"max_members" json:"max_members"`
	MemberCount int           `bson:"member_count" json:"member_count"`

	// Invitation & Join Settings
	InviteCode      string `bson:"invite_code" json:"invite_code"`
	InviteLink      string `bson:"invite_link" json:"invite_link"`
	IsPublic        bool   `bson:"is_public" json:"is_public"`
	RequireApproval bool   `bson:"require_approval" json:"require_approval"`

	// Group Settings
	Settings GroupSettings `bson:"settings" json:"settings"`

	// Permissions
	Permissions GroupPermissions `bson:"permissions" json:"permissions"`

	// Status and Activity
	IsActive     bool      `bson:"is_active" json:"is_active"`
	LastActivity time.Time `bson:"last_activity" json:"last_activity"`

	// Pending Requests
	JoinRequests   []JoinRequest   `bson:"join_requests" json:"join_requests"`
	PendingInvites []PendingInvite `bson:"pending_invites" json:"pending_invites"`

	// Rules and Guidelines
	Rules []GroupRule `bson:"rules" json:"rules"`

	// Announcements
	Announcements  []Announcement       `bson:"announcements" json:"announcements"`
	PinnedMessages []primitive.ObjectID `bson:"pinned_messages" json:"pinned_messages"`

	// Events and Activities
	Events []GroupEvent `bson:"events" json:"events"`

	// Statistics
	Stats GroupStats `bson:"stats" json:"stats"`

	// Timestamps
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`

	// Admin Notes and Moderation
	AdminNotes string        `bson:"admin_notes,omitempty" json:"admin_notes,omitempty"`
	IsReported bool          `bson:"is_reported" json:"is_reported"`
	Reports    []GroupReport `bson:"reports,omitempty" json:"reports,omitempty"`

	// Backup and Export
	BackupSettings BackupSettings `bson:"backup_settings" json:"backup_settings"`

	// Custom Fields
	CustomFields map[string]interface{} `bson:"custom_fields,omitempty" json:"custom_fields,omitempty"`
	Tags         []string               `bson:"tags,omitempty" json:"tags,omitempty"`
}

type GroupMember struct {
	UserID       primitive.ObjectID  `bson:"user_id" json:"user_id"`
	Role         GroupRole           `bson:"role" json:"role"`
	JoinedAt     time.Time           `bson:"joined_at" json:"joined_at"`
	InvitedBy    *primitive.ObjectID `bson:"invited_by,omitempty" json:"invited_by,omitempty"`
	LastActiveAt time.Time           `bson:"last_active_at" json:"last_active_at"`
	MessageCount int64               `bson:"message_count" json:"message_count"`
	IsActive     bool                `bson:"is_active" json:"is_active"`
	IsMuted      bool                `bson:"is_muted" json:"is_muted"`
	MutedUntil   *time.Time          `bson:"muted_until,omitempty" json:"muted_until,omitempty"`
	MutedBy      *primitive.ObjectID `bson:"muted_by,omitempty" json:"muted_by,omitempty"`
	Warnings     []MemberWarning     `bson:"warnings,omitempty" json:"warnings,omitempty"`
	CustomTitle  string              `bson:"custom_title,omitempty" json:"custom_title,omitempty"`
	Permissions  MemberPermissions   `bson:"permissions" json:"permissions"`
}

type GroupRole string

const (
	GroupRoleOwner      GroupRole = "owner"
	GroupRoleAdmin      GroupRole = "admin"
	GroupRoleModerator  GroupRole = "moderator"
	GroupRoleMember     GroupRole = "member"
	GroupRoleRestricted GroupRole = "restricted"
)

type GroupSettings struct {
	// Message Settings
	AllowMessages        bool `bson:"allow_messages" json:"allow_messages"`
	AllowMediaSharing    bool `bson:"allow_media_sharing" json:"allow_media_sharing"`
	AllowLinks           bool `bson:"allow_links" json:"allow_links"`
	AllowForwarding      bool `bson:"allow_forwarding" json:"allow_forwarding"`
	OnlyAdminsCanMessage bool `bson:"only_admins_can_message" json:"only_admins_can_message"`

	// Member Management
	OnlyAdminsCanAddMembers    bool `bson:"only_admins_can_add_members" json:"only_admins_can_add_members"`
	OnlyAdminsCanRemoveMembers bool `bson:"only_admins_can_remove_members" json:"only_admins_can_remove_members"`
	OnlyAdminsCanEditInfo      bool `bson:"only_admins_can_edit_info" json:"only_admins_can_edit_info"`
	ApprovalRequired           bool `bson:"approval_required" json:"approval_required"`

	// Privacy Settings
	ShowMemberList     bool `bson:"show_member_list" json:"show_member_list"`
	ShowMemberJoinDate bool `bson:"show_member_join_date" json:"show_member_join_date"`
	ShowLastSeen       bool `bson:"show_last_seen" json:"show_last_seen"`

	// Call Settings
	AllowVoiceCalls   bool `bson:"allow_voice_calls" json:"allow_voice_calls"`
	AllowVideoCalls   bool `bson:"allow_video_calls" json:"allow_video_calls"`
	OnlyAdminsCanCall bool `bson:"only_admins_can_call" json:"only_admins_can_call"`

	// Content Moderation
	AutoModeration   AutoModerationSettings `bson:"auto_moderation" json:"auto_moderation"`
	SlowModeEnabled  bool                   `bson:"slow_mode_enabled" json:"slow_mode_enabled"`
	SlowModeInterval time.Duration          `bson:"slow_mode_interval" json:"slow_mode_interval"`

	// Notifications
	MentionSettings  GroupMentionSettings `bson:"mention_settings" json:"mention_settings"`
	AnnouncementOnly bool                 `bson:"announcement_only" json:"announcement_only"`

	// Advanced Settings
	AutoDelete     AutoDeleteSetting `bson:"auto_delete" json:"auto_delete"`
	WelcomeMessage WelcomeMessage    `bson:"welcome_message" json:"welcome_message"`

	// Security
	RequirePhoneVerification bool `bson:"require_phone_verification" json:"require_phone_verification"`
	RequireEmailVerification bool `bson:"require_email_verification" json:"require_email_verification"`
	RestrictedMode           bool `bson:"restricted_mode" json:"restricted_mode"`
}

type GroupPermissions struct {
	// Message Permissions
	CanSendMessages bool `bson:"can_send_messages" json:"can_send_messages"`
	CanSendMedia    bool `bson:"can_send_media" json:"can_send_media"`
	CanSendStickers bool `bson:"can_send_stickers" json:"can_send_stickers"`
	CanSendGifs     bool `bson:"can_send_gifs" json:"can_send_gifs"`
	CanSendPolls    bool `bson:"can_send_polls" json:"can_send_polls"`
	CanEmbedLinks   bool `bson:"can_embed_links" json:"can_embed_links"`

	// Management Permissions
	CanAddMembers     bool `bson:"can_add_members" json:"can_add_members"`
	CanRemoveMembers  bool `bson:"can_remove_members" json:"can_remove_members"`
	CanBanMembers     bool `bson:"can_ban_members" json:"can_ban_members"`
	CanChangeInfo     bool `bson:"can_change_info" json:"can_change_info"`
	CanPinMessages    bool `bson:"can_pin_messages" json:"can_pin_messages"`
	CanDeleteMessages bool `bson:"can_delete_messages" json:"can_delete_messages"`
	CanEditMessages   bool `bson:"can_edit_messages" json:"can_edit_messages"`

	// Moderation Permissions
	CanRestrictMembers  bool `bson:"can_restrict_members" json:"can_restrict_members"`
	CanPromoteMembers   bool `bson:"can_promote_members" json:"can_promote_members"`
	CanManageVoiceChats bool `bson:"can_manage_voice_chats" json:"can_manage_voice_chats"`
	CanInviteUsers      bool `bson:"can_invite_users" json:"can_invite_users"`
}

type MemberPermissions struct {
	CanSendMessages bool       `bson:"can_send_messages" json:"can_send_messages"`
	CanSendMedia    bool       `bson:"can_send_media" json:"can_send_media"`
	CanAddMembers   bool       `bson:"can_add_members" json:"can_add_members"`
	CanPinMessages  bool       `bson:"can_pin_messages" json:"can_pin_messages"`
	RestrictedUntil *time.Time `bson:"restricted_until,omitempty" json:"restricted_until,omitempty"`
}

type JoinRequest struct {
	UserID      primitive.ObjectID  `bson:"user_id" json:"user_id"`
	RequestedAt time.Time           `bson:"requested_at" json:"requested_at"`
	Message     string              `bson:"message,omitempty" json:"message,omitempty"`
	Status      RequestStatus       `bson:"status" json:"status"`
	ReviewedBy  *primitive.ObjectID `bson:"reviewed_by,omitempty" json:"reviewed_by,omitempty"`
	ReviewedAt  *time.Time          `bson:"reviewed_at,omitempty" json:"reviewed_at,omitempty"`
	ReviewNote  string              `bson:"review_note,omitempty" json:"review_note,omitempty"`
}

type PendingInvite struct {
	InviteeID  primitive.ObjectID `bson:"invitee_id" json:"invitee_id"`
	InviterID  primitive.ObjectID `bson:"inviter_id" json:"inviter_id"`
	InvitedAt  time.Time          `bson:"invited_at" json:"invited_at"`
	ExpiresAt  *time.Time         `bson:"expires_at,omitempty" json:"expires_at,omitempty"`
	Message    string             `bson:"message,omitempty" json:"message,omitempty"`
	Status     InviteStatus       `bson:"status" json:"status"`
	AcceptedAt *time.Time         `bson:"accepted_at,omitempty" json:"accepted_at,omitempty"`
}

type GroupRule struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title       string             `bson:"title" json:"title"`
	Description string             `bson:"description" json:"description"`
	Order       int                `bson:"order" json:"order"`
	IsActive    bool               `bson:"is_active" json:"is_active"`
	CreatedBy   primitive.ObjectID `bson:"created_by" json:"created_by"`
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
}

type Announcement struct {
	ID        primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	Title     string               `bson:"title" json:"title"`
	Content   string               `bson:"content" json:"content"`
	Author    primitive.ObjectID   `bson:"author" json:"author"`
	CreatedAt time.Time            `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time            `bson:"updated_at" json:"updated_at"`
	IsActive  bool                 `bson:"is_active" json:"is_active"`
	IsPinned  bool                 `bson:"is_pinned" json:"is_pinned"`
	ExpiresAt *time.Time           `bson:"expires_at,omitempty" json:"expires_at,omitempty"`
	ViewCount int                  `bson:"view_count" json:"view_count"`
	ReadBy    []primitive.ObjectID `bson:"read_by" json:"read_by"`
}

type GroupEvent struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title        string             `bson:"title" json:"title"`
	Description  string             `bson:"description" json:"description"`
	StartTime    time.Time          `bson:"start_time" json:"start_time"`
	EndTime      time.Time          `bson:"end_time" json:"end_time"`
	Location     string             `bson:"location,omitempty" json:"location,omitempty"`
	CreatedBy    primitive.ObjectID `bson:"created_by" json:"created_by"`
	Attendees    []EventAttendee    `bson:"attendees" json:"attendees"`
	MaxAttendees int                `bson:"max_attendees,omitempty" json:"max_attendees,omitempty"`
	IsPublic     bool               `bson:"is_public" json:"is_public"`
	CreatedAt    time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt    time.Time          `bson:"updated_at" json:"updated_at"`
}

type EventAttendee struct {
	UserID      primitive.ObjectID `bson:"user_id" json:"user_id"`
	Status      AttendanceStatus   `bson:"status" json:"status"`
	RespondedAt time.Time          `bson:"responded_at" json:"responded_at"`
}

type GroupStats struct {
	TotalMessages     int64     `bson:"total_messages" json:"total_messages"`
	TotalMembers      int       `bson:"total_members" json:"total_members"`
	ActiveMembers     int       `bson:"active_members" json:"active_members"`
	MessagesToday     int64     `bson:"messages_today" json:"messages_today"`
	MessagesThisWeek  int64     `bson:"messages_this_week" json:"messages_this_week"`
	MessagesThisMonth int64     `bson:"messages_this_month" json:"messages_this_month"`
	PeakOnlineMembers int       `bson:"peak_online_members" json:"peak_online_members"`
	LastStatsUpdate   time.Time `bson:"last_stats_update" json:"last_stats_update"`
}

type MemberWarning struct {
	ID        primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	Reason    string              `bson:"reason" json:"reason"`
	IssuedBy  primitive.ObjectID  `bson:"issued_by" json:"issued_by"`
	IssuedAt  time.Time           `bson:"issued_at" json:"issued_at"`
	ExpiresAt *time.Time          `bson:"expires_at,omitempty" json:"expires_at,omitempty"`
	IsActive  bool                `bson:"is_active" json:"is_active"`
	MessageID *primitive.ObjectID `bson:"message_id,omitempty" json:"message_id,omitempty"`
}

type AutoModerationSettings struct {
	Enabled                bool     `bson:"enabled" json:"enabled"`
	FilterProfanity        bool     `bson:"filter_profanity" json:"filter_profanity"`
	FilterSpam             bool     `bson:"filter_spam" json:"filter_spam"`
	FilterLinks            bool     `bson:"filter_links" json:"filter_links"`
	AllowedDomains         []string `bson:"allowed_domains" json:"allowed_domains"`
	BlockedWords           []string `bson:"blocked_words" json:"blocked_words"`
	MaxMessageLength       int      `bson:"max_message_length" json:"max_message_length"`
	MaxConsecutiveMessages int      `bson:"max_consecutive_messages" json:"max_consecutive_messages"`
	AntiFloodEnabled       bool     `bson:"anti_flood_enabled" json:"anti_flood_enabled"`
}

type GroupMentionSettings struct {
	AllowMentions                bool `bson:"allow_mentions" json:"allow_mentions"`
	AllowEveryoneMention         bool `bson:"allow_everyone_mention" json:"allow_everyone_mention"`
	OnlyAdminsCanMentionEveryone bool `bson:"only_admins_can_mention_everyone" json:"only_admins_can_mention_everyone"`
}

type WelcomeMessage struct {
	Enabled     bool          `bson:"enabled" json:"enabled"`
	Message     string        `bson:"message" json:"message"`
	MediaURL    string        `bson:"media_url,omitempty" json:"media_url,omitempty"`
	AutoDelete  bool          `bson:"auto_delete" json:"auto_delete"`
	DeleteAfter time.Duration `bson:"delete_after" json:"delete_after"`
}

type BackupSettings struct {
	Enabled        bool          `bson:"enabled" json:"enabled"`
	Frequency      time.Duration `bson:"frequency" json:"frequency"`
	LastBackup     *time.Time    `bson:"last_backup,omitempty" json:"last_backup,omitempty"`
	BackupLocation string        `bson:"backup_location,omitempty" json:"backup_location,omitempty"`
	RetentionDays  int           `bson:"retention_days" json:"retention_days"`
}

type GroupReport struct {
	ReporterID primitive.ObjectID `bson:"reporter_id" json:"reporter_id"`
	Reason     string             `bson:"reason" json:"reason"`
	Details    string             `bson:"details,omitempty" json:"details,omitempty"`
	ReportedAt time.Time          `bson:"reported_at" json:"reported_at"`
	Status     string             `bson:"status" json:"status"` // "pending", "reviewed", "dismissed"
}

// Enums
type RequestStatus string

const (
	RequestStatusPending  RequestStatus = "pending"
	RequestStatusApproved RequestStatus = "approved"
	RequestStatusRejected RequestStatus = "rejected"
)

type InviteStatus string

const (
	InviteStatusPending  InviteStatus = "pending"
	InviteStatusAccepted InviteStatus = "accepted"
	InviteStatusDeclined InviteStatus = "declined"
	InviteStatusExpired  InviteStatus = "expired"
)

type AttendanceStatus string

const (
	AttendanceStatusGoing    AttendanceStatus = "going"
	AttendanceStatusNotGoing AttendanceStatus = "not_going"
	AttendanceStatusMaybe    AttendanceStatus = "maybe"
	AttendanceStatusPending  AttendanceStatus = "pending"
)

// Request/Response structs

type CreateGroupRequest struct {
	Name        string               `json:"name" validate:"required"`
	Description string               `json:"description,omitempty"`
	Avatar      string               `json:"avatar,omitempty"`
	Members     []primitive.ObjectID `json:"members,omitempty"`
	IsPublic    bool                 `json:"is_public"`
	Settings    *GroupSettings       `json:"settings,omitempty"`
	Rules       []GroupRule          `json:"rules,omitempty"`
}

type UpdateGroupRequest struct {
	Name        *string        `json:"name,omitempty"`
	Description *string        `json:"description,omitempty"`
	Avatar      *string        `json:"avatar,omitempty"`
	Settings    *GroupSettings `json:"settings,omitempty"`
}

type AddMemberRequest struct {
	UserID primitive.ObjectID `json:"user_id" validate:"required"`
	Role   GroupRole          `json:"role,omitempty"`
}

type UpdateMemberRoleRequest struct {
	Role GroupRole `json:"role" validate:"required"`
}

type JoinGroupRequest struct {
	InviteCode string `json:"invite_code,omitempty"`
	Message    string `json:"message,omitempty"`
}

type GroupResponse struct {
	Group
	MyRole        GroupRole             `json:"my_role"`
	MyPermissions MemberPermissions     `json:"my_permissions"`
	CanLeave      bool                  `json:"can_leave"`
	CanInvite     bool                  `json:"can_invite"`
	OnlineMembers int                   `json:"online_members"`
	MemberDetails []GroupMemberResponse `json:"member_details,omitempty"`
}

type GroupMemberResponse struct {
	GroupMember
	UserInfo UserPublicInfo `json:"user_info"`
}

// Helper methods

// BeforeCreate sets default values before creating group
func (g *Group) BeforeCreate() {
	now := time.Now()
	g.CreatedAt = now
	g.UpdatedAt = now
	g.LastActivity = now
	g.IsActive = true
	g.MaxMembers = 1000 // Default max members
	g.MemberCount = 0

	// Generate invite code
	g.InviteCode = generateInviteCode()
	g.InviteLink = "https://chat.app/invite/" + g.InviteCode

	// Set default settings
	g.Settings = GroupSettings{
		AllowMessages:              true,
		AllowMediaSharing:          true,
		AllowLinks:                 true,
		AllowForwarding:            true,
		OnlyAdminsCanMessage:       false,
		OnlyAdminsCanAddMembers:    false,
		OnlyAdminsCanRemoveMembers: true,
		OnlyAdminsCanEditInfo:      true,
		ApprovalRequired:           false,
		ShowMemberList:             true,
		ShowMemberJoinDate:         true,
		ShowLastSeen:               true,
		AllowVoiceCalls:            true,
		AllowVideoCalls:            true,
		OnlyAdminsCanCall:          false,
		AutoModeration: AutoModerationSettings{
			Enabled:                false,
			FilterProfanity:        false,
			FilterSpam:             false,
			FilterLinks:            false,
			MaxMessageLength:       4096,
			MaxConsecutiveMessages: 5,
			AntiFloodEnabled:       true,
		},
		SlowModeEnabled:  false,
		SlowModeInterval: 0,
		MentionSettings: GroupMentionSettings{
			AllowMentions:                true,
			AllowEveryoneMention:         true,
			OnlyAdminsCanMentionEveryone: false,
		},
		AnnouncementOnly: false,
		AutoDelete: AutoDeleteSetting{
			Enabled:  false,
			Duration: 0,
		},
		WelcomeMessage: WelcomeMessage{
			Enabled:     false,
			Message:     "Welcome to the group!",
			AutoDelete:  false,
			DeleteAfter: 24 * time.Hour,
		},
		RequirePhoneVerification: false,
		RequireEmailVerification: false,
		RestrictedMode:           false,
	}

	// Set default permissions
	g.Permissions = GroupPermissions{
		CanSendMessages:     true,
		CanSendMedia:        true,
		CanSendStickers:     true,
		CanSendGifs:         true,
		CanSendPolls:        true,
		CanEmbedLinks:       true,
		CanAddMembers:       false,
		CanRemoveMembers:    false,
		CanBanMembers:       false,
		CanChangeInfo:       false,
		CanPinMessages:      false,
		CanDeleteMessages:   false,
		CanEditMessages:     false,
		CanRestrictMembers:  false,
		CanPromoteMembers:   false,
		CanManageVoiceChats: false,
		CanInviteUsers:      true,
	}

	// Initialize arrays
	g.Members = []GroupMember{}
	g.Admins = []primitive.ObjectID{}
	g.JoinRequests = []JoinRequest{}
	g.PendingInvites = []PendingInvite{}
	g.Rules = []GroupRule{}
	g.Announcements = []Announcement{}
	g.PinnedMessages = []primitive.ObjectID{}
	g.Events = []GroupEvent{}
	g.Reports = []GroupReport{}

	// Initialize stats
	g.Stats = GroupStats{
		TotalMessages:     0,
		TotalMembers:      0,
		ActiveMembers:     0,
		MessagesToday:     0,
		MessagesThisWeek:  0,
		MessagesThisMonth: 0,
		PeakOnlineMembers: 0,
		LastStatsUpdate:   now,
	}

	// Set backup settings
	g.BackupSettings = BackupSettings{
		Enabled:       false,
		Frequency:     7 * 24 * time.Hour, // Weekly
		RetentionDays: 30,
	}
}

// BeforeUpdate sets updated timestamp
func (g *Group) BeforeUpdate() {
	g.UpdatedAt = time.Now()
	g.LastActivity = time.Now()
}

// AddMember adds a new member to the group
func (g *Group) AddMember(userID, inviterID primitive.ObjectID, role GroupRole) error {
	// Check if user is already a member
	if g.IsMember(userID) {
		return fmt.Errorf("user is already a member")
	}

	// Check max members limit
	if g.MemberCount >= g.MaxMembers {
		return fmt.Errorf("group has reached maximum member limit")
	}

	member := GroupMember{
		UserID:       userID,
		Role:         role,
		JoinedAt:     time.Now(),
		InvitedBy:    &inviterID,
		LastActiveAt: time.Now(),
		MessageCount: 0,
		IsActive:     true,
		IsMuted:      false,
		Warnings:     []MemberWarning{},
		Permissions: MemberPermissions{
			CanSendMessages: true,
			CanSendMedia:    true,
			CanAddMembers:   false,
			CanPinMessages:  false,
		},
	}

	g.Members = append(g.Members, member)
	g.MemberCount++

	// Add to admins if admin role
	if role == GroupRoleAdmin || role == GroupRoleOwner {
		g.Admins = append(g.Admins, userID)
	}

	// Set as owner if first member or specified
	if role == GroupRoleOwner {
		g.Owner = userID
	}

	g.BeforeUpdate()
	return nil
}

// RemoveMember removes a member from the group
func (g *Group) RemoveMember(userID primitive.ObjectID) error {
	// Find and remove member
	for i, member := range g.Members {
		if member.UserID == userID {
			g.Members = append(g.Members[:i], g.Members[i+1:]...)
			g.MemberCount--
			break
		}
	}

	// Remove from admins if admin
	for i, adminID := range g.Admins {
		if adminID == userID {
			g.Admins = append(g.Admins[:i], g.Admins[i+1:]...)
			break
		}
	}

	// Cannot remove owner
	if g.Owner == userID {
		return fmt.Errorf("cannot remove group owner")
	}

	g.BeforeUpdate()
	return nil
}

// UpdateMemberRole updates a member's role
func (g *Group) UpdateMemberRole(userID primitive.ObjectID, newRole GroupRole) error {
	// Find member
	for i := range g.Members {
		if g.Members[i].UserID == userID {
			oldRole := g.Members[i].Role
			g.Members[i].Role = newRole

			// Update admin list
			if oldRole == GroupRoleAdmin || oldRole == GroupRoleOwner {
				// Remove from admins
				for j, adminID := range g.Admins {
					if adminID == userID {
						g.Admins = append(g.Admins[:j], g.Admins[j+1:]...)
						break
					}
				}
			}

			if newRole == GroupRoleAdmin || newRole == GroupRoleOwner {
				// Add to admins
				g.Admins = append(g.Admins, userID)
			}

			if newRole == GroupRoleOwner {
				g.Owner = userID
			}

			g.BeforeUpdate()
			return nil
		}
	}

	return fmt.Errorf("member not found")
}

// IsMember checks if a user is a member
func (g *Group) IsMember(userID primitive.ObjectID) bool {
	for _, member := range g.Members {
		if member.UserID == userID && member.IsActive {
			return true
		}
	}
	return false
}

// IsAdmin checks if a user is an admin
func (g *Group) IsAdmin(userID primitive.ObjectID) bool {
	for _, adminID := range g.Admins {
		if adminID == userID {
			return true
		}
	}
	return false
}

// IsOwner checks if a user is the owner
func (g *Group) IsOwner(userID primitive.ObjectID) bool {
	return g.Owner == userID
}

// GetMember returns a member by user ID
func (g *Group) GetMember(userID primitive.ObjectID) *GroupMember {
	for i := range g.Members {
		if g.Members[i].UserID == userID {
			return &g.Members[i]
		}
	}
	return nil
}

// CanUserPerformAction checks if a user can perform a specific action
func (g *Group) CanUserPerformAction(userID primitive.ObjectID, action string) bool {
	member := g.GetMember(userID)
	if member == nil || !member.IsActive {
		return false
	}

	// Owner can do everything
	if g.IsOwner(userID) {
		return true
	}

	// Check admin privileges
	isAdmin := g.IsAdmin(userID)

	switch action {
	case "send_message":
		if g.Settings.OnlyAdminsCanMessage && !isAdmin {
			return false
		}
		return member.Permissions.CanSendMessages

	case "send_media":
		if !g.Settings.AllowMediaSharing {
			return false
		}
		return member.Permissions.CanSendMedia

	case "add_member":
		if g.Settings.OnlyAdminsCanAddMembers && !isAdmin {
			return false
		}
		return member.Permissions.CanAddMembers

	case "remove_member":
		return isAdmin || member.Permissions.CanAddMembers

	case "edit_info":
		if g.Settings.OnlyAdminsCanEditInfo && !isAdmin {
			return false
		}
		return isAdmin

	case "pin_message":
		return isAdmin || member.Permissions.CanPinMessages

	case "delete_message":
		return isAdmin

	default:
		return false
	}
}

// MuteMember mutes a member
func (g *Group) MuteMember(userID, mutedBy primitive.ObjectID, mutedUntil *time.Time) error {
	member := g.GetMember(userID)
	if member == nil {
		return fmt.Errorf("member not found")
	}

	member.IsMuted = true
	member.MutedUntil = mutedUntil
	member.MutedBy = &mutedBy

	g.BeforeUpdate()
	return nil
}

// UnmuteMember unmutes a member
func (g *Group) UnmuteMember(userID primitive.ObjectID) error {
	member := g.GetMember(userID)
	if member == nil {
		return fmt.Errorf("member not found")
	}

	member.IsMuted = false
	member.MutedUntil = nil
	member.MutedBy = nil

	g.BeforeUpdate()
	return nil
}

// AddWarning adds a warning to a member
func (g *Group) AddWarning(userID, issuedBy primitive.ObjectID, reason string, expiresAt *time.Time) error {
	member := g.GetMember(userID)
	if member == nil {
		return fmt.Errorf("member not found")
	}

	warning := MemberWarning{
		ID:        primitive.NewObjectID(),
		Reason:    reason,
		IssuedBy:  issuedBy,
		IssuedAt:  time.Now(),
		ExpiresAt: expiresAt,
		IsActive:  true,
	}

	member.Warnings = append(member.Warnings, warning)
	g.BeforeUpdate()
	return nil
}

// AddJoinRequest adds a join request
func (g *Group) AddJoinRequest(userID primitive.ObjectID, message string) error {
	// Check if request already exists
	for _, request := range g.JoinRequests {
		if request.UserID == userID && request.Status == RequestStatusPending {
			return fmt.Errorf("join request already exists")
		}
	}

	request := JoinRequest{
		UserID:      userID,
		RequestedAt: time.Now(),
		Message:     message,
		Status:      RequestStatusPending,
	}

	g.JoinRequests = append(g.JoinRequests, request)
	g.BeforeUpdate()
	return nil
}

// ApproveJoinRequest approves a join request
func (g *Group) ApproveJoinRequest(userID, reviewerID primitive.ObjectID) error {
	for i := range g.JoinRequests {
		if g.JoinRequests[i].UserID == userID && g.JoinRequests[i].Status == RequestStatusPending {
			g.JoinRequests[i].Status = RequestStatusApproved
			g.JoinRequests[i].ReviewedBy = &reviewerID
			now := time.Now()
			g.JoinRequests[i].ReviewedAt = &now

			// Add as member
			return g.AddMember(userID, reviewerID, GroupRoleMember)
		}
	}
	return fmt.Errorf("join request not found")
}

// RejectJoinRequest rejects a join request
func (g *Group) RejectJoinRequest(userID, reviewerID primitive.ObjectID, note string) error {
	for i := range g.JoinRequests {
		if g.JoinRequests[i].UserID == userID && g.JoinRequests[i].Status == RequestStatusPending {
			g.JoinRequests[i].Status = RequestStatusRejected
			g.JoinRequests[i].ReviewedBy = &reviewerID
			now := time.Now()
			g.JoinRequests[i].ReviewedAt = &now
			g.JoinRequests[i].ReviewNote = note

			g.BeforeUpdate()
			return nil
		}
	}
	return fmt.Errorf("join request not found")
}

// AddAnnouncement adds a new announcement
func (g *Group) AddAnnouncement(title, content string, author primitive.ObjectID) *Announcement {
	announcement := Announcement{
		ID:        primitive.NewObjectID(),
		Title:     title,
		Content:   content,
		Author:    author,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		IsActive:  true,
		IsPinned:  false,
		ViewCount: 0,
		ReadBy:    []primitive.ObjectID{},
	}

	g.Announcements = append(g.Announcements, announcement)
	g.BeforeUpdate()
	return &announcement
}

// PinMessage pins a message
func (g *Group) PinMessage(messageID primitive.ObjectID) {
	// Check if already pinned
	for _, pinnedID := range g.PinnedMessages {
		if pinnedID == messageID {
			return
		}
	}

	g.PinnedMessages = append(g.PinnedMessages, messageID)
	g.BeforeUpdate()
}

// UnpinMessage unpins a message
func (g *Group) UnpinMessage(messageID primitive.ObjectID) {
	for i, pinnedID := range g.PinnedMessages {
		if pinnedID == messageID {
			g.PinnedMessages = append(g.PinnedMessages[:i], g.PinnedMessages[i+1:]...)
			break
		}
	}
	g.BeforeUpdate()
}

// UpdateStats updates group statistics
func (g *Group) UpdateStats() {
	now := time.Now()
	g.Stats.LastStatsUpdate = now
	g.Stats.TotalMembers = g.MemberCount

	// Count active members (active in last 7 days)
	activeCount := 0
	for _, member := range g.Members {
		if member.IsActive && now.Sub(member.LastActiveAt) <= 7*24*time.Hour {
			activeCount++
		}
	}
	g.Stats.ActiveMembers = activeCount

	g.BeforeUpdate()
}

// Helper function to generate invite code
func generateInviteCode() string {
	// This would generate a random invite code
	// For now, return a placeholder
	return primitive.NewObjectID().Hex()[:8]
}
