package models

import (
	"fmt"
	"math/rand"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type User struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	PhoneNumber     string             `bson:"phone_number" json:"phone_number" validate:"required"`
	CountryCode     string             `bson:"country_code" json:"country_code" validate:"required"`
	FullPhoneNumber string             `bson:"full_phone_number" json:"full_phone_number"`
	IsPhoneVerified bool               `bson:"is_phone_verified" json:"is_phone_verified"`

	// Profile Information
	Name     string `bson:"name" json:"name" validate:"required"`
	Bio      string `bson:"bio" json:"bio"`
	Avatar   string `bson:"avatar" json:"avatar"`
	Email    string `bson:"email" json:"email"`
	Username string `bson:"username" json:"username"`

	// Security
	PasswordHash     string `bson:"password_hash" json:"-"`
	TwoFactorEnabled bool   `bson:"two_factor_enabled" json:"two_factor_enabled"`
	TwoFactorSecret  string `bson:"two_factor_secret" json:"-"`

	// Privacy Settings
	Privacy PrivacySettings `bson:"privacy" json:"privacy"`

	// Status & Activity
	Status       UserStatus    `bson:"status" json:"status"`
	LastSeen     time.Time     `bson:"last_seen" json:"last_seen"`
	IsOnline     bool          `bson:"is_online" json:"is_online"`
	DeviceTokens []DeviceToken `bson:"device_tokens" json:"device_tokens"`

	// Contacts & Social
	Contacts     []primitive.ObjectID `bson:"contacts" json:"contacts"`
	BlockedUsers []primitive.ObjectID `bson:"blocked_users" json:"blocked_users"`
	BlockedBy    []primitive.ObjectID `bson:"blocked_by" json:"blocked_by"`

	// Account Management
	IsActive     bool       `bson:"is_active" json:"is_active"`
	IsDeleted    bool       `bson:"is_deleted" json:"is_deleted"`
	DeletionDate *time.Time `bson:"deletion_date,omitempty" json:"deletion_date,omitempty"`

	// Admin
	Role       UserRole `bson:"role" json:"role"`
	AdminNotes string   `bson:"admin_notes" json:"admin_notes,omitempty"`

	// Timestamps
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`

	// OTP for verification
	OTP OTPData `bson:"otp" json:"-"`

	// Subscription & Premium Features
	Subscription SubscriptionData `bson:"subscription" json:"subscription"`
}

type PrivacySettings struct {
	ProfilePhoto    PrivacyLevel `bson:"profile_photo" json:"profile_photo"`
	LastSeen        PrivacyLevel `bson:"last_seen" json:"last_seen"`
	About           PrivacyLevel `bson:"about" json:"about"`
	Status          PrivacyLevel `bson:"status" json:"status"`
	ReadReceipts    bool         `bson:"read_receipts" json:"read_receipts"`
	GroupInvites    PrivacyLevel `bson:"group_invites" json:"group_invites"`
	CallPermissions PrivacyLevel `bson:"call_permissions" json:"call_permissions"`
}

type UserStatus struct {
	Text      string    `bson:"text" json:"text"`
	Emoji     string    `bson:"emoji" json:"emoji"`
	ExpiresAt time.Time `bson:"expires_at" json:"expires_at"`
	IsActive  bool      `bson:"is_active" json:"is_active"`
}

type DeviceToken struct {
	Token      string    `bson:"token" json:"token"`
	Platform   string    `bson:"platform" json:"platform"` // "ios", "android", "web"
	DeviceID   string    `bson:"device_id" json:"device_id"`
	AppVersion string    `bson:"app_version" json:"app_version"`
	IsActive   bool      `bson:"is_active" json:"is_active"`
	CreatedAt  time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt  time.Time `bson:"updated_at" json:"updated_at"`
}

type OTPData struct {
	Code      string    `bson:"code" json:"-"`
	ExpiresAt time.Time `bson:"expires_at" json:"-"`
	Attempts  int       `bson:"attempts" json:"-"`
	IsUsed    bool      `bson:"is_used" json:"-"`
	CreatedAt time.Time `bson:"created_at" json:"-"`
}

type SubscriptionData struct {
	Plan      string    `bson:"plan" json:"plan"` // "free", "premium", "business"
	IsActive  bool      `bson:"is_active" json:"is_active"`
	ExpiresAt time.Time `bson:"expires_at" json:"expires_at"`
	Features  []string  `bson:"features" json:"features"`
}

type PrivacyLevel string

const (
	PrivacyEveryone PrivacyLevel = "everyone"
	PrivacyContacts PrivacyLevel = "contacts"
	PrivacyNobody   PrivacyLevel = "nobody"
	PrivacyExcept   PrivacyLevel = "except" // Contacts except specific users
	PrivacyOnly     PrivacyLevel = "only"   // Only specific users
)

type UserRole string

const (
	RoleUser      UserRole = "user"
	RoleModerator UserRole = "moderator"
	RoleAdmin     UserRole = "admin"
	RoleSuper     UserRole = "super_admin"
)

// UserLoginRequest represents login request
type UserLoginRequest struct {
	PhoneNumber string `json:"phone_number" validate:"required"`
	CountryCode string `json:"country_code" validate:"required"`
	Password    string `json:"password,omitempty"`
}

// UserRegisterRequest represents registration request
type UserRegisterRequest struct {
	PhoneNumber string `json:"phone_number" validate:"required"`
	CountryCode string `json:"country_code" validate:"required"`
	Name        string `json:"name" validate:"required"`
	Password    string `json:"password,omitempty"`
}

// OTPVerificationRequest represents OTP verification request
type OTPVerificationRequest struct {
	PhoneNumber string `json:"phone_number" validate:"required"`
	CountryCode string `json:"country_code" validate:"required"`
	OTP         string `json:"otp" validate:"required,len=6"`
}

// UpdateProfileRequest represents profile update request
type UpdateProfileRequest struct {
	Name     string `json:"name,omitempty"`
	Bio      string `json:"bio,omitempty"`
	Email    string `json:"email,omitempty"`
	Username string `json:"username,omitempty"`
}

// PrivacyUpdateRequest represents privacy settings update request
type PrivacyUpdateRequest struct {
	ProfilePhoto    PrivacyLevel `json:"profile_photo,omitempty"`
	LastSeen        PrivacyLevel `json:"last_seen,omitempty"`
	About           PrivacyLevel `json:"about,omitempty"`
	Status          PrivacyLevel `json:"status,omitempty"`
	ReadReceipts    *bool        `json:"read_receipts,omitempty"`
	GroupInvites    PrivacyLevel `json:"group_invites,omitempty"`
	CallPermissions PrivacyLevel `json:"call_permissions,omitempty"`
}

// ContactRequest represents add contact request
type ContactRequest struct {
	PhoneNumber string `json:"phone_number" validate:"required"`
	CountryCode string `json:"country_code" validate:"required"`
	Name        string `json:"name,omitempty"`
}

// UserPublicInfo represents public user information
type UserPublicInfo struct {
	ID       primitive.ObjectID `json:"id"`
	Name     string             `json:"name"`
	Bio      string             `json:"bio,omitempty"`
	Avatar   string             `json:"avatar,omitempty"`
	Username string             `json:"username,omitempty"`
	IsOnline bool               `json:"is_online"`
	LastSeen time.Time          `json:"last_seen,omitempty"`
	Status   UserStatus         `json:"status,omitempty"`
}

// UserSearchResult represents search result
type UserSearchResult struct {
	ID          primitive.ObjectID `json:"id"`
	Name        string             `json:"name"`
	PhoneNumber string             `json:"phone_number,omitempty"`
	Avatar      string             `json:"avatar,omitempty"`
	Bio         string             `json:"bio,omitempty"`
	IsContact   bool               `json:"is_contact"`
	IsBlocked   bool               `json:"is_blocked"`
}

// Helper methods

// BeforeCreate sets default values before creating user
func (u *User) BeforeCreate() {
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now
	u.IsActive = true
	u.IsOnline = false
	u.Role = RoleUser

	// Set default privacy settings
	u.Privacy = PrivacySettings{
		ProfilePhoto:    PrivacyContacts,
		LastSeen:        PrivacyContacts,
		About:           PrivacyContacts,
		Status:          PrivacyContacts,
		ReadReceipts:    true,
		GroupInvites:    PrivacyContacts,
		CallPermissions: PrivacyContacts,
	}

	// Set default subscription
	u.Subscription = SubscriptionData{
		Plan:     "free",
		IsActive: true,
		Features: []string{"basic_messaging", "voice_calls", "file_sharing"},
	}

	// Combine country code and phone number
	u.FullPhoneNumber = u.CountryCode + u.PhoneNumber
}

// BeforeUpdate sets updated timestamp
func (u *User) BeforeUpdate() {
	u.UpdatedAt = time.Now()
}

// IsContactOf checks if user is contact of another user
func (u *User) IsContactOf(userID primitive.ObjectID) bool {
	for _, contactID := range u.Contacts {
		if contactID == userID {
			return true
		}
	}
	return false
}

// IsBlockedBy checks if user is blocked by another user
func (u *User) IsBlockedBy(userID primitive.ObjectID) bool {
	for _, blockedID := range u.BlockedUsers {
		if blockedID == userID {
			return true
		}
	}
	return false
}

// HasBlockedUser checks if user has blocked another user
func (u *User) HasBlockedUser(userID primitive.ObjectID) bool {
	for _, blockedByID := range u.BlockedBy {
		if blockedByID == userID {
			return true
		}
	}
	return false
}

// CanSeeProfile checks if another user can see this user's profile
func (u *User) CanSeeProfile(requesterID primitive.ObjectID) bool {
	if u.ID == requesterID {
		return true
	}

	// Check if blocked
	if u.HasBlockedUser(requesterID) || u.IsBlockedBy(requesterID) {
		return false
	}

	switch u.Privacy.ProfilePhoto {
	case PrivacyEveryone:
		return true
	case PrivacyContacts:
		return u.IsContactOf(requesterID)
	case PrivacyNobody:
		return false
	default:
		return false
	}
}

// CanSeeLastSeen checks if another user can see this user's last seen
func (u *User) CanSeeLastSeen(requesterID primitive.ObjectID) bool {
	if u.ID == requesterID {
		return true
	}

	// Check if blocked
	if u.HasBlockedUser(requesterID) || u.IsBlockedBy(requesterID) {
		return false
	}

	switch u.Privacy.LastSeen {
	case PrivacyEveryone:
		return true
	case PrivacyContacts:
		return u.IsContactOf(requesterID)
	case PrivacyNobody:
		return false
	default:
		return false
	}
}

// GetPublicInfo returns public information about the user
func (u *User) GetPublicInfo(requesterID primitive.ObjectID) UserPublicInfo {
	info := UserPublicInfo{
		ID:       u.ID,
		Name:     u.Name,
		Username: u.Username,
		IsOnline: u.IsOnline,
	}

	// Apply privacy settings
	if u.CanSeeProfile(requesterID) {
		info.Avatar = u.Avatar
		info.Bio = u.Bio
	}

	if u.CanSeeLastSeen(requesterID) {
		info.LastSeen = u.LastSeen
	}

	if u.Privacy.Status == PrivacyEveryone || (u.Privacy.Status == PrivacyContacts && u.IsContactOf(requesterID)) {
		info.Status = u.Status
	}

	return info
}

// GenerateOTP generates a new OTP for the user
func (u *User) GenerateOTP() string {
	// Generate 6-digit OTP
	otp := fmt.Sprintf("%06d", rand.Intn(1000000))

	u.OTP = OTPData{
		Code:      otp,
		ExpiresAt: time.Now().Add(10 * time.Minute), // 10 minutes expiry
		Attempts:  0,
		IsUsed:    false,
		CreatedAt: time.Now(),
	}

	return otp
}

// ValidateOTP validates the provided OTP
func (u *User) ValidateOTP(otp string) bool {
	if u.OTP.IsUsed {
		return false
	}

	if time.Now().After(u.OTP.ExpiresAt) {
		return false
	}

	if u.OTP.Attempts >= 3 {
		return false
	}

	u.OTP.Attempts++

	if u.OTP.Code == otp {
		u.OTP.IsUsed = true
		u.IsPhoneVerified = true
		return true
	}

	return false
}
