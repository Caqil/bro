package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/mongo"

	"bro/internal/config"
	"bro/internal/handlers"
	"bro/internal/middleware"
	"bro/internal/migrations"
	"bro/internal/services"
	"bro/internal/webrtc"
	"bro/internal/websocket"
	"bro/pkg/database"
	"bro/pkg/logger"
	"bro/pkg/redis"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Initialize logger
	logger.Init()

	// Load configuration
	cfg := config.Load()

	// Initialize database
	_, err := database.Connect(cfg.MongoURI)
	if err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}

	// Get MongoDB database instance for services that need it
	mongoDB := database.GetDB()

	// Run database migrations and seeding
	if err := runDatabaseSetup(mongoDB, cfg.Production); err != nil {
		log.Fatal("Failed to setup database:", err)
	}

	// Initialize Redis
	redisClient := redis.NewClient(cfg.RedisURL)

	// Initialize WebSocket hub with correct parameters
	hubConfig := websocket.DefaultHubConfig()
	hub := websocket.NewHub(redisClient, cfg.JWTSecret, hubConfig)
	go hub.Run()

	// Initialize services in correct dependency order
	authService := services.NewAuthService(mongoDB, redisClient, cfg)
	smsService := services.NewSMSService(cfg)
	pushService := services.NewPushService(cfg)
	fileService := services.NewFileService(cfg)

	// Initialize WebRTC signaling server first (CallService depends on it)
	signalingServer, err := webrtc.NewSignalingServer(cfg, hub, nil, pushService)
	if err != nil {
		log.Fatal("Failed to initialize WebRTC signaling server:", err)
	}

	// Initialize call service with correct parameters and error handling
	callService, err := services.NewCallService(cfg, signalingServer, hub, pushService, smsService)
	if err != nil {
		log.Fatal("Failed to initialize call service:", err)
	}

	// Initialize handlers with correct constructors
	authHandler := handlers.NewAuthHandler(authService)
	groupHandler := handlers.NewGroupHandler()
	callHandler := handlers.NewCallHandler(callService)
	fileHandler := handlers.NewFileHandler(fileService)
	adminHandler := handlers.NewAdminHandler(
		authService,
		fileService,
		callService,
		pushService,
		smsService,
	)

	// Note: ChatHandler and MessageHandler need ChatService and MessageService
	// which don't exist yet, so we'll comment them out for now
	// chatHandler := handlers.NewChatHandler(chatService)
	// messageHandler := handlers.NewMessageHandler(messageService)

	// Setup Gin router
	if cfg.Production {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// CORS middleware
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Rate limiting middleware
	router.Use(middleware.RateLimit())

	// Setup Socket.IO server
	//server := setupSocketIO(hub, signalingServer, cfg)
	// router.GET("/socket.io/*any", gin.WrapH(server))
	// router.POST("/socket.io/*any", gin.WrapH(server))

	// API routes
	api := router.Group("/api")
	{
		// Public authentication routes
		auth := api.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/verify", authHandler.VerifyOTP)
			auth.POST("/login", authHandler.Login)
			auth.POST("/refresh", authHandler.RefreshToken)
			auth.POST("/resend-otp", authHandler.ResendOTP)
		}

		// Protected routes
		protected := api.Group("/")
		protected.Use(middleware.AuthMiddleware(cfg.JWTSecret))
		{
			// User profile routes
			protected.GET("/profile", authHandler.GetProfile)
			protected.PUT("/profile", authHandler.UpdateProfile)
			protected.PUT("/change-password", authHandler.ChangePassword)
			protected.POST("/logout", authHandler.Logout)
			protected.GET("/validate", authHandler.ValidateToken)

			// Contact routes

			// Group routes
			groups := protected.Group("/groups")
			{
				groups.POST("", groupHandler.CreateGroup)
				groups.GET("/:groupId", groupHandler.GetGroup)
				groups.PUT("/:groupId", groupHandler.UpdateGroup)
				groups.DELETE("/:groupId", groupHandler.DeleteGroup)
				groups.GET("/:groupId/members", groupHandler.GetGroupMembers)
				groups.POST("/:groupId/members", groupHandler.AddMember)
				groups.PUT("/:groupId/members/:userId", groupHandler.UpdateMemberRole)
				groups.DELETE("/:groupId/members/:userId", groupHandler.RemoveMember)
				groups.POST("/:groupId/leave", groupHandler.LeaveGroup)
				groups.GET("/my", groupHandler.GetMyGroups)

				// Member management actions
				groups.POST("/:groupId/members/:userId/mute", groupHandler.MuteMember)
				groups.DELETE("/:groupId/members/:userId/mute", groupHandler.UnmuteMember)
				groups.POST("/:groupId/members/:userId/warn", groupHandler.WarnMember)
				groups.POST("/:groupId/members/:userId/ban", groupHandler.BanMember)
				groups.DELETE("/:groupId/members/:userId/ban", groupHandler.UnbanMember)

				// Join requests
				groups.GET("/:groupId/requests", groupHandler.GetJoinRequests)
				groups.POST("/:groupId/join", groupHandler.RequestToJoin)
				groups.POST("/:groupId/requests/:userId/approve", groupHandler.ApproveJoinRequest)
				groups.POST("/:groupId/requests/:userId/reject", groupHandler.RejectJoinRequest)

				// Invitations
				groups.POST("/:groupId/invite", groupHandler.InviteUsers)
				groups.GET("/:groupId/invites", groupHandler.GetPendingInvites)
				groups.POST("/invites/:inviteId/accept", groupHandler.AcceptInvite)
				groups.POST("/invites/:inviteId/decline", groupHandler.DeclineInvite)

				// Group content
				groups.GET("/:groupId/announcements", groupHandler.GetAnnouncements)
				groups.POST("/:groupId/announcements", groupHandler.CreateAnnouncement)
				groups.PUT("/:groupId/announcements/:announcementId", groupHandler.UpdateAnnouncement)
				groups.DELETE("/:groupId/announcements/:announcementId", groupHandler.DeleteAnnouncement)

				groups.GET("/:groupId/rules", groupHandler.GetGroupRules)
				groups.POST("/:groupId/rules", groupHandler.CreateGroupRule)
				groups.PUT("/:groupId/rules/:ruleId", groupHandler.UpdateGroupRule)
				groups.DELETE("/:groupId/rules/:ruleId", groupHandler.DeleteGroupRule)

				groups.GET("/:groupId/events", groupHandler.GetGroupEvents)
				groups.POST("/:groupId/events", groupHandler.CreateGroupEvent)
				groups.PUT("/:groupId/events/:eventId", groupHandler.UpdateGroupEvent)
				groups.DELETE("/:groupId/events/:eventId", groupHandler.DeleteGroupEvent)
				groups.POST("/:groupId/events/:eventId/attend", groupHandler.AttendEvent)

				// Group discovery
				groups.GET("/search", groupHandler.SearchGroups)
				groups.GET("/public", groupHandler.GetPublicGroups)

				// Group statistics
				groups.GET("/:groupId/stats", groupHandler.GetGroupStats)

				// Group settings
				groups.GET("/:groupId/settings", groupHandler.GetGroupSettings)
				groups.PUT("/:groupId/settings", groupHandler.UpdateGroupSettings)

				// Invite links
				groups.POST("/:groupId/invite-link", groupHandler.GenerateInviteLink)
				groups.GET("/join/:inviteCode", groupHandler.JoinByInviteCode)
			}

			// Call routes
			calls := protected.Group("/calls")
			{
				calls.POST("/initiate", callHandler.InitiateCall)
				calls.POST("/:id/answer", callHandler.AnswerCall)
				calls.POST("/:id/end", callHandler.EndCall)
				calls.POST("/:id/join", callHandler.JoinCall)
				calls.POST("/:id/leave", callHandler.LeaveCall)
				calls.GET("/:id", callHandler.GetCall)
				calls.GET("/history", callHandler.GetCallHistory)
				calls.PUT("/:id/media", callHandler.UpdateMediaState)
				calls.PUT("/:id/quality", callHandler.UpdateQualityMetrics)
				calls.POST("/:id/recording/start", callHandler.StartRecording)
				calls.POST("/:id/recording/stop", callHandler.StopRecording)
			}

			// File routes
			files := protected.Group("/files")
			{
				files.POST("/upload", fileHandler.UploadFile)
				files.GET("/:fileId", fileHandler.DownloadFile)
				files.GET("/:fileId/download", fileHandler.DownloadFile)
				files.GET("/:fileId/info", fileHandler.GetFileInfo)
				files.GET("/:fileId/thumbnail", fileHandler.GetThumbnail)
				files.DELETE("/:fileId", fileHandler.DeleteFile)
				files.GET("/", fileHandler.GetUserFiles)
				files.GET("/search", fileHandler.SearchFiles)
				files.GET("/chat/:chatId", fileHandler.GetChatFiles)
				files.GET("/stats", fileHandler.GetFileStats)
			}

			// Device management routes
			devices := protected.Group("/devices")
			{
				devices.GET("", authHandler.GetDevices)
			}

			// TODO: Add chat and message routes when services are implemented
			// chats := protected.Group("/chats")
			// messages := protected.Group("/messages")
		}

		// Admin routes
		admin := api.Group("/admin")
		admin.Use(middleware.AdminMiddleware(cfg.JWTSecret))
		{
			// Dashboard and analytics
			admin.GET("/dashboard", adminHandler.GetDashboard)
			admin.GET("/analytics", adminHandler.GetAnalytics)
			admin.GET("/stats", adminHandler.GetSystemStats)

			// User management
			users := admin.Group("/users")
			{
				users.GET("/", adminHandler.GetUsers)
				users.GET("/:userId", adminHandler.GetUser)
				users.PUT("/:userId", adminHandler.UpdateUser)
				users.DELETE("/:userId", adminHandler.DeleteUser)
				users.POST("/:userId/ban", adminHandler.BanUser)
				users.DELETE("/:userId/ban", adminHandler.UnbanUser)
				users.POST("/:userId/suspend", adminHandler.SuspendUser)
				users.DELETE("/:userId/suspend", adminHandler.UnsuspendUser)
				users.PUT("/:userId/role", adminHandler.UpdateUserRole)
				users.GET("/:userId/activity", adminHandler.GetUserActivity)
				users.POST("/:userId/reset-password", adminHandler.ResetUserPassword)
				users.POST("/:userId/verify", adminHandler.VerifyUser)
			}

			// Content moderation
			moderation := admin.Group("/moderation")
			{
				moderation.GET("/reports", adminHandler.GetReports)
				moderation.GET("/reports/:reportId", adminHandler.GetReport)
				moderation.PUT("/reports/:reportId", adminHandler.UpdateReport)
				moderation.POST("/reports/:reportId/action", adminHandler.TakeAction)
				moderation.GET("/messages", adminHandler.GetReportedMessages)
				moderation.DELETE("/messages/:messageId", adminHandler.DeleteMessage)
				moderation.PUT("/messages/:messageId/flag", adminHandler.FlagMessage)
				moderation.GET("/groups", adminHandler.GetReportedGroups)
				moderation.DELETE("/groups/:groupId", adminHandler.DeleteGroup)
				moderation.PUT("/groups/:groupId/suspend", adminHandler.SuspendGroup)
			}

			// System management
			system := admin.Group("/system")
			{
				system.GET("/health", adminHandler.GetSystemHealth)
				system.GET("/logs", adminHandler.GetSystemLogs)
				system.POST("/maintenance", adminHandler.SetMaintenanceMode)
				system.POST("/broadcast", adminHandler.BroadcastMessage)
				system.POST("/cleanup", adminHandler.RunCleanup)
				system.GET("/config", adminHandler.GetSystemConfig)
				system.PUT("/config", adminHandler.UpdateSystemConfig)
			}

			// File management
			adminFiles := admin.Group("/files")
			{
				adminFiles.GET("/", adminHandler.GetFiles)
				adminFiles.DELETE("/:fileId", adminHandler.DeleteFile)
				adminFiles.GET("/stats", adminHandler.GetFileStats)
				adminFiles.POST("/cleanup", adminHandler.CleanupFiles)
				adminFiles.GET("/storage", adminHandler.GetStorageStats)
			}

			// Call management
			adminCalls := admin.Group("/calls")
			{
				adminCalls.GET("/", adminHandler.GetCalls)
				adminCalls.GET("/active", adminHandler.GetActiveCalls)
				adminCalls.POST("/:callId/end", adminHandler.EndCall)
				adminCalls.GET("/stats", adminHandler.GetCallStats)
			}

			// Security
			security := admin.Group("/security")
			{
				security.GET("/sessions", adminHandler.GetActiveSessions)
				security.DELETE("/sessions/:sessionId", adminHandler.TerminateSession)
				security.GET("/failed-logins", adminHandler.GetFailedLogins)
				security.POST("/ip-ban", adminHandler.BanIP)
				security.DELETE("/ip-ban/:ip", adminHandler.UnbanIP)
			}
		}

		// Call statistics endpoint (for admin)
		api.GET("/stats/calls", middleware.AdminMiddleware(cfg.JWTSecret), func(c *gin.Context) {
			stats := callService.GetCallStatistics()
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data":    stats,
			})
		})

		// Migration status endpoint (admin only)
		api.GET("/admin/migrations/status", middleware.AdminMiddleware(cfg.JWTSecret), func(c *gin.Context) {
			migrationService := migrations.NewMigrationService(mongoDB)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			status, err := migrationService.GetMigrationStatus(ctx)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"error":   err.Error(),
				})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data":    status,
			})
		})
	}

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"timestamp": time.Now().Unix(),
			"version":   "1.0.0",
			"services": gin.H{
				"database":  "connected",
				"redis":     "connected",
				"websocket": "running",
				"webrtc":    "running",
			},
		})
	})

	// Static files
	router.Static("/static", "./web/static")
	router.Static("/uploads", "./web/static/uploads")

	// Admin interface
	router.LoadHTMLFiles("web/admin/index.html")
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{
			"title": "BRO Chat Admin",
		})
	})

	// Start server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Graceful shutdown
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	log.Printf("🚀 BRO Chat Server started on port %s", cfg.Port)
	log.Printf("📚 API Documentation: http://localhost:%s/health", cfg.Port)
	log.Printf("👤 Admin Panel: http://localhost:%s/", cfg.Port)

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exited")
}

// runDatabaseSetup handles database migrations and seeding
func runDatabaseSetup(db *mongo.Database, isProduction bool) error {
	log.Println("🔧 Setting up database...")

	// Initialize migration service
	migrationService := migrations.NewMigrationService(db)

	// Run migrations
	if err := migrationService.RunMigrations(); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	// Only seed data in development mode
	if !isProduction {
		log.Println("🌱 Seeding development data...")

		seederService := migrations.NewSeederService(db)
		if err := seederService.SeedData(); err != nil {
			log.Printf("⚠️  Warning: Failed to seed data: %v", err)
			// Don't fail startup if seeding fails, it's optional for development
		} else {
			log.Println("✅ Development data seeded successfully")
			log.Println("📋 Test Users Created:")
			log.Println("   👤 Admin: +11234567890 (password: password123)")
			log.Println("   👤 User: +10987654321 (password: password123)")
			log.Println("   👤 Moderator: +15555551234 (password: password123)")
			log.Println("   👤 User: +17777777777 (password: password123)")
			log.Println("   👤 User: +19999999999 (password: password123)")
		}
	} else {
		log.Println("🏭 Production mode: Skipping data seeding")
	}

	log.Println("✅ Database setup completed")
	return nil
}
