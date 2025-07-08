import { NextRequest, NextResponse } from 'next/server';
import { UserRepository } from '@/lib/database/repositories/user';
import { jwtService } from '@/lib/auth/jwt';
import { authMiddleware } from '@/lib/auth/middleware';
import { logger } from '@/lib/monitoring/logging';
import { analyticsService } from '@/lib/monitoring/analytics';
import connectDB from '@/lib/database/mongodb';

export async function POST(request: NextRequest) {
  try {
    await connectDB();
    
    // Extract token for blacklisting
    const token = request.headers.get('authorization')?.replace('Bearer ', '') || 
                  request.cookies.get('accessToken')?.value;

    if (!token) {
      return NextResponse.json(
        { error: 'No token provided' },
        { status: 400 }
      );
    }

    // Get token info
    const tokenInfo = await jwtService.getTokenInfo(token);
    if (!tokenInfo.valid || tokenInfo.type !== 'access') {
      return NextResponse.json(
        { error: 'Invalid token' },
        { status: 401 }
      );
    }

    const { userId, deviceId, jti } = tokenInfo.payload;
    
    // Get logout type from body
    const body = await request.json().catch(() => ({}));
    const { logoutType = 'current' } = body; // 'current', 'device', 'all'

    const userRepository = new UserRepository();

    // Update user offline status
    await userRepository.updateOnlineStatus(userId, false);

    // Handle different logout types
    switch (logoutType) {
      case 'current':
        // Blacklist current token only
        await jwtService.blacklistToken(jti);
        break;
        
      case 'device':
        // Invalidate all tokens for current device
        await jwtService.invalidateDeviceTokens(userId, deviceId);
        break;
        
      case 'all':
        // Invalidate all user tokens
        await jwtService.invalidateAllUserTokens(userId);
        break;
        
      default:
        await jwtService.blacklistToken(jti);
    }

    // Track logout
    analyticsService.track('user_logout', {
      userId,
      deviceId,
      logoutType,
    }, { req: request });

    logger.info('User logged out successfully', {
      userId,
      deviceId,
      logoutType,
    });

    return NextResponse.json({
      message: 'Logged out successfully',
      logoutType,
    });

  } catch (error) {
    logger.error('Logout endpoint error', error);
    analyticsService.trackError(error as Error, { req: request });
    
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    );
  }
}
