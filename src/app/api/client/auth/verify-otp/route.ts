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
    
    // Get registration data from the request or session
    // In a real implementation, you might store this in Redis during registration
    const { email, displayName, method } = body;

    if (!displayName) {
      return NextResponse.json(
        { error: 'Registration data not found. Please restart registration.' },
        { status: 400 }
      );
    }

    // Verify OTP
    const identifier = method === 'email' ? `email:${email}` : `sms:${sanitizedPhoneNumber}`;
    const otpResult = await otpService.verifyOTP(identifier, otp, 'registration');

    if (!otpResult.success) {
      // Track failed OTP verification
      analyticsService.track('otp_verification_failed', {
        phoneNumber: sanitizedPhoneNumber,
        method,
        error: otpResult.error,
        attemptsRemaining: otpResult.attemptsRemaining,
      }, { req: request });

      logger.warn('OTP verification failed', {
        phoneNumber: sanitizedPhoneNumber,
        method,
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

    const userRepository = new UserRepository();

    // Check if user was created between registration and verification
    const existingUser = await userRepository.findByPhoneNumber(sanitizedPhoneNumber);
    if (existingUser) {
      return NextResponse.json(
        { error: 'User already exists' },
        { status: 409 }
      );
    }

    // Create user
    const user = await userRepository.create({
      phoneNumber: sanitizedPhoneNumber,
      email: email ? DataSanitizer.sanitizeEmail(email) : undefined,
      displayName: DataSanitizer.sanitizePlainText(displayName),
      isVerified: true,
      isOnline: true,
      lastSeen: new Date(),
    });

    // Generate device ID for this session
    const deviceId = CryptoUtils.generateUUID();

    // Generate JWT tokens
    const tokens = jwtService.generateTokenPair(user, deviceId);

    // Track successful registration
    analyticsService.trackUserRegistration(user, request);

    logger.info('User registration completed successfully', {
      userId: user._id.toString(),
      phoneNumber: sanitizedPhoneNumber,
      hasEmail: !!email,
      method,
    });

    // Return user data and tokens
    return NextResponse.json({
      message: 'Registration completed successfully',
      user: {
        id: user._id.toString(),
        phoneNumber: user.phoneNumber,
        email: user.email,
        displayName: user.displayName,
        avatar: user.avatar,
        isVerified: user.isVerified,
      },
      tokens: {
        accessToken: tokens.accessToken,
        refreshToken: tokens.refreshToken,
        expiresIn: tokens.expiresIn,
      },
    });

  } catch (error) {
    logger.error('OTP verification endpoint error', error);
    analyticsService.trackError(error as Error, { req: request });
    
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    );
  }
}

// Apply rate limiting
export const middleware = [authMiddleware.otpRateLimit()];
