import { redisConfig } from '../config/redis';
import { CryptoUtils } from '../utils/crypto';
import { DateUtils } from '../utils/date';
import { smtpService } from '../communication/smtp';
import { smsService } from '../communication/sms';
import { logger } from '../monitoring/logging';
import { AUTH_CONSTANTS } from '../utils/constants';

export interface OTPOptions {
  length?: number;
  expiresIn?: number; // in milliseconds
  maxAttempts?: number;
  resendCooldown?: number; // in milliseconds
}

export interface OTPRecord {
  code: string;
  hashedCode: string;
  expiresAt: Date;
  attempts: number;
  maxAttempts: number;
  createdAt: Date;
  lastAttemptAt?: Date;
}

export interface OTPVerificationResult {
  success: boolean;
  error?: string;
  attemptsRemaining?: number;
  cooldownUntil?: Date;
}

export interface OTPSendResult {
  success: boolean;
  error?: string;
  cooldownUntil?: Date;
  expiresAt: Date;
}

export class OTPService {
  private redis = redisConfig.getClient();
  private defaultOptions: Required<OTPOptions> = {
    length: AUTH_CONSTANTS.OTP_LENGTH,
    expiresIn: AUTH_CONSTANTS.OTP_EXPIRES_IN,
    maxAttempts: 3,
    resendCooldown: 60 * 1000, // 1 minute
  };

  // Generate OTP
  generateOTP(options?: Partial<OTPOptions>): {
    code: string;
    hashedCode: string;
    expiresAt: Date;
  } {
    const opts = { ...this.defaultOptions, ...options };
    const code = CryptoUtils.generateOTP(opts.length);
    const hashedCode = CryptoUtils.hash(code);
    const expiresAt = DateUtils.addMinutes(new Date(), opts.expiresIn / (60 * 1000));

    return { code, hashedCode, expiresAt };
  }

  // Send OTP via email
  async sendEmailOTP(
    email: string,
    userName: string,
    purpose: 'registration' | 'login' | 'verification' = 'verification',
    options?: Partial<OTPOptions>
  ): Promise<OTPSendResult> {
    try {
      const identifier = `email:${email}:${purpose}`;
      
      // Check cooldown
      const cooldownCheck = await this.checkSendCooldown(identifier);
      if (!cooldownCheck.allowed) {
        return {
          success: false,
          error: 'Please wait before requesting another OTP',
          cooldownUntil: cooldownCheck.cooldownUntil,
          expiresAt: new Date(),
        };
      }

      // Generate OTP
      const otp = this.generateOTP(options);
      
      // Store OTP
      await this.storeOTP(identifier, {
        code: otp.code,
        hashedCode: otp.hashedCode,
        expiresAt: otp.expiresAt,
        attempts: 0,
        maxAttempts: options?.maxAttempts || this.defaultOptions.maxAttempts,
        createdAt: new Date(),
      });

      // Send email
      const emailResult = await smtpService.sendOTPEmail(
        email,
        otp.code,
        userName,
        Math.ceil((options?.expiresIn || this.defaultOptions.expiresIn) / (60 * 1000))
      );

      if (!emailResult.success) {
        logger.error('Failed to send OTP email', new Error(emailResult.error), {
          email,
          purpose,
        });
        return {
          success: false,
          error: 'Failed to send OTP email',
          expiresAt: otp.expiresAt,
        };
      }

      // Set send cooldown
      await this.setSendCooldown(identifier, options?.resendCooldown || this.defaultOptions.resendCooldown);

      logger.info('OTP email sent successfully', {
        email,
        purpose,
        expiresAt: otp.expiresAt,
      });

      return {
        success: true,
        expiresAt: otp.expiresAt,
      };
    } catch (error) {
      logger.error('Error sending email OTP', error, { email, purpose });
      return {
        success: false,
        error: 'Failed to send OTP',
        expiresAt: new Date(),
      };
    }
  }

  // Send OTP via SMS
  async sendSMSOTP(
    phoneNumber: string,
    purpose: 'registration' | 'login' | 'verification' = 'verification',
    options?: Partial<OTPOptions>
  ): Promise<OTPSendResult> {
    try {
      const identifier = `sms:${phoneNumber}:${purpose}`;
      
      // Check cooldown
      const cooldownCheck = await this.checkSendCooldown(identifier);
      if (!cooldownCheck.allowed) {
        return {
          success: false,
          error: 'Please wait before requesting another OTP',
          cooldownUntil: cooldownCheck.cooldownUntil,
          expiresAt: new Date(),
        };
      }

      // Generate OTP
      const otp = this.generateOTP(options);
      
      // Store OTP
      await this.storeOTP(identifier, {
        code: otp.code,
        hashedCode: otp.hashedCode,
        expiresAt: otp.expiresAt,
        attempts: 0,
        maxAttempts: options?.maxAttempts || this.defaultOptions.maxAttempts,
        createdAt: new Date(),
      });

      // Send SMS
      const smsResult = await smsService.sendOTPSMS(
        phoneNumber,
        otp.code,
        Math.ceil((options?.expiresIn || this.defaultOptions.expiresIn) / (60 * 1000))
      );

      if (!smsResult.success) {
        logger.error('Failed to send OTP SMS', new Error(smsResult.error), {
          phoneNumber,
          purpose,
        });
        return {
          success: false,
          error: 'Failed to send OTP SMS',
          expiresAt: otp.expiresAt,
        };
      }

      // Set send cooldown
      await this.setSendCooldown(identifier, options?.resendCooldown || this.defaultOptions.resendCooldown);

      logger.info('OTP SMS sent successfully', {
        phoneNumber,
        purpose,
        expiresAt: otp.expiresAt,
      });

      return {
        success: true,
        expiresAt: otp.expiresAt,
      };
    } catch (error) {
      logger.error('Error sending SMS OTP', error, { phoneNumber, purpose });
      return {
        success: false,
        error: 'Failed to send OTP',
        expiresAt: new Date(),
      };
    }
  }

  // Verify OTP
 async verifyOTP(
    identifier: string,
    code: string,
    purpose: string
  ): Promise<OTPVerificationResult> {
    try {
      const fullIdentifier = `${identifier}:${purpose}`;
      const record = await this.getOTP(fullIdentifier);

      if (!record) {
        return {
          success: false,
          error: 'OTP not found or expired',
        };
      }

      // Check if expired
      if (new Date() > record.expiresAt) {
        await this.deleteOTP(fullIdentifier);
        return {
          success: false,
          error: 'OTP has expired',
        };
      }

      // Check attempt limits
      if (record.attempts >= record.maxAttempts) {
        return {
          success: false,
          error: 'Maximum verification attempts exceeded',
          attemptsRemaining: 0,
        };
      }

      // Verify code
      const hashedInputCode = CryptoUtils.hash(code);
      const isValid = CryptoUtils.constantTimeEqual(hashedInputCode, record.hashedCode);

      // Increment attempt count
      await this.incrementAttempts(fullIdentifier);

      if (!isValid) {
        const attemptsRemaining = record.maxAttempts - (record.attempts + 1);
        
        console.warn('Invalid OTP attempt:', {
          identifier: fullIdentifier,
          attempts: record.attempts + 1,
          attemptsRemaining,
        });

        return {
          success: false,
          error: 'Invalid OTP code',
          attemptsRemaining,
        };
      }

      // Valid OTP - clean up
      await this.deleteOTP(fullIdentifier);

      console.log('✅ OTP verified successfully');

      return {
        success: true,
      };
    } catch (error) {
      console.error('Error verifying OTP:', error);
      return {
        success: false,
        error: 'OTP verification failed',
      };
    }
  }

  // Check if OTP exists and is valid
  async checkOTPExists(identifier: string, purpose: string): Promise<{
    exists: boolean;
    expiresAt?: Date;
    attemptsRemaining?: number;
  }> {
    try {
      const fullIdentifier = `${identifier}:${purpose}`;
      const record = await this.getOTP(fullIdentifier);

      if (!record) {
        return { exists: false };
      }

      if (new Date() > record.expiresAt) {
        await this.deleteOTP(fullIdentifier);
        return { exists: false };
      }

      return {
        exists: true,
        expiresAt: record.expiresAt,
        attemptsRemaining: record.maxAttempts - record.attempts,
      };
    } catch (error) {
      logger.error(
        'Error checking OTP existence',
        error instanceof Error ? error : new Error(String(error))
      );
      return { exists: false };
    }
  }

  // Delete OTP
  async deleteOTP(identifier: string): Promise<void> {
    if (!this.redis) return;

    try {
      await this.redis.del(`otp:${identifier}`);
    } catch (error) {
      logger.error('Error deleting OTP', error instanceof Error ? error : new Error(String(error)), { identifier });
    }
  }

  // Private helper methods
  private async storeOTP(identifier: string, record: OTPRecord): Promise<void> {
    if (!this.redis) {
      throw new Error('Redis not available for OTP storage');
    }

    try {
      const key = `otp:${identifier}`;
      const ttl = Math.ceil((record.expiresAt.getTime() - Date.now()) / 1000);
      
      await this.redis.setex(key, ttl, JSON.stringify({
        hashedCode: record.hashedCode,
        expiresAt: record.expiresAt.toISOString(),
        attempts: record.attempts,
        maxAttempts: record.maxAttempts,
        createdAt: record.createdAt.toISOString(),
      }));
    } catch (error) {
      logger.error('Error storing OTP', error, { identifier });
      throw error;
    }
  }

  private async getOTP(identifier: string): Promise<OTPRecord | null> {
    if (!this.redis) return null;

    try {
      const data = await this.redis.get(`otp:${identifier}`);
      if (!data) return null;

      const parsed = JSON.parse(data);
      return {
        code: '', // Don't store plaintext code
        hashedCode: parsed.hashedCode,
        expiresAt: new Date(parsed.expiresAt),
        attempts: parsed.attempts,
        maxAttempts: parsed.maxAttempts,
        createdAt: new Date(parsed.createdAt),
        lastAttemptAt: parsed.lastAttemptAt ? new Date(parsed.lastAttemptAt) : undefined,
      };
    } catch (error) {
      logger.error('Error retrieving OTP', error, { identifier });
      return null;
    }
  }

  private async incrementAttempts(identifier: string): Promise<void> {
    if (!this.redis) return;

    try {
      const record = await this.getOTP(identifier);
      if (!record) return;

      const key = `otp:${identifier}`;
      const ttl = Math.ceil((record.expiresAt.getTime() - Date.now()) / 1000);
      
      await this.redis.setex(key, ttl, JSON.stringify({
        hashedCode: record.hashedCode,
        expiresAt: record.expiresAt.toISOString(),
        attempts: record.attempts + 1,
        maxAttempts: record.maxAttempts,
        createdAt: record.createdAt.toISOString(),
        lastAttemptAt: new Date().toISOString(),
      }));
    } catch (error) {
      logger.error('Error incrementing OTP attempts', error, { identifier });
    }
  }

  private async checkSendCooldown(identifier: string): Promise<{
    allowed: boolean;
    cooldownUntil?: Date;
  }> {
    if (!this.redis) return { allowed: true };

    try {
      const cooldownKey = `otp_cooldown:${identifier}`;
      const ttl = await this.redis.ttl(cooldownKey);
      
      if (ttl > 0) {
        return {
          allowed: false,
          cooldownUntil: DateUtils.addHours(new Date(), ttl),
        };
      }

      return { allowed: true };
    } catch (error) {
      logger.error('Error checking send cooldown', error, { identifier });
      return { allowed: true }; // Fail open
    }
  }

  private async setSendCooldown(identifier: string, cooldownMs: number): Promise<void> {
    if (!this.redis) return;

    try {
      const cooldownKey = `otp_cooldown:${identifier}`;
      const cooldownSeconds = Math.ceil(cooldownMs / 1000);
      
      await this.redis.setex(cooldownKey, cooldownSeconds, 'cooldown');
    } catch (error) {
      logger.error('Error setting send cooldown', error, { identifier });
    }
  }
}

export const otpService = new OTPService();
