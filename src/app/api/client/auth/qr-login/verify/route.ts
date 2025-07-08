import { NextRequest, NextResponse } from 'next/server';
import { qrAuthService } from '@/lib/auth/qr-auth';
import { authMiddleware } from '@/lib/auth/middleware';
import { logger } from '@/lib/monitoring/logging';
import { analyticsService } from '@/lib/monitoring/analytics';
import connectDB from '@/lib/database/mongodb';

export async function POST(request: NextRequest) {
  try {
    await connectDB();
    
    const body = await request.json();
    const { qrText, action = 'scan' } = body; // action: 'scan', 'confirm', 'reject'

    if (!qrText) {
      return NextResponse.json(
        { error: 'QR text is required' },
        { status: 400 }
      );
    }

    // Get user from authenticated middleware
    const userId = (request as any).user?.userId;
    if (!userId) {
      return NextResponse.json(
        { error: 'Authentication required' },
        { status: 401 }
      );
    }

    let result;
    
    switch (action) {
      case 'scan':
        result = await qrAuthService.scanQRCode(qrText, userId);
        
        if (result.success) {
          analyticsService.track('qr_login_scanned', {
            userId,
          }, { req: request });
        }
        
        break;
        
      case 'confirm':
        const { qrId } = body;
        const deviceId = body.deviceId || 'mobile-app';
        
        result = await qrAuthService.confirmQRLogin(qrId, userId, deviceId);
        
        if (result.success) {
          analyticsService.track('qr_login_confirmed', {
            userId,
            qrId,
          }, { req: request });
        }
        
        break;
        
      case 'reject':
        const { qrId: rejectQrId } = body;
        
        result = await qrAuthService.rejectQRLogin(rejectQrId, userId);
        
        if (result.success) {
          analyticsService.track('qr_login_rejected', {
            userId,
            qrId: rejectQrId,
          }, { req: request });
        }
        
        break;
        
      default:
        return NextResponse.json(
          { error: 'Invalid action' },
          { status: 400 }
        );
    }

    if (!result.success) {
      return NextResponse.json(
        { error: result.error },
        { status: 400 }
      );
    }

    logger.info(`QR login ${action} successful`, {
      userId,
      action,
    });

    return NextResponse.json({
      message: `QR ${action} successful`,
      ...result,
    });

  } catch (error) {
    logger.error('QR verify endpoint error', error);
    analyticsService.trackError(error as Error, { req: request });
    
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    );
  }
}

// Apply authentication middleware
export const middleware = [authMiddleware.authenticate()];
