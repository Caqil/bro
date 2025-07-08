import jwt from 'jsonwebtoken';
import { Types } from 'mongoose';
import { environmentConfig } from '../config/environment';
import { redisConfig } from '../config/redis';
import { logger } from '../monitoring/logging';
import { CryptoUtils } from '../utils/crypto';

export interface JWTPayload {
  userId: string;
  email?: string;
  phoneNumber: string;
  displayName: string;
  isVerified: boolean;
  deviceId: string;
  iat: number;
  exp: number;
  jti: string; // JWT ID for blacklisting
}

export interface RefreshTokenPayload {
  userId: string;
  deviceId: string;
  tokenFamily: string; // For token rotation
  iat: number;
  exp: number;
  jti: string;
}

export interface TokenPair {
  accessToken: string;
  refreshToken: string;
  expiresIn: number;
  refreshExpiresIn: number;
}

export interface TokenValidationResult {
  valid: boolean;
  payload?: JWTPayload;
  error?: string;
  expired?: boolean;
}

export class JWTService {
  private jwtSecret: string;
  private refreshSecret: string;
  private accessTokenExpiry: string;
  private refreshTokenExpiry: string;
  private redis = redisConfig.getClient();

  constructor() {
    const env = environmentConfig.get();
    this.jwtSecret = env.JWT_SECRET;
    this.refreshSecret = env.REFRESH_TOKEN_SECRET;
    this.accessTokenExpiry = env.JWT_EXPIRES_IN;
    this.refreshTokenExpiry = env.REFRESH_TOKEN_EXPIRES_IN;
  }

  // Generate access token
  generateAccessToken(user: {
    _id: Types.ObjectId | string;
    email?: string;
    phoneNumber: string;
    displayName: string;
    isVerified: boolean;
  }, deviceId: string): string {
    const jti = CryptoUtils.generateUUID();
    
    const payload: Omit<JWTPayload, 'iat' | 'exp'> = {
      userId: user._id.toString(),
      email: user.email,
      phoneNumber: user.phoneNumber,
      displayName: user.displayName,
      isVerified: user.isVerified,
      deviceId,
      jti,
    };

    const token = jwt.sign(payload, this.jwtSecret, {
      expiresIn: this.accessTokenExpiry,
      issuer: 'chatapp',
      audience: 'chatapp-client',
    });

    // Store token metadata in Redis for blacklisting
    this.storeTokenMetadata(jti, user._id.toString(), 'access');

    return token;
  }

  // Generate refresh token
  generateRefreshToken(userId: string, deviceId: string, tokenFamily?: string): string {
    const jti = CryptoUtils.generateUUID();
    const family = tokenFamily || CryptoUtils.generateUUID();

    const payload: Omit<RefreshTokenPayload, 'iat' | 'exp'> = {
      userId,
      deviceId,
      tokenFamily: family,
      jti,
    };

    const token = jwt.sign(payload, this.refreshSecret, {
      expiresIn: this.refreshTokenExpiry,
      issuer: 'chatapp',
      audience: 'chatapp-refresh',
    });

    // Store refresh token metadata
    this.storeTokenMetadata(jti, userId, 'refresh', family);

    return token;
  }

  // Generate token pair (access + refresh)
  generateTokenPair(user: {
    _id: Types.ObjectId | string;
    email?: string;
    phoneNumber: string;
    displayName: string;
    isVerified: boolean;
  }, deviceId: string, existingTokenFamily?: string): TokenPair {
    const accessToken = this.generateAccessToken(user, deviceId);
    const refreshToken = this.generateRefreshToken(user._id.toString(), deviceId, existingTokenFamily);

    return {
      accessToken,
      refreshToken,
      expiresIn: this.parseExpiry(this.accessTokenExpiry),
      refreshExpiresIn: this.parseExpiry(this.refreshTokenExpiry),
    };
  }

  // Verify access token
  async verifyAccessToken(token: string): Promise<TokenValidationResult> {
    try {
      const payload = jwt.verify(token, this.jwtSecret, {
        issuer: 'chatapp',
        audience: 'chatapp-client',
      }) as JWTPayload;

      // Check if token is blacklisted
      const isBlacklisted = await this.isTokenBlacklisted(payload.jti);
      if (isBlacklisted) {
        return {
          valid: false,
          error: 'Token has been revoked',
        };
      }

      return {
        valid: true,
        payload,
      };
    } catch (error) {
      if (error instanceof jwt.TokenExpiredError) {
        return {
          valid: false,
          error: 'Token expired',
          expired: true,
        };
      }

      if (error instanceof jwt.JsonWebTokenError) {
        return {
          valid: false,
          error: 'Invalid token',
        };
      }

      logger.error('JWT verification error', error);
      return {
        valid: false,
        error: 'Token verification failed',
      };
    }
  }

  // Verify refresh token
  async verifyRefreshToken(token: string): Promise<{
    valid: boolean;
    payload?: RefreshTokenPayload;
    error?: string;
  }> {
    try {
      const payload = jwt.verify(token, this.refreshSecret, {
        issuer: 'chatapp',
        audience: 'chatapp-refresh',
      }) as RefreshTokenPayload;

      // Check if token is blacklisted
      const isBlacklisted = await this.isTokenBlacklisted(payload.jti);
      if (isBlacklisted) {
        return {
          valid: false,
          error: 'Refresh token has been revoked',
        };
      }

      // Check if token family is valid
      const isFamilyValid = await this.isTokenFamilyValid(payload.tokenFamily);
      if (!isFamilyValid) {
        return {
          valid: false,
          error: 'Token family has been invalidated',
        };
      }

      return {
        valid: true,
        payload,
      };
    } catch (error) {
      if (error instanceof jwt.TokenExpiredError) {
        return {
          valid: false,
          error: 'Refresh token expired',
        };
      }

      return {
        valid: false,
        error: 'Invalid refresh token',
      };
    }
  }

  // Refresh token pair
  async refreshTokens(refreshToken: string, user: {
    _id: Types.ObjectId | string;
    email?: string;
    phoneNumber: string;
    displayName: string;
    isVerified: boolean;
  }): Promise<TokenPair | null> {
    const refreshResult = await this.verifyRefreshToken(refreshToken);
    
    if (!refreshResult.valid || !refreshResult.payload) {
      return null;
    }

    // Blacklist the old refresh token
    await this.blacklistToken(refreshResult.payload.jti);

    // Generate new token pair with same token family
    return this.generateTokenPair(user, refreshResult.payload.deviceId, refreshResult.payload.tokenFamily);
  }

  // Blacklist token
  async blacklistToken(jti: string): Promise<void> {
    if (!this.redis) return;

    try {
      const key = `blacklist:${jti}`;
      // Set expiration longer than the longest possible token expiry
      await this.redis.setex(key, 60 * 60 * 24 * 31, 'blacklisted'); // 31 days
      
      logger.debug('Token blacklisted', { jti });
    } catch (error) {
      logger.error('Failed to blacklist token', error, { jti });
    }
  }

  // Invalidate all tokens for a user
  async invalidateAllUserTokens(userId: string): Promise<void> {
    if (!this.redis) return;

    try {
      const pattern = `token:${userId}:*`;
      const keys = await this.redis.keys(pattern);
      
      if (keys.length > 0) {
        // Get all token JTIs for this user
        const tokenData = await this.redis.mget(keys);
        const jtis = tokenData.filter(data => data).map(data => JSON.parse(data!).jti);
        
        // Blacklist all tokens
        const pipeline = this.redis.pipeline();
        jtis.forEach(jti => {
          pipeline.setex(`blacklist:${jti}`, 60 * 60 * 24 * 31, 'blacklisted');
        });
        
        // Remove token metadata
        pipeline.del(...keys);
        
        await pipeline.exec();
        
        logger.info('All user tokens invalidated', { userId, tokenCount: jtis.length });
      }
    } catch (error) {
      logger.error('Failed to invalidate user tokens', error, { userId });
    }
  }

  // Invalidate tokens for a specific device
  async invalidateDeviceTokens(userId: string, deviceId: string): Promise<void> {
    if (!this.redis) return;

    try {
      const pattern = `token:${userId}:*:${deviceId}`;
      const keys = await this.redis.keys(pattern);
      
      if (keys.length > 0) {
        const tokenData = await this.redis.mget(keys);
        const jtis = tokenData.filter(data => data).map(data => JSON.parse(data!).jti);
        
        // Blacklist device tokens
        const pipeline = this.redis.pipeline();
        jtis.forEach(jti => {
          pipeline.setex(`blacklist:${jti}`, 60 * 60 * 24 * 31, 'blacklisted');
        });
        
        pipeline.del(...keys);
        await pipeline.exec();
        
        logger.info('Device tokens invalidated', { userId, deviceId, tokenCount: jtis.length });
      }
    } catch (error) {
      logger.error('Failed to invalidate device tokens', error, { userId, deviceId });
    }
  }

  // Invalidate token family (for security breaches)
  async invalidateTokenFamily(tokenFamily: string): Promise<void> {
    if (!this.redis) return;

    try {
      const key = `token_family:${tokenFamily}`;
      await this.redis.setex(key, 60 * 60 * 24 * 31, 'invalidated'); // 31 days
      
      logger.warn('Token family invalidated', { tokenFamily });
    } catch (error) {
      logger.error('Failed to invalidate token family', error, { tokenFamily });
    }
  }

  // Get token info
  async getTokenInfo(token: string): Promise<{
    type: 'access' | 'refresh' | 'unknown';
    payload?: any;
    valid: boolean;
  }> {
    // Try to decode without verification to determine token type
    try {
      const decoded = jwt.decode(token) as any;
      
      if (!decoded) {
        return { type: 'unknown', valid: false };
      }

      // Check audience to determine token type
      if (decoded.aud === 'chatapp-client') {
        const result = await this.verifyAccessToken(token);
        return {
          type: 'access',
          payload: result.payload,
          valid: result.valid,
        };
      } else if (decoded.aud === 'chatapp-refresh') {
        const result = await this.verifyRefreshToken(token);
        return {
          type: 'refresh',
          payload: result.payload,
          valid: result.valid,
        };
      }

      return { type: 'unknown', valid: false };
    } catch (error) {
      return { type: 'unknown', valid: false };
    }
  }

  // Private helper methods
  private async storeTokenMetadata(jti: string, userId: string, type: 'access' | 'refresh', tokenFamily?: string): Promise<void> {
    if (!this.redis) return;

    try {
      const key = `token:${userId}:${jti}:${type === 'access' ? 'access' : 'refresh'}`;
      const metadata = {
        jti,
        userId,
        type,
        tokenFamily,
        createdAt: new Date().toISOString(),
      };

      const expiry = type === 'access' 
        ? this.parseExpiry(this.accessTokenExpiry)
        : this.parseExpiry(this.refreshTokenExpiry);

      await this.redis.setex(key, expiry, JSON.stringify(metadata));
    } catch (error) {
      logger.error('Failed to store token metadata', error, { jti, userId, type });
    }
  }

  private async isTokenBlacklisted(jti: string): Promise<boolean> {
    if (!this.redis) return false;

    try {
      const result = await this.redis.get(`blacklist:${jti}`);
      return result !== null;
    } catch (error) {
      logger.error('Failed to check token blacklist', error, { jti });
      return false; // Fail open
    }
  }

  private async isTokenFamilyValid(tokenFamily: string): Promise<boolean> {
    if (!this.redis) return true;

    try {
      const result = await this.redis.get(`token_family:${tokenFamily}`);
      return result === null; // Valid if not in invalidated list
    } catch (error) {
      logger.error('Failed to check token family validity', error, { tokenFamily });
      return true; // Fail open
    }
  }

  private parseExpiry(expiry: string): number {
    // Convert JWT expiry format to seconds
    const match = expiry.match(/^(\d+)([smhd])$/);
    if (!match) return 3600; // Default 1 hour

    const value = parseInt(match[1]);
    const unit = match[2];

    switch (unit) {
      case 's': return value;
      case 'm': return value * 60;
      case 'h': return value * 60 * 60;
      case 'd': return value * 60 * 60 * 24;
      default: return 3600;
    }
  }
}

export const jwtService = new JWTService();
