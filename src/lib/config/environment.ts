import { z } from 'zod';

// Create conditional schema based on environment
const createEnvSchema = (isDevelopment: boolean) => {
  const requiredInProduction = (schema: z.ZodSchema) => 
    isDevelopment ? schema.optional() : schema;

  return z.object({
    // Application
    NODE_ENV: z.enum(['development', 'production', 'test']).default('development'),
    PORT: z.string().transform(Number).default('3000'),
    APP_VERSION: z.string().default('1.0.0'),
    APP_NAME: z.string().default('ChatApp'),
    
    // URLs
    FRONTEND_URL: z.string().url().default('http://localhost:3000'),
    API_URL: z.string().url().default('http://localhost:3000/api'),
    WEBSITE_URL: z.string().url().default('https://chatapp.com'),
    
    // Database (always required)
    MONGODB_URI: z.string().min(1, 'MongoDB URI is required'),
    
    // Redis (optional)
    REDIS_URL: z.string().optional(),
    
    // JWT (always required but with defaults in dev)
    JWT_SECRET: isDevelopment 
      ? z.string().default('dev-jwt-secret-key-min-32-characters-long-for-development-only')
      : z.string().min(32, 'JWT secret must be at least 32 characters'),
    JWT_EXPIRES_IN: z.string().default('7d'),
    REFRESH_TOKEN_SECRET: isDevelopment
      ? z.string().default('dev-refresh-secret-key-min-32-characters-long-for-development-only')
      : z.string().min(32, 'Refresh token secret must be at least 32 characters'),
    REFRESH_TOKEN_EXPIRES_IN: z.string().default('30d'),
    
    // AWS S3 (optional in dev)
    AWS_REGION: z.string().default('us-east-1'),
    AWS_ACCESS_KEY_ID: requiredInProduction(z.string().default('dev-access-key')),
    AWS_SECRET_ACCESS_KEY: requiredInProduction(z.string().default('dev-secret-key')),
    AWS_S3_BUCKET: requiredInProduction(z.string().default('dev-bucket')),
    AWS_S3_ENDPOINT: z.string().optional(),
    
    // SMTP (optional in dev)
    SMTP_HOST: z.string().default('smtp.gmail.com'),
    SMTP_PORT: z.string().transform(Number).default('587'),
    SMTP_SECURE: z.string().transform(val => val === 'true').default('false'),
    SMTP_USER: requiredInProduction(z.string().default('dev@example.com')),
    SMTP_PASS: requiredInProduction(z.string().default('dev-password')),
    SMTP_FROM: z.string().email().default('noreply@chatapp.com'),
    SMTP_REPLY_TO: z.string().email().optional(),
    
    // Twilio SMS (optional in dev)
    TWILIO_ACCOUNT_SID: requiredInProduction(z.string().default('dev-account-sid')),
    TWILIO_AUTH_TOKEN: requiredInProduction(z.string().default('dev-auth-token')),
    TWILIO_FROM_NUMBER: requiredInProduction(z.string().default('+1234567890')),
    
    // Firebase (optional in dev)
    FIREBASE_PROJECT_ID: requiredInProduction(z.string().default('dev-project')),
    FIREBASE_PRIVATE_KEY: requiredInProduction(z.string().default('dev-private-key')),
    FIREBASE_CLIENT_EMAIL: requiredInProduction(z.string().email().default('dev@example.com')),
    
    // CoTURN (optional in dev)
    COTURN_PRIMARY_HOST: z.string().default('turn.example.com'),
    COTURN_SECRET: requiredInProduction(z.string().default('dev-coturn-secret')),
    COTURN_REALM: z.string().default('chatapp.com'),
    COTURN_TTL: z.string().transform(Number).default('86400'),
    COTURN_FALLBACK_HOSTS: z.string().optional(),
    
    // Logging
    LOG_LEVEL: z.enum(['error', 'warn', 'info', 'debug']).default('info'),
    
    // Rate Limiting
    RATE_LIMIT_REDIS_URL: z.string().optional(),
    RATE_LIMIT_WINDOW_MS: z.string().transform(Number).default('900000'),
    RATE_LIMIT_MAX_REQUESTS: z.string().transform(Number).default('100'),
    
    // Security
    ENCRYPTION_KEY: isDevelopment
      ? z.string().default('dev-encryption-key-must-be-at-least-64-characters-long-for-secure-operations')
      : z.string().min(64, 'Encryption key must be at least 64 characters'),
    CORS_ORIGIN: z.string().optional(),
    ALLOWED_ORIGINS: z.string().optional(),
    
    // Monitoring
    ANALYTICS_ENABLED: z.string().transform(val => val === 'true').default('true'),
    METRICS_ENABLED: z.string().transform(val => val === 'true').default('true'),
    HEALTH_CHECK_ENABLED: z.string().transform(val => val === 'true').default('true'),
  });
};

export type Environment = z.infer<ReturnType<typeof createEnvSchema>>;

class EnvironmentConfig {
  private config: Environment;
  private isLoaded = false;

  constructor() {
    this.config = this.loadConfig();
  }

  private loadConfig(): Environment {
    try {
      const isDevelopment = process.env.NODE_ENV !== 'production';
      const envSchema = createEnvSchema(isDevelopment);
      
      // Load from process.env
      const rawConfig = {
        NODE_ENV: process.env.NODE_ENV,
        PORT: process.env.PORT,
        APP_VERSION: process.env.APP_VERSION,
        APP_NAME: process.env.APP_NAME,
        
        FRONTEND_URL: process.env.FRONTEND_URL,
        API_URL: process.env.API_URL,
        WEBSITE_URL: process.env.WEBSITE_URL,
        
        MONGODB_URI: process.env.MONGODB_URI,
        REDIS_URL: process.env.REDIS_URL,
        
        JWT_SECRET: process.env.JWT_SECRET,
        JWT_EXPIRES_IN: process.env.JWT_EXPIRES_IN,
        REFRESH_TOKEN_SECRET: process.env.REFRESH_TOKEN_SECRET,
        REFRESH_TOKEN_EXPIRES_IN: process.env.REFRESH_TOKEN_EXPIRES_IN,
        
        AWS_REGION: process.env.AWS_REGION,
        AWS_ACCESS_KEY_ID: process.env.AWS_ACCESS_KEY_ID,
        AWS_SECRET_ACCESS_KEY: process.env.AWS_SECRET_ACCESS_KEY,
        AWS_S3_BUCKET: process.env.AWS_S3_BUCKET,
        AWS_S3_ENDPOINT: process.env.AWS_S3_ENDPOINT,
        
        SMTP_HOST: process.env.SMTP_HOST,
        SMTP_PORT: process.env.SMTP_PORT,
        SMTP_SECURE: process.env.SMTP_SECURE,
        SMTP_USER: process.env.SMTP_USER,
        SMTP_PASS: process.env.SMTP_PASS,
        SMTP_FROM: process.env.SMTP_FROM,
        SMTP_REPLY_TO: process.env.SMTP_REPLY_TO,
        
        TWILIO_ACCOUNT_SID: process.env.TWILIO_ACCOUNT_SID,
        TWILIO_AUTH_TOKEN: process.env.TWILIO_AUTH_TOKEN,
        TWILIO_FROM_NUMBER: process.env.TWILIO_FROM_NUMBER,
        
        FIREBASE_PROJECT_ID: process.env.FIREBASE_PROJECT_ID,
        FIREBASE_PRIVATE_KEY: process.env.FIREBASE_PRIVATE_KEY,
        FIREBASE_CLIENT_EMAIL: process.env.FIREBASE_CLIENT_EMAIL,
        
        COTURN_PRIMARY_HOST: process.env.COTURN_PRIMARY_HOST,
        COTURN_SECRET: process.env.COTURN_SECRET,
        COTURN_REALM: process.env.COTURN_REALM,
        COTURN_TTL: process.env.COTURN_TTL,
        COTURN_FALLBACK_HOSTS: process.env.COTURN_FALLBACK_HOSTS,
        
        LOG_LEVEL: process.env.LOG_LEVEL,
        
        RATE_LIMIT_REDIS_URL: process.env.RATE_LIMIT_REDIS_URL,
        RATE_LIMIT_WINDOW_MS: process.env.RATE_LIMIT_WINDOW_MS,
        RATE_LIMIT_MAX_REQUESTS: process.env.RATE_LIMIT_MAX_REQUESTS,
        
        ENCRYPTION_KEY: process.env.ENCRYPTION_KEY,
        CORS_ORIGIN: process.env.CORS_ORIGIN,
        ALLOWED_ORIGINS: process.env.ALLOWED_ORIGINS,
        
        ANALYTICS_ENABLED: process.env.ANALYTICS_ENABLED,
        METRICS_ENABLED: process.env.METRICS_ENABLED,
        HEALTH_CHECK_ENABLED: process.env.HEALTH_CHECK_ENABLED,
      };

      const config = envSchema.parse(rawConfig);
      this.isLoaded = true;
      
      console.log(`✅ Environment configuration loaded successfully`, {
        environment: config.NODE_ENV,
        port: config.PORT,
        version: config.APP_VERSION,
      });

      return config;
    } catch (error) {
      console.error('❌ Failed to load environment configuration:', error);
      
      if (error instanceof z.ZodError) {
        console.error('Validation errors:');
        error.errors.forEach(err => {
          console.error(`  - ${err.path.join('.')}: ${err.message}`);
        });
      }
      
      throw new Error('Invalid environment configuration');
    }
  }

  // Get configuration
  get(): Environment {
    if (!this.isLoaded) {
      throw new Error('Configuration not loaded');
    }
    return this.config;
  }

  // Get specific config value
  getValue<K extends keyof Environment>(key: K): Environment[K] {
    return this.get()[key];
  }

  // Check if in development
  isDevelopment(): boolean {
    return this.getValue('NODE_ENV') === 'development';
  }

  // Check if in production
  isProduction(): boolean {
    return this.getValue('NODE_ENV') === 'production';
  }

  // Check if in test
  isTest(): boolean {
    return this.getValue('NODE_ENV') === 'test';
  }

  // Check if service is properly configured
  isServiceConfigured(service: 'smtp' | 'twilio' | 'aws' | 'firebase' | 'redis'): boolean {
    const config = this.get();
    
    switch (service) {
      case 'smtp':
        return !!(config.SMTP_USER && config.SMTP_PASS && config.SMTP_USER !== 'dev@example.com');
      case 'twilio':
        return !!(config.TWILIO_ACCOUNT_SID && config.TWILIO_AUTH_TOKEN && config.TWILIO_ACCOUNT_SID !== 'dev-account-sid');
      case 'aws':
        return !!(config.AWS_ACCESS_KEY_ID && config.AWS_SECRET_ACCESS_KEY && config.AWS_ACCESS_KEY_ID !== 'dev-access-key');
      case 'firebase':
        return !!(config.FIREBASE_PROJECT_ID && config.FIREBASE_PRIVATE_KEY && config.FIREBASE_PROJECT_ID !== 'dev-project');
      case 'redis':
        return !!config.REDIS_URL;
      default:
        return false;
    }
  }

  // Validate required services (only in production)
  validateServices(): void {
    if (!this.isProduction()) {
      console.log('⚠️  Development mode: Skipping service validation');
      return;
    }

    const config = this.get();
    const errors: string[] = [];

    // Check required environment variables for production
    if (!config.MONGODB_URI) {
      errors.push('MONGODB_URI is required');
    }

    if (!config.JWT_SECRET || config.JWT_SECRET.length < 32) {
      errors.push('JWT_SECRET must be at least 32 characters');
    }

    if (!config.ENCRYPTION_KEY || config.ENCRYPTION_KEY.length < 64) {
      errors.push('ENCRYPTION_KEY must be at least 64 characters');
    }

    if (errors.length > 0) {
      throw new Error(`Production configuration validation failed:\n${errors.join('\n')}`);
    }

    console.log('✅ Production service configurations validated successfully');
  }

  // Get database configuration
  getDatabaseConfig() {
    const config = this.get();
    return {
      uri: config.MONGODB_URI,
      options: {
        maxPoolSize: 10,
        serverSelectionTimeoutMS: 5000,
        socketTimeoutMS: 45000,
        bufferCommands: false,
        bufferMaxEntries: 0,
      },
    };
  }

  // Get Redis configuration
  getRedisConfig() {
    const config = this.get();
    return config.REDIS_URL ? {
      url: config.REDIS_URL,
      maxRetriesPerRequest: 3,
      retryDelayOnFailover: 100,
      enableReadyCheck: false,
    } : null;
  }

  // Get JWT configuration
  getJWTConfig() {
    const config = this.get();
    return {
      secret: config.JWT_SECRET,
      expiresIn: config.JWT_EXPIRES_IN,
      refreshSecret: config.REFRESH_TOKEN_SECRET,
      refreshExpiresIn: config.REFRESH_TOKEN_EXPIRES_IN,
    };
  }
}

export const environmentConfig = new EnvironmentConfig();