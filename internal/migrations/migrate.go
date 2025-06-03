package migrations

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MigrationService handles database migrations and schema setup
type MigrationService struct {
	db *mongo.Database
}

// NewMigrationService creates a new migration service
func NewMigrationService(db *mongo.Database) *MigrationService {
	return &MigrationService{
		db: db,
	}
}

// RunMigrations executes all pending migrations
func (m *MigrationService) RunMigrations() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Println("Starting database migrations...")

	// Create collections if they don't exist
	if err := m.createCollections(ctx); err != nil {
		return fmt.Errorf("failed to create collections: %w", err)
	}

	// Create indexes
	if err := m.createIndexes(ctx); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	// Create admin config if it doesn't exist
	if err := m.createDefaultAdminConfig(ctx); err != nil {
		return fmt.Errorf("failed to create admin config: %w", err)
	}

	log.Println("Database migrations completed successfully")
	return nil
}

// createCollections creates all required collections
func (m *MigrationService) createCollections(ctx context.Context) error {
	collections := []string{
		"users",
		"chats",
		"messages",
		"groups",
		"calls",
		"files",
		"admin_configs",
		"push_tokens",
		"sessions",
	}

	for _, collName := range collections {
		err := m.db.CreateCollection(ctx, collName)
		if err != nil {
			// Ignore "collection already exists" error
			if !mongo.IsDuplicateKeyError(err) && err.Error() != "collection already exists" {
				log.Printf("Warning: Failed to create collection %s: %v", collName, err)
			}
		}
		log.Printf("Collection '%s' ready", collName)
	}

	return nil
}

// createIndexes creates all required database indexes
func (m *MigrationService) createIndexes(ctx context.Context) error {
	log.Println("Creating database indexes...")

	// Users collection indexes
	if err := m.createUsersIndexes(ctx); err != nil {
		return err
	}

	// Chats collection indexes
	if err := m.createChatsIndexes(ctx); err != nil {
		return err
	}

	// Messages collection indexes
	if err := m.createMessagesIndexes(ctx); err != nil {
		return err
	}

	// Groups collection indexes
	if err := m.createGroupsIndexes(ctx); err != nil {
		return err
	}

	// Calls collection indexes
	if err := m.createCallsIndexes(ctx); err != nil {
		return err
	}

	// Files collection indexes
	if err := m.createFilesIndexes(ctx); err != nil {
		return err
	}

	log.Println("All indexes created successfully")
	return nil
}

// createUsersIndexes creates indexes for users collection
func (m *MigrationService) createUsersIndexes(ctx context.Context) error {
	collection := m.db.Collection("users")

	// First, clean up any conflicting indexes
	if err := m.cleanupConflictingIndexes(ctx, collection); err != nil {
		log.Printf("Warning: Failed to cleanup conflicting indexes: %v", err)
	}

	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "phone_number", Value: 1}, {Key: "country_code", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("phone_unique"),
		},
		{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetUnique(true).SetSparse(true).SetName("email_unique"),
		},
		{
			Keys:    bson.D{{Key: "username", Value: 1}},
			Options: options.Index().SetUnique(true).SetSparse(true).SetName("username_unique"),
		},
		{
			Keys:    bson.D{{Key: "full_phone_number", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("full_phone_unique"),
		},
		{
			Keys:    bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index().SetName("created_at_desc"),
		},
		{
			Keys:    bson.D{{Key: "last_seen", Value: -1}},
			Options: options.Index().SetName("last_seen_desc"),
		},
		{
			Keys:    bson.D{{Key: "is_active", Value: 1}},
			Options: options.Index().SetName("is_active"),
		},
		{
			Keys:    bson.D{{Key: "role", Value: 1}},
			Options: options.Index().SetName("role"),
		},
	}

	// Create indexes safely
	err := m.createIndexesSafely(ctx, collection, indexes)
	if err != nil {
		return fmt.Errorf("failed to create users indexes: %w", err)
	}

	log.Println("Users indexes created")
	return nil
}

// createChatsIndexes creates indexes for chats collection
func (m *MigrationService) createChatsIndexes(ctx context.Context) error {
	collection := m.db.Collection("chats")

	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "participants", Value: 1}},
			Options: options.Index().SetName("participants"),
		},
		{
			Keys:    bson.D{{Key: "type", Value: 1}},
			Options: options.Index().SetName("type"),
		},
		{
			Keys:    bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index().SetName("created_at_desc"),
		},
		{
			Keys:    bson.D{{Key: "last_activity", Value: -1}},
			Options: options.Index().SetName("last_activity_desc"),
		},
		{
			Keys:    bson.D{{Key: "is_active", Value: 1}},
			Options: options.Index().SetName("is_active"),
		},
		{
			Keys:    bson.D{{Key: "created_by", Value: 1}},
			Options: options.Index().SetName("created_by"),
		},
	}

	err := m.createIndexesSafely(ctx, collection, indexes)
	if err != nil {
		return fmt.Errorf("failed to create chats indexes: %w", err)
	}

	log.Println("Chats indexes created")
	return nil
}

// createMessagesIndexes creates indexes for messages collection
func (m *MigrationService) createMessagesIndexes(ctx context.Context) error {
	collection := m.db.Collection("messages")

	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "chat_id", Value: 1}, {Key: "created_at", Value: -1}},
			Options: options.Index().SetName("chat_created_desc"),
		},
		{
			Keys:    bson.D{{Key: "sender_id", Value: 1}},
			Options: options.Index().SetName("sender_id"),
		},
		{
			Keys:    bson.D{{Key: "type", Value: 1}},
			Options: options.Index().SetName("type"),
		},
		{
			Keys:    bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index().SetName("created_at_desc"),
		},
		{
			Keys:    bson.D{{Key: "is_deleted", Value: 1}},
			Options: options.Index().SetName("is_deleted"),
		},
		{
			Keys:    bson.D{{Key: "reply_to_id", Value: 1}},
			Options: options.Index().SetSparse(true).SetName("reply_to_id"),
		},
		{
			Keys:    bson.D{{Key: "mentions", Value: 1}},
			Options: options.Index().SetName("mentions"),
		},
		{
			Keys:    bson.D{{Key: "scheduled_at", Value: 1}},
			Options: options.Index().SetSparse(true).SetName("scheduled_at"),
		},
		{
			Keys:    bson.D{{Key: "content", Value: "text"}},
			Options: options.Index().SetName("content_text"),
		},
	}

	err := m.createIndexesSafely(ctx, collection, indexes)
	if err != nil {
		return fmt.Errorf("failed to create messages indexes: %w", err)
	}

	log.Println("Messages indexes created")
	return nil
}

// createGroupsIndexes creates indexes for groups collection
func (m *MigrationService) createGroupsIndexes(ctx context.Context) error {
	collection := m.db.Collection("groups")

	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "chat_id", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("chat_id_unique"),
		},
		{
			Keys:    bson.D{{Key: "owner", Value: 1}},
			Options: options.Index().SetName("owner"),
		},
		{
			Keys:    bson.D{{Key: "created_by", Value: 1}},
			Options: options.Index().SetName("created_by"),
		},
		{
			Keys:    bson.D{{Key: "members.user_id", Value: 1}},
			Options: options.Index().SetName("members_user_id"),
		},
		{
			Keys:    bson.D{{Key: "admins", Value: 1}},
			Options: options.Index().SetName("admins"),
		},
		{
			Keys:    bson.D{{Key: "is_active", Value: 1}},
			Options: options.Index().SetName("is_active"),
		},
		{
			Keys:    bson.D{{Key: "is_public", Value: 1}},
			Options: options.Index().SetName("is_public"),
		},
		{
			Keys:    bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index().SetName("created_at_desc"),
		},
		{
			Keys:    bson.D{{Key: "last_activity", Value: -1}},
			Options: options.Index().SetName("last_activity_desc"),
		},
		{
			Keys:    bson.D{{Key: "invite_code", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("invite_code_unique"),
		},
		{
			Keys:    bson.D{{Key: "name", Value: "text"}},
			Options: options.Index().SetName("name_text"),
		},
	}

	err := m.createIndexesSafely(ctx, collection, indexes)
	if err != nil {
		return fmt.Errorf("failed to create groups indexes: %w", err)
	}

	log.Println("Groups indexes created")
	return nil
}

// createCallsIndexes creates indexes for calls collection
func (m *MigrationService) createCallsIndexes(ctx context.Context) error {
	collection := m.db.Collection("calls")

	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "chat_id", Value: 1}},
			Options: options.Index().SetName("chat_id"),
		},
		{
			Keys:    bson.D{{Key: "initiator_id", Value: 1}},
			Options: options.Index().SetName("initiator_id"),
		},
		{
			Keys:    bson.D{{Key: "participants.user_id", Value: 1}},
			Options: options.Index().SetName("participants_user_id"),
		},
		{
			Keys:    bson.D{{Key: "status", Value: 1}},
			Options: options.Index().SetName("status"),
		},
		{
			Keys:    bson.D{{Key: "type", Value: 1}},
			Options: options.Index().SetName("type"),
		},
		{
			Keys:    bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index().SetName("created_at_desc"),
		},
		{
			Keys:    bson.D{{Key: "initiated_at", Value: -1}},
			Options: options.Index().SetName("initiated_at_desc"),
		},
		{
			Keys:    bson.D{{Key: "ended_at", Value: -1}},
			Options: options.Index().SetSparse(true).SetName("ended_at_desc"),
		},
		{
			Keys:    bson.D{{Key: "session_id", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("session_id_unique"),
		},
	}

	err := m.createIndexesSafely(ctx, collection, indexes)
	if err != nil {
		return fmt.Errorf("failed to create calls indexes: %w", err)
	}

	log.Println("Calls indexes created")
	return nil
}

// createFilesIndexes creates indexes for files collection
func (m *MigrationService) createFilesIndexes(ctx context.Context) error {
	collection := m.db.Collection("files")

	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}},
			Options: options.Index().SetName("user_id"),
		},
		{
			Keys:    bson.D{{Key: "chat_id", Value: 1}},
			Options: options.Index().SetSparse(true).SetName("chat_id"),
		},
		{
			Keys:    bson.D{{Key: "message_id", Value: 1}},
			Options: options.Index().SetSparse(true).SetName("message_id"),
		},
		{
			Keys:    bson.D{{Key: "content_type", Value: 1}},
			Options: options.Index().SetName("content_type"),
		},
		{
			Keys:    bson.D{{Key: "purpose", Value: 1}},
			Options: options.Index().SetName("purpose"),
		},
		{
			Keys:    bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index().SetName("created_at_desc"),
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetSparse(true).SetName("expires_at"),
		},
		{
			Keys:    bson.D{{Key: "is_public", Value: 1}},
			Options: options.Index().SetName("is_public"),
		},
		{
			Keys:    bson.D{{Key: "file_name", Value: "text"}},
			Options: options.Index().SetName("file_name_text"),
		},
	}

	err := m.createIndexesSafely(ctx, collection, indexes)
	if err != nil {
		return fmt.Errorf("failed to create files indexes: %w", err)
	}

	log.Println("Files indexes created")
	return nil
}

// createDefaultAdminConfig creates default admin configuration
func (m *MigrationService) createDefaultAdminConfig(ctx context.Context) error {
	collection := m.db.Collection("admin_configs")

	// Check if admin config already exists
	count, err := collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("failed to check admin config: %w", err)
	}

	if count > 0 {
		log.Println("Admin config already exists, skipping creation")
		return nil
	}

	// Create default admin config
	adminConfig := bson.M{
		"app_settings": bson.M{
			"app_name":            "BRO Chat",
			"app_version":         "1.0.0",
			"app_description":     "A modern real-time chat application",
			"default_language":    "en",
			"supported_languages": []string{"en", "es", "fr", "de"},
			"timezone":            "UTC",
			"maintenance_mode":    false,
			"maintenance_message": "",
		},
		"server_config": bson.M{
			"max_connections":    10000,
			"request_timeout":    30000000000, // 30 seconds in nanoseconds
			"keep_alive_timeout": 60000000000, // 60 seconds in nanoseconds
			"enable_https":       true,
			"cors_origins":       []string{"*"},
			"cors_methods":       []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		},
		"feature_flags": bson.M{
			"enable_registration":        true,
			"enable_guest_mode":          false,
			"enable_group_chats":         true,
			"enable_voice_calls":         true,
			"enable_video_calls":         true,
			"enable_screen_sharing":      true,
			"enable_file_sharing":        true,
			"enable_voice_messages":      true,
			"enable_message_encryption":  true,
			"enable_delete_for_everyone": true,
			"enable_message_editing":     true,
			"enable_forwarding":          true,
		},
		"environment":     "development",
		"config_version":  "1.0.0",
		"created_at":      time.Now(),
		"updated_at":      time.Now(),
		"last_updated_at": time.Now(),
	}

	_, err = collection.InsertOne(ctx, adminConfig)
	if err != nil {
		return fmt.Errorf("failed to create admin config: %w", err)
	}

	log.Println("Default admin config created")
	return nil
}

// DropAllIndexes drops all indexes (useful for development)
func (m *MigrationService) DropAllIndexes(ctx context.Context) error {
	collections := []string{"users", "chats", "messages", "groups", "calls", "files"}

	for _, collName := range collections {
		collection := m.db.Collection(collName)
		_, err := collection.Indexes().DropAll(ctx)
		if err != nil {
			return fmt.Errorf("failed to drop indexes for %s: %w", collName, err)
		}
		log.Printf("Dropped all indexes for collection: %s", collName)
	}

	return nil
}

// GetMigrationStatus returns the current migration status
func (m *MigrationService) GetMigrationStatus(ctx context.Context) (map[string]interface{}, error) {
	status := map[string]interface{}{
		"timestamp": time.Now(),
		"status":    "completed",
	}

	// Check collections
	collections := []string{"users", "chats", "messages", "groups", "calls", "files", "admin_configs"}
	collectionStatus := make(map[string]bool)

	for _, collName := range collections {
		names, err := m.db.ListCollectionNames(ctx, bson.M{"name": collName})
		collectionStatus[collName] = err == nil && len(names) > 0
	}

	status["collections"] = collectionStatus

	// Check indexes count
	indexCounts := make(map[string]int)
	for _, collName := range collections {
		if collectionStatus[collName] {
			collection := m.db.Collection(collName)
			cursor, err := collection.Indexes().List(ctx)
			if err == nil {
				var indexes []bson.M
				cursor.All(ctx, &indexes)
				indexCounts[collName] = len(indexes)
			}
		}
	}

	status["index_counts"] = indexCounts

	return status, nil
}

// Helper functions for safe index creation

// cleanupConflictingIndexes removes indexes that might conflict with our desired indexes
func (m *MigrationService) cleanupConflictingIndexes(ctx context.Context, collection *mongo.Collection) error {
	// Get all existing indexes
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	var existingIndexes []bson.M
	if err := cursor.All(ctx, &existingIndexes); err != nil {
		return err
	}

	// Define the key patterns we want to create with their desired names
	desiredIndexes := map[string]string{
		"phone_number_1_country_code_1": "phone_unique",
		"email_1":                       "email_unique",
		"username_1":                    "username_unique",
		"full_phone_number_1":           "full_phone_unique",
	}

	// Check for conflicts and drop indexes with wrong names
	for _, index := range existingIndexes {
		indexName, hasName := index["name"].(string)
		if !hasName || indexName == "_id_" {
			continue // Skip _id index and indexes without names
		}

		// Get the key pattern
		keyDoc, hasKey := index["key"].(bson.M)
		if !hasKey {
			continue
		}

		keyPattern := m.createKeyPatternString(keyDoc)

		// Check if this key pattern conflicts with our desired indexes
		if desiredName, exists := desiredIndexes[keyPattern]; exists {
			if indexName != desiredName {
				log.Printf("Dropping conflicting index '%s' with key pattern '%s'", indexName, keyPattern)
				_, err := collection.Indexes().DropOne(ctx, indexName)
				if err != nil {
					log.Printf("Warning: Failed to drop index %s: %v", indexName, err)
				}
			}
		}
	}

	return nil
}

// createKeyPatternString creates a consistent string representation of index keys
func (m *MigrationService) createKeyPatternString(keys bson.M) string {
	// Convert to a slice for sorting
	type keyValue struct {
		key   string
		value interface{}
	}

	var pairs []keyValue
	for k, v := range keys {
		pairs = append(pairs, keyValue{key: k, value: v})
	}

	// Sort by key name for consistency
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].key < pairs[j].key
	})

	// Build the pattern string
	var parts []string
	for _, pair := range pairs {
		parts = append(parts, fmt.Sprintf("%s_%v", pair.key, pair.value))
	}

	return strings.Join(parts, "_")
}

// createIndexesSafely creates indexes while checking for conflicts
func (m *MigrationService) createIndexesSafely(ctx context.Context, collection *mongo.Collection, indexes []mongo.IndexModel) error {
	// Get existing indexes
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	var existingIndexes []bson.M
	if err := cursor.All(ctx, &existingIndexes); err != nil {
		return err
	}

	// Create a map of existing index names and key patterns
	existingNames := make(map[string]bool)
	existingKeyPatterns := make(map[string]string)

	for _, index := range existingIndexes {
		if name, ok := index["name"].(string); ok {
			existingNames[name] = true

			if keyDoc, hasKey := index["key"].(bson.M); hasKey {
				keyPattern := m.createKeyPatternString(keyDoc)
				existingKeyPatterns[keyPattern] = name
			}
		}
	}

	// Create indexes that don't already exist
	var indexesToCreate []mongo.IndexModel
	for _, index := range indexes {
		indexName := ""
		if index.Options != nil && index.Options.Name != nil {
			indexName = *index.Options.Name
		}

		// Check if index with this name already exists
		if existingNames[indexName] {
			log.Printf("Index '%s' already exists, skipping", indexName)
			continue
		}

		// Check if index with same key pattern already exists
		keyDoc := make(bson.M)
		if d, ok := index.Keys.(bson.D); ok {
			for _, elem := range d {
				keyDoc[elem.Key] = elem.Value
			}
		}
		keyPattern := m.createKeyPatternString(keyDoc)

		if existingName, exists := existingKeyPatterns[keyPattern]; exists {
			log.Printf("Index with key pattern '%s' already exists as '%s', skipping", keyPattern, existingName)
			continue
		}

		indexesToCreate = append(indexesToCreate, index)
	}

	// Create the new indexes
	if len(indexesToCreate) > 0 {
		log.Printf("Creating %d new indexes", len(indexesToCreate))
		_, err := collection.Indexes().CreateMany(ctx, indexesToCreate)
		if err != nil {
			return err
		}
	}

	return nil
}
