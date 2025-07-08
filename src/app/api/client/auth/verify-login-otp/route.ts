import { NextRequest, NextResponse } from 'next/server';
import { UserRepository } from '@/lib/database/repositories/user';
import { verifyOTPSchema } from '@/lib/database/schemas/auth';
import { otpService } from '@/lib/auth/otp';
import { jwtService } from '@/lib/auth/jwt';
import { DataSanitizer } from '@/lib/security/sanitization';
import { logger } from '@/lib/monitoring/logging';
import { analyticsService } from '@/lib/monitoring/analytics';
import { CryptoUtils } from '@/lib/utils/crypto';
import connectDB from '@/lib/database/mongodb';

export async function POST(request: NextRequest) {
  try {
    await connectDB();
    
    const body = await request.json();
    
    // Validate request body
    const validationResult = verifyOTPSchema.safeParse(body);
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

    const { phoneNumber, otp } = validationResult.data;
    const sanitizedPhoneNumber = DataSanitizer.sanitizePhoneNumber(phoneNumber);
    
    const userRepository = new UserRepository();

    // Get user
    const user = await userRepository.findByPhoneNumber(sanitizedPhoneNumber);
    if (!user) {
      return NextResponse.json(
        { error: 'User not found' },
        { status: 404 }
      );
    }

    // Check if user is banned
    if (user.isBanned) {
      return NextResponse.json(
        { error: 'Account has been suspended' },
        { status: 403 }
      );
    }

    // Verify OTP
    const otpMethod = user.email ? 'email' : 'sms';
    const identifier = otpMethod === 'email' ? `email:${user.email}` : `sms:${sanitizedPhoneNumber}`;
    const otpResult = await otpService.verifyOTP(identifier, otp, 'login');

    if (!otpResult.success) {
      // Track failed login OTP verification
      analyticsService.track('login_otp_verification_failed', {
        userId: user._id.toString(),
        phoneNumber: sanitizedPhoneNumber,
        method: otpMethod,
        error: otpResult.error,
        attemptsRemaining: otpResult.attemptsRemaining,
      }, { req: request });

      logger.warn('Login OTP verification failed', {
        userId: user._id.toString(),
        phoneNumber: sanitizedPhoneNumber,
        method: otpMethod,
        error: otpResult.error,
        attemptsRemaining: otpResult.attemptsRemaining,
      });

      return NextResponse.json(
        {
          error: otpResult.error,
          attemptsRemaining: otpResult.attemptsRemaining,
        },
        { status: 400 }
      );
    }

    // Generate device ID for this session
    const deviceId = body.deviceId || CryptoUtils.generateUUID();

    // Update user online status
    await userRepository.updateOnlineStatus(user._id, true);

    // Generate JWT tokens
    const tokens = jwtService.generateTokenPair(user, deviceId);

    // Track successful login
    analyticsService.trackUserLogin(user, 'otp', request);

    logger.info('User login completed successfully', {
      userId: user._id.toString(),
      phoneNumber: sanitizedPhoneNumber,
      method: otpMethod,
      deviceId,
    });

    // Return user data and tokens
    return NextResponse.json({
      message: 'Login successful',
      user: {
        id: user._id.toString(),
        phoneNumber: user.phoneNumber,
        email: user.email,
        displayName: user.displayName,
        avatar: user.avatar,
        status: user.status,
        isVerified: user.isVerified,
        privacySettings: user.privacySettings,
        notificationSettings: user.notificationSettings,
      },
      tokens: {
        accessToken: tokens.accessToken,
        refreshToken: tokens.refreshToken,
        expiresIn: tokens.expiresIn,
      },
      deviceId,
    });

  } catch (error) {
    logger.error('Login OTP verification endpoint error', error);
    analyticsService.trackError(error as Error, { req: request });
    
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    );
  }
}

// Apply rate limiting
export const middleware = [authMiddleware.otpRateLimit()];
