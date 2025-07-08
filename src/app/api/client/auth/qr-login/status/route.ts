import { NextRequest, NextResponse } from 'next/server';
import { qrAuthService } from '@/lib/auth/qr-auth';
import { logger } from '@/lib/monitoring/logging';

export async function GET(request: NextRequest) {
  try {
    const { searchParams } = new URL(request.url);
    const qrId = searchParams.get('qrId');

    if (!qrId) {
      return NextResponse.json(
        { error: 'QR ID is required' },
        { status: 400 }
      );
    }

    // Get QR status
    const status = await qrAuthService.getQRStatus(qrId);

    if (!status) {
      return NextResponse.json(
        { error: 'QR session not found or expired' },
        { status: 404 }
      );
    }

    return NextResponse.json({
      status: status.status,
      userInfo: status.userInfo,
      expiresAt: status.expiresAt,
    });

  } catch (error) {
    logger.error('QR status endpoint error', error);
    
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    );
  }
}