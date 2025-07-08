import { NextRequest, NextResponse } from 'next/server';
import { UserRepository } from '@/lib/database/repositories/user';
import { otpService } from '@/lib/auth/otp';
import { DataSanitizer } from '@/lib/security/sanitization';
import { logger } from '@/lib/monitoring/logging';
import { analyticsService } from '@/lib/monitoring/analytics';
import { authMiddleware } from '@/lib/auth/middleware';
import connectDB from '@/lib/database/mongodb';
import { IUser } from '@/lib/database/models/user';

export async function POST(request: NextRequest) {
  try {
    await connectDB();
    
    const body = await request.json();
    const { phoneNumber, email, purpose = 'verification' } = body;

    if (!phoneNumber && !email) {
      return NextResponse.json(
        { error: 'Phone number or email is required' },
        { status: 400 }
      );
    }

    const userRepository = new UserRepository();
    let user: IUser | null = null;
    let identifier = '';
    let method = '';

    // Find user and determine method
    if (phoneNumber) {
      const sanitizedPhoneNumber = DataSanitizer.sanitizePhoneNumber(phoneNumber);
      user = await userRepository.findByPhoneNumber(sanitizedPhoneNumber);
      identifier = sanitizedPhoneNumber;
      method = 'sms';
    } else if (email) {
      const sanitizedEmail = DataSanitizer.sanitizeEmail(email);
      user = await userRepository.findByEmail(sanitizedEmail);
      identifier = sanitizedEmail;
      method = 'email';
    }

    // For registration, user might not exist yet
    if (purpose === 'registration') {
      if (!user) {
        // This is fine for registration resend
      } else {
        return NextResponse.json(
          { error: 'User already exists' },
          { status: 409 }
        );
      }
    } else {
      // For login/verification, user must exist
      if (!user) {
        return NextResponse.json(
          { error: 'User not found' },
          { status: 404 }
        );
      }

      if (user.isBanned) {
        return NextResponse.json(
          { error: 'Account has been suspended' },
          { status: 403 }
        );
      }
    }

    // Send OTP
    let otpResult;
    const displayName = user?.displayName || body.displayName || 'User';

    if (method === 'email') {
      otpResult = await otpService.sendEmailOTP(
        identifier,
        displayName,
        purpose as any
      );
    } else {
      otpResult = await otpService.sendSMSOTP(
        identifier,
        purpose as any
      );
    }

    if (!otpResult.success) {
      logger.warn('Failed to resend OTP', {
        identifier: method === 'email' ? identifier : phoneNumber,
        method,
        purpose,
        error: otpResult.error,
        cooldownUntil: otpResult.cooldownUntil,
      });

      return NextResponse.json(
        { 
          error: otpResult.error,
          cooldownUntil: otpResult.cooldownUntil,
        },
        { status: 429 }
      );
    }

    // Track OTP resend
    analyticsService.track('otp_resent', {
      userId: user?._id.toString(),
      identifier: method === 'email' ? identifier : phoneNumber,
      method,
      purpose,
    });

    logger.info('OTP resent successfully', {
      userId: user?._id.toString(),
      identifier: method === 'email' ? identifier : phoneNumber,
      method,
      purpose,
      expiresAt: otpResult.expiresAt,
    });

    return NextResponse.json({
      message: 'Verification code resent successfully',
      method,
      expiresAt: otpResult.expiresAt,
      identifier: method === 'email' ? identifier : phoneNumber,
    });

  } catch (error) {
    logger.error('Resend OTP endpoint error', error);
    analyticsService.trackError(error as Error );
    
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    );
  }
}

// Apply rate limiting
export const middleware = [authMiddleware.otpRateLimit()];