import winston from 'winston';
import { Request } from 'express';

interface LogContext {
  userId?: string;
  sessionId?: string;
  requestId?: string;
  ip?: string;
  userAgent?: string;
  method?: string;
  url?: string;
  statusCode?: number;
  responseTime?: number;
  error?: {
    name: string;
    message: string;
    stack?: string;
  };
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

  // Error level logging
  error(message: string, error?: Error, context?: LogContext): void {
    const logContext = { ...context };
    
    if (error) {
      logContext.error = {
        name: error.name,
        message: error.message,
        stack: error.stack,
      };
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
      userId: (req as any).user?.id,
    };

    const level = statusCode >= 400 ? 'warn' : 'info';
    const message = `${req.method} ${req.originalUrl} ${statusCode} - ${responseTime}ms`;

    this.winston.log(level, message, { context });
  }

  // User action logging
  logUserAction(action: string, userId: string, details?: any): void {
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
  logDatabaseOperation(operation: string, collection: string, duration: number, error?: Error): void {
    if (error) {
      this.error(`Database operation failed: ${operation} on ${collection}`, error, {
        responseTime: duration,
      });
    } else {
      this.debug(`Database operation: ${operation} on ${collection} - ${duration}ms`);
    }
  }

  // External service logging
  logExternalService(service: string, operation: string, duration: number, success: boolean): void {
    const message = `External service ${service}: ${operation} - ${duration}ms`;
    
    if (success) {
      this.info(message);
    } else {
      this.warn(`${message} - FAILED`);
    }
  }

  // Performance logging
  logPerformance(operation: string, duration: number, context?: LogContext): void {
    if (duration > 1000) { // Log slow operations (> 1s)
      this.warn(`Slow operation: ${operation} - ${duration}ms`, context);
    } else {
      this.debug(`Performance: ${operation} - ${duration}ms`, context);
    }
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
    
    childLogger.error = (message: string, error?: Error, context?: LogContext) => 
      originalError(message, error, { ...defaultContext, ...context });
    
    childLogger.debug = (message: string, context?: LogContext) => 
      originalDebug(message, { ...defaultContext, ...context });

    return childLogger;
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