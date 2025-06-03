package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
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
}

// Config represents database configuration
type Config struct {
	URI               string
	Database          string
	MaxPoolSize       uint64
	MinPoolSize       uint64
	MaxConnIdleTime   time.Duration
	ConnectTimeout    time.Duration
	ServerSelection   time.Duration
	SocketTimeout     time.Duration
	HeartbeatInterval time.Duration
}

var (
	globalDB *Database
)

// LoadConfigFromEnv loads database configuration from environment variables
func LoadConfigFromEnv() *Config {
	return &Config{
		URI:               getEnv("MONGO_URI", "mongodb://localhost:27017/bro_chat"),
		Database:          getEnv("MONGO_DATABASE", "chatapp"),
		MaxPoolSize:       uint64(getEnvInt("MONGO_MAX_POOL_SIZE", 100)),
		MinPoolSize:       uint64(getEnvInt("MONGO_MIN_POOL_SIZE", 5)),
		MaxConnIdleTime:   time.Duration(getEnvInt("MONGO_MAX_IDLE_TIME_MINUTES", 10)) * time.Minute,
		ConnectTimeout:    time.Duration(getEnvInt("MONGO_CONNECT_TIMEOUT_SECONDS", 10)) * time.Second,
		ServerSelection:   time.Duration(getEnvInt("MONGO_SERVER_SELECTION_TIMEOUT_SECONDS", 5)) * time.Second,
		SocketTimeout:     time.Duration(getEnvInt("MONGO_SOCKET_TIMEOUT_SECONDS", 30)) * time.Second,
		HeartbeatInterval: time.Duration(getEnvInt("MONGO_HEARTBEAT_INTERVAL_SECONDS", 10)) * time.Second,
	}
}

// DefaultConfig returns default database configuration
func DefaultConfig() *Config {
	return &Config{
		URI:               "mongodb://localhost:27017/",
		Database:          "chatapp",
		MaxPoolSize:       100,
		MinPoolSize:       5,
		MaxConnIdleTime:   10 * time.Minute,
		ConnectTimeout:    10 * time.Second,
		ServerSelection:   5 * time.Second,
		SocketTimeout:     30 * time.Second,
		HeartbeatInterval: 10 * time.Second,
	}
}

// Connect establishes connection to MongoDB using environment variables
func Connect() (*Database, error) {
	config := LoadConfigFromEnv()
	return ConnectWithConfig(config)
}

// ConnectWithURI establishes connection to MongoDB with provided URI (backward compatibility)
func ConnectWithURI(mongoURI string) (*Database, error) {
	if mongoURI == "" {
		return Connect() // Fall back to env-based connection
	}

	config := LoadConfigFromEnv()
	config.URI = mongoURI
	return ConnectWithConfig(config)
}

// ConnectWithConfig establishes connection to MongoDB with custom configuration
func ConnectWithConfig(config *Config) (*Database, error) {
	ctx, cancel := context.WithTimeout(context.Background(), config.ConnectTimeout)
	defer cancel()

	// Client options
	clientOptions := options.Client().
		ApplyURI(config.URI).
		SetMaxPoolSize(config.MaxPoolSize).
		SetMinPoolSize(config.MinPoolSize).
		SetMaxConnIdleTime(config.MaxConnIdleTime).
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

	// Extract database name from config or URI
	dbName := config.Database
	if dbName == "" || dbName == "chatapp" {
		// Try to extract from URI if available
		if config.URI != "" {
			// Simple extraction - you might want to use a proper URI parser
			dbName = "bro_chat" // Default for this project
		}
	}

	// Get database
	db := client.Database(dbName)

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
	database := &Database{
		Client:      client,
		DB:          db,
		Collections: collections,
	}

	// Store global instance
	globalDB = database

	// Create indexes
	if err := createIndexes(database); err != nil {
		log.Printf("Warning: Failed to create some indexes: %v", err)
	}

	log.Printf("Successfully connected to MongoDB at %s (database: %s)", config.URI, dbName)
	return database, nil
}

// Environment helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if result, err := strconv.Atoi(value); err == nil {
			return result
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if result, err := strconv.ParseBool(value); err == nil {
			return result
		}
	}
	return defaultValue
}

// GetDB returns the global database instance
func GetDB() *mongo.Database {
	if globalDB == nil {
		return nil
	}
	return globalDB.DB
}

// GetClient returns the global MongoDB client
func GetClient() *mongo.Client {
	if globalDB == nil {
		return nil
	}
	return globalDB.Client
}

// GetCollections returns the collections from global instance
func GetCollections() *Collections {
	if globalDB == nil {
		return nil
	}
	return globalDB.Collections
}

// GetDatabase returns the global database instance
func GetDatabase() *Database {
	return globalDB
}

// Close closes the database connection
func Close() error {
	if globalDB == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return globalDB.Client.Disconnect(ctx)
}

// Health checks database connection health
func Health() error {
	if globalDB == nil {
		return fmt.Errorf("database not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	return globalDB.Client.Ping(ctx, readpref.Primary())
}

// Reconnect attempts to reconnect to the database
func Reconnect() error {
	if globalDB != nil {
		Close() // Close existing connection
	}

	_, err := Connect()
	return err
}

// GetConnectionInfo returns information about the current connection
func GetConnectionInfo() map[string]interface{} {
	if globalDB == nil {
		return map[string]interface{}{
			"connected": false,
			"error":     "database not initialized",
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	info := map[string]interface{}{
		"connected": true,
		"database":  globalDB.DB.Name(),
	}

	// Try to get server status
	var serverStatus bson.M
	err := globalDB.DB.RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).Decode(&serverStatus)
	if err == nil {
		if host, ok := serverStatus["host"]; ok {
			info["host"] = host
		}
		if version, ok := serverStatus["version"]; ok {
			info["version"] = version
		}
		if uptime, ok := serverStatus["uptime"]; ok {
			info["uptime"] = uptime
		}
	}

	return info
}

// createIndexes creates necessary indexes for optimal performance
func createIndexes(db *Database) error {
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

	if _, err := db.Collections.Users.Indexes().CreateMany(ctx, userIndexes); err != nil {
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

	if _, err := db.Collections.Chats.Indexes().CreateMany(ctx, chatIndexes); err != nil {
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

	if _, err := db.Collections.Messages.Indexes().CreateMany(ctx, messageIndexes); err != nil {
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

	if _, err := db.Collections.Groups.Indexes().CreateMany(ctx, groupIndexes); err != nil {
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

	if _, err := db.Collections.Calls.Indexes().CreateMany(ctx, callIndexes); err != nil {
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

	if _, err := db.Collections.AdminConfigs.Indexes().CreateMany(ctx, adminConfigIndexes); err != nil {
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

	if _, err := db.Collections.Sessions.Indexes().CreateMany(ctx, sessionIndexes); err != nil {
		indexErrors = append(indexErrors, fmt.Errorf("sessions indexes: %w", err))
	}

	// Files collection indexes
	fileIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "uploaded_by", Value: 1}},
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

	if _, err := db.Collections.Files.Indexes().CreateMany(ctx, fileIndexes); err != nil {
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

	if _, err := db.Collections.Notifications.Indexes().CreateMany(ctx, notificationIndexes); err != nil {
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

	if _, err := db.Collections.Analytics.Indexes().CreateMany(ctx, analyticsIndexes); err != nil {
		indexErrors = append(indexErrors, fmt.Errorf("analytics indexes: %w", err))
	}

	if len(indexErrors) > 0 {
		return fmt.Errorf("failed to create some indexes: %v", indexErrors)
	}

	log.Println("All database indexes created successfully")
	return nil
}

// CreateTTLIndexes creates Time-To-Live indexes for automatic document expiration
func CreateTTLIndexes() error {
	if globalDB == nil {
		return fmt.Errorf("database not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// OTP cleanup - expire after 15 minutes
	otpTTLIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: "otp.created_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(900), // 15 minutes
	}

	if _, err := globalDB.Collections.Users.Indexes().CreateOne(ctx, otpTTLIndex); err != nil {
		return fmt.Errorf("failed to create OTP TTL index: %w", err)
	}

	// Session cleanup - expire based on expires_at field
	sessionTTLIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: "expires_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0),
	}

	if _, err := globalDB.Collections.Sessions.Indexes().CreateOne(ctx, sessionTTLIndex); err != nil {
		return fmt.Errorf("failed to create session TTL index: %w", err)
	}

	log.Println("TTL indexes created successfully")
	return nil
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

// Transaction executes a function within a MongoDB transaction
func Transaction(fn func(ctx mongo.SessionContext) error) error {
	if globalDB == nil {
		return fmt.Errorf("database not initialized")
	}

	session, err := globalDB.Client.StartSession()
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	defer session.EndSession(context.Background())

	_, err = session.WithTransaction(context.Background(), func(ctx mongo.SessionContext) (interface{}, error) {
		return nil, fn(ctx)
	})

	return err
}

// GetStats returns database statistics
func GetStats() (map[string]interface{}, error) {
	if globalDB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stats := make(map[string]interface{})

	// Get database stats
	var dbStats bson.M
	err := globalDB.DB.RunCommand(ctx, bson.D{{Key: "dbStats", Value: 1}}).Decode(&dbStats)
	if err != nil {
		return nil, fmt.Errorf("failed to get database stats: %w", err)
	}
	stats["database"] = dbStats

	// Get collection stats
	collections := map[string]*mongo.Collection{
		"users":         globalDB.Collections.Users,
		"chats":         globalDB.Collections.Chats,
		"messages":      globalDB.Collections.Messages,
		"groups":        globalDB.Collections.Groups,
		"calls":         globalDB.Collections.Calls,
		"admin_configs": globalDB.Collections.AdminConfigs,
		"sessions":      globalDB.Collections.Sessions,
		"files":         globalDB.Collections.Files,
		"notifications": globalDB.Collections.Notifications,
		"analytics":     globalDB.Collections.Analytics,
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
