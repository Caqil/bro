import { NextRequest, NextResponse } from 'next/server';
import { UserRepository } from '@/lib/database/repositories/user';
import { refreshTokenSchema } from '@/lib/database/schemas/auth';
import { jwtService } from '@/lib/auth/jwt';
import { logger } from '@/lib/monitoring/logging';
import { analyticsService } from '@/lib/monitoring/analytics';
import connectDB from '@/lib/database/mongodb';

export async function POST(request: NextRequest) {
  try {
    await connectDB();
    
    const body = await request.json();
    
    // Validate request body
    const validationResult = refreshTokenSchema.safeParse(body);
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

    const { refreshToken } = validationResult.data;
    
    // Verify refresh token
    const refreshResult = await jwtService.verifyRefreshToken(refreshToken);
    if (!refreshResult.valid || !refreshResult.payload) {
      analyticsService.track('refresh_token_invalid', {
        error: refreshResult.error,
      });

      return NextResponse.json(
        { error: refreshResult.error || 'Invalid refresh token' },
        { status: 401 }
      );
    }

    const { userId, deviceId } = refreshResult.payload;
    
    const userRepository = new UserRepository();
    
    // Get current user data
    const user = await userRepository.findById(userId);
    if (!user) {
      await jwtService.blacklistToken(refreshResult.payload.jti);
      return NextResponse.json(
        { error: 'User not found' },
        { status: 404 }
      );
    }

    // Check if user is banned
    if (user.isBanned) {
      // Invalidate all user tokens
      await jwtService.invalidateAllUserTokens(userId);
      
      analyticsService.track('refresh_token_banned_user', {
        userId,
        banReason: user.banReason,
      });

      return NextResponse.json(
        { error: 'Account has been suspended' },
        { status: 403 }
      );
    }

    // Generate new token pair
    const newTokens = await jwtService.refreshTokens(refreshToken, user);
    if (!newTokens) {
      return NextResponse.json(
        { error: 'Failed to refresh tokens' },
        { status: 401 }
      );
    }

    // Track successful token refresh
    analyticsService.track('token_refreshed', {
      userId,
      deviceId,
    });

    logger.debug('Tokens refreshed successfully', {
      userId,
      deviceId,
    });

    return NextResponse.json({
      message: 'Tokens refreshed successfully',
      tokens: {
        accessToken: newTokens.accessToken,
        refreshToken: newTokens.refreshToken,
        expiresIn: newTokens.expiresIn,
      },
    });

  } catch (error) {
    logger.error('Token refresh endpoint error', error);
    analyticsService.trackError(error as Error);
    
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    );
  }
}
