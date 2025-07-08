import { environmentConfig } from './environment';
import { redisConfig } from './redis';

export interface RateLimitConfig {
  windowMs: number;
  maxRequests: number;
  message: string;
  standardHeaders: boolean;
  legacyHeaders: boolean;
  store?: 'memory' | 'redis';
  skipSuccessfulRequests: boolean;
  skipFailedRequests: boolean;
  keyGenerator: (req: any) => string;
}

export interface EndpointRateLimits {
  auth: RateLimitConfig;
  otp: RateLimitConfig;
  general: RateLimitConfig;
  messaging: RateLimitConfig;
  upload: RateLimitConfig;
  admin: RateLimitConfig;
  calls: RateLimitConfig;
}

class RateLimitConfiguration {
  private config: EndpointRateLimits;

  constructor() {
    this.config = this.createConfig();
  }

  private createConfig(): EndpointRateLimits {
    const env = environmentConfig.get();
    const hasRedis = redisConfig.getConfig() !== null;
    
    const baseConfig = {
      standardHeaders: true,
      legacyHeaders: false,
      store: hasRedis ? 'redis' as const : 'memory' as const,
      skipSuccessfulRequests: false,
      skipFailedRequests: false,
    };

    return {
      // Authentication endpoints - strict limits
      auth: {
        ...baseConfig,
        windowMs: 15 * 60 * 1000, // 15 minutes
        maxRequests: 5,
        message: 'Too many authentication attempts, please try again later.',
        keyGenerator: (req: any) => `auth:${req.ip}:${req.body?.phoneNumber || req.body?.email || req.ip}`,
      },

      // OTP endpoints - very strict
      otp: {
        ...baseConfig,
        windowMs: 60 * 1000, // 1 minute
        maxRequests: 3,
        message: 'Too many OTP requests, please wait before requesting again.',
        keyGenerator: (req: any) => `otp:${req.body?.phoneNumber || req.body?.email || req.ip}`,
      },

      // General API - moderate limits
      general: {
        ...baseConfig,
        windowMs: env.RATE_LIMIT_WINDOW_MS,
        maxRequests: env.RATE_LIMIT_MAX_REQUESTS,
        message: 'Too many requests from this IP, please try again later.',
        keyGenerator: (req: any) => `general:${req.ip}`,
      },

      // Messaging endpoints
      messaging: {
        ...baseConfig,
        windowMs: 60 * 1000, // 1 minute
        maxRequests: 30,
        message: 'Message rate limit exceeded, please slow down.',
        keyGenerator: (req: any) => `msg:${req.user?.id || req.ip}`,
      },

      // File upload endpoints
      upload: {
        ...baseConfig,
        windowMs: 60 * 1000, // 1 minute
        maxRequests: 10,
        message: 'Upload rate limit exceeded, please wait before uploading again.',
        keyGenerator: (req: any) => `upload:${req.user?.id || req.ip}`,
      },

      // Admin endpoints
      admin: {
        ...baseConfig,
        windowMs: 60 * 1000, // 1 minute
        maxRequests: 60,
        message: 'Admin API rate limit exceeded.',
        keyGenerator: (req: any) => `admin:${req.user?.id || req.ip}`,
      },

      // Call endpoints
      calls: {
        ...baseConfig,
        windowMs: 60 * 1000, // 1 minute
        maxRequests: 10,
        message: 'Call rate limit exceeded, please wait before initiating another call.',
        keyGenerator: (req: any) => `call:${req.user?.id || req.ip}`,
      },
    };
  }

  getConfig(): EndpointRateLimits {
    return this.config;
  }

  // Get configuration for specific endpoint
  getEndpointConfig(endpoint: keyof EndpointRateLimits): RateLimitConfig {
    return this.config[endpoint];
  }

  // Create rate limiter instance
  createRateLimiter(endpoint: keyof EndpointRateLimits) {
    const config = this.getEndpointConfig(endpoint);
    const { createRateLimiters } = require('../security/rate-limiting');
    
    // Get Redis client if available
    const redisClient = redisConfig.getClient();
    const limiters = createRateLimiters(redisClient);
    
    return limiters[endpoint] || limiters.general;
  }

  // Get all rate limiters
  getAllRateLimiters() {
    const { createRateLimiters } = require('../security/rate-limiting');
    const redisClient = redisConfig.getClient();
    
    return createRateLimiters(redisClient);
  }
}

export const rateLimitConfig = new RateLimitConfiguration();
