import { NextRequest, NextResponse } from 'next/server';
import { qrAuthService } from '@/lib/auth/qr-auth';
import { logger } from '@/lib/monitoring/logging';
import { analyticsService } from '@/lib/monitoring/analytics';

export async function GET(request: NextRequest) {
  try {
    const userAgent = request.headers.get('user-agent') || 'unknown';
    const ip = request.headers.get('x-forwarded-for')?.split(',')[0] || 
               request.headers.get('x-real-ip') || 
               'unknown';

    // Generate QR code
    const qrResult = await qrAuthService.generateQRCode({
      platform: 'web',
      userAgent,
      ip,
    });

    // Track QR generation
    analyticsService.track('qr_login_generated', {
      qrId: qrResult.qrId,
      platform: 'web',
      userAgent,
    }, { req: request });

    logger.info('QR code generated for web login', {
      qrId: qrResult.qrId,
      userAgent,
      expiresAt: qrResult.expiresAt,
    });

    return NextResponse.json({
      qrId: qrResult.qrId,
      qrCode: qrResult.qrCode,
      expiresAt: qrResult.expiresAt,
      checkUrl: qrResult.checkUrl,
    });

  } catch (error) {
    logger.error('QR generation endpoint error', error);
    analyticsService.trackError(error as Error, { req: request });
    
    return NextResponse.json(
      { error: 'Failed to generate QR code' },
      { status: 500 }
    );
  }
}
