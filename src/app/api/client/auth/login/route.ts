import { NextRequest, NextResponse } from 'next/server';
import { UserRepository } from '@/lib/database/repositories/user';
import { loginSchema } from '@/lib/database/schemas/auth';
import { otpService } from '@/lib/auth/otp';
import { DataSanitizer } from '@/lib/security/sanitization';
import { logger } from '@/lib/monitoring/logging';
import { analyticsService } from '@/lib/monitoring/analytics';
import connectDB from '@/lib/database/mongodb';

export async function POST(request: NextRequest) {
  try {
    await connectDB();
    
    const body = await request.json();
    
    // Validate request body
    const validationResult = loginSchema.safeParse(body);
    if (!validationResult.success) {
      return NextResponse.json(
        {
          error: 'Validation failed',
          details: validationResult.error.errors.map(err => ({
            field: err.path.join('.'),
            message: err.message,
          })),
        },
        { status: 400 }
      );
    }

    const { phoneNumber } = validationResult.data;
    const sanitizedPhoneNumber = DataSanitizer.sanitizePhoneNumber(phoneNumber);
    
    const userRepository = new UserRepository();

    // Check if user exists
    const user = await userRepository.findByPhoneNumber(sanitizedPhoneNumber);
    if (!user) {
      // Track login attempt for non-existent user
      analyticsService.track('login_attempt_non_existent_user', {
        phoneNumber: sanitizedPhoneNumber,
      }, { req: request });

      return NextResponse.json(
        { error: 'User not found. Please register first.' },
        { status: 404 }
      );
    }

    // Check if user is banned
    if (user.isBanned) {
      analyticsService.track('login_attempt_banned_user', {
        userId: user._id.toString(),
        phoneNumber: sanitizedPhoneNumber,
        banReason: user.banReason,
      }, { req: request });

      logger.warn('Login attempt by banned user', {
        userId: user._id.toString(),
        phoneNumber: sanitizedPhoneNumber,
        banReason: user.banReason,
      });

      return NextResponse.json(
        { error: 'Account has been suspended' },
        { status: 403 }
      );
    }

    // Determine OTP method (prefer email if available)
    const otpMethod = user.email ? 'email' : 'sms';
    let otpResult;

    if (otpMethod === 'email' && user.email) {
      otpResult = await otpService.sendEmailOTP(
        user.email,
        user.displayName,
        'login'
      );
    } else {
      otpResult = await otpService.sendSMSOTP(
        sanitizedPhoneNumber,
        'login'
      );
    }

    if (!otpResult.success) {
      logger.error('Failed to send login OTP', new Error(otpResult.error), {
        userId: user._id.toString(),
        phoneNumber: sanitizedPhoneNumber,
        method: otpMethod,
      });

      return NextResponse.json(
        { 
          error: 'Failed to send verification code',
          cooldownUntil: otpResult.cooldownUntil,
        },
        { status: 500 }
      );
    }

    // Track successful login OTP sent
    analyticsService.track('login_otp_sent', {
      userId: user._id.toString(),
      phoneNumber: sanitizedPhoneNumber,
      method: otpMethod,
    }, { req: request });

    logger.info('Login OTP sent successfully', {
      userId: user._id.toString(),
      phoneNumber: sanitizedPhoneNumber,
      method: otpMethod,
      expiresAt: otpResult.expiresAt,
    });

    return NextResponse.json({
      message: 'Verification code sent successfully',
      method: otpMethod,
      expiresAt: otpResult.expiresAt,
      identifier: otpMethod === 'email' ? user.email : sanitizedPhoneNumber,
      user: {
        id: user._id.toString(),
        displayName: user.displayName,
        avatar: user.avatar,
      },
    });

  } catch (error) {
    logger.error('Login endpoint error', error);
    analyticsService.trackError(error as Error, { req: request });
    
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    );
  }
}

// Apply rate limiting
export const middleware = [authMiddleware.authRateLimit()];
