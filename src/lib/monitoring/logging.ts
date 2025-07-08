import winston from 'winston';
import { Request } from 'express';

interface LogContext {
  // Core request context
  userId?: string;
  sessionId?: string;
  requestId?: string;
  ip?: string;
  userAgent?: string;
  method?: string;
  url?: string;
  statusCode?: number;
  responseTime?: number;
  
  // Authentication context
  jti?: string; // JWT ID
  deviceId?: string;
  phoneNumber?: string;
  email?: string;
  
  // Call context
  callId?: string;
  qrId?: string;
  platform?: string;
  
  // Message/Chat context
  messageId?: string;
  chatId?: string;
  groupId?: string;
  
  // User management context
  banReason?: string;
  attemptsRemaining?: number;
  cooldownUntil?: Date;
  expiresAt?: Date;
  
  // OTP context
  otpMethod?: string;
  
  // File context
  fileType?: string;
  fileSize?: number;
  
  // Error context
  error?: {
    name: string;
    message: string;
    stack?: string;
  };
  
  // Allow any additional properties for flexibility
  [key: string]: any;
}

interface LogEntry {
  level: string;
  message: string;
  timestamp: string;
  context?: LogContext;
  service: string;
  environment: string;
}

export class Logger {
  private winston: winston.Logger;
  private serviceName: string;
  private environment: string;

  constructor(serviceName: string = 'chatapp-api') {
    this.serviceName = serviceName;
    this.environment = process.env.NODE_ENV || 'development';

    const formats = [
      winston.format.timestamp(),
      winston.format.errors({ stack: true }),
      winston.format.json(),
    ];

    // Add colorization for development
    if (this.environment === 'development') {
      formats.unshift(winston.format.colorize());
      formats.push(winston.format.simple());
    }

    this.winston = winston.createLogger({
      level: process.env.LOG_LEVEL || 'info',
      format: winston.format.combine(...formats),
      defaultMeta: {
        service: this.serviceName,
        environment: this.environment,
      },
      transports: [
        // Console transport
        new winston.transports.Console({
          stderrLevels: ['error'],
        }),

        // File transports for production
        ...(this.environment === 'production' ? [
          new winston.transports.File({
            filename: 'logs/error.log',
            level: 'error',
            maxsize: 50 * 1024 * 1024, // 50MB
            maxFiles: 10,
          }),
          new winston.transports.File({
            filename: 'logs/combined.log',
            maxsize: 100 * 1024 * 1024, // 100MB
            maxFiles: 20,
          }),
        ] : []),
      ],
      exceptionHandlers: [
        new winston.transports.File({
          filename: 'logs/exceptions.log',
          maxsize: 50 * 1024 * 1024,
          maxFiles: 5,
        }),
      ],
      rejectionHandlers: [
        new winston.transports.File({
          filename: 'logs/rejections.log',
          maxsize: 50 * 1024 * 1024,
          maxFiles: 5,
        }),
      ],
    });
  }

  // Info level logging
  info(message: string, context?: LogContext): void {
    this.winston.info(message, { context });
  }

  // Warning level logging
  warn(message: string, context?: LogContext): void {
    this.winston.warn(message, { context });
  }

  // Error level logging with improved error handling
  error(message: string, error?: unknown, context?: LogContext): void {
    const logContext = { ...context };
    
    if (error) {
      // Handle different types of error objects
      if (error instanceof Error) {
        logContext.error = {
          name: error.name,
          message: error.message,
          stack: error.stack,
        };
      } else if (typeof error === 'string') {
        logContext.error = {
          name: 'StringError',
          message: error,
        };
      } else if (typeof error === 'object' && error !== null) {
        logContext.error = {
          name: 'UnknownError',
          message: JSON.stringify(error),
        };
      } else {
        logContext.error = {
          name: 'UnknownError',
          message: String(error),
        };
      }
    }

    this.winston.error(message, { context: logContext });
  }

  // Debug level logging
  debug(message: string, context?: LogContext): void {
    this.winston.debug(message, { context });
  }

  // HTTP request logging
  logRequest(req: Request, statusCode: number, responseTime: number): void {
    const context: LogContext = {
      requestId: req.get('X-Request-ID') || req.get('X-Correlation-ID'),
      ip: req.ip,
      userAgent: req.get('User-Agent'),
      method: req.method,
      url: req.originalUrl,
      statusCode,
      responseTime,
      userId: (req as any).user?.userId || (req as any).user?.id,
    };

    const level = statusCode >= 400 ? 'warn' : 'info';
    const message = `${req.method} ${req.originalUrl} ${statusCode} - ${responseTime}ms`;

    this.winston.log(level, message, { context });
  }

  // User action logging
  logUserAction(action: string, userId: string, details?: LogContext): void {
    this.info(`User action: ${action}`, {
      userId,
      ...details,
    });
  }

  // Security event logging
  logSecurityEvent(event: string, context: LogContext): void {
    this.warn(`Security event: ${event}`, context);
  }

  // Database operation logging
  logDatabaseOperation(operation: string, collection: string, duration: number, error?: unknown): void {
    if (error) {
      this.error(`Database operation failed: ${operation} on ${collection}`, error, {
        responseTime: duration,
        operation,
        collection,
      });
    } else {
      this.debug(`Database operation: ${operation} on ${collection} - ${duration}ms`, {
        operation,
        collection,
        responseTime: duration,
      });
    }
  }

  // External service logging
  logExternalService(service: string, operation: string, duration: number, success: boolean): void {
    const message = `External service ${service}: ${operation} - ${duration}ms`;
    const context: LogContext = {
      service,
      operation,
      responseTime: duration,
      success,
    };
    
    if (success) {
      this.info(message, context);
    } else {
      this.warn(`${message} - FAILED`, context);
    }
  }

  // Performance logging
  logPerformance(operation: string, duration: number, context?: LogContext): void {
    const logContext: LogContext = {
      operation,
      responseTime: duration,
      ...context,
    };

    if (duration > 1000) { // Log slow operations (> 1s)
      this.warn(`Slow operation: ${operation} - ${duration}ms`, logContext);
    } else {
      this.debug(`Performance: ${operation} - ${duration}ms`, logContext);
    }
  }

  // Authentication logging
  logAuth(event: string, context: LogContext): void {
    this.info(`Auth event: ${event}`, context);
  }

  // OTP logging
  logOTP(event: string, context: LogContext): void {
    this.info(`OTP event: ${event}`, context);
  }

  // Call logging
  logCall(event: string, context: LogContext): void {
    this.info(`Call event: ${event}`, context);
  }

  // Media logging
  logMedia(event: string, context: LogContext): void {
    this.info(`Media event: ${event}`, context);
  }

  // Create child logger with default context
  child(defaultContext: LogContext): Logger {
    const childLogger = new Logger(this.serviceName);
    
    // Override methods to include default context
    const originalInfo = childLogger.info.bind(childLogger);
    const originalWarn = childLogger.warn.bind(childLogger);
    const originalError = childLogger.error.bind(childLogger);
    const originalDebug = childLogger.debug.bind(childLogger);

    childLogger.info = (message: string, context?: LogContext) => 
      originalInfo(message, { ...defaultContext, ...context });
    
    childLogger.warn = (message: string, context?: LogContext) => 
      originalWarn(message, { ...defaultContext, ...context });
    
    childLogger.error = (message: string, error?: unknown, context?: LogContext) => 
      originalError(message, error, { ...defaultContext, ...context });
    
    childLogger.debug = (message: string, context?: LogContext) => 
      originalDebug(message, { ...defaultContext, ...context });

    return childLogger;
  }

  // Structured logging for specific events
  logEvent(event: string, level: 'info' | 'warn' | 'error' | 'debug' = 'info', context?: LogContext): void {
    this[level](`Event: ${event}`, context);
  }

  // Helper method for sanitizing sensitive data in logs
  private sanitizeContext(context: LogContext): LogContext {
    const sanitized = { ...context };
    
    // Remove or mask sensitive data
    if (sanitized.phoneNumber) {
      sanitized.phoneNumber = this.maskPhoneNumber(sanitized.phoneNumber);
    }
    
    if (sanitized.email) {
      sanitized.email = this.maskEmail(sanitized.email);
    }
    
    // Remove sensitive tokens (keep only first/last few characters)
    if (sanitized.jti && sanitized.jti.length > 8) {
      sanitized.jti = sanitized.jti.substring(0, 4) + '***' + sanitized.jti.slice(-4);
    }
    
    return sanitized;
  }

  private maskPhoneNumber(phone: string): string {
    if (phone.length <= 4) return phone;
    return phone.substring(0, 2) + '*'.repeat(phone.length - 4) + phone.slice(-2);
  }

  private maskEmail(email: string): string {
    const [username, domain] = email.split('@');
    if (username.length <= 2) return email;
    return username.charAt(0) + '*'.repeat(username.length - 2) + username.charAt(username.length - 1) + '@' + domain;
  }
}

export const logger = new Logger();

// Request logging middleware
export function requestLoggingMiddleware() {
  return (req: any, res: any, next: any) => {
    const startTime = Date.now();
    
    // Generate request ID if not present
    if (!req.get('X-Request-ID')) {
      req.headers['x-request-id'] = require('crypto').randomUUID();
    }

    // Override res.end to log when response is sent
    const originalEnd = res.end;
    res.end = function(chunk: any, encoding: any) {
      const responseTime = Date.now() - startTime;
      logger.logRequest(req, res.statusCode, responseTime);
      originalEnd.call(res, chunk, encoding);
    };

    next();
  };
}

// Export types for use in other modules
export type { LogContext, LogEntry };