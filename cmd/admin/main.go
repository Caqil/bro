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
	"go.mongodb.org/mongo-driver/bson/primitive"

	"bro/internal/config"
	"bro/internal/handlers"
	"bro/internal/middleware"
	"bro/internal/models"
	"bro/internal/services"
	"bro/pkg/database"
	"bro/pkg/logger"
	"bro/pkg/redis"
)

// AdminServerConfig holds configuration for the admin server
type AdminServerConfig struct {
	Port         string
	AllowedHosts []string
	JWTSecret    string
}

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Initialize logger
	logger.Init()

	// Load configuration
	cfg := config.Load()

	// Admin server specific configuration
	adminConfig := &AdminServerConfig{
		Port:         getEnv("ADMIN_PORT", "9090"),
		AllowedHosts: []string{"localhost", "127.0.0.1"},
		JWTSecret:    cfg.JWTSecret,
	}

	// Initialize database connection
	_, err := database.Connect()
	if err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}

	// Initialize Redis
	redisClient := redis.NewClient(cfg.RedisURL)
	if redisClient == nil {
		log.Println("Warning: Redis connection failed, some features may not work")
	}

	// Get MongoDB database instance
	mongoDB := database.GetDB()

	// Initialize services
	authService := services.NewAuthService(mongoDB, redisClient, cfg)
	smsService := services.NewSMSService(cfg)
	pushService := services.NewPushService(cfg)
	fileService := services.NewFileService(cfg)

	// Initialize call service (if needed for admin)
	var callService *services.CallService
	// Note: CallService requires WebRTC signaling server which might not be needed for admin
	// callService, _ = services.NewCallService(cfg, nil, nil, pushService, smsService)

	// Initialize admin handler
	adminHandler := handlers.NewAdminHandler(
		authService,
		fileService,
		callService,
		pushService,
		smsService,
	)

	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	// Middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// CORS middleware for admin interface
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:9090", "http://127.0.0.1:9090"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Rate limiting middleware
	router.Use(middleware.RateLimit())

	// Security middleware for admin interface
	router.Use(func(c *gin.Context) {
		// Add security headers
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com; style-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com; font-src 'self' https://cdnjs.cloudflare.com; img-src 'self' data: https:; connect-src 'self'")
		c.Next()
	})

	// Static files for admin interface
	router.Static("/css", "./web/admin/css")
	router.Static("/js", "./web/admin/js")
	router.Static("/assets", "./web/admin/assets")
	router.Static("/uploads", "./web/static/uploads")

	// Admin authentication routes (separate from main API)
	auth := router.Group("/auth")
	{
		auth.POST("/login", adminLogin(authService))
		auth.POST("/refresh", adminRefreshToken(authService))
		auth.POST("/logout", middleware.AuthMiddleware(cfg.JWTSecret), adminLogout(authService))
		auth.GET("/validate", middleware.AuthMiddleware(cfg.JWTSecret), adminValidateToken)
	}

	// Admin API routes
	api := router.Group("/api")
	{
		// All admin API routes require authentication and admin role
		adminAPI := api.Group("/admin")
		adminAPI.Use(middleware.AdminMiddleware(cfg.JWTSecret))
		{
			// Dashboard and analytics
			adminAPI.GET("/dashboard", adminHandler.GetDashboard)
			adminAPI.GET("/analytics", adminHandler.GetAnalytics)
			adminAPI.GET("/stats", adminHandler.GetSystemStats)

			// User management
			users := adminAPI.Group("/users")
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
			moderation := adminAPI.Group("/moderation")
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
			system := adminAPI.Group("/system")
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
			files := adminAPI.Group("/files")
			{
				files.GET("/", adminHandler.GetFiles)
				files.DELETE("/:fileId", adminHandler.DeleteFile)
				files.GET("/stats", adminHandler.GetFileStats)
				files.POST("/cleanup", adminHandler.CleanupFiles)
				files.GET("/storage", adminHandler.GetStorageStats)
			}

			// Call management
			calls := adminAPI.Group("/calls")
			{
				calls.GET("/", adminHandler.GetCalls)
				calls.GET("/active", adminHandler.GetActiveCalls)
				calls.POST("/:callId/end", adminHandler.EndCall)
				calls.GET("/stats", adminHandler.GetCallStats)
			}

			// Security
			security := adminAPI.Group("/security")
			{
				security.GET("/sessions", adminHandler.GetActiveSessions)
				security.DELETE("/sessions/:sessionId", adminHandler.TerminateSession)
				security.GET("/failed-logins", adminHandler.GetFailedLogins)
				security.POST("/ip-ban", adminHandler.BanIP)
				security.DELETE("/ip-ban/:ip", adminHandler.UnbanIP)
			}
		}
	}

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"timestamp": time.Now().Unix(),
			"version":   "1.0.0",
			"service":   "admin",
		})
	})

	// Main admin interface route
	router.LoadHTMLFiles("web/admin/index.html")
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{
			"title": "BRO Chat Admin Dashboard",
		})
	})

	// Admin login page (if you want a separate login page)
	router.GET("/login", func(c *gin.Context) {
		c.HTML(http.StatusOK, "login.html", gin.H{
			"title": "Admin Login",
		})
	})

	// Start server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", adminConfig.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Graceful shutdown
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start admin server: %v", err)
		}
	}()

	log.Printf("🚀 BRO Chat Admin Server started on port %s", adminConfig.Port)
	log.Printf("👤 Admin Dashboard: http://localhost:%s/", adminConfig.Port)
	log.Printf("📊 Health Check: http://localhost:%s/health", adminConfig.Port)

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down admin server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Admin server forced to shutdown:", err)
	}

	log.Println("Admin server exited")
}

// Admin-specific authentication handlers

func adminLogin(authService *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			PhoneNumber string `json:"phone_number" binding:"required"`
			CountryCode string `json:"country_code" binding:"required"`
			Password    string `json:"password" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request format",
			})
			return
		}

		loginReq := &models.UserLoginRequest{
			PhoneNumber: req.PhoneNumber,
			CountryCode: req.CountryCode,
			Password:    req.Password,
		}

		result, err := authService.LoginWithPhone(loginReq)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid credentials or insufficient permissions",
			})
			return
		}

		// Check if user has admin role
		if result.User.Role != models.RoleAdmin && result.User.Role != models.RoleSuper {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "Admin access required",
			})
			return
		}

		// Log admin login
		logger.LogUserAction(result.User.ID.Hex(), "admin_login", "admin_server", map[string]interface{}{
			"ip":         c.ClientIP(),
			"user_agent": c.Request.UserAgent(),
		})

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"user":          result.User.GetPublicInfo(result.User.ID),
				"access_token":  result.AccessToken,
				"refresh_token": result.RefreshToken,
				"expires_at":    result.ExpiresAt.Unix(),
				"token_type":    "Bearer",
			},
		})
	}
}

func adminRefreshToken(authService *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			RefreshToken string `json:"refresh_token" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request format",
			})
			return
		}

		tokens, err := authService.RefreshToken(req.RefreshToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid or expired refresh token",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"access_token":  tokens.AccessToken,
				"refresh_token": tokens.RefreshToken,
				"expires_at":    tokens.ExpiresAt.Unix(),
				"token_type":    "Bearer",
			},
		})
	}
}

func adminLogout(authService *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "User not authenticated",
			})
			return
		}

		sessionID, _ := c.Get("session_id")
		sessionIDStr := ""
		if sessionID != nil {
			sessionIDStr = sessionID.(string)
		}

		err := authService.Logout(userID.(primitive.ObjectID), sessionIDStr)
		if err != nil {
			logger.Errorf("Failed to logout admin user: %v", err)
		}

		// Log admin logout
		logger.LogUserAction(userID.(primitive.ObjectID).Hex(), "admin_logout", "admin_server", map[string]interface{}{
			"ip": c.ClientIP(),
		})

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Logout successful",
		})
	}
}

func adminValidateToken(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Invalid token",
		})
		return
	}

	userObj := user.(*models.User)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"valid": true,
			"user":  userObj.GetPublicInfo(userObj.ID),
		},
	})
}

// Utility function
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
