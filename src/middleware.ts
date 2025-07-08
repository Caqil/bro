import { NextRequest, NextResponse } from 'next/server';
import { edgeLogger } from './lib/monitoring/edge-logger'; // Use edge logger
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
  const responseTime = Date.now() - startTime;
  response.headers.set('X-Response-Time', `${responseTime}ms`);
  response.headers.set('X-API-Version', process.env.APP_VERSION || '1.0.0');
  
  // Log request using edge logger
  if (process.env.NODE_ENV === 'development') {
    edgeLogger.logRequest(request, response.status || 200, responseTime);
  }
  
  return response;
}

function addSecurityHeaders(response: NextResponse) {
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
    'Content-Security-Policy': csp,
    'X-Content-Type-Options': 'nosniff',
    'X-XSS-Protection': '1; mode=block',
    'X-Frame-Options': 'DENY',
    'Referrer-Policy': 'strict-origin-when-cross-origin',
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
    'X-Powered-By': 'ChatApp',
  };
  
  // Add HTTPS enforcement in production
  if (process.env.NODE_ENV === 'production') {
    securityHeaders['Strict-Transport-Security'] = 'max-age=31536000; includeSubDomains; preload';
  }
  
  Object.entries(securityHeaders).forEach(([key, value]) => {
    response.headers.set(key, value);
  });
}

function handleCORS(request: NextRequest, response: NextResponse) {
  const origin = request.headers.get('origin');
  const corsHeaders = getCorsHeaders(origin || undefined);
  
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

function getCorsHeaders(origin?: string): Record<string, string> {
  const headers: Record<string, string> = {};
  
  // Simple CORS for development
  if (process.env.NODE_ENV === 'development') {
    headers['Access-Control-Allow-Origin'] = '*';
  } else if (origin) {
    // Add your production origins here
    const allowedOrigins = [
      process.env.FRONTEND_URL,
      process.env.WEBSITE_URL,
    ].filter(Boolean);
    
    if (allowedOrigins.includes(origin)) {
      headers['Access-Control-Allow-Origin'] = origin;
    }
  }
  
  headers['Access-Control-Allow-Methods'] = 'GET, POST, PUT, DELETE, PATCH, OPTIONS';
  headers['Access-Control-Allow-Headers'] = 'Origin, X-Requested-With, Content-Type, Accept, Authorization, X-Request-ID';
  headers['Access-Control-Allow-Credentials'] = 'true';
  headers['Access-Control-Max-Age'] = '86400';
  
  return headers;
}

// Configuration for middleware
export const config = {
  matcher: [
    '/((?!_next/static|_next/image|favicon.ico|public).*)',
  ],
};