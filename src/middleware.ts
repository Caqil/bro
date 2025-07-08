import { NextRequest, NextResponse } from 'next/server';
import { logger } from './lib/monitoring/logging';
import { corsConfig } from './lib/config/cors';
import { environmentConfig } from './lib/config/environment';

export function middleware(request: NextRequest) {
  const startTime = Date.now();
  const requestId = request.headers.get('x-request-id') || crypto.randomUUID();
  
  // Create response
  const response = NextResponse.next();
  
  // Add request ID to headers
  response.headers.set('X-Request-ID', requestId);
  
  // Add security headers
  addSecurityHeaders(response);
  
  // Handle CORS for API routes
  if (request.nextUrl.pathname.startsWith('/api/')) {
    handleCORS(request, response);
  }
  
  // Add monitoring headers
  response.headers.set('X-Response-Time', `${Date.now() - startTime}ms`);
  response.headers.set('X-API-Version', environmentConfig.getValue('APP_VERSION'));
  
  // Log request (in production, this might be too verbose)
  if (environmentConfig.isDevelopment()) {
    logger.info(`${request.method} ${request.nextUrl.pathname}`, {
      requestId,
      ip: getClientIP(request),
      userAgent: request.headers.get('user-agent') || undefined,
    });
  }
  
  return response;
}

function addSecurityHeaders(response: NextResponse) {
  const env = environmentConfig.get();
  
  // Content Security Policy
  const csp = [
    "default-src 'self'",
    "script-src 'self' 'unsafe-inline' 'unsafe-eval'",
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' data: https:",
    "font-src 'self' data:",
    "connect-src 'self' ws: wss:",
    "media-src 'self' blob:",
    "frame-src 'none'",
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
  ].join('; ');
  
  // Security headers
  const securityHeaders = {
    // Content Security Policy
    'Content-Security-Policy': csp,
    
    // Prevent MIME type sniffing
    'X-Content-Type-Options': 'nosniff',
    
    // XSS Protection
    'X-XSS-Protection': '1; mode=block',
    
    // Clickjacking protection
    'X-Frame-Options': 'DENY',
    
    // HTTPS enforcement (only in production)
    ...(env.NODE_ENV === 'production' && {
      'Strict-Transport-Security': 'max-age=31536000; includeSubDomains; preload',
    }),
    
    // Referrer policy
    'Referrer-Policy': 'strict-origin-when-cross-origin',
    
    // Feature policy
    'Permissions-Policy': [
      'camera=(self)',
      'microphone=(self)',
      'geolocation=(self)',
      'payment=()',
      'usb=()',
      'magnetometer=()',
      'gyroscope=()',
      'accelerometer=()',
    ].join(', '),
    
    // Server information hiding
    'X-Powered-By': 'ChatApp',
  };
  
  Object.entries(securityHeaders).forEach(([key, value]) => {
    response.headers.set(key, value);
  });
}

function handleCORS(request: NextRequest, response: NextResponse) {
  const origin = request.headers.get('origin');
  const corsHeaders = corsConfig.getCorsHeaders(origin || undefined);
  
  // Add CORS headers
  Object.entries(corsHeaders).forEach(([key, value]) => {
    response.headers.set(key, value);
  });
  
  // Handle preflight requests
  if (request.method === 'OPTIONS') {
    return new NextResponse(null, {
      status: 204,
      headers: corsHeaders,
    });
  }
}

function getClientIP(request: NextRequest): string {
  // Check various headers for the real IP
  const xForwardedFor = request.headers.get('x-forwarded-for');
  if (xForwardedFor) {
    return xForwardedFor.split(',')[0].trim();
  }
  
  const xRealIP = request.headers.get('x-real-ip');
  if (xRealIP) {
    return xRealIP;
  }
  
  const cfConnectingIP = request.headers.get('cf-connecting-ip');
  if (cfConnectingIP) {
    return cfConnectingIP;
  }
  
  // Fallback to a default IP
  return '127.0.0.1';
}

// Configuration for middleware
export const config = {
  matcher: [
    /*
     * Match all request paths except for the ones starting with:
     * - _next/static (static files)
     * - _next/image (image optimization files)
     * - favicon.ico (favicon file)
     * - public folder
     */
    '/((?!_next/static|_next/image|favicon.ico|public).*)',
  ],
};