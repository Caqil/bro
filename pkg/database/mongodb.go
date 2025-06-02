package database

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

// Database represents the MongoDB database connection
type Database struct {
	Client    *mongo.Client
	DB        *mongo.Database
	Context   context.Context
	Cancel    context.CancelFunc
	Connected bool
}

// Collections represents all database collections
type Collections struct {
	Users         *mongo.Collection
	Chats         *mongo.Collection
	Messages      *mongo.Collection
	Groups        *mongo.Collection
	Calls         *mongo.Collection
	Files         *mongo.Collection
	AdminConfigs  *mongo.Collection
	Sessions      *mongo.Collection
	OTPCodes      *mongo.Collection
	Notifications *mongo.Collection
	Reports       *mongo.Collection
	Analytics     *mongo.Collection
}

var (
	// Global database instance
	DB *Database
	// Global collections
	Collections *Collections
)

// ConnectionConfig represents MongoDB connection configuration
type ConnectionConfig struct {
	URI                    string
	DatabaseName           string
	MaxPoolSize            uint64
	MinPoolSize            uint64
	MaxConnIdleTime        time.Duration
	ConnectTimeout         time.Duration
	ServerSelectionTimeout time.Duration
	HeartbeatInterval      time.Duration
	LocalThreshold         time.Duration
}

// DefaultConfig returns default MongoDB configuration
func DefaultConfig(uri string) *ConnectionConfig {
	return &ConnectionConfig{
		URI:                    uri,
		DatabaseName:           "chatapp",
		MaxPoolSize:            100,
		MinPoolSize:            5,
		MaxConnIdleTime:        30 * time.Minute,
		ConnectTimeout:         10 * time.Second,
		ServerSelectionTimeout: 5 * time.Second,
		HeartbeatInterval:      10 * time.Second,
		LocalThreshold:         15 * time.Millisecond,
	}
}

// Connect establishes connection to MongoDB
func Connect(uri string) (*Database, error) {
	config := DefaultConfig(uri)
	return ConnectWithConfig(config)
}

// ConnectWithConfig establishes connection to MongoDB with custom config
func ConnectWithConfig(config *ConnectionConfig) (*Database, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), config.ConnectTimeout)

	// Set client options
	clientOptions := options.Client().
		ApplyURI(config.URI).
		SetMaxPoolSize(config.MaxPoolSize).
		SetMinPoolSize(config.MinPoolSize).
		SetMaxConnIdleTime(config.MaxConnIdleTime).
		SetServerSelectionTimeout(config.ServerSelectionTimeout).
		SetHeartbeatInterval(config.HeartbeatInterval).
		SetLocalThreshold(config.LocalThreshold)

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Test the connection
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		cancel()
		client.Disconnect(ctx)
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	// Get database
	database := client.Database(config.DatabaseName)

	// Create database instance
	db := &Database{
		Client:    client,
		DB:        database,
		Context:   ctx,
		Cancel:    cancel,
		Connected: true,
	}

	// Set global database instance
	DB = db

	// Initialize collections
	if err := initializeCollections(db); err != nil {
		cancel()
		client.Disconnect(ctx)
		return nil, fmt.Errorf("failed to initialize collections: %w", err)
	}

	log.Printf("Successfully connected to MongoDB: %s", config.DatabaseName)

	// Create indexes
	go func() {
		if err := createIndexes(db); err != nil {
			log.Printf("Warning: Failed to create some indexes: %v", err)
		}
	}()

	return db, nil
}

// initializeCollections initializes all database collections
func initializeCollections(db *Database) error {
	Collections = &Collections{
		Users:         db.DB.Collection("users"),
		Chats:         db.DB.Collection("chats"),
		Messages:      db.DB.Collection("messages"),
		Groups:        db.DB.Collection("groups"),
		Calls:         db.DB.Collection("calls"),
		Files:         db.DB.Collection("files"),
		AdminConfigs:  db.DB.Collection("admin_configs"),
		Sessions:      db.DB.Collection("sessions"),
		OTPCodes:      db.DB.Collection("otp_codes"),
		Notifications: db.DB.Collection("notifications"),
		Reports:       db.DB.Collection("reports"),
		Analytics:     db.DB.Collection("analytics"),
	}

	log.Println("Database collections initialized successfully")
	return nil
}

// createIndexes creates necessary database indexes
func createIndexes(db *Database) error {
	ctx := context.Background()

	// User indexes
	userIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "phone_number", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "full_phone_number", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetUnique(true).SetSparse(true),
		},
		{
			Keys:    bson.D{{Key: "username", Value: 1}},
			Options: options.Index().SetUnique(true).SetSparse(true),
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "is_active", Value: 1}},
		},
	}

	if _, err := Collections.Users.Indexes().CreateMany(ctx, userIndexes); err != nil {
		log.Printf("Error creating user indexes: %v", err)
	}

	// Chat indexes
	chatIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "participants", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "type", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "last_activity", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "is_active", Value: 1}},
		},
	}

	if _, err := Collections.Chats.Indexes().CreateMany(ctx, chatIndexes); err != nil {
		log.Printf("Error creating chat indexes: %v", err)
	}

	// Message indexes
	messageIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "chat_id", Value: 1}, {Key: "created_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "sender_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "type", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "mentions", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "is_deleted", Value: 1}},
		},
		{
			Keys:    bson.D{{Key: "reply_to_id", Value: 1}},
			Options: options.Index().SetSparse(true),
		},
	}

	if _, err := Collections.Messages.Indexes().CreateMany(ctx, messageIndexes); err != nil {
		log.Printf("Error creating message indexes: %v", err)
	}

	// Group indexes
	groupIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "chat_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "invite_code", Value: 1}},
			Options: options.Index().SetUnique(true).SetSparse(true),
		},
		{
			Keys: bson.D{{Key: "members.user_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "is_active", Value: 1}},
		},
	}

	if _, err := Collections.Groups.Indexes().CreateMany(ctx, groupIndexes); err != nil {
		log.Printf("Error creating group indexes: %v", err)
	}

	// Call indexes
	callIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "participants.user_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "initiator_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "status", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "initiated_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "chat_id", Value: 1}},
		},
		{
			Keys:    bson.D{{Key: "session_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	}

	if _, err := Collections.Calls.Indexes().CreateMany(ctx, callIndexes); err != nil {
		log.Printf("Error creating call indexes: %v", err)
	}

	// File indexes
	fileIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "uploaded_by", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "chat_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "message_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "file_type", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "is_deleted", Value: 1}},
		},
	}

	if _, err := Collections.Files.Indexes().CreateMany(ctx, fileIndexes); err != nil {
		log.Printf("Error creating file indexes: %v", err)
	}

	// Session indexes
	sessionIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "user_id", Value: 1}},
		},
		{
			Keys:    bson.D{{Key: "token_hash", Value: 1}},
			Options: options.Index().SetUnique(true),
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

	if _, err := Collections.Sessions.Indexes().CreateMany(ctx, sessionIndexes); err != nil {
		log.Printf("Error creating session indexes: %v", err)
	}

	// OTP indexes
	otpIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "phone_number", Value: 1}},
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0),
		},
		{
			Keys: bson.D{{Key: "is_used", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
	}

	if _, err := Collections.OTPCodes.Indexes().CreateMany(ctx, otpIndexes); err != nil {
		log.Printf("Error creating OTP indexes: %v", err)
	}

	// Analytics indexes
	analyticsIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "date", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "metric_type", Value: 1}},
		},
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}},
			Options: options.Index().SetSparse(true),
		},
	}

	if _, err := Collections.Analytics.Indexes().CreateMany(ctx, analyticsIndexes); err != nil {
		log.Printf("Error creating analytics indexes: %v", err)
	}

	log.Println("Database indexes created successfully")
	return nil
}

// Disconnect closes the database connection
func (db *Database) Disconnect() error {
	if !db.Connected {
		return nil
	}

	db.Cancel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.Client.Disconnect(ctx); err != nil {
		return fmt.Errorf("failed to disconnect from MongoDB: %w", err)
	}

	db.Connected = false
	log.Println("Disconnected from MongoDB")
	return nil
}

// Ping tests the database connection
func (db *Database) Ping() error {
	if !db.Connected {
		return fmt.Errorf("database not connected")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.Client.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	return nil
}

// GetStats returns database statistics
func (db *Database) GetStats() (map[string]interface{}, error) {
	if !db.Connected {
		return nil, fmt.Errorf("database not connected")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stats := make(map[string]interface{})

	// Get database stats
	result := db.DB.RunCommand(ctx, bson.D{{Key: "dbStats", Value: 1}})
	var dbStats bson.M
	if err := result.Decode(&dbStats); err == nil {
		stats["database"] = dbStats
	}

	// Get collection stats
	collections := map[string]*mongo.Collection{
		"users":         Collections.Users,
		"chats":         Collections.Chats,
		"messages":      Collections.Messages,
		"groups":        Collections.Groups,
		"calls":         Collections.Calls,
		"files":         Collections.Files,
		"admin_configs": Collections.AdminConfigs,
		"sessions":      Collections.Sessions,
		"otp_codes":     Collections.OTPCodes,
		"notifications": Collections.Notifications,
		"reports":       Collections.Reports,
		"analytics":     Collections.Analytics,
	}

	collectionStats := make(map[string]interface{})
	for name, collection := range collections {
		count, err := collection.CountDocuments(ctx, bson.M{})
		if err == nil {
			collectionStats[name] = map[string]interface{}{
				"count": count,
			}
		}
	}
	stats["collections"] = collectionStats

	return stats, nil
}

// CreateBackup creates a backup of specific collections
func (db *Database) CreateBackup(collections []string) error {
	if !db.Connected {
		return fmt.Errorf("database not connected")
	}

	// This is a basic implementation
	// In production, you might want to use mongodump or a more sophisticated backup solution
	log.Printf("Creating backup for collections: %v", collections)

	// Implementation would depend on your backup strategy
	// For now, just log the action

	return nil
}

// Transaction executes a function within a MongoDB transaction
func (db *Database) Transaction(fn func(sessCtx mongo.SessionContext) error) error {
	if !db.Connected {
		return fmt.Errorf("database not connected")
	}

	session, err := db.Client.StartSession()
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	defer session.EndSession(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (interface{}, error) {
		return nil, fn(sessCtx)
	})

	if err != nil {
		return fmt.Errorf("transaction failed: %w", err)
	}

	return nil
}

// Health represents database health status
type Health struct {
	Status      string                 `json:"status"`
	Connected   bool                   `json:"connected"`
	Latency     time.Duration          `json:"latency"`
	Collections map[string]interface{} `json:"collections"`
	Error       string                 `json:"error,omitempty"`
}

// HealthCheck performs a comprehensive health check
func (db *Database) HealthCheck() *Health {
	health := &Health{
		Status:      "unhealthy",
		Connected:   db.Connected,
		Collections: make(map[string]interface{}),
	}

	if !db.Connected {
		health.Error = "database not connected"
		return health
	}

	// Measure ping latency
	start := time.Now()
	if err := db.Ping(); err != nil {
		health.Error = err.Error()
		return health
	}
	health.Latency = time.Since(start)

	// Check collections
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	collections := map[string]*mongo.Collection{
		"users":    Collections.Users,
		"chats":    Collections.Chats,
		"messages": Collections.Messages,
		"groups":   Collections.Groups,
		"calls":    Collections.Calls,
	}

	for name, collection := range collections {
		count, err := collection.CountDocuments(ctx, bson.M{})
		if err != nil {
			health.Collections[name] = map[string]interface{}{
				"status": "error",
				"error":  err.Error(),
			}
		} else {
			health.Collections[name] = map[string]interface{}{
				"status": "healthy",
				"count":  count,
			}
		}
	}

	health.Status = "healthy"
	return health
}

// Migration represents a database migration
type Migration struct {
	Version     string
	Description string
	Up          func(*Database) error
	Down        func(*Database) error
}

// RunMigrations runs pending database migrations
func (db *Database) RunMigrations(migrations []Migration) error {
	if !db.Connected {
		return fmt.Errorf("database not connected")
	}

	// Create migrations collection if it doesn't exist
	migrationsCol := db.DB.Collection("migrations")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, migration := range migrations {
		// Check if migration has already been run
		count, err := migrationsCol.CountDocuments(ctx, bson.M{"version": migration.Version})
		if err != nil {
			return fmt.Errorf("failed to check migration status: %w", err)
		}

		if count > 0 {
			log.Printf("Migration %s already applied, skipping", migration.Version)
			continue
		}

		log.Printf("Running migration %s: %s", migration.Version, migration.Description)

		// Run migration
		if err := migration.Up(db); err != nil {
			return fmt.Errorf("migration %s failed: %w", migration.Version, err)
		}

		// Record migration
		_, err = migrationsCol.InsertOne(ctx, bson.M{
			"version":     migration.Version,
			"description": migration.Description,
			"applied_at":  time.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to record migration: %w", err)
		}

		log.Printf("Migration %s completed successfully", migration.Version)
	}

	return nil
}

// GetConnection returns the global database connection
func GetConnection() *Database {
	return DB
}

// GetCollections returns the global collections
func GetCollections() *Collections {
	return Collections
}

// IsConnected returns true if database is connected
func IsConnected() bool {
	return DB != nil && DB.Connected
}
