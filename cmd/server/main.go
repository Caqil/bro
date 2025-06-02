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
	socketio "github.com/googollee/go-socket.io"
	"github.com/googollee/go-socket.io/engineio"
	"github.com/googollee/go-socket.io/engineio/transport"
	"github.com/googollee/go-socket.io/engineio/transport/polling"
	"github.com/googollee/go-socket.io/engineio/transport/websocket"
	"github.com/joho/godotenv"

	"chat-app/internal/config"
	"chat-app/internal/handlers"
	"chat-app/internal/middleware"
	"chat-app/internal/services"
	"chat-app/internal/webrtc"
	"chat-app/internal/websocket"
	"chat-app/pkg/database"
	"chat-app/pkg/logger"
	"chat-app/pkg/redis"
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
	db, err := database.Connect(cfg.MongoURI)
	if err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}

	// Initialize Redis
	redisClient := redis.NewClient(cfg.RedisURL)

	// Initialize services
	authService := services.NewAuthService(db, redisClient, cfg)
	smsService := services.NewSMSService(cfg)
	pushService := services.NewPushService(cfg)
	fileService := services.NewFileService(cfg)
	callService := services.NewCallService(db, cfg)

	// Initialize WebSocket hub
	hub := websocket.NewHub()
	go hub.Run()

	// Initialize WebRTC manager
	rtcManager := webrtc.NewManager(cfg.COTURNConfig)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authService, smsService)
	chatHandler := handlers.NewChatHandler(db, hub, pushService)
	messageHandler := handlers.NewMessageHandler(db, hub, pushService)
	groupHandler := handlers.NewGroupHandler(db, hub, pushService)
	callHandler := handlers.NewCallHandler(callService, rtcManager, hub)
	fileHandler := handlers.NewFileHandler(fileService)
	adminHandler := handlers.NewAdminHandler(db, cfg)

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
	server := setupSocketIO(hub, rtcManager, cfg)
	router.GET("/socket.io/*any", gin.WrapH(server))
	router.POST("/socket.io/*any", gin.WrapH(server))

	// API routes
	api := router.Group("/api")
	{
		// Public routes
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
			// User routes
			protected.GET("/profile", authHandler.GetProfile)
			protected.PUT("/profile", authHandler.UpdateProfile)
			protected.POST("/logout", authHandler.Logout)

			// Chat routes
			chats := protected.Group("/chats")
			{
				chats.GET("", chatHandler.GetChats)
				chats.POST("", chatHandler.CreateChat)
				chats.GET("/:id", chatHandler.GetChat)
				chats.PUT("/:id", chatHandler.UpdateChat)
				chats.DELETE("/:id", chatHandler.DeleteChat)
				chats.GET("/:id/messages", messageHandler.GetMessages)
				chats.POST("/:id/messages", messageHandler.SendMessage)
				chats.PUT("/:id/read", messageHandler.MarkAsRead)
			}

			// Message routes
			messages := protected.Group("/messages")
			{
				messages.PUT("/:id", messageHandler.UpdateMessage)
				messages.DELETE("/:id", messageHandler.DeleteMessage)
				messages.PUT("/:id/status", messageHandler.UpdateMessageStatus)
			}

			// Group routes
			groups := protected.Group("/groups")
			{
				groups.POST("", groupHandler.CreateGroup)
				groups.GET("/:id", groupHandler.GetGroup)
				groups.PUT("/:id", groupHandler.UpdateGroup)
				groups.DELETE("/:id", groupHandler.DeleteGroup)
				groups.POST("/:id/members", groupHandler.AddMember)
				groups.DELETE("/:id/members/:userId", groupHandler.RemoveMember)
				groups.PUT("/:id/members/:userId/role", groupHandler.UpdateMemberRole)
			}

			// Call routes
			calls := protected.Group("/calls")
			{
				calls.POST("/initiate", callHandler.InitiateCall)
				calls.POST("/:id/answer", callHandler.AnswerCall)
				calls.POST("/:id/end", callHandler.EndCall)
				calls.GET("/history", callHandler.GetCallHistory)
				calls.POST("/:id/record/start", callHandler.StartRecording)
				calls.POST("/:id/record/stop", callHandler.StopRecording)
			}

			// File routes
			files := protected.Group("/files")
			{
				files.POST("/upload", fileHandler.UploadFile)
				files.GET("/:id", fileHandler.DownloadFile)
				files.DELETE("/:id", fileHandler.DeleteFile)
				files.GET("/:id/thumbnail", fileHandler.GetThumbnail)
			}

			// Contact routes
			contacts := protected.Group("/contacts")
			{
				contacts.GET("", authHandler.GetContacts)
				contacts.POST("", authHandler.AddContact)
				contacts.DELETE("/:id", authHandler.RemoveContact)
				contacts.POST("/:id/block", authHandler.BlockContact)
				contacts.POST("/:id/unblock", authHandler.UnblockContact)
			}
		}

		// Admin routes
		admin := api.Group("/admin")
		admin.Use(middleware.AdminMiddleware(cfg.JWTSecret))
		{
			admin.GET("/users", adminHandler.GetUsers)
			admin.GET("/users/:id", adminHandler.GetUser)
			admin.PUT("/users/:id/status", adminHandler.UpdateUserStatus)
			admin.GET("/chats", adminHandler.GetChats)
			admin.GET("/analytics", adminHandler.GetAnalytics)
			admin.GET("/config", adminHandler.GetConfig)
			admin.PUT("/config", adminHandler.UpdateConfig)
			admin.GET("/logs", adminHandler.GetLogs)
			admin.POST("/broadcast", adminHandler.SendBroadcast)
		}
	}

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"timestamp": time.Now().Unix(),
			"version":   "1.0.0",
		})
	})

	// Static files
	router.Static("/static", "./web/static")
	router.Static("/uploads", "./web/static/uploads")

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

	log.Printf("Server started on port %s", cfg.Port)

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

func setupSocketIO(hub *websocket.Hub, rtcManager *webrtc.Manager, cfg *config.Config) *socketio.Server {
	server := socketio.NewServer(&engineio.Options{
		Transports: []transport.Transport{
			&polling.Transport{
				CheckOrigin: func(r *http.Request) bool {
					return true
				},
			},
			&websocket.Transport{
				CheckOrigin: func(r *http.Request) bool {
					return true
				},
			},
		},
	})

	server.OnConnect("/", func(s socketio.Conn) error {
		log.Printf("Socket connected: %s", s.ID())
		return nil
	})

	server.OnError("/", func(s socketio.Conn, e error) {
		log.Printf("Socket error: %v", e)
	})

	server.OnDisconnect("/", func(s socketio.Conn, reason string) {
		log.Printf("Socket disconnected: %s, reason: %s", s.ID(), reason)
		hub.UnregisterClient(s.ID())
	})

	// Chat events
	server.OnEvent("/", "join_chat", func(s socketio.Conn, data map[string]interface{}) {
		hub.JoinChat(s.ID(), data)
	})

	server.OnEvent("/", "leave_chat", func(s socketio.Conn, data map[string]interface{}) {
		hub.LeaveChat(s.ID(), data)
	})

	server.OnEvent("/", "send_message", func(s socketio.Conn, data map[string]interface{}) {
		hub.BroadcastMessage(s.ID(), data)
	})

	server.OnEvent("/", "typing_start", func(s socketio.Conn, data map[string]interface{}) {
		hub.BroadcastTyping(s.ID(), data, true)
	})

	server.OnEvent("/", "typing_stop", func(s socketio.Conn, data map[string]interface{}) {
		hub.BroadcastTyping(s.ID(), data, false)
	})

	// WebRTC signaling events
	server.OnEvent("/", "call_offer", func(s socketio.Conn, data map[string]interface{}) {
		rtcManager.HandleOffer(s.ID(), data)
	})

	server.OnEvent("/", "call_answer", func(s socketio.Conn, data map[string]interface{}) {
		rtcManager.HandleAnswer(s.ID(), data)
	})

	server.OnEvent("/", "ice_candidate", func(s socketio.Conn, data map[string]interface{}) {
		rtcManager.HandleICECandidate(s.ID(), data)
	})

	server.OnEvent("/", "call_end", func(s socketio.Conn, data map[string]interface{}) {
		rtcManager.HandleCallEnd(s.ID(), data)
	})

	// Video/Audio control events
	server.OnEvent("/", "toggle_video", func(s socketio.Conn, data map[string]interface{}) {
		rtcManager.ToggleVideo(s.ID(), data)
	})

	server.OnEvent("/", "toggle_audio", func(s socketio.Conn, data map[string]interface{}) {
		rtcManager.ToggleAudio(s.ID(), data)
	})

	server.OnEvent("/", "screen_share_start", func(s socketio.Conn, data map[string]interface{}) {
		rtcManager.StartScreenShare(s.ID(), data)
	})

	server.OnEvent("/", "screen_share_stop", func(s socketio.Conn, data map[string]interface{}) {
		rtcManager.StopScreenShare(s.ID(), data)
	})

	return server
}
