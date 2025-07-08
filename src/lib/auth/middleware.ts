import { Request, Response, NextFunction } from 'express';
import { jwtService, JWTPayload } from './jwt';
import { UserRepository } from '../database/repositories/user';
import { permissionService, Permission } from '../security/permissions';
import { rateLimitConfig } from '../config/rate-limits';
import { logger } from '../monitoring/logging';
import { ErrorHandler } from '../utils/error-handler';

// Extend Express Request to include user info
declare global {
  namespace Express {
    interface Request {
      user?: JWTPayload & {
        permissions: Permission[];
      };
      deviceId?: string;
    }
  }
}

export interface AuthOptions {
  required?: boolean;
  permissions?: Permission[];
  roles?: string[];
  verifiedOnly?: boolean;
}

class AuthMiddleware {
  private userRepository: UserRepository;

  constructor() {
    this.userRepository = new UserRepository();
  }

  // Basic JWT authentication middleware
  authenticate(options: AuthOptions = { required: true }) {
    return async (req: Request, res: Response, next: NextFunction) => {
      try {
        const token = this.extractToken(req);

        if (!token) {
          if (options.required) {
            return next(ErrorHandler.authenticationError('Authentication token required'));
          }
          return next();
        }

        // Verify token
        const tokenResult = await jwtService.verifyAccessToken(token);

        if (!tokenResult.valid) {
          if (tokenResult.expired) {
            return next(ErrorHandler.authenticationError('Token expired'));
          }
          return next(ErrorHandler.authenticationError(tokenResult.error || 'Invalid token'));
        }

        const payload = tokenResult.payload!;

        // Get user from database to check current status
        const user = await this.userRepository.findById(payload.userId);
        if (!user) {
          return next(ErrorHandler.authenticationError('User not found'));
        }

        // Check if user is banned
        if (user.isBanned) {
          // Invalidate all user tokens
          await jwtService.invalidateAllUserTokens(user._id.toString());
          return next(ErrorHandler.authenticationError('Account has been suspended'));
        }

        // Check verification requirement
        if (options.verifiedOnly && !user.isVerified) {
          return next(ErrorHandler.authorizationError('Account verification required'));
        }

        // Get user permissions
        const permissions = permissionService.getUserPermissions(user);

        // Check required permissions
        if (options.permissions && options.permissions.length > 0) {
          const hasPermissions = permissionService.hasPermissions(user, options.permissions);
          if (!hasPermissions) {
            return next(ErrorHandler.authorizationError('Insufficient permissions'));
          }
        }

        // Attach user info to request
        req.user = {
          ...payload,
          permissions,
        };
        req.deviceId = payload.deviceId;

        // Log authentication success
        logger.debug('User authenticated successfully', {
          userId: payload.userId,
          deviceId: payload.deviceId,
          path: req.path,
        });

        next();
      } catch (error) {
        logger.error(
          'Authentication middleware error',
          error instanceof Error ? error : new Error(String(error))
        );
        next(ErrorHandler.authenticationError('Authentication failed'));
      }
    };
  }

  // Admin authentication middleware
  authenticateAdmin(requiredPermissions?: Permission[]) {
    return async (req: Request, res: Response, next: NextFunction) => {
      try {
        const token = this.extractToken(req);

        if (!token) {
          return next(ErrorHandler.authenticationError('Admin authentication required'));
        }

        // Verify token
        const tokenResult = await jwtService.verifyAccessToken(token);
        if (!tokenResult.valid) {
          return next(ErrorHandler.authenticationError('Invalid admin token'));
        }

        const payload = tokenResult.payload!;

        // Check if user has admin role (this would need to be implemented in user model)
        const user = await this.userRepository.findById(payload.userId);
        if (!user || user.isBanned) {
          return next(ErrorHandler.authenticationError('Admin account not found or suspended'));
        }

        // In a real implementation, you'd check admin roles here
        // For now, we'll assume the token contains admin permissions
        
        // Check required permissions if specified
        if (requiredPermissions && requiredPermissions.length > 0) {
          const hasPermissions = permissionService.hasPermissions(user, requiredPermissions);
          if (!hasPermissions) {
            return next(ErrorHandler.authorizationError('Insufficient admin permissions'));
          }
        }

        req.user = {
          ...payload,
          permissions: permissionService.getUserPermissions(user),
        };

        logger.info('Admin authenticated', {
          userId: payload.userId,
          path: req.path,
          permissions: requiredPermissions,
        });

        next();
      } catch (error) {
        logger.error(
          'Admin authentication error',
          error instanceof Error ? error : new Error(String(error))
        );
        next(ErrorHandler.authenticationError('Admin authentication failed'));
      }
    };
  }

  // Optional authentication middleware
  authenticateOptional() {
    return this.authenticate({ required: false });
  }

  // WebSocket authentication middleware
  authenticateSocket() {
    return async (socket: any, next: (err?: Error) => void) => {
      try {
        const token = socket.handshake.auth.token || socket.handshake.headers.authorization;

        if (!token) {
          return next(new Error('Authentication token required'));
        }

        const cleanToken = token.replace('Bearer ', '');
        const tokenResult = await jwtService.verifyAccessToken(cleanToken);

        if (!tokenResult.valid) {
          return next(new Error('Invalid or expired token'));
        }

        const payload = tokenResult.payload!;

        // Verify user exists and is not banned
        const user = await this.userRepository.findById(payload.userId);
        if (!user || user.isBanned) {
          return next(new Error('User not found or banned'));
        }

        // Attach user info to socket
        socket.userId = payload.userId;
        socket.user = {
          _id: payload.userId,
          displayName: payload.displayName,
          avatar: user.avatar,
          phoneNumber: payload.phoneNumber,
        };

        logger.debug('Socket authenticated', {
          userId: payload.userId,
          socketId: socket.id,
        });

        next();
      } catch (error) {
        logger.error(
          'Socket authentication error',
          error instanceof Error ? error : new Error(String(error))
        );
        next(new Error('Authentication failed'));
      }
    };
  }

  // Rate limiting middleware for authentication endpoints
  authRateLimit() {
    const rateLimiter = rateLimitConfig.createRateLimiter('auth');
    return rateLimiter.middleware();
  }

  // OTP rate limiting middleware
  otpRateLimit() {
    const rateLimiter = rateLimitConfig.createRateLimiter('otp');
    return rateLimiter.middleware();
  }

  // Extract token from request
  private extractToken(req: Request): string | null {
    // Check Authorization header
    const authHeader = req.headers.authorization;
    if (authHeader && authHeader.startsWith('Bearer ')) {
      return authHeader.substring(7);
    }

    // Check cookie (for web app)
    if (req.cookies && req.cookies.accessToken) {
      return req.cookies.accessToken;
    }

    // Check query parameter (not recommended for production)
    if (req.query.token && typeof req.query.token === 'string') {
      return req.query.token;
    }

    return null;
  }

  // Middleware to check specific permissions
  requirePermissions(...permissions: Permission[]) {
    return (req: Request, res: Response, next: NextFunction) => {
      if (!req.user) {
        return next(ErrorHandler.authenticationError('Authentication required'));
      }

      const hasPermissions = permissions.every(permission => 
        req.user!.permissions.includes(permission)
      );

      if (!hasPermissions) {
        return next(ErrorHandler.authorizationError('Insufficient permissions'));
      }

      next();
    };
  }

  // Middleware to check if user owns resource
  requireOwnership(getResourceUserId: (req: Request) => string) {
    return (req: Request, res: Response, next: NextFunction) => {
      if (!req.user) {
        return next(ErrorHandler.authenticationError('Authentication required'));
      }

      const resourceUserId = getResourceUserId(req);
      if (req.user.userId !== resourceUserId) {
        return next(ErrorHandler.authorizationError('Access denied: not resource owner'));
      }

      next();
    };
  }
}

export const authMiddleware = new AuthMiddleware();
