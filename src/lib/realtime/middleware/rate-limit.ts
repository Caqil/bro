import { Socket } from 'socket.io';
import { AuthenticatedSocket } from '../socket';

interface RateLimitConfig {
  windowMs: number;
  maxRequests: number;
}

const defaultConfig: RateLimitConfig = {
  windowMs: 60000, // 1 minute
  maxRequests: 100, // 100 requests per minute
};

const userRequests = new Map<string, { count: number; resetTime: number }>();

export const socketRateLimitMiddleware = (socket: AuthenticatedSocket, next: (err?: Error) => void) => {
  const userId = socket.userId;
  const now = Date.now();
  
  // Clean up old entries
  if (Math.random() < 0.01) { // 1% chance to cleanup
    for (const [key, value] of userRequests.entries()) {
      if (now > value.resetTime) {
        userRequests.delete(key);
      }
    }
  }

  const userLimit = userRequests.get(userId);
  
  if (!userLimit || now > userLimit.resetTime) {
    // Reset or initialize rate limit for user
    userRequests.set(userId, {
      count: 1,
      resetTime: now + defaultConfig.windowMs
    });
    return next();
  }

  if (userLimit.count >= defaultConfig.maxRequests) {
    return next(new Error('Rate limit exceeded'));
  }

  userLimit.count++;
  next();
};

// Rate limiting for specific events
export const createEventRateLimit = (config: Partial<RateLimitConfig> = {}) => {
  const finalConfig = { ...defaultConfig, ...config };
  const eventRequests = new Map<string, { count: number; resetTime: number }>();

  return (socket: AuthenticatedSocket, eventName: string): boolean => {
    const userId = socket.userId;
    const key = `${userId}:${eventName}`;
    const now = Date.now();

    const userEventLimit = eventRequests.get(key);

    if (!userEventLimit || now > userEventLimit.resetTime) {
      eventRequests.set(key, {
        count: 1,
        resetTime: now + finalConfig.windowMs
      });
      return true;
    }

    if (userEventLimit.count >= finalConfig.maxRequests) {
      socket.emit('error', { message: `Rate limit exceeded for ${eventName}` });
      return false;
    }

    userEventLimit.count++;
    return true;
  };
};