package migrations

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"

	"bro/internal/models"
)

// SeederService handles database seeding with mock data
type SeederService struct {
	db *mongo.Database
}

// NewSeederService creates a new seeder service
func NewSeederService(db *mongo.Database) *SeederService {
	return &SeederService{
		db: db,
	}
}

// SeedData seeds the database with mock data for development/testing
func (s *SeederService) SeedData() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	log.Println("Starting database seeding...")

	// Check if data already exists
	if s.dataExists(ctx) {
		log.Println("Database already contains data, skipping seeding")
		return nil
	}

	// Seed users
	users, err := s.seedUsers(ctx)
	if err != nil {
		return fmt.Errorf("failed to seed users: %w", err)
	}

	// Seed chats
	chats, err := s.seedChats(ctx, users)
	if err != nil {
		return fmt.Errorf("failed to seed chats: %w", err)
	}

	// Seed groups
	groups, err := s.seedGroups(ctx, users, chats)
	if err != nil {
		return fmt.Errorf("failed to seed groups: %w", err)
	}

	// Seed messages
	if err := s.seedMessages(ctx, users, chats); err != nil {
		return fmt.Errorf("failed to seed messages: %w", err)
	}

	// Seed calls
	if err := s.seedCalls(ctx, users, chats); err != nil {
		return fmt.Errorf("failed to seed calls: %w", err)
	}

	log.Printf("Database seeding completed successfully")
	log.Printf("Created: %d users, %d chats, %d groups", len(users), len(chats), len(groups))
	return nil
}

// dataExists checks if the database already contains user data
func (s *SeederService) dataExists(ctx context.Context) bool {
	count, err := s.db.Collection("users").CountDocuments(ctx, bson.M{})
	return err == nil && count > 0
}

// seedUsers creates mock users
func (s *SeederService) seedUsers(ctx context.Context) ([]models.User, error) {
	collection := s.db.Collection("users")

	// Hash password for all users
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	mockUsers := []models.User{
		{
			ID:               primitive.NewObjectID(),
			PhoneNumber:      "1234567890",
			CountryCode:      "+1",
			FullPhoneNumber:  "+11234567890",
			IsPhoneVerified:  true,
			Name:             "John Doe",
			Bio:              "Software Developer passionate about technology",
			Avatar:           "",
			Email:            "john.doe@example.com",
			Username:         "johndoe",
			PasswordHash:     string(hashedPassword),
			TwoFactorEnabled: false,
			Role:             models.RoleAdmin,
			IsActive:         true,
			IsOnline:         true,
			LastSeen:         time.Now(),
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		},
		{
			ID:               primitive.NewObjectID(),
			PhoneNumber:      "0987654321",
			CountryCode:      "+1",
			FullPhoneNumber:  "+10987654321",
			IsPhoneVerified:  true,
			Name:             "Jane Smith",
			Bio:              "Digital Marketing Specialist and content creator",
			Avatar:           "",
			Email:            "jane.smith@example.com",
			Username:         "janesmith",
			PasswordHash:     string(hashedPassword),
			TwoFactorEnabled: false,
			Role:             models.RoleUser,
			IsActive:         true,
			IsOnline:         false,
			LastSeen:         time.Now().Add(-2 * time.Hour),
			CreatedAt:        time.Now().Add(-48 * time.Hour),
			UpdatedAt:        time.Now().Add(-2 * time.Hour),
		},
		{
			ID:               primitive.NewObjectID(),
			PhoneNumber:      "5555551234",
			CountryCode:      "+1",
			FullPhoneNumber:  "+15555551234",
			IsPhoneVerified:  true,
			Name:             "Bob Johnson",
			Bio:              "Photographer and travel enthusiast",
			Avatar:           "",
			Email:            "bob.johnson@example.com",
			Username:         "bobjohnson",
			PasswordHash:     string(hashedPassword),
			TwoFactorEnabled: true,
			Role:             models.RoleModerator,
			IsActive:         true,
			IsOnline:         true,
			LastSeen:         time.Now().Add(-30 * time.Minute),
			CreatedAt:        time.Now().Add(-24 * time.Hour),
			UpdatedAt:        time.Now().Add(-30 * time.Minute),
		},
		{
			ID:               primitive.NewObjectID(),
			PhoneNumber:      "7777777777",
			CountryCode:      "+1",
			FullPhoneNumber:  "+17777777777",
			IsPhoneVerified:  true,
			Name:             "Alice Wilson",
			Bio:              "UI/UX Designer creating beautiful experiences",
			Avatar:           "",
			Email:            "alice.wilson@example.com",
			Username:         "alicewilson",
			PasswordHash:     string(hashedPassword),
			TwoFactorEnabled: false,
			Role:             models.RoleUser,
			IsActive:         true,
			IsOnline:         false,
			LastSeen:         time.Now().Add(-4 * time.Hour),
			CreatedAt:        time.Now().Add(-72 * time.Hour),
			UpdatedAt:        time.Now().Add(-4 * time.Hour),
		},
		{
			ID:               primitive.NewObjectID(),
			PhoneNumber:      "9999999999",
			CountryCode:      "+1",
			FullPhoneNumber:  "+19999999999",
			IsPhoneVerified:  true,
			Name:             "Charlie Brown",
			Bio:              "Project Manager and team lead",
			Avatar:           "",
			Email:            "charlie.brown@example.com",
			Username:         "charliebrown",
			PasswordHash:     string(hashedPassword),
			TwoFactorEnabled: false,
			Role:             models.RoleUser,
			IsActive:         true,
			IsOnline:         true,
			LastSeen:         time.Now().Add(-15 * time.Minute),
			CreatedAt:        time.Now().Add(-96 * time.Hour),
			UpdatedAt:        time.Now().Add(-15 * time.Minute),
		},
	}

	// Set default privacy settings for all users
	for i := range mockUsers {
		mockUsers[i].Privacy = models.PrivacySettings{
			ProfilePhoto:    models.PrivacyContacts,
			LastSeen:        models.PrivacyContacts,
			About:           models.PrivacyContacts,
			Status:          models.PrivacyContacts,
			ReadReceipts:    true,
			GroupInvites:    models.PrivacyContacts,
			CallPermissions: models.PrivacyContacts,
		}

		mockUsers[i].Subscription = models.SubscriptionData{
			Plan:     "free",
			IsActive: true,
			Features: []string{"basic_messaging", "voice_calls", "file_sharing"},
		}

		// Add some contacts
		if i > 0 {
			mockUsers[i].Contacts = []primitive.ObjectID{mockUsers[0].ID}
			if i > 1 {
				mockUsers[i].Contacts = append(mockUsers[i].Contacts, mockUsers[1].ID)
			}
		}
	}

	// Insert users
	var documents []interface{}
	for _, user := range mockUsers {
		documents = append(documents, user)
	}

	_, err = collection.InsertMany(ctx, documents)
	if err != nil {
		return nil, fmt.Errorf("failed to insert users: %w", err)
	}

	log.Printf("Created %d mock users", len(mockUsers))
	return mockUsers, nil
}

// seedChats creates mock chats
func (s *SeederService) seedChats(ctx context.Context, users []models.User) ([]models.Chat, error) {
	collection := s.db.Collection("chats")

	chats := []models.Chat{
		// Private chat between John and Jane
		{
			ID:           primitive.NewObjectID(),
			Type:         models.ChatTypePrivate,
			Participants: []primitive.ObjectID{users[0].ID, users[1].ID},
			CreatedBy:    users[0].ID,
			Name:         "",
			IsActive:     true,
			IsEncrypted:  true,
			MessageCount: 0,
			CreatedAt:    time.Now().Add(-24 * time.Hour),
			UpdatedAt:    time.Now().Add(-1 * time.Hour),
			LastActivity: time.Now().Add(-1 * time.Hour),
		},
		// Private chat between John and Bob
		{
			ID:           primitive.NewObjectID(),
			Type:         models.ChatTypePrivate,
			Participants: []primitive.ObjectID{users[0].ID, users[2].ID},
			CreatedBy:    users[0].ID,
			Name:         "",
			IsActive:     true,
			IsEncrypted:  true,
			MessageCount: 0,
			CreatedAt:    time.Now().Add(-12 * time.Hour),
			UpdatedAt:    time.Now().Add(-2 * time.Hour),
			LastActivity: time.Now().Add(-2 * time.Hour),
		},
		// Group chat
		{
			ID:           primitive.NewObjectID(),
			Type:         models.ChatTypeGroup,
			Participants: []primitive.ObjectID{users[0].ID, users[1].ID, users[2].ID, users[3].ID},
			CreatedBy:    users[0].ID,
			Name:         "Development Team",
			Description:  "Main communication channel for the development team",
			IsActive:     true,
			IsEncrypted:  true,
			MessageCount: 0,
			CreatedAt:    time.Now().Add(-48 * time.Hour),
			UpdatedAt:    time.Now().Add(-30 * time.Minute),
			LastActivity: time.Now().Add(-30 * time.Minute),
		},
	}

	// Set default settings for chats
	for i := range chats {
		chats[i].Settings = models.ChatSettings{
			AllowMessages:      true,
			AllowMediaSharing:  true,
			AllowVoiceMessages: true,
			AllowDocuments:     true,
			AllowVoiceCalls:    true,
			AllowVideoCalls:    true,
			ReadReceipts:       true,
			TypingIndicators:   true,
			OnlineStatus:       true,
			AllowBackup:        true,
			AllowExport:        true,
		}

		// Initialize unread counts for all participants
		for _, participantID := range chats[i].Participants {
			chats[i].UnreadCounts = append(chats[i].UnreadCounts, models.UnreadCount{
				UserID:       participantID,
				Count:        0,
				LastReadAt:   chats[i].CreatedAt,
				MentionCount: 0,
			})
		}
	}

	// Insert chats
	var documents []interface{}
	for _, chat := range chats {
		documents = append(documents, chat)
	}

	_, err := collection.InsertMany(ctx, documents)
	if err != nil {
		return nil, fmt.Errorf("failed to insert chats: %w", err)
	}

	log.Printf("Created %d mock chats", len(chats))
	return chats, nil
}

// seedGroups creates mock groups
func (s *SeederService) seedGroups(ctx context.Context, users []models.User, chats []models.Chat) ([]models.Group, error) {
	collection := s.db.Collection("groups")

	// Find the group chat to create a group for
	var groupChat *models.Chat
	for _, chat := range chats {
		if chat.Type == models.ChatTypeGroup {
			groupChat = &chat
			break
		}
	}

	if groupChat == nil {
		return nil, fmt.Errorf("no group chat found")
	}

	group := models.Group{
		ID:           primitive.NewObjectID(),
		ChatID:       groupChat.ID,
		Name:         "Development Team",
		Description:  "A group for development team collaboration and discussions",
		CreatedBy:    users[0].ID,
		Owner:        users[0].ID,
		IsPublic:     false,
		IsActive:     true,
		MaxMembers:   100,
		MemberCount:  4,
		InviteCode:   generateInviteCode(),
		InviteLink:   "https://chat.app/invite/" + generateInviteCode(),
		CreatedAt:    groupChat.CreatedAt,
		UpdatedAt:    time.Now(),
		LastActivity: time.Now().Add(-30 * time.Minute),
	}

	// Add members
	group.Members = []models.GroupMember{
		{
			UserID:       users[0].ID,
			Role:         models.GroupRoleOwner,
			JoinedAt:     group.CreatedAt,
			IsActive:     true,
			LastActiveAt: time.Now().Add(-15 * time.Minute),
			MessageCount: 5,
			Permissions: models.MemberPermissions{
				CanSendMessages: true,
				CanSendMedia:    true,
				CanAddMembers:   true,
				CanPinMessages:  true,
			},
		},
		{
			UserID:       users[1].ID,
			Role:         models.GroupRoleMember,
			JoinedAt:     group.CreatedAt.Add(1 * time.Hour),
			InvitedBy:    &users[0].ID,
			IsActive:     true,
			LastActiveAt: time.Now().Add(-2 * time.Hour),
			MessageCount: 3,
			Permissions: models.MemberPermissions{
				CanSendMessages: true,
				CanSendMedia:    true,
				CanAddMembers:   false,
				CanPinMessages:  false,
			},
		},
		{
			UserID:       users[2].ID,
			Role:         models.GroupRoleAdmin,
			JoinedAt:     group.CreatedAt.Add(2 * time.Hour),
			InvitedBy:    &users[0].ID,
			IsActive:     true,
			LastActiveAt: time.Now().Add(-1 * time.Hour),
			MessageCount: 7,
			Permissions: models.MemberPermissions{
				CanSendMessages: true,
				CanSendMedia:    true,
				CanAddMembers:   true,
				CanPinMessages:  true,
			},
		},
		{
			UserID:       users[3].ID,
			Role:         models.GroupRoleMember,
			JoinedAt:     group.CreatedAt.Add(4 * time.Hour),
			InvitedBy:    &users[0].ID,
			IsActive:     true,
			LastActiveAt: time.Now().Add(-3 * time.Hour),
			MessageCount: 2,
			Permissions: models.MemberPermissions{
				CanSendMessages: true,
				CanSendMedia:    true,
				CanAddMembers:   false,
				CanPinMessages:  false,
			},
		},
	}

	// Set admins
	group.Admins = []primitive.ObjectID{users[0].ID, users[2].ID}

	// Set default group settings
	group.Settings = models.GroupSettings{
		AllowMessages:              true,
		AllowMediaSharing:          true,
		AllowLinks:                 true,
		AllowForwarding:            true,
		OnlyAdminsCanMessage:       false,
		OnlyAdminsCanAddMembers:    false,
		OnlyAdminsCanRemoveMembers: true,
		OnlyAdminsCanEditInfo:      true,
		ShowMemberList:             true,
		ShowMemberJoinDate:         true,
		ShowLastSeen:               true,
		AllowVoiceCalls:            true,
		AllowVideoCalls:            true,
		OnlyAdminsCanCall:          false,
		AutoModeration: models.AutoModerationSettings{
			Enabled:                false,
			FilterProfanity:        false,
			FilterSpam:             false,
			FilterLinks:            false,
			MaxMessageLength:       4096,
			MaxConsecutiveMessages: 5,
			AntiFloodEnabled:       true,
		},
		SlowModeEnabled: false,
		MentionSettings: models.GroupMentionSettings{
			AllowMentions:                true,
			AllowEveryoneMention:         true,
			OnlyAdminsCanMentionEveryone: false,
		},
		AnnouncementOnly: false,
		WelcomeMessage: models.WelcomeMessage{
			Enabled: true,
			Message: "Welcome to the Development Team group! 🎉",
		},
	}

	// Add a sample rule
	group.Rules = []models.GroupRule{
		{
			ID:          primitive.NewObjectID(),
			Title:       "Be Respectful",
			Description: "Treat all members with respect and maintain professional communication.",
			Order:       1,
			IsActive:    true,
			CreatedBy:   users[0].ID,
			CreatedAt:   group.CreatedAt,
		},
	}

	// Initialize stats
	group.Stats = models.GroupStats{
		TotalMessages:     15,
		TotalMembers:      4,
		ActiveMembers:     3,
		MessagesToday:     5,
		MessagesThisWeek:  12,
		MessagesThisMonth: 15,
		PeakOnlineMembers: 3,
		LastStatsUpdate:   time.Now(),
	}

	_, err := collection.InsertOne(ctx, group)
	if err != nil {
		return nil, fmt.Errorf("failed to insert group: %w", err)
	}

	log.Printf("Created 1 mock group")
	return []models.Group{group}, nil
}

// seedMessages creates mock messages
func (s *SeederService) seedMessages(ctx context.Context, users []models.User, chats []models.Chat) error {
	collection := s.db.Collection("messages")

	var messages []models.Message

	// Messages for private chat (John and Jane)
	privateChat := chats[0]
	messages = append(messages, []models.Message{
		{
			ID:        primitive.NewObjectID(),
			ChatID:    privateChat.ID,
			SenderID:  users[0].ID,
			Type:      models.MessageTypeText,
			Content:   "Hey Jane, how are you doing?",
			Status:    models.MessageStatusRead,
			CreatedAt: time.Now().Add(-23 * time.Hour),
			UpdatedAt: time.Now().Add(-23 * time.Hour),
		},
		{
			ID:        primitive.NewObjectID(),
			ChatID:    privateChat.ID,
			SenderID:  users[1].ID,
			Type:      models.MessageTypeText,
			Content:   "Hi John! I'm doing great, thanks for asking. How about you?",
			Status:    models.MessageStatusRead,
			CreatedAt: time.Now().Add(-22 * time.Hour),
			UpdatedAt: time.Now().Add(-22 * time.Hour),
		},
		{
			ID:        primitive.NewObjectID(),
			ChatID:    privateChat.ID,
			SenderID:  users[0].ID,
			Type:      models.MessageTypeText,
			Content:   "I'm good too! Are you ready for the project meeting tomorrow?",
			Status:    models.MessageStatusRead,
			CreatedAt: time.Now().Add(-21 * time.Hour),
			UpdatedAt: time.Now().Add(-21 * time.Hour),
		},
	}...)

	// Messages for group chat
	groupChat := chats[2]
	messages = append(messages, []models.Message{
		{
			ID:        primitive.NewObjectID(),
			ChatID:    groupChat.ID,
			SenderID:  users[0].ID,
			Type:      models.MessageTypeText,
			Content:   "Welcome everyone to our development team chat! 🎉",
			Status:    models.MessageStatusRead,
			CreatedAt: time.Now().Add(-47 * time.Hour),
			UpdatedAt: time.Now().Add(-47 * time.Hour),
		},
		{
			ID:        primitive.NewObjectID(),
			ChatID:    groupChat.ID,
			SenderID:  users[1].ID,
			Type:      models.MessageTypeText,
			Content:   "Thanks for setting this up, John! Looking forward to collaborating here.",
			Status:    models.MessageStatusRead,
			CreatedAt: time.Now().Add(-46 * time.Hour),
			UpdatedAt: time.Now().Add(-46 * time.Hour),
		},
		{
			ID:        primitive.NewObjectID(),
			ChatID:    groupChat.ID,
			SenderID:  users[2].ID,
			Type:      models.MessageTypeText,
			Content:   "Great idea! This will make our communication much easier.",
			Status:    models.MessageStatusRead,
			CreatedAt: time.Now().Add(-45 * time.Hour),
			UpdatedAt: time.Now().Add(-45 * time.Hour),
		},
		{
			ID:        primitive.NewObjectID(),
			ChatID:    groupChat.ID,
			SenderID:  users[3].ID,
			Type:      models.MessageTypeText,
			Content:   "Hello team! Excited to be part of this project. 🚀",
			Status:    models.MessageStatusRead,
			CreatedAt: time.Now().Add(-44 * time.Hour),
			UpdatedAt: time.Now().Add(-44 * time.Hour),
		},
		{
			ID:        primitive.NewObjectID(),
			ChatID:    groupChat.ID,
			SenderID:  users[0].ID,
			Type:      models.MessageTypeText,
			Content:   "Quick update: The new features are almost ready for testing. I'll share the details in our next standup.",
			Status:    models.MessageStatusDelivered,
			CreatedAt: time.Now().Add(-1 * time.Hour),
			UpdatedAt: time.Now().Add(-1 * time.Hour),
		},
	}...)

	// Set default values for all messages
	for i := range messages {
		messages[i].IsEncrypted = true
		messages[i].ReadBy = []models.ReadReceipt{}
		messages[i].DeliveredTo = []models.DeliveryReceipt{}
		messages[i].Reactions = []models.Reaction{}

		// Add some reactions to group messages
		if messages[i].ChatID == groupChat.ID && i > 0 {
			messages[i].Reactions = []models.Reaction{
				{
					UserID:  users[(i+1)%len(users)].ID,
					Emoji:   "👍",
					AddedAt: messages[i].CreatedAt.Add(5 * time.Minute),
				},
			}
		}
	}

	// Insert messages
	var documents []interface{}
	for _, message := range messages {
		documents = append(documents, message)
	}

	_, err := collection.InsertMany(ctx, documents)
	if err != nil {
		return fmt.Errorf("failed to insert messages: %w", err)
	}

	log.Printf("Created %d mock messages", len(messages))
	return nil
}

// seedCalls creates mock call records
func (s *SeederService) seedCalls(ctx context.Context, users []models.User, chats []models.Chat) error {
	collection := s.db.Collection("calls")

	calls := []models.Call{
		{
			ID:          primitive.NewObjectID(),
			Type:        models.CallTypeVideo,
			Status:      models.CallStatusEnded,
			InitiatorID: users[0].ID,
			ChatID:      chats[0].ID,
			SessionID:   primitive.NewObjectID().Hex(),
			InitiatedAt: time.Now().Add(-3 * time.Hour),
			StartedAt:   func() *time.Time { t := time.Now().Add(-3*time.Hour + 15*time.Second); return &t }(),
			EndedAt:     func() *time.Time { t := time.Now().Add(-3*time.Hour + 15*time.Minute); return &t }(),
			Duration:    900, // 15 minutes
			EndReason:   models.EndReasonNormal,
			EndedBy:     &users[0].ID,
			CreatedAt:   time.Now().Add(-3 * time.Hour),
			UpdatedAt:   time.Now().Add(-3*time.Hour + 15*time.Minute),
		},
		{
			ID:          primitive.NewObjectID(),
			Type:        models.CallTypeVoice,
			Status:      models.CallStatusEnded,
			InitiatorID: users[1].ID,
			ChatID:      chats[1].ID,
			SessionID:   primitive.NewObjectID().Hex(),
			InitiatedAt: time.Now().Add(-6 * time.Hour),
			StartedAt:   func() *time.Time { t := time.Now().Add(-6*time.Hour + 5*time.Second); return &t }(),
			EndedAt:     func() *time.Time { t := time.Now().Add(-6*time.Hour + 25*time.Minute); return &t }(),
			Duration:    1500, // 25 minutes
			EndReason:   models.EndReasonNormal,
			EndedBy:     &users[2].ID,
			CreatedAt:   time.Now().Add(-6 * time.Hour),
			UpdatedAt:   time.Now().Add(-6*time.Hour + 25*time.Minute),
		},
	}

	// Set participants for calls
	calls[0].Participants = []models.CallParticipant{
		{
			UserID:   users[0].ID,
			JoinedAt: calls[0].StartedAt,
			LeftAt:   calls[0].EndedAt,
			Duration: 900,
			Status:   models.ParticipantStatusDisconnected,
			Role:     models.ParticipantRoleInitiator,
		},
		{
			UserID:   users[1].ID,
			JoinedAt: func() *time.Time { t := calls[0].StartedAt.Add(5 * time.Second); return &t }(),
			LeftAt:   calls[0].EndedAt,
			Duration: 895,
			Status:   models.ParticipantStatusDisconnected,
			Role:     models.ParticipantRoleParticipant,
		},
	}

	calls[1].Participants = []models.CallParticipant{
		{
			UserID:   users[1].ID,
			JoinedAt: calls[1].StartedAt,
			LeftAt:   calls[1].EndedAt,
			Duration: 1500,
			Status:   models.ParticipantStatusDisconnected,
			Role:     models.ParticipantRoleInitiator,
		},
		{
			UserID:   users[2].ID,
			JoinedAt: func() *time.Time { t := calls[1].StartedAt.Add(3 * time.Second); return &t }(),
			LeftAt:   calls[1].EndedAt,
			Duration: 1497,
			Status:   models.ParticipantStatusDisconnected,
			Role:     models.ParticipantRoleParticipant,
		},
	}

	// Set default call properties
	for i := range calls {
		calls[i].Features = models.CallFeatures{
			VideoCall:         calls[i].Type == models.CallTypeVideo,
			ScreenShare:       false,
			Recording:         false,
			ChatDuringCall:    true,
			FileSharing:       true,
			BackgroundEffects: true,
			NoiseReduction:    true,
			EchoCancellation:  true,
		}

		calls[i].Quality = models.CallQuality{
			OverallRating:     4.5,
			AudioQuality:      4.8,
			VideoQuality:      4.2,
			ConnectionQuality: 4.6,
		}

		calls[i].Analytics = models.CallAnalytics{
			ConnectionAttempts:      1,
			SuccessfulConnections:   1,
			FailedConnections:       0,
			AverageQuality:          4.5,
			PeakParticipants:        len(calls[i].Participants),
			TotalParticipantMinutes: int(calls[i].Duration / 60 * int64(len(calls[i].Participants))),
			FeaturesUsed:            []string{"voice", "video"},
		}
	}

	// Insert calls
	var documents []interface{}
	for _, call := range calls {
		documents = append(documents, call)
	}

	_, err := collection.InsertMany(ctx, documents)
	if err != nil {
		return fmt.Errorf("failed to insert calls: %w", err)
	}

	log.Printf("Created %d mock calls", len(calls))
	return nil
}

// generateInviteCode generates a random invite code
func generateInviteCode() string {
	return primitive.NewObjectID().Hex()[:8]
}

// ClearAllData removes all seeded data (useful for testing)
func (s *SeederService) ClearAllData() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collections := []string{"users", "chats", "messages", "groups", "calls", "files"}

	for _, collName := range collections {
		collection := s.db.Collection(collName)
		_, err := collection.DeleteMany(ctx, bson.M{})
		if err != nil {
			return fmt.Errorf("failed to clear collection %s: %w", collName, err)
		}
		log.Printf("Cleared collection: %s", collName)
	}

	log.Println("All seeded data cleared")
	return nil
}
