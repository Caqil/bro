package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bro/internal/config"
	"bro/pkg/database"
	"bro/pkg/logger"
)

// IndexDefinition represents a MongoDB index
type IndexDefinition struct {
	Collection string
	Keys       bson.D
	Options    *options.IndexOptions
}

func main() {
	// Initialize logger
	logger.Init()

	// Load configuration
	cfg := config.Load()

	// Connect to MongoDB
	_, err := database.Connect(cfg.MongoURI)
	if err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}
	//defer client.Disconnect(context.Background())

	db := database.GetDB()

	log.Println("Starting database migration...")

	// Run migrations
	if err := runMigrations(db); err != nil {
		log.Fatal("Migration failed:", err)
	}

	log.Println("Database migration completed successfully!")
}

func runMigrations(db *mongo.Database) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create collections with validation (optional)
	if err := createCollections(ctx, db); err != nil {
		return fmt.Errorf("failed to create collections: %w", err)
	}

	// Create indexes
	if err := createIndexes(ctx, db); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	// Run data migrations
	if err := runDataMigrations(ctx, db); err != nil {
		return fmt.Errorf("failed to run data migrations: %w", err)
	}

	return nil
}

func createCollections(ctx context.Context, db *mongo.Database) error {
	log.Println("Creating collections...")

	collections := []string{
		"users",
		"chats",
		"messages",
		"groups",
		"calls",
		"files",
		"admin_configs",
		"sessions",
		"push_tokens",
		"reports",
		"analytics_events",
	}

	for _, collName := range collections {
		// Check if collection already exists
		colls, err := db.ListCollectionNames(ctx, bson.M{"name": collName})
		if err != nil {
			return fmt.Errorf("failed to list collections: %w", err)
		}

		if len(colls) == 0 {
			// Create collection with validation for critical collections
			opts := options.CreateCollection()

			switch collName {
			case "users":
				// Add validation for users collection
				validator := bson.M{
					"$jsonSchema": bson.M{
						"bsonType": "object",
						"required": []string{"phone_number", "country_code", "name", "created_at"},
						"properties": bson.M{
							"phone_number": bson.M{
								"bsonType":    "string",
								"description": "must be a string and is required",
							},
							"country_code": bson.M{
								"bsonType":    "string",
								"description": "must be a string and is required",
							},
							"name": bson.M{
								"bsonType":    "string",
								"description": "must be a string and is required",
							},
							"email": bson.M{
								"bsonType":    "string",
								"pattern":     `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`,
								"description": "must be a valid email address",
							},
							"role": bson.M{
								"enum":        []string{"user", "moderator", "admin", "super_admin"},
								"description": "must be a valid role",
							},
						},
					},
				}
				opts.SetValidator(validator)

			case "messages":
				// Add validation for messages collection
				validator := bson.M{
					"$jsonSchema": bson.M{
						"bsonType": "object",
						"required": []string{"chat_id", "sender_id", "type", "created_at"},
						"properties": bson.M{
							"type": bson.M{
								"enum": []string{
									"text", "image", "video", "audio", "document",
									"voice_note", "location", "contact", "sticker",
									"gif", "call", "system", "deleted", "join_group",
									"leave_group", "group_created", "group_renamed", "photo_changed",
								},
								"description": "must be a valid message type",
							},
						},
					},
				}
				opts.SetValidator(validator)

			case "chats":
				// Add validation for chats collection
				validator := bson.M{
					"$jsonSchema": bson.M{
						"bsonType": "object",
						"required": []string{"type", "participants", "created_by", "created_at"},
						"properties": bson.M{
							"type": bson.M{
								"enum":        []string{"private", "group", "broadcast", "bot", "support"},
								"description": "must be a valid chat type",
							},
						},
					},
				}
				opts.SetValidator(validator)
			}

			if err := db.CreateCollection(ctx, collName, opts); err != nil {
				return fmt.Errorf("failed to create collection %s: %w", collName, err)
			}
			log.Printf("Created collection: %s", collName)
		} else {
			log.Printf("Collection already exists: %s", collName)
		}
	}

	return nil
}

func createIndexes(ctx context.Context, db *mongo.Database) error {
	log.Println("Creating indexes...")

	indexes := []IndexDefinition{
		// Users collection indexes
		{
			Collection: "users",
			Keys:       bson.D{{Key: "phone_number", Value: 1}, {Key: "country_code", Value: 1}},
			Options:    options.Index().SetUnique(true).SetName("phone_unique"),
		},
		{
			Collection: "users",
			Keys:       bson.D{{Key: "email", Value: 1}},
			Options:    options.Index().SetUnique(true).SetSparse(true).SetName("email_unique"),
		},
		{
			Collection: "users",
			Keys:       bson.D{{Key: "username", Value: 1}},
			Options:    options.Index().SetUnique(true).SetSparse(true).SetName("username_unique"),
		},
		{
			Collection: "users",
			Keys:       bson.D{{Key: "full_phone_number", Value: 1}},
			Options:    options.Index().SetUnique(true).SetName("full_phone_unique"),
		},
		{
			Collection: "users",
			Keys:       bson.D{{Key: "is_active", Value: 1}, {Key: "last_seen", Value: -1}},
			Options:    options.Index().SetName("active_users"),
		},
		{
			Collection: "users",
			Keys:       bson.D{{Key: "role", Value: 1}},
			Options:    options.Index().SetName("user_role"),
		},
		{
			Collection: "users",
			Keys:       bson.D{{Key: "created_at", Value: -1}},
			Options:    options.Index().SetName("user_created"),
		},

		// Chats collection indexes
		{
			Collection: "chats",
			Keys:       bson.D{{Key: "participants", Value: 1}},
			Options:    options.Index().SetName("chat_participants"),
		},
		{
			Collection: "chats",
			Keys:       bson.D{{Key: "type", Value: 1}},
			Options:    options.Index().SetName("chat_type"),
		},
		{
			Collection: "chats",
			Keys:       bson.D{{Key: "created_by", Value: 1}},
			Options:    options.Index().SetName("chat_creator"),
		},
		{
			Collection: "chats",
			Keys:       bson.D{{Key: "last_activity", Value: -1}},
			Options:    options.Index().SetName("chat_activity"),
		},
		{
			Collection: "chats",
			Keys:       bson.D{{Key: "is_active", Value: 1}},
			Options:    options.Index().SetName("active_chats"),
		},

		// Messages collection indexes
		{
			Collection: "messages",
			Keys:       bson.D{{Key: "chat_id", Value: 1}, {Key: "created_at", Value: -1}},
			Options:    options.Index().SetName("chat_messages"),
		},
		{
			Collection: "messages",
			Keys:       bson.D{{Key: "sender_id", Value: 1}},
			Options:    options.Index().SetName("message_sender"),
		},
		{
			Collection: "messages",
			Keys:       bson.D{{Key: "type", Value: 1}},
			Options:    options.Index().SetName("message_type"),
		},
		{
			Collection: "messages",
			Keys:       bson.D{{Key: "reply_to_id", Value: 1}},
			Options:    options.Index().SetSparse(true).SetName("message_replies"),
		},
		{
			Collection: "messages",
			Keys:       bson.D{{Key: "is_deleted", Value: 1}},
			Options:    options.Index().SetName("deleted_messages"),
		},
		{
			Collection: "messages",
			Keys:       bson.D{{Key: "mentions", Value: 1}},
			Options:    options.Index().SetSparse(true).SetName("message_mentions"),
		},
		{
			Collection: "messages",
			Keys:       bson.D{{Key: "scheduled_at", Value: 1}},
			Options:    options.Index().SetSparse(true).SetName("scheduled_messages"),
		},
		{
			Collection: "messages",
			Keys:       bson.D{{Key: "content", Value: "text"}},
			Options:    options.Index().SetName("message_search"),
		},

		// Groups collection indexes
		{
			Collection: "groups",
			Keys:       bson.D{{Key: "chat_id", Value: 1}},
			Options:    options.Index().SetUnique(true).SetName("group_chat"),
		},
		{
			Collection: "groups",
			Keys:       bson.D{{Key: "owner", Value: 1}},
			Options:    options.Index().SetName("group_owner"),
		},
		{
			Collection: "groups",
			Keys:       bson.D{{Key: "members.user_id", Value: 1}},
			Options:    options.Index().SetName("group_members"),
		},
		{
			Collection: "groups",
			Keys:       bson.D{{Key: "is_public", Value: 1}, {Key: "is_active", Value: 1}},
			Options:    options.Index().SetName("public_groups"),
		},
		{
			Collection: "groups",
			Keys:       bson.D{{Key: "invite_code", Value: 1}},
			Options:    options.Index().SetUnique(true).SetName("group_invite_code"),
		},
		{
			Collection: "groups",
			Keys:       bson.D{{Key: "last_activity", Value: -1}},
			Options:    options.Index().SetName("group_activity"),
		},
		{
			Collection: "groups",
			Keys:       bson.D{{Key: "name", Value: "text"}},
			Options:    options.Index().SetName("group_search"),
		},

		// Calls collection indexes
		{
			Collection: "calls",
			Keys:       bson.D{{Key: "chat_id", Value: 1}},
			Options:    options.Index().SetName("call_chat"),
		},
		{
			Collection: "calls",
			Keys:       bson.D{{Key: "initiator_id", Value: 1}},
			Options:    options.Index().SetName("call_initiator"),
		},
		{
			Collection: "calls",
			Keys:       bson.D{{Key: "participants.user_id", Value: 1}},
			Options:    options.Index().SetName("call_participants"),
		},
		{
			Collection: "calls",
			Keys:       bson.D{{Key: "status", Value: 1}},
			Options:    options.Index().SetName("call_status"),
		},
		{
			Collection: "calls",
			Keys:       bson.D{{Key: "type", Value: 1}},
			Options:    options.Index().SetName("call_type"),
		},
		{
			Collection: "calls",
			Keys:       bson.D{{Key: "initiated_at", Value: -1}},
			Options:    options.Index().SetName("call_time"),
		},
		{
			Collection: "calls",
			Keys:       bson.D{{Key: "session_id", Value: 1}},
			Options:    options.Index().SetUnique(true).SetName("call_session"),
		},

		// Files collection indexes
		{
			Collection: "files",
			Keys:       bson.D{{Key: "user_id", Value: 1}},
			Options:    options.Index().SetName("file_owner"),
		},
		{
			Collection: "files",
			Keys:       bson.D{{Key: "chat_id", Value: 1}},
			Options:    options.Index().SetSparse(true).SetName("file_chat"),
		},
		{
			Collection: "files",
			Keys:       bson.D{{Key: "message_id", Value: 1}},
			Options:    options.Index().SetSparse(true).SetName("file_message"),
		},
		{
			Collection: "files",
			Keys:       bson.D{{Key: "content_type", Value: 1}},
			Options:    options.Index().SetName("file_type"),
		},
		{
			Collection: "files",
			Keys:       bson.D{{Key: "purpose", Value: 1}},
			Options:    options.Index().SetName("file_purpose"),
		},
		{
			Collection: "files",
			Keys:       bson.D{{Key: "uploaded_at", Value: -1}},
			Options:    options.Index().SetName("file_upload_time"),
		},
		{
			Collection: "files",
			Keys:       bson.D{{Key: "expires_at", Value: 1}},
			Options:    options.Index().SetSparse(true).SetName("file_expiry"),
		},
		{
			Collection: "files",
			Keys:       bson.D{{Key: "file_name", Value: "text"}},
			Options:    options.Index().SetName("file_search"),
		},

		// Sessions collection indexes
		{
			Collection: "sessions",
			Keys:       bson.D{{Key: "user_id", Value: 1}},
			Options:    options.Index().SetName("session_user"),
		},
		{
			Collection: "sessions",
			Keys:       bson.D{{Key: "session_id", Value: 1}},
			Options:    options.Index().SetUnique(true).SetName("session_id"),
		},
		{
			Collection: "sessions",
			Keys:       bson.D{{Key: "expires_at", Value: 1}},
			Options:    options.Index().SetExpireAfterSeconds(0).SetName("session_expiry"),
		},
		{
			Collection: "sessions",
			Keys:       bson.D{{Key: "is_active", Value: 1}},
			Options:    options.Index().SetName("active_sessions"),
		},

		// Push tokens collection indexes
		{
			Collection: "push_tokens",
			Keys:       bson.D{{Key: "user_id", Value: 1}},
			Options:    options.Index().SetName("token_user"),
		},
		{
			Collection: "push_tokens",
			Keys:       bson.D{{Key: "token", Value: 1}},
			Options:    options.Index().SetUnique(true).SetName("token_unique"),
		},
		{
			Collection: "push_tokens",
			Keys:       bson.D{{Key: "platform", Value: 1}},
			Options:    options.Index().SetName("token_platform"),
		},
		{
			Collection: "push_tokens",
			Keys:       bson.D{{Key: "is_active", Value: 1}},
			Options:    options.Index().SetName("active_tokens"),
		},

		// Admin configs collection indexes
		{
			Collection: "admin_configs",
			Keys:       bson.D{{Key: "environment", Value: 1}},
			Options:    options.Index().SetName("config_environment"),
		},
		{
			Collection: "admin_configs",
			Keys:       bson.D{{Key: "config_version", Value: 1}},
			Options:    options.Index().SetName("config_version"),
		},

		// Reports collection indexes
		{
			Collection: "reports",
			Keys:       bson.D{{Key: "reporter_id", Value: 1}},
			Options:    options.Index().SetName("report_user"),
		},
		{
			Collection: "reports",
			Keys:       bson.D{{Key: "reported_content_id", Value: 1}},
			Options:    options.Index().SetName("reported_content"),
		},
		{
			Collection: "reports",
			Keys:       bson.D{{Key: "status", Value: 1}},
			Options:    options.Index().SetName("report_status"),
		},
		{
			Collection: "reports",
			Keys:       bson.D{{Key: "created_at", Value: -1}},
			Options:    options.Index().SetName("report_time"),
		},

		// Analytics events collection indexes
		{
			Collection: "analytics_events",
			Keys:       bson.D{{Key: "user_id", Value: 1}},
			Options:    options.Index().SetName("analytics_user"),
		},
		{
			Collection: "analytics_events",
			Keys:       bson.D{{Key: "event_type", Value: 1}},
			Options:    options.Index().SetName("analytics_event_type"),
		},
		{
			Collection: "analytics_events",
			Keys:       bson.D{{Key: "timestamp", Value: -1}},
			Options:    options.Index().SetName("analytics_time"),
		},
		{
			Collection: "analytics_events",
			Keys:       bson.D{{Key: "timestamp", Value: 1}},
			Options:    options.Index().SetExpireAfterSeconds(int32(90 * 24 * 3600)).SetName("analytics_cleanup"), // 90 days retention
		},
	}

	for _, idx := range indexes {
		collection := db.Collection(idx.Collection)

		// Check if index already exists
		cursor, err := collection.Indexes().List(ctx)
		if err != nil {
			return fmt.Errorf("failed to list indexes for %s: %w", idx.Collection, err)
		}

		var existingIndexes []bson.M
		if err := cursor.All(ctx, &existingIndexes); err != nil {
			return fmt.Errorf("failed to decode indexes for %s: %w", idx.Collection, err)
		}

		indexExists := false
		indexName := idx.Options.Name
		if indexName == nil {
			// Generate default name if not provided
			name := ""
			for i, key := range idx.Keys {
				if i > 0 {
					name += "_"
				}
				name += fmt.Sprintf("%s_%v", key.Key, key.Value)
			}
			indexName = &name
		}

		for _, existing := range existingIndexes {
			if existing["name"] == *indexName {
				indexExists = true
				break
			}
		}

		if !indexExists {
			_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys:    idx.Keys,
				Options: idx.Options,
			})
			if err != nil {
				return fmt.Errorf("failed to create index %s on %s: %w", *indexName, idx.Collection, err)
			}
			log.Printf("Created index: %s on collection %s", *indexName, idx.Collection)
		} else {
			log.Printf("Index already exists: %s on collection %s", *indexName, idx.Collection)
		}
	}

	return nil
}

func runDataMigrations(ctx context.Context, db *mongo.Database) error {
	log.Println("Running data migrations...")

	// Migration 1: Ensure all users have full_phone_number field
	if err := migrateUserPhoneNumbers(ctx, db); err != nil {
		return fmt.Errorf("failed to migrate user phone numbers: %w", err)
	}

	// Migration 2: Ensure all chats have proper unread counts
	if err := migrateChatUnreadCounts(ctx, db); err != nil {
		return fmt.Errorf("failed to migrate chat unread counts: %w", err)
	}

	// Migration 3: Add missing timestamps
	if err := addMissingTimestamps(ctx, db); err != nil {
		return fmt.Errorf("failed to add missing timestamps: %w", err)
	}

	log.Println("Data migrations completed")
	return nil
}

func migrateUserPhoneNumbers(ctx context.Context, db *mongo.Database) error {
	collection := db.Collection("users")

	// Find users without full_phone_number
	filter := bson.M{
		"$or": []bson.M{
			{"full_phone_number": bson.M{"$exists": false}},
			{"full_phone_number": ""},
		},
	}

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	var updates []mongo.WriteModel
	for cursor.Next(ctx) {
		var user bson.M
		if err := cursor.Decode(&user); err != nil {
			continue
		}

		userID := user["_id"]
		countryCode, _ := user["country_code"].(string)
		phoneNumber, _ := user["phone_number"].(string)

		if countryCode != "" && phoneNumber != "" {
			fullPhoneNumber := countryCode + phoneNumber

			update := mongo.NewUpdateOneModel()
			update.SetFilter(bson.M{"_id": userID})
			update.SetUpdate(bson.M{
				"$set": bson.M{
					"full_phone_number": fullPhoneNumber,
					"updated_at":        time.Now(),
				},
			})
			updates = append(updates, update)
		}
	}

	if len(updates) > 0 {
		_, err := collection.BulkWrite(ctx, updates)
		if err != nil {
			return err
		}
		log.Printf("Updated %d users with full_phone_number", len(updates))
	}

	return nil
}

func migrateChatUnreadCounts(ctx context.Context, db *mongo.Database) error {
	collection := db.Collection("chats")

	// Find chats without unread_counts
	filter := bson.M{
		"unread_counts": bson.M{"$exists": false},
	}

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	var updates []mongo.WriteModel
	for cursor.Next(ctx) {
		var chat bson.M
		if err := cursor.Decode(&chat); err != nil {
			continue
		}

		chatID := chat["_id"]
		participants, ok := chat["participants"].(bson.A)
		if !ok {
			continue
		}

		unreadCounts := bson.A{}
		now := time.Now()

		for _, p := range participants {
			if participantID, ok := p.(primitive.ObjectID); ok {
				unreadCount := bson.M{
					"user_id":       participantID,
					"count":         0,
					"last_read_at":  now,
					"mention_count": 0,
				}
				unreadCounts = append(unreadCounts, unreadCount)
			}
		}

		update := mongo.NewUpdateOneModel()
		update.SetFilter(bson.M{"_id": chatID})
		update.SetUpdate(bson.M{
			"$set": bson.M{
				"unread_counts": unreadCounts,
				"updated_at":    now,
			},
		})
		updates = append(updates, update)
	}

	if len(updates) > 0 {
		_, err := collection.BulkWrite(ctx, updates)
		if err != nil {
			return err
		}
		log.Printf("Updated %d chats with unread_counts", len(updates))
	}

	return nil
}

func addMissingTimestamps(ctx context.Context, db *mongo.Database) error {
	collections := []string{"users", "chats", "messages", "groups", "calls", "files"}
	now := time.Now()

	for _, collName := range collections {
		collection := db.Collection(collName)

		// Update documents missing created_at
		filter := bson.M{"created_at": bson.M{"$exists": false}}
		update := bson.M{
			"$set": bson.M{
				"created_at": now,
				"updated_at": now,
			},
		}

		result, err := collection.UpdateMany(ctx, filter, update)
		if err != nil {
			return fmt.Errorf("failed to update timestamps for %s: %w", collName, err)
		}

		if result.ModifiedCount > 0 {
			log.Printf("Added timestamps to %d documents in %s", result.ModifiedCount, collName)
		}
	}

	return nil
}
