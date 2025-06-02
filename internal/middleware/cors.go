package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"bro/pkg/logger"
)

// CORSConfig represents CORS configuration
type CORSConfig struct {
	// AllowOrigins is a list of origins that may access the resource
	AllowOrigins []string

	// AllowOriginFunc is a custom function to validate the origin
	AllowOriginFunc func(origin string) bool

	// AllowMethods is a list of allowed HTTP methods
	AllowMethods []string

	// AllowHeaders is a list of allowed headers
	AllowHeaders []string

	// ExposeHeaders is a list of headers that are exposed to the client
	ExposeHeaders []string

	// AllowCredentials indicates whether credentials can be included
	AllowCredentials bool

	// MaxAge indicates how long the results of a preflight request can be cached
	MaxAge time.Duration

	// AllowWildcard allows usage of wildcards in AllowOrigins
	AllowWildcard bool

	// AllowBrowserExtensions allows browser extension origins
	AllowBrowserExtensions bool

	// AllowWebSockets allows WebSocket upgrade requests
	AllowWebSockets bool

	// AllowFiles allows file:// origins (for mobile apps)
	AllowFiles bool
}

// DefaultCORSConfig returns a default CORS configuration
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodHead,
			http.MethodOptions,
		},
		AllowHeaders: []string{
			"Origin",
			"Content-Length",
			"Content-Type",
			"Authorization",
			"X-Requested-With",
			"X-Device-ID",
			"X-Platform",
			"X-App-Version",
			"X-Session-ID",
			"Accept",
			"Cache-Control",
		},
		ExposeHeaders: []string{
			"Content-Length",
			"Content-Type",
			"X-Request-ID",
			"X-Rate-Limit-Limit",
			"X-Rate-Limit-Remaining",
			"X-Rate-Limit-Reset",
		},
		AllowCredentials:       true,
		MaxAge:                 12 * time.Hour,
		AllowWildcard:          true,
		AllowBrowserExtensions: false,
		AllowWebSockets:        true,
		AllowFiles:             false,
	}
}

// DevelopmentCORSConfig returns a permissive CORS configuration for development
func DevelopmentCORSConfig() CORSConfig {
	config := DefaultCORSConfig()
	config.AllowOrigins = []string{
		"http://localhost:3000",
		"http://localhost:3001",
		"http://localhost:8080",
		"http://localhost:8081",
		"http://127.0.0.1:3000",
		"http://127.0.0.1:8080",
		"*", // Allow all for development
	}
	config.AllowBrowserExtensions = true
	config.AllowFiles = true
	return config
}

// ProductionCORSConfig returns a secure CORS configuration for production
func ProductionCORSConfig(allowedDomains []string) CORSConfig {
	config := DefaultCORSConfig()
	config.AllowOrigins = allowedDomains
	config.AllowWildcard = false
	config.AllowBrowserExtensions = false
	config.AllowFiles = false

	// More restrictive headers for production
	config.AllowHeaders = []string{
		"Origin",
		"Content-Type",
		"Authorization",
		"X-Requested-With",
		"X-Device-ID",
		"X-Platform",
		"X-App-Version",
	}

	return config
}

// CORS creates a CORS middleware with default configuration
func CORS() gin.HandlerFunc {
	return CORSWithConfig(DefaultCORSConfig())
}

// CORSWithConfig creates a CORS middleware with custom configuration
func CORSWithConfig(config CORSConfig) gin.HandlerFunc {
	// Normalize configuration
	config = normalizeConfig(config)

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		method := c.Request.Method
		path := c.Request.URL.Path

		// Log CORS request for debugging
		logger.WithFields(map[string]interface{}{
			"origin": origin,
			"method": method,
			"path":   path,
			"type":   "cors_request",
		}).Debug("CORS request received")

		// Check if origin is allowed
		if origin != "" && !isOriginAllowed(origin, config) {
			logger.WithFields(map[string]interface{}{
				"origin":          origin,
				"method":          method,
				"path":            path,
				"allowed_origins": config.AllowOrigins,
				"type":            "cors_blocked",
			}).Warn("CORS request blocked - origin not allowed")

			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "CORS: origin not allowed",
			})
			return
		}

		// Set CORS headers
		setCORSHeaders(c, origin, config)

		// Handle preflight request
		if method == http.MethodOptions {
			handlePreflightRequest(c, config)
			return
		}

		// Continue with the request
		c.Next()
	}
}

// ChatAppCORS creates CORS middleware specifically configured for the chat application
func ChatAppCORS(isDevelopment bool, allowedDomains []string) gin.HandlerFunc {
	var config CORSConfig

	if isDevelopment {
		config = DevelopmentCORSConfig()
		logger.Info("CORS middleware initialized for development environment")
	} else {
		config = ProductionCORSConfig(allowedDomains)
		logger.WithFields(map[string]interface{}{
			"allowed_domains": allowedDomains,
		}).Info("CORS middleware initialized for production environment")
	}

	// Add chat-specific headers
	config.AllowHeaders = append(config.AllowHeaders,
		"X-Socket-ID",
		"X-Chat-ID",
		"X-Message-ID",
		"X-Call-ID",
		"X-File-Upload",
	)

	config.ExposeHeaders = append(config.ExposeHeaders,
		"X-Upload-Progress",
		"X-Call-Status",
		"X-Message-Status",
	)

	return CORSWithConfig(config)
}

// normalizeConfig normalizes and validates the CORS configuration
func normalizeConfig(config CORSConfig) CORSConfig {
	// Set default values if not provided
	if len(config.AllowMethods) == 0 {
		config.AllowMethods = []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodDelete,
			http.MethodOptions,
		}
	}

	if len(config.AllowHeaders) == 0 {
		config.AllowHeaders = []string{
			"Origin",
			"Content-Type",
			"Authorization",
		}
	}

	if config.MaxAge == 0 {
		config.MaxAge = 12 * time.Hour
	}

	// Normalize methods to uppercase
	for i, method := range config.AllowMethods {
		config.AllowMethods[i] = strings.ToUpper(method)
	}

	// Add OPTIONS method if not present
	hasOptions := false
	for _, method := range config.AllowMethods {
		if method == http.MethodOptions {
			hasOptions = true
			break
		}
	}
	if !hasOptions {
		config.AllowMethods = append(config.AllowMethods, http.MethodOptions)
	}

	return config
}

// isOriginAllowed checks if the given origin is allowed
func isOriginAllowed(origin string, config CORSConfig) bool {
	origin = strings.ToLower(origin)

	// Check against custom function first
	if config.AllowOriginFunc != nil {
		return config.AllowOriginFunc(origin)
	}

	// Check for wildcard
	for _, allowedOrigin := range config.AllowOrigins {
		if allowedOrigin == "*" && config.AllowWildcard {
			return true
		}

		// Exact match
		if strings.ToLower(allowedOrigin) == origin {
			return true
		}

		// Wildcard subdomain matching
		if config.AllowWildcard && strings.HasPrefix(allowedOrigin, "*.") {
			domain := allowedOrigin[2:]
			if strings.HasSuffix(origin, domain) {
				return true
			}
		}
	}

	// Check for browser extensions
	if config.AllowBrowserExtensions {
		if strings.HasPrefix(origin, "chrome-extension://") ||
			strings.HasPrefix(origin, "moz-extension://") ||
			strings.HasPrefix(origin, "safari-extension://") ||
			strings.HasPrefix(origin, "ms-browser-extension://") {
			return true
		}
	}

	// Check for file protocol
	if config.AllowFiles && strings.HasPrefix(origin, "file://") {
		return true
	}

	// Check for WebSocket upgrade
	if config.AllowWebSockets && origin == "" {
		// WebSocket connections might not have an origin
		return true
	}

	return false
}

// setCORSHeaders sets the appropriate CORS headers
func setCORSHeaders(c *gin.Context, origin string, config CORSConfig) {
	// Access-Control-Allow-Origin
	if origin != "" {
		if isOriginAllowed(origin, config) {
			c.Header("Access-Control-Allow-Origin", origin)
		}
	} else if len(config.AllowOrigins) == 1 && config.AllowOrigins[0] == "*" {
		c.Header("Access-Control-Allow-Origin", "*")
	}

	// Access-Control-Allow-Credentials
	if config.AllowCredentials {
		c.Header("Access-Control-Allow-Credentials", "true")
	}

	// Access-Control-Expose-Headers
	if len(config.ExposeHeaders) > 0 {
		c.Header("Access-Control-Expose-Headers", strings.Join(config.ExposeHeaders, ", "))
	}

	// Vary header for proper caching
	if origin != "" {
		c.Header("Vary", "Origin")
	}
}

// handlePreflightRequest handles OPTIONS preflight requests
func handlePreflightRequest(c *gin.Context, config CORSConfig) {
	origin := c.Request.Header.Get("Origin")
	method := c.Request.Header.Get("Access-Control-Request-Method")
	headers := c.Request.Header.Get("Access-Control-Request-Headers")

	// Log preflight request
	logger.WithFields(map[string]interface{}{
		"origin":            origin,
		"requested_method":  method,
		"requested_headers": headers,
		"type":              "cors_preflight",
	}).Debug("CORS preflight request")

	// Check if requested method is allowed
	methodAllowed := false
	for _, allowedMethod := range config.AllowMethods {
		if strings.ToUpper(method) == allowedMethod {
			methodAllowed = true
			break
		}
	}

	if !methodAllowed {
		logger.WithFields(map[string]interface{}{
			"origin":           origin,
			"requested_method": method,
			"allowed_methods":  config.AllowMethods,
			"type":             "cors_method_blocked",
		}).Warn("CORS preflight blocked - method not allowed")

		c.AbortWithStatusJSON(http.StatusMethodNotAllowed, gin.H{
			"error": "CORS: method not allowed",
		})
		return
	}

	// Check if requested headers are allowed
	if headers != "" {
		requestedHeaders := strings.Split(headers, ",")
		for _, header := range requestedHeaders {
			header = strings.TrimSpace(header)
			headerAllowed := false

			for _, allowedHeader := range config.AllowHeaders {
				if strings.EqualFold(header, allowedHeader) {
					headerAllowed = true
					break
				}
			}

			if !headerAllowed {
				logger.WithFields(map[string]interface{}{
					"origin":           origin,
					"requested_header": header,
					"allowed_headers":  config.AllowHeaders,
					"type":             "cors_header_blocked",
				}).Warn("CORS preflight blocked - header not allowed")

				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": "CORS: header not allowed",
				})
				return
			}
		}
	}

	// Set preflight response headers
	c.Header("Access-Control-Allow-Methods", strings.Join(config.AllowMethods, ", "))
	c.Header("Access-Control-Allow-Headers", strings.Join(config.AllowHeaders, ", "))
	c.Header("Access-Control-Max-Age", strconv.Itoa(int(config.MaxAge.Seconds())))

	// Set origin header
	if origin != "" && isOriginAllowed(origin, config) {
		c.Header("Access-Control-Allow-Origin", origin)
	}

	// Set credentials header
	if config.AllowCredentials {
		c.Header("Access-Control-Allow-Credentials", "true")
	}

	// Respond with 204 No Content
	c.Status(http.StatusNoContent)
}

// SecureCORS creates a security-focused CORS middleware
func SecureCORS(trustedDomains []string) gin.HandlerFunc {
	config := CORSConfig{
		AllowOrigins:           trustedDomains,
		AllowMethods:           []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:           []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials:       true,
		MaxAge:                 1 * time.Hour, // Shorter cache time for security
		AllowWildcard:          false,
		AllowBrowserExtensions: false,
		AllowWebSockets:        false,
		AllowFiles:             false,
	}

	return CORSWithConfig(config)
}

// FlexibleCORS creates a flexible CORS middleware that adapts based on environment
func FlexibleCORS(environment string, config map[string]interface{}) gin.HandlerFunc {
	var corsConfig CORSConfig

	switch environment {
	case "development", "dev":
		corsConfig = DevelopmentCORSConfig()
	case "production", "prod":
		if domains, ok := config["allowed_domains"].([]string); ok {
			corsConfig = ProductionCORSConfig(domains)
		} else {
			corsConfig = ProductionCORSConfig([]string{})
		}
	case "staging", "test":
		corsConfig = DefaultCORSConfig()
		if domains, ok := config["allowed_domains"].([]string); ok {
			corsConfig.AllowOrigins = domains
		}
	default:
		corsConfig = DefaultCORSConfig()
	}

	// Override with custom config if provided
	if allowCredentials, ok := config["allow_credentials"].(bool); ok {
		corsConfig.AllowCredentials = allowCredentials
	}

	if maxAge, ok := config["max_age"].(time.Duration); ok {
		corsConfig.MaxAge = maxAge
	}

	if methods, ok := config["allow_methods"].([]string); ok {
		corsConfig.AllowMethods = methods
	}

	if headers, ok := config["allow_headers"].([]string); ok {
		corsConfig.AllowHeaders = headers
	}

	logger.WithFields(map[string]interface{}{
		"environment":       environment,
		"allow_origins":     corsConfig.AllowOrigins,
		"allow_methods":     corsConfig.AllowMethods,
		"allow_headers":     corsConfig.AllowHeaders,
		"allow_credentials": corsConfig.AllowCredentials,
	}).Info("CORS middleware configured")

	return CORSWithConfig(corsConfig)
}

// WebSocketCORS creates CORS middleware specifically for WebSocket connections
func WebSocketCORS(allowedOrigins []string) gin.HandlerFunc {
	config := CORSConfig{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Upgrade", "Connection", "Sec-WebSocket-Key", "Sec-WebSocket-Version", "Sec-WebSocket-Protocol"},
		AllowWebSockets:  true,
		AllowCredentials: true,
		MaxAge:           24 * time.Hour,
	}

	return CORSWithConfig(config)
}

// MobileCORS creates CORS middleware for mobile applications
func MobileCORS() gin.HandlerFunc {
	config := CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{
			"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH",
		},
		AllowHeaders: []string{
			"Origin", "Content-Type", "Authorization",
			"X-Device-ID", "X-Platform", "X-App-Version",
			"X-Device-Token", "X-Push-Token",
		},
		ExposeHeaders: []string{
			"Content-Length", "X-Request-ID",
		},
		AllowCredentials: true,
		AllowFiles:       true, // Allow file:// protocol for mobile apps
		MaxAge:           24 * time.Hour,
	}

	return CORSWithConfig(config)
}

// APIGatewayCORS creates CORS middleware for API gateway usage
func APIGatewayCORS() gin.HandlerFunc {
	config := CORSConfig{
		AllowOriginFunc: func(origin string) bool {
			// Custom logic for API gateway
			// Could check against a database or external service
			return true
		},
		AllowMethods: []string{
			"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS",
		},
		AllowHeaders: []string{
			"Origin", "Content-Type", "Authorization",
			"X-API-Key", "X-Forwarded-For", "X-Real-IP",
			"X-Request-ID", "User-Agent",
		},
		ExposeHeaders: []string{
			"X-Request-ID", "X-Rate-Limit-Limit",
			"X-Rate-Limit-Remaining", "X-Rate-Limit-Reset",
		},
		AllowCredentials: false, // Usually false for API gateways
		MaxAge:           6 * time.Hour,
	}

	return CORSWithConfig(config)
}
