import { Request, Response, NextFunction } from 'express';

interface RateLimitOptions {
  windowMs: number;
  maxRequests: number;
  message?: string;
  skipSuccessfulRequests?: boolean;
  skipFailedRequests?: boolean;
  keyGenerator?: (req: Request) => string;
}

interface RateLimitStore {
  get(key: string): Promise<number | null>;
  set(key: string, value: number, ttl: number): Promise<void>;
  increment(key: string, ttl: number): Promise<number>;
}

// In-memory store (for development)
class MemoryStore implements RateLimitStore {
  private store = new Map<string, { count: number; resetTime: number }>();

  async get(key: string): Promise<number | null> {
    const entry = this.store.get(key);
    if (!entry) return null;
    
    if (Date.now() > entry.resetTime) {
      this.store.delete(key);
      return null;
    }
    
    return entry.count;
  }

  async set(key: string, value: number, ttl: number): Promise<void> {
    this.store.set(key, {
      count: value,
      resetTime: Date.now() + ttl
    });
  }

  async increment(key: string, ttl: number): Promise<number> {
    const current = await this.get(key);
    const newCount = (current || 0) + 1;
    await this.set(key, newCount, ttl);
    return newCount;
  }
}

// Redis store (for production)
class RedisStore implements RateLimitStore {
  private redis: any; // Redis client

  constructor(redisClient: any) {
    this.redis = redisClient;
  }

  async get(key: string): Promise<number | null> {
    const value = await this.redis.get(key);
    return value ? parseInt(value, 10) : null;
  }

  async set(key: string, value: number, ttl: number): Promise<void> {
    await this.redis.setex(key, Math.ceil(ttl / 1000), value.toString());
  }

  async increment(key: string, ttl: number): Promise<number> {
    const multi = this.redis.multi();
    multi.incr(key);
    multi.expire(key, Math.ceil(ttl / 1000));
    const results = await multi.exec();
    return results[0][1];
  }
}

export class RateLimiter {
  private options: Required<RateLimitOptions>;
  private store: RateLimitStore;

  constructor(options: RateLimitOptions, store?: RateLimitStore) {
    this.options = {
      windowMs: options.windowMs,
      maxRequests: options.maxRequests,
      message: options.message || 'Too many requests, please try again later.',
      skipSuccessfulRequests: options.skipSuccessfulRequests || false,
      skipFailedRequests: options.skipFailedRequests || false,
      keyGenerator: options.keyGenerator || ((req: Request) => req.ip || 'unknown'),
    };
    
    this.store = store || new MemoryStore();
  }

  middleware() {
    return async (req: Request, res: Response, next: NextFunction) => {
      try {
        const key = this.options.keyGenerator(req);
        const current = await this.store.increment(key, this.options.windowMs);

        // Set rate limit headers
        res.set({
          'X-RateLimit-Limit': this.options.maxRequests.toString(),
          'X-RateLimit-Remaining': Math.max(0, this.options.maxRequests - current).toString(),
          'X-RateLimit-Reset': new Date(Date.now() + this.options.windowMs).toISOString(),
        });

        if (current > this.options.maxRequests) {
          res.status(429).json({
            error: 'Rate limit exceeded',
            message: this.options.message,
            retryAfter: Math.ceil(this.options.windowMs / 1000),
          });
          return;
        }

        // Skip counting successful/failed requests if configured
        const originalSend = res.send;
        res.send = function(this: RateLimiter, data) {
          const statusCode = res.statusCode;
          const shouldSkip = 
            (statusCode < 400 && this.options.skipSuccessfulRequests) ||
            (statusCode >= 400 && this.options.skipFailedRequests);

          if (shouldSkip) {
            // Decrement counter
            this.store.increment(key, -1);
          }

          return originalSend.call(this, data);
        }.bind(this);

        next();
      } catch (error) {
        console.error('Rate limiting error:', error);
        next(); // Continue without rate limiting on error
      }
    };
  }
}

// Predefined rate limiters for different endpoints
export const createRateLimiters = (redisClient?: any) => {
  const store = redisClient ? new RedisStore(redisClient) : new MemoryStore();

  return {
    // General API rate limit
    general: new RateLimiter({
      windowMs: 15 * 60 * 1000, // 15 minutes
      maxRequests: 100,
      message: 'Too many requests from this IP, please try again later.',
    }, store),

    // Authentication endpoints
    auth: new RateLimiter({
      windowMs: 15 * 60 * 1000, // 15 minutes
      maxRequests: 5,
      message: 'Too many authentication attempts, please try again later.',
      keyGenerator: (req: Request) => `auth:${req.ip}:${req.body.phoneNumber || req.body.email || req.ip}`,
    }, store),

    // OTP endpoints
    otp: new RateLimiter({
      windowMs: 60 * 1000, // 1 minute
      maxRequests: 3,
      message: 'Too many OTP requests, please wait before requesting again.',
      keyGenerator: (req: Request) => `otp:${req.body.phoneNumber || req.body.email}`,
    }, store),

    // Message sending
    messaging: new RateLimiter({
      windowMs: 60 * 1000, // 1 minute
      maxRequests: 30,
      message: 'Message rate limit exceeded, please slow down.',
      keyGenerator: (req: Request) => `msg:${req.user?.id || req.ip}`,
    }, store),

    // File uploads
    upload: new RateLimiter({
      windowMs: 60 * 1000, // 1 minute
      maxRequests: 10,
      message: 'Upload rate limit exceeded, please wait before uploading again.',
      keyGenerator: (req: Request) => `upload:${req.user?.id || req.ip}`,
    }, store),

    // Admin endpoints
    admin: new RateLimiter({
      windowMs: 60 * 1000, // 1 minute
      maxRequests: 60,
      message: 'Admin API rate limit exceeded.',
      keyGenerator: (req: Request) => `admin:${req.user?.id || req.ip}`,
    }, store),
  };
};
