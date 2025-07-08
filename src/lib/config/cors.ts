import { environmentConfig } from './environment';

export interface CorsConfig {
  origin: string[] | boolean;
  methods: string[];
  allowedHeaders: string[];
  exposedHeaders: string[];
  credentials: boolean;
  maxAge: number;
  preflightContinue: boolean;
  optionsSuccessStatus: number;
}

class CorsConfiguration {
  private config: CorsConfig;

  constructor() {
    this.config = this.createConfig();
  }

  private createConfig(): CorsConfig {
    const env = environmentConfig.get();
    
    // Parse allowed origins
    const getAllowedOrigins = (): string[] => {
      const origins: string[] = [];
      
      // Add frontend URL
      if (env.FRONTEND_URL) {
        origins.push(env.FRONTEND_URL);
      }
      
      // Add website URL
      if (env.WEBSITE_URL) {
        origins.push(env.WEBSITE_URL);
      }
      
      // Add CORS_ORIGIN if specified
      if (env.CORS_ORIGIN) {
        origins.push(env.CORS_ORIGIN);
      }
      
      // Add ALLOWED_ORIGINS if specified
      if (env.ALLOWED_ORIGINS) {
        const additionalOrigins = env.ALLOWED_ORIGINS.split(',').map(o => o.trim());
        origins.push(...additionalOrigins);
      }
      
      // Add localhost for development
      if (env.NODE_ENV === 'development') {
        origins.push('http://localhost:3000');
        origins.push('http://localhost:3001');
        origins.push('http://127.0.0.1:3000');
        origins.push('http://127.0.0.1:3001');
      }
      
      return [...new Set(origins)]; // Remove duplicates
    };

    return {
      origin: env.NODE_ENV === 'development' ? true : getAllowedOrigins(),
      methods: ['GET', 'POST', 'PUT', 'DELETE', 'PATCH', 'OPTIONS'],
      allowedHeaders: [
        'Origin',
        'X-Requested-With',
        'Content-Type',
        'Accept',
        'Authorization',
        'X-Request-ID',
        'X-Correlation-ID',
        'App-Version',
        'User-Agent',
      ],
      exposedHeaders: [
        'X-Request-ID',
        'X-RateLimit-Limit',
        'X-RateLimit-Remaining',
        'X-RateLimit-Reset',
      ],
      credentials: true,
      maxAge: 86400, // 24 hours
      preflightContinue: false,
      optionsSuccessStatus: 204,
    };
  }

  getConfig(): CorsConfig {
    return this.config;
  }

  // Check if origin is allowed
  isOriginAllowed(origin: string): boolean {
    const { origin: allowedOrigins } = this.config;
    
    if (allowedOrigins === true) {
      return true;
    }
    
    if (Array.isArray(allowedOrigins)) {
      return allowedOrigins.includes(origin);
    }
    
    return false;
  }

  // Get CORS headers for manual implementation
  getCorsHeaders(origin?: string): Record<string, string> {
    const headers: Record<string, string> = {};
    
    if (origin && this.isOriginAllowed(origin)) {
      headers['Access-Control-Allow-Origin'] = origin;
    }
    
    headers['Access-Control-Allow-Methods'] = this.config.methods.join(', ');
    headers['Access-Control-Allow-Headers'] = this.config.allowedHeaders.join(', ');
    headers['Access-Control-Expose-Headers'] = this.config.exposedHeaders.join(', ');
    headers['Access-Control-Allow-Credentials'] = this.config.credentials.toString();
    headers['Access-Control-Max-Age'] = this.config.maxAge.toString();
    
    return headers;
  }
}

export const corsConfig = new CorsConfiguration();
