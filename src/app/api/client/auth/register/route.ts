import { NextRequest, NextResponse } from 'next/server';
import { UserRepository } from '@/lib/database/repositories/user';
import { registerSchema } from '@/lib/database/schemas/auth';
import { otpService } from '@/lib/auth/otp';
import { validateBody } from '@/lib/security/validation';
import { DataSanitizer } from '@/lib/security/sanitization';
import { ErrorHandler } from '@/lib/utils/error-handler';
import { logger } from '@/lib/monitoring/logging';
import { analyticsService } from '@/lib/monitoring/analytics';
import { authMiddleware } from '@/lib/auth/middleware';
import connectDB from '@/lib/database/mongodb';
import type { OTPSendResult } from '@/lib/auth/otp';

export async function POST(request: NextRequest) {
  try {
    await connectDB();
    
    const body = await request.json();
    
    // Validate request body
    const validationResult = registerSchema.safeParse(body);
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

    const { phoneNumber, email, displayName } = validationResult.data;
    
    // Sanitize inputs
    const sanitizedData = {
      phoneNumber: DataSanitizer.sanitizePhoneNumber(phoneNumber),
      email: email ? DataSanitizer.sanitizeEmail(email) : undefined,
      displayName: DataSanitizer.sanitizePlainText(displayName),
    };

    const userRepository = new UserRepository();

    // Check if user already exists
    const existingUser = await userRepository.findByPhoneNumber(sanitizedData.phoneNumber);
    if (existingUser) {
      // Track registration attempt for existing user
      analyticsService.track('registration_attempt_existing_user', {
        phoneNumber: sanitizedData.phoneNumber,
        hasEmail: !!sanitizedData.email,
      }, { req: request });

      return NextResponse.json(
        { error: 'User with this phone number already exists' },
        { status: 409 }
      );
    }

    // Check email if provided
    if (sanitizedData.email) {
      const existingEmailUser = await userRepository.findByEmail(sanitizedData.email);
      if (existingEmailUser) {
        return NextResponse.json(
          { error: 'User with this email already exists' },
          { status: 409 }
        );
      }
    }

    // Send OTP based on preferred method
    const otpMethod = sanitizedData.email ? 'email' : 'sms';
    let otpResult: OTPSendResult;

    if (otpMethod === 'email' && sanitizedData.email) {
      otpResult = await otpService.sendEmailOTP(
        sanitizedData.email,
        sanitizedData.displayName,
        'registration'
      );
    } else {
      otpResult = await otpService.sendSMSOTP(
        sanitizedData.phoneNumber,
        'registration'
      );
    }

    if (!otpResult.success) {
      logger.error('Failed to send registration OTP', new Error(otpResult.error), {
        phoneNumber: sanitizedData.phoneNumber,
        email: sanitizedData.email,
        method: otpMethod,
      });

      return NextResponse.json(
        { error: 'Failed to send verification code' },
        { status: 500 }
      );
    }

    // Store registration data temporarily (you might want to use Redis for this)
    const registrationData = {
      phoneNumber: sanitizedData.phoneNumber,
      email: sanitizedData.email,
      displayName: sanitizedData.displayName,
      method: otpMethod,
    };

    // Track successful registration initiation
    analyticsService.track('registration_initiated', {
      phoneNumber: sanitizedData.phoneNumber,
      hasEmail: !!sanitizedData.email,
      method: otpMethod,
    }, { req: request });

    logger.info('Registration initiated successfully', {
      phoneNumber: sanitizedData.phoneNumber,
      method: otpMethod,
      expiresAt: otpResult.expiresAt,
    });

    return NextResponse.json({
      message: 'Verification code sent successfully',
      method: otpMethod,
      expiresAt: otpResult.expiresAt,
      identifier: otpMethod === 'email' ? sanitizedData.email : sanitizedData.phoneNumber,
    });

  } catch (error) {
    logger.error('Registration endpoint error', error);
    analyticsService.trackError(error as Error, { req: request });
    
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    );
  }
}

// Apply rate limiting
export const middleware = [authMiddleware.authRateLimit()];
