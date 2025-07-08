import { z } from 'zod';
import { logger } from '../monitoring/logging';

// Environment schema validation
const envSchema = z.object({
  // Application
  NODE_ENV: z.enum(['development', 'production', 'test']).default('development'),
  PORT: z.string().transform(Number).default('3000'),
  APP_VERSION: z.string().default('1.0.0'),
  APP_NAME: z.string().default('ChatApp'),
  
  // URLs
  FRONTEND_URL: z.string().url().default('http://localhost:3000'),
  API_URL: z.string().url().default('http://localhost:3000/api'),
  WEBSITE_URL: z.string().url().default('https://chatapp.com'),
  
  // Database
  MONGODB_URI: z.string().min(1, 'MongoDB URI is required'),
  
  // Redis (optional)
  REDIS_URL: z.string().optional(),
  
  // JWT
  JWT_SECRET: z.string().min(32, 'JWT secret must be at least 32 characters'),
  JWT_EXPIRES_IN: z.string().default('7d'),
  REFRESH_TOKEN_SECRET: z.string().min(32, 'Refresh token secret must be at least 32 characters'),
  REFRESH_TOKEN_EXPIRES_IN: z.string().default('30d'),
  
  // AWS S3
  AWS_REGION: z.string().default('us-east-1'),
  AWS_ACCESS_KEY_ID: z.string().min(1, 'AWS Access Key ID is required'),
  AWS_SECRET_ACCESS_KEY: z.string().min(1, 'AWS Secret Access Key is required'),
  AWS_S3_BUCKET: z.string().min(1, 'AWS S3 bucket is required'),
  AWS_S3_ENDPOINT: z.string().optional(),
  
  // SMTP
  SMTP_HOST: z.string().default('smtp.gmail.com'),
  SMTP_PORT: z.string().transform(Number).default('587'),
  SMTP_SECURE: z.string().transform(val => val === 'true').default('false'),
  SMTP_USER: z.string().min(1, 'SMTP user is required'),
  SMTP_PASS: z.string().min(1, 'SMTP password is required'),
  SMTP_FROM: z.string().email().default('noreply@chatapp.com'),
  SMTP_REPLY_TO: z.string().email().optional(),
  
  // Twilio (SMS)
  TWILIO_ACCOUNT_SID: z.string().min(1, 'Twilio Account SID is required'),
  TWILIO_AUTH_TOKEN: z.string().min(1, 'Twilio Auth Token is required'),
  TWILIO_FROM_NUMBER: z.string().min(1, 'Twilio From Number is required'),
  
  // Firebase (Push Notifications)
  FIREBASE_PROJECT_ID: z.string().min(1, 'Firebase Project ID is required'),
  FIREBASE_PRIVATE_KEY: z.string().min(1, 'Firebase Private Key is required'),
  FIREBASE_CLIENT_EMAIL: z.string().email('Firebase Client Email must be valid'),
  
  // CoTURN
  COTURN_PRIMARY_HOST: z.string().default('turn.example.com'),
  COTURN_SECRET: z.string().min(1, 'CoTURN secret is required'),
  COTURN_REALM: z.string().default('chatapp.com'),
  COTURN_TTL: z.string().transform(Number).default('86400'),
  COTURN_FALLBACK_HOSTS: z.string().optional(),
  
  // Logging
  LOG_LEVEL: z.enum(['error', 'warn', 'info', 'debug']).default('info'),
  
  // Rate Limiting
  RATE_LIMIT_REDIS_URL: z.string().optional(),
  RATE_LIMIT_WINDOW_MS: z.string().transform(Number).default('900000'), // 15 minutes
  RATE_LIMIT_MAX_REQUESTS: z.string().transform(Number).default('100'),
  
  // Security
  ENCRYPTION_KEY: z.string().min(64, 'Encryption key must be at least 64 characters'),
  CORS_ORIGIN: z.string().optional(),
  ALLOWED_ORIGINS: z.string().optional(),
  
  // Monitoring
  ANALYTICS_ENABLED: z.string().transform(val => val === 'true').default('true'),
  METRICS_ENABLED: z.string().transform(val => val === 'true').default('true'),
  HEALTH_CHECK_ENABLED: z.string().transform(val => val === 'true').default('true'),
});

export type Environment = z.infer<typeof envSchema>;

class EnvironmentConfig {
  private config: Environment;
  private isLoaded = false;

  constructor() {
    this.config = this.loadConfig();
  }

  private loadConfig(): Environment {
    try {
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
      
      logger.info('Environment configuration loaded successfully', {
        environment: config.NODE_ENV,
        port: config.PORT,
        version: config.APP_VERSION,
      });

      return config;
    } catch (error) {
      logger.error('Failed to load environment configuration', error instanceof Error ? error : new Error(String(error)));
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

  // Validate required services
  validateServices(): void {
    const config = this.get();
    const errors: string[] = [];

    // Check required environment variables based on features
    if (!config.MONGODB_URI) {
      errors.push('MONGODB_URI is required');
    }

    if (!config.JWT_SECRET || config.JWT_SECRET.length < 32) {
      errors.push('JWT_SECRET must be at least 32 characters');
    }

    if (!config.ENCRYPTION_KEY || config.ENCRYPTION_KEY.length < 64) {
      errors.push('ENCRYPTION_KEY must be at least 64 characters');
    }

    // AWS S3 validation
    if (!config.AWS_ACCESS_KEY_ID || !config.AWS_SECRET_ACCESS_KEY || !config.AWS_S3_BUCKET) {
      errors.push('AWS S3 configuration is incomplete');
    }

    // SMTP validation
    if (!config.SMTP_USER || !config.SMTP_PASS) {
      errors.push('SMTP configuration is incomplete');
    }

    // Twilio validation
    if (!config.TWILIO_ACCOUNT_SID || !config.TWILIO_AUTH_TOKEN || !config.TWILIO_FROM_NUMBER) {
      errors.push('Twilio configuration is incomplete');
    }

    // Firebase validation
    if (!config.FIREBASE_PROJECT_ID || !config.FIREBASE_PRIVATE_KEY || !config.FIREBASE_CLIENT_EMAIL) {
      errors.push('Firebase configuration is incomplete');
    }

    if (errors.length > 0) {
      throw new Error(`Configuration validation failed:\n${errors.join('\n')}`);
    }

    logger.info('All service configurations validated successfully');
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
