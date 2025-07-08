import QRCode from 'qrcode';
import { redisConfig } from '../config/redis';
import { CryptoUtils } from '../utils/crypto';
import { jwtService } from './jwt';
import { UserRepository } from '../database/repositories/user';
import { socketManager } from '../realtime/socket';
import { logger } from '../monitoring/logging';
import { DateUtils } from '../utils/date';

export interface QRAuthSession {
  qrId: string;
  secret: string;
  status: 'pending' | 'scanned' | 'confirmed' | 'expired' | 'rejected';
  deviceInfo: {
    platform: string;
    userAgent: string;
    ip?: string;
  };
  createdAt: Date;
  expiresAt: Date;
  scannedAt?: Date;
  confirmedAt?: Date;
  userId?: string;
}

export interface QRGenerationResult {
  qrId: string;
  qrCode: string; // Base64 QR code image
  qrText: string; // Text that was encoded
  expiresAt: Date;
  checkUrl: string;
}

export interface QRScanResult {
  success: boolean;
  error?: string;
  userInfo?: {
    displayName: string;
    avatar?: string;
  };
}

export interface QRConfirmationResult {
  success: boolean;
  error?: string;
  tokens?: {
    accessToken: string;
    refreshToken: string;
    expiresIn: number;
  };
}

export class QRAuthService {
  private redis = redisConfig.getClient();
  private userRepository: UserRepository;
  private qrExpiryTime = 5 * 60 * 1000; // 5 minutes

  constructor() {
    this.userRepository = new UserRepository();
  }

  // Generate QR code for web login
  async generateQRCode(deviceInfo: {
    platform: string;
    userAgent: string;
    ip?: string;
  }): Promise<QRGenerationResult> {
    try {
      // Generate unique QR ID and secret
      const qrId = CryptoUtils.generateUUID();
      const secret = CryptoUtils.generateRandomString(32);
      
      // Create session
      const session: QRAuthSession = {
        qrId,
        secret,
        status: 'pending',
        deviceInfo,
        createdAt: new Date(),
        expiresAt: DateUtils.addMinutes(new Date(), this.qrExpiryTime / (60 * 1000)),
      };

      // Store session in Redis
      await this.storeSession(session);

      // Create QR code data
      const qrData = {
        id: qrId,
        secret,
        timestamp: Date.now(),
        version: '1.0',
      };

      const qrText = JSON.stringify(qrData);

      // Generate QR code image
      const qrCode = await QRCode.toDataURL(qrText, {
        width: 256,
        margin: 2,
        color: {
          dark: '#000000',
          light: '#FFFFFF',
        },
      });

      // Create check URL for status polling
      const checkUrl = `/api/client/auth/qr-login/status?qrId=${qrId}`;

      logger.info('QR code generated for web login', {
        qrId,
        platform: deviceInfo.platform,
        expiresAt: session.expiresAt,
      });

      return {
        qrId,
        qrCode,
        qrText,
        expiresAt: session.expiresAt,
        checkUrl,
      };
    } catch (error) {
      logger.error('Error generating QR code', error);
      throw new Error('Failed to generate QR code');
    }
  }

  // Scan QR code (mobile app)
  async scanQRCode(qrText: string, userId: string): Promise<QRScanResult> {
    try {
      // Parse QR data
      let qrData;
      try {
        qrData = JSON.parse(qrText);
      } catch {
        return {
          success: false,
          error: 'Invalid QR code format',
        };
      }

      // Validate QR data structure
      if (!qrData.id || !qrData.secret || !qrData.timestamp) {
        return {
          success: false,
          error: 'Invalid QR code data',
        };
      }

      // Check QR code age (prevent replay attacks)
      const qrAge = Date.now() - qrData.timestamp;
      if (qrAge > this.qrExpiryTime) {
        return {
          success: false,
          error: 'QR code has expired',
        };
      }

      // Get session
      const session = await this.getSession(qrData.id);
      if (!session) {
        return {
          success: false,
          error: 'QR session not found or expired',
        };
      }

      // Verify secret
      if (!CryptoUtils.constantTimeEqual(session.secret, qrData.secret)) {
        return {
          success: false,
          error: 'Invalid QR code secret',
        };
      }

      // Check session status
      if (session.status !== 'pending') {
        return {
          success: false,
          error: 'QR code has already been processed',
        };
      }

      // Get user info
      const user = await this.userRepository.findById(userId);
      if (!user || user.isBanned) {
        return {
          success: false,
          error: 'User not found or account suspended',
        };
      }

      // Update session status
      session.status = 'scanned';
      session.scannedAt = new Date();
      session.userId = userId;
      
      await this.storeSession(session);

      // Notify web client via WebSocket
      socketManager.emitToUser(`qr:${session.qrId}`, 'qr:scanned', {
        qrId: session.qrId,
        userInfo: {
          displayName: user.displayName,
          avatar: user.avatar,
        },
      });

      logger.info('QR code scanned successfully', {
        qrId: session.qrId,
        userId,
        deviceInfo: session.deviceInfo,
      });

      return {
        success: true,
        userInfo: {
          displayName: user.displayName,
          avatar: user.avatar,
        },
      };
    } catch (error) {
      logger.error('Error scanning QR code', error, { userId });
      return {
        success: false,
        error: 'Failed to scan QR code',
      };
    }
  }

  // Confirm QR login (mobile app)
  async confirmQRLogin(qrId: string, userId: string, deviceId: string): Promise<QRConfirmationResult> {
    try {
      // Get session
      const session = await this.getSession(qrId);
      if (!session) {
        return {
          success: false,
          error: 'QR session not found or expired',
        };
      }

      // Verify session state
      if (session.status !== 'scanned' || session.userId !== userId) {
        return {
          success: false,
          error: 'Invalid session state',
        };
      }

      // Get user
      const user = await this.userRepository.findById(userId);
      if (!user || user.isBanned) {
        return {
          success: false,
          error: 'User not found or account suspended',
        };
      }

      // Generate tokens for web client
      const webDeviceId = `web-${CryptoUtils.generateRandomString(8)}`;
      const tokens = jwtService.generateTokenPair(user, webDeviceId);

      // Update session status
      session.status = 'confirmed';
      session.confirmedAt = new Date();
      await this.storeSession(session);

      // Notify web client with tokens
      socketManager.emitToUser(`qr:${qrId}`, 'qr:confirmed', {
        qrId,
        tokens: {
          accessToken: tokens.accessToken,
          refreshToken: tokens.refreshToken,
          expiresIn: tokens.expiresIn,
        },
      });

      // Clean up session after successful login
      setTimeout(() => {
        this.deleteSession(qrId);
      }, 30000); // Clean up after 30 seconds

      logger.info('QR login confirmed successfully', {
        qrId,
        userId,
        deviceInfo: session.deviceInfo,
      });

      return {
        success: true,
        tokens: {
          accessToken: tokens.accessToken,
          refreshToken: tokens.refreshToken,
          expiresIn: tokens.expiresIn,
        },
      };
    } catch (error) {
      logger.error('Error confirming QR login', error, { qrId, userId });
      return {
        success: false,
        error: 'Failed to confirm QR login',
      };
    }
  }

  // Reject QR login (mobile app)
  async rejectQRLogin(qrId: string, userId: string): Promise<{ success: boolean; error?: string }> {
    try {
      // Get session
      const session = await this.getSession(qrId);
      if (!session) {
        return {
          success: false,
          error: 'QR session not found or expired',
        };
      }

      // Verify session state
      if (session.status !== 'scanned' || session.userId !== userId) {
        return {
          success: false,
          error: 'Invalid session state',
        };
      }

      // Update session status
      session.status = 'rejected';
      await this.storeSession(session);

      // Notify web client
      socketManager.emitToUser(`qr:${qrId}`, 'qr:rejected', {
        qrId,
      });

      // Clean up session
      setTimeout(() => {
        this.deleteSession(qrId);
      }, 5000);

      logger.info('QR login rejected', {
        qrId,
        userId,
        deviceInfo: session.deviceInfo,
      });

      return { success: true };
    } catch (error) {
      logger.error('Error rejecting QR login', error, { qrId, userId });
      return {
        success: false,
        error: 'Failed to reject QR login',
      };
    }
  }

  // Get QR session status (web client polling)
  async getQRStatus(qrId: string): Promise<{
    status: QRAuthSession['status'];
    userInfo?: {
      displayName: string;
      avatar?: string;
    };
    expiresAt: Date;
  } | null> {
    try {
      const session = await this.getSession(qrId);
      if (!session) {
        return null;
      }

      const result: any = {
        status: session.status,
        expiresAt: session.expiresAt,
      };

      // Include user info if scanned
      if (session.status === 'scanned' && session.userId) {
        const user = await this.userRepository.findById(session.userId);
        if (user) {
          result.userInfo = {
            displayName: user.displayName,
            avatar: user.avatar,
          };
        }
      }

      return result;
    } catch (error) {
      logger.error('Error getting QR status', error, { qrId });
      return null;
    }
  }

  // Clean up expired sessions
  async cleanupExpiredSessions(): Promise<void> {
    // This would be called periodically by a cron job
    // Implementation would scan Redis for expired sessions and clean them up
    try {
      if (!this.redis) return;

      // In a real implementation, you'd use Redis SCAN to find expired sessions
      // For now, we rely on Redis TTL to automatically expire sessions
      logger.debug('QR session cleanup completed');
    } catch (error) {
      logger.error('Error cleaning up QR sessions', error);
    }
  }

  // Private helper methods
  private async storeSession(session: QRAuthSession): Promise<void> {
    if (!this.redis) {
      throw new Error('Redis not available for QR session storage');
    }

    try {
      const key = `qr_session:${session.qrId}`;
      const ttl = Math.ceil((session.expiresAt.getTime() - Date.now()) / 1000);
      
      await this.redis.setex(key, ttl, JSON.stringify({
        ...session,
        createdAt: session.createdAt.toISOString(),
        expiresAt: session.expiresAt.toISOString(),
        scannedAt: session.scannedAt?.toISOString(),
        confirmedAt: session.confirmedAt?.toISOString(),
      }));
    } catch (error) {
      logger.error('Error storing QR session', error, { qrId: session.qrId });
      throw error;
    }
  }

  private async getSession(qrId: string): Promise<QRAuthSession | null> {
    if (!this.redis) return null;

    try {
      const data = await this.redis.get(`qr_session:${qrId}`);
      if (!data) return null;

      const parsed = JSON.parse(data);
      return {
        ...parsed,
        createdAt: new Date(parsed.createdAt),
        expiresAt: new Date(parsed.expiresAt),
        scannedAt: parsed.scannedAt ? new Date(parsed.scannedAt) : undefined,
        confirmedAt: parsed.confirmedAt ? new Date(parsed.confirmedAt) : undefined,
      };
    } catch (error) {
      logger.error('Error retrieving QR session', error, { qrId });
      return null;
    }
  }

  private async deleteSession(qrId: string): Promise<void> {
    if (!this.redis) return;

    try {
      await this.redis.del(`qr_session:${qrId}`);
    } catch (error) {
      logger.error('Error deleting QR session', error, { qrId });
    }
  }
}

export const qrAuthService = new QRAuthService();