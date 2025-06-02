package config

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// Collections represents all MongoDB collections
type Collections struct {
	Users         *mongo.Collection
	Chats         *mongo.Collection
	Messages      *mongo.Collection
	Groups        *mongo.Collection
	Calls         *mongo.Collection
	AdminConfigs  *mongo.Collection
	Sessions      *mongo.Collection
	Files         *mongo.Collection
	Notifications *mongo.Collection
	Analytics     *mongo.Collection
}

// Database represents the database connection and collections
type Database struct {
	Client      *mongo.Client
	DB          *mongo.Database
	Collections *Collections
	ctx         context.Context
	cancel      context.CancelFunc
}

var (
	instance *Database
	dbName   = "chatapp"
)

// ConnectionConfig represents database connection configuration
type ConnectionConfig struct {
	URI               string
	Database          string
	MaxPoolSize       uint64
	MinPoolSize       uint64
	MaxIdleTime       time.Duration
	ConnectTimeout    time.Duration
	ServerSelection   time.Duration
	SocketTimeout     time.Duration
	HeartbeatInterval time.Duration
}

// DefaultConnectionConfig returns default database configuration
func DefaultConnectionConfig() *ConnectionConfig {
	return &ConnectionConfig{
		URI:               "mongodb://localhost:27017",
		Database:          "chatapp",
		MaxPoolSize:       100,
		MinPoolSize:       5,
		MaxIdleTime:       10 * time.Minute,
		ConnectTimeout:    10 * time.Second,
		ServerSelection:   5 * time.Second,
		SocketTimeout:     30 * time.Second,
		HeartbeatInterval: 10 * time.Second,
	}
}

// Connect establishes connection to MongoDB
func Connect(mongoURI string) (*Database, error) {
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	// Parse database name from URI or use default
	if mongoURI != "" {
		// Extract database name from URI if present
		// For simplicity, we'll use the default database name
		dbName = "chatapp"
	}

	config := DefaultConnectionConfig()
	config.URI = mongoURI
	config.Database = dbName

	return ConnectWithConfig(config)
}

// ConnectWithConfig establishes connection to MongoDB with custom configuration
func ConnectWithConfig(config *ConnectionConfig) (*Database, error) {
	ctx, cancel := context.WithTimeout(context.Background(), config.ConnectTimeout)
	defer cancel()

	// Client options
	clientOptions := options.Client().
		ApplyURI(config.URI).
		SetMaxPoolSize(config.MaxPoolSize).
		SetMinPoolSize(config.MinPoolSize).
		SetMaxConnIdleTime(config.MaxIdleTime).
		SetConnectTimeout(config.ConnectTimeout).
		SetServerSelectionTimeout(config.ServerSelection).
		SetSocketTimeout(config.SocketTimeout).
		SetHeartbeatInterval(config.HeartbeatInterval)

	// Create client
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Test connection
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	// Get database
	db := client.Database(config.Database)

	// Create collections structure
	collections := &Collections{
		Users:         db.Collection("users"),
		Chats:         db.Collection("chats"),
		Messages:      db.Collection("messages"),
		Groups:        db.Collection("groups"),
		Calls:         db.Collection("calls"),
		AdminConfigs:  db.Collection("admin_configs"),
		Sessions:      db.Collection("sessions"),
		Files:         db.Collection("files"),
		Notifications: db.Collection("notifications"),
		Analytics:     db.Collection("analytics"),
	}

	// Create database instance
	appCtx, appCancel := context.WithCancel(context.Background())
	database := &Database{
		Client:      client,
		DB:          db,
		Collections: collections,
		ctx:         appCtx,
		cancel:      appCancel,
	}

	// Store global instance
	instance = database

	// Create indexes
	if err := database.createIndexes(); err != nil {
		log.Printf("Warning: Failed to create some indexes: %v", err)
	}

	log.Printf("Successfully connected to MongoDB at %s", config.URI)
	return database, nil
}

// GetDatabase returns the global database instance
func GetDatabase() *Database {
	return instance
}

// GetCollections returns the collections from global instance
func GetCollections() *Collections {
	if instance == nil {
		return nil
	}
	return instance.Collections
}

// Health checks database connection health
func (d *Database) Health() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	return d.Client.Ping(ctx, readpref.Primary())
}

// Close closes the database connection
func (d *Database) Close() error {
	if d.cancel != nil {
		d.cancel()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return d.Client.Disconnect(ctx)
}

// createIndexes creates necessary indexes for optimal performance
func (d *Database) createIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var indexErrors []error

	// Users collection indexes
	userIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "phone_number", Value: 1},
				{Key: "country_code", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "full_phone_number", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "username", Value: 1}},
			Options: options.Index().SetUnique(true).SetSparse(true),
		},
		{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetUnique(true).SetSparse(true),
		},
		{
			Keys: bson.D{{Key: "is_active", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "last_seen", Value: -1}},
		},
	}

	if _, err := d.Collections.Users.Indexes().CreateMany(ctx, userIndexes); err != nil {
		indexErrors = append(indexErrors, fmt.Errorf("users indexes: %w", err))
	}

	// Chats collection indexes
	chatIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "participants", Value: 1}},
		},
		{
			Keys: bson.D{
				{Key: "type", Value: 1},
				{Key: "is_active", Value: 1},
			},
		},
		{
			Keys: bson.D{{Key: "last_activity", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "created_by", Value: 1}},
		},
	}

	if _, err := d.Collections.Chats.Indexes().CreateMany(ctx, chatIndexes); err != nil {
		indexErrors = append(indexErrors, fmt.Errorf("chats indexes: %w", err))
	}

	// Messages collection indexes
	messageIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "chat_id", Value: 1},
				{Key: "created_at", Value: -1},
			},
		},
		{
			Keys: bson.D{{Key: "sender_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "type", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "status", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "is_deleted", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
		{
			Keys:    bson.D{{Key: "reply_to_id", Value: 1}},
			Options: options.Index().SetSparse(true),
		},
		{
			Keys:    bson.D{{Key: "mentions", Value: 1}},
			Options: options.Index().SetSparse(true),
		},
		{
			Keys:    bson.D{{Key: "scheduled_at", Value: 1}},
			Options: options.Index().SetSparse(true),
		},
	}

	if _, err := d.Collections.Messages.Indexes().CreateMany(ctx, messageIndexes); err != nil {
		indexErrors = append(indexErrors, fmt.Errorf("messages indexes: %w", err))
	}

	// Groups collection indexes
	groupIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "chat_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "created_by", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "owner", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "admins", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "members.user_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "is_active", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "is_public", Value: 1}},
		},
		{
			Keys:    bson.D{{Key: "invite_code", Value: 1}},
			Options: options.Index().SetUnique(true).SetSparse(true),
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "last_activity", Value: -1}},
		},
	}

	if _, err := d.Collections.Groups.Indexes().CreateMany(ctx, groupIndexes); err != nil {
		indexErrors = append(indexErrors, fmt.Errorf("groups indexes: %w", err))
	}

	// Calls collection indexes
	callIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "chat_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "initiator_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "participants.user_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "type", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "status", Value: 1}},
		},
		{
			Keys:    bson.D{{Key: "session_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "initiated_at", Value: -1}},
		},
		{
			Keys:    bson.D{{Key: "started_at", Value: -1}},
			Options: options.Index().SetSparse(true),
		},
		{
			Keys:    bson.D{{Key: "ended_at", Value: -1}},
			Options: options.Index().SetSparse(true),
		},
	}

	if _, err := d.Collections.Calls.Indexes().CreateMany(ctx, callIndexes); err != nil {
		indexErrors = append(indexErrors, fmt.Errorf("calls indexes: %w", err))
	}

	// Admin Configs collection indexes
	adminConfigIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "environment", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "config_version", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "last_updated_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
	}

	if _, err := d.Collections.AdminConfigs.Indexes().CreateMany(ctx, adminConfigIndexes); err != nil {
		indexErrors = append(indexErrors, fmt.Errorf("admin_configs indexes: %w", err))
	}

	// Sessions collection indexes
	sessionIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "user_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "device_id", Value: 1}},
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0),
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "is_active", Value: 1}},
		},
	}

	if _, err := d.Collections.Sessions.Indexes().CreateMany(ctx, sessionIndexes); err != nil {
		indexErrors = append(indexErrors, fmt.Errorf("sessions indexes: %w", err))
	}

	// Files collection indexes
	fileIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "upload_by", Value: 1}},
		},
		{
			Keys:    bson.D{{Key: "chat_id", Value: 1}},
			Options: options.Index().SetSparse(true),
		},
		{
			Keys:    bson.D{{Key: "message_id", Value: 1}},
			Options: options.Index().SetSparse(true),
		},
		{
			Keys: bson.D{{Key: "file_type", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0).SetSparse(true),
		},
	}

	if _, err := d.Collections.Files.Indexes().CreateMany(ctx, fileIndexes); err != nil {
		indexErrors = append(indexErrors, fmt.Errorf("files indexes: %w", err))
	}

	// Notifications collection indexes
	notificationIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "user_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "type", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "is_read", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0).SetSparse(true),
		},
	}

	if _, err := d.Collections.Notifications.Indexes().CreateMany(ctx, notificationIndexes); err != nil {
		indexErrors = append(indexErrors, fmt.Errorf("notifications indexes: %w", err))
	}

	// Analytics collection indexes
	analyticsIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "event_type", Value: 1}},
		},
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}},
			Options: options.Index().SetSparse(true),
		},
		{
			Keys: bson.D{
				{Key: "timestamp", Value: -1},
				{Key: "event_type", Value: 1},
			},
		},
		{
			Keys:    bson.D{{Key: "session_id", Value: 1}},
			Options: options.Index().SetSparse(true),
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
	}

	if _, err := d.Collections.Analytics.Indexes().CreateMany(ctx, analyticsIndexes); err != nil {
		indexErrors = append(indexErrors, fmt.Errorf("analytics indexes: %w", err))
	}

	if len(indexErrors) > 0 {
		return fmt.Errorf("failed to create some indexes: %v", indexErrors)
	}

	log.Println("All database indexes created successfully")
	return nil
}

// CreateTTLIndexes creates Time-To-Live indexes for automatic document expiration
func (d *Database) CreateTTLIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// OTP cleanup - expire after 15 minutes
	otpTTLIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: "otp.created_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(900), // 15 minutes
	}

	if _, err := d.Collections.Users.Indexes().CreateOne(ctx, otpTTLIndex); err != nil {
		return fmt.Errorf("failed to create OTP TTL index: %w", err)
	}

	// Session cleanup - expire based on expires_at field
	sessionTTLIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: "expires_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0),
	}

	if _, err := d.Collections.Sessions.Indexes().CreateOne(ctx, sessionTTLIndex); err != nil {
		return fmt.Errorf("failed to create session TTL index: %w", err)
	}

	log.Println("TTL indexes created successfully")
	return nil
}

// DropIndexes drops all custom indexes (useful for development/testing)
func (d *Database) DropIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collections := []*mongo.Collection{
		d.Collections.Users,
		d.Collections.Chats,
		d.Collections.Messages,
		d.Collections.Groups,
		d.Collections.Calls,
		d.Collections.AdminConfigs,
		d.Collections.Sessions,
		d.Collections.Files,
		d.Collections.Notifications,
		d.Collections.Analytics,
	}

	for _, collection := range collections {
		if _, err := collection.Indexes().DropAll(ctx); err != nil {
			log.Printf("Warning: Failed to drop indexes for collection %s: %v", collection.Name(), err)
		}
	}

	log.Println("All custom indexes dropped")
	return nil
}

// GetStats returns database statistics
func (d *Database) GetStats() (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stats := make(map[string]interface{})

	// Get database stats
	var dbStats bson.M
	err := d.DB.RunCommand(ctx, bson.D{{Key: "dbStats", Value: 1}}).Decode(&dbStats)
	if err != nil {
		return nil, fmt.Errorf("failed to get database stats: %w", err)
	}
	stats["database"] = dbStats

	// Get collection stats
	collections := map[string]*mongo.Collection{
		"users":         d.Collections.Users,
		"chats":         d.Collections.Chats,
		"messages":      d.Collections.Messages,
		"groups":        d.Collections.Groups,
		"calls":         d.Collections.Calls,
		"admin_configs": d.Collections.AdminConfigs,
		"sessions":      d.Collections.Sessions,
		"files":         d.Collections.Files,
		"notifications": d.Collections.Notifications,
		"analytics":     d.Collections.Analytics,
	}

	collectionStats := make(map[string]interface{})
	for name, collection := range collections {
		count, err := collection.CountDocuments(ctx, bson.M{})
		if err != nil {
			log.Printf("Warning: Failed to count documents in %s: %v", name, err)
			continue
		}
		collectionStats[name] = map[string]interface{}{
			"count": count,
		}
	}
	stats["collections"] = collectionStats

	return stats, nil
}

// Migrate runs database migrations
func (d *Database) Migrate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	log.Println("Starting database migration...")

	// Check if migration is needed by looking for admin config
	count, err := d.Collections.AdminConfigs.CountDocuments(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("failed to check admin config: %w", err)
	}

	// If no admin config exists, create default one
	if count == 0 {
		defaultConfig := bson.M{
			"config_version": "1.0.0",
			"environment":    "production",
			"created_at":     time.Now(),
			"updated_at":     time.Now(),
			"app_settings": bson.M{
				"app_name":         "ChatApp",
				"app_version":      "1.0.0",
				"default_language": "en",
				"timezone":         "UTC",
				"maintenance_mode": false,
			},
			"feature_flags": bson.M{
				"enable_registration":       true,
				"enable_group_chats":        true,
				"enable_voice_calls":        true,
				"enable_video_calls":        true,
				"enable_file_sharing":       true,
				"enable_message_encryption": true,
			},
		}

		_, err := d.Collections.AdminConfigs.InsertOne(ctx, defaultConfig)
		if err != nil {
			return fmt.Errorf("failed to create default admin config: %w", err)
		}
		log.Println("Default admin configuration created")
	}

	// Create TTL indexes
	if err := d.CreateTTLIndexes(); err != nil {
		log.Printf("Warning: Failed to create TTL indexes: %v", err)
	}

	log.Println("Database migration completed successfully")
	return nil
}

// Cleanup performs database cleanup operations
func (d *Database) Cleanup() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Println("Starting database cleanup...")

	// Clean expired OTPs
	otpFilter := bson.M{
		"otp.created_at": bson.M{
			"$lt": time.Now().Add(-15 * time.Minute),
		},
	}
	update := bson.M{
		"$unset": bson.M{"otp": ""},
	}
	_, err := d.Collections.Users.UpdateMany(ctx, otpFilter, update)
	if err != nil {
		log.Printf("Warning: Failed to clean expired OTPs: %v", err)
	}

	// Clean old deleted messages (older than 30 days)
	messageFilter := bson.M{
		"is_deleted": true,
		"deleted_at": bson.M{
			"$lt": time.Now().Add(-30 * 24 * time.Hour),
		},
	}
	result, err := d.Collections.Messages.DeleteMany(ctx, messageFilter)
	if err != nil {
		log.Printf("Warning: Failed to clean old deleted messages: %v", err)
	} else if result.DeletedCount > 0 {
		log.Printf("Cleaned %d old deleted messages", result.DeletedCount)
	}

	// Clean old analytics data (older than 90 days)
	analyticsFilter := bson.M{
		"created_at": bson.M{
			"$lt": time.Now().Add(-90 * 24 * time.Hour),
		},
	}
	result, err = d.Collections.Analytics.DeleteMany(ctx, analyticsFilter)
	if err != nil {
		log.Printf("Warning: Failed to clean old analytics data: %v", err)
	} else if result.DeletedCount > 0 {
		log.Printf("Cleaned %d old analytics records", result.DeletedCount)
	}

	log.Println("Database cleanup completed")
	return nil
}

// Backup creates a backup of specified collections
func (d *Database) Backup(collections []string, backupPath string) error {
	log.Println("Database backup functionality would be implemented here")
	// This would typically use mongodump or a similar tool
	// Implementation depends on deployment environment
	return nil
}

// Restore restores database from backup
func (d *Database) Restore(backupPath string) error {
	log.Println("Database restore functionality would be implemented here")
	// This would typically use mongorestore or a similar tool
	// Implementation depends on deployment environment
	return nil
}

// Transaction executes a function within a MongoDB transaction
func (d *Database) Transaction(fn func(ctx mongo.SessionContext) error) error {
	session, err := d.Client.StartSession()
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	defer session.EndSession(context.Background())

	_, err = session.WithTransaction(context.Background(), func(ctx mongo.SessionContext) (interface{}, error) {
		return nil, fn(ctx)
	})

	return err
}

// Helper functions for common database operations

// EnsureConnection ensures database connection is alive
func EnsureConnection() error {
	if instance == nil {
		return fmt.Errorf("database not initialized")
	}
	return instance.Health()
}

// GetClient returns the MongoDB client
func GetClient() *mongo.Client {
	if instance == nil {
		return nil
	}
	return instance.Client
}

// GetDB returns the MongoDB database
func GetDB() *mongo.Database {
	if instance == nil {
		return nil
	}
	return instance.DB
}

// IsDocumentExists checks if a document exists in a collection
func IsDocumentExists(collection *mongo.Collection, filter bson.M) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := collection.CountDocuments(ctx, filter, options.Count().SetLimit(1))
	return count > 0, err
}

// GetDocumentCount returns the count of documents matching filter
func GetDocumentCount(collection *mongo.Collection, filter bson.M) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, filter)
}
