import { Request, Response, NextFunction } from 'express';
import { ZodError } from 'zod';
import { MongoError } from 'mongodb';

export interface AppError extends Error {
  statusCode?: number;
  code?: string;
  isOperational?: boolean;
}

export class ErrorHandler {
  // Create app error
  static createError(message: string, statusCode: number = 500, code?: string): AppError {
    const error: AppError = new Error(message);
    error.statusCode = statusCode;
    error.code = code;
    error.isOperational = true;
    return error;
  }

  // Handle Zod validation errors
  static handleZodError(error: ZodError): AppError {
    const message = error.errors.map(err => `${err.path.join('.')}: ${err.message}`).join(', ');
    return this.createError(`Validation error: ${message}`, 400, 'VALIDATION_ERROR');
  }

  // Handle MongoDB errors
  static handleMongoError(error: MongoError): AppError {
    switch (error.code) {
      case 11000:
        // Duplicate key error
        const field = Object.keys((error as any).keyPattern)[0];
        return this.createError(`${field} already exists`, 409, 'DUPLICATE_KEY');
      
      case 121:
        // Document validation error
        return this.createError('Document validation failed', 400, 'DOCUMENT_VALIDATION');
      
      default:
        return this.createError('Database error', 500, 'DATABASE_ERROR');
    }
  }

  // Handle JWT errors
  static handleJWTError(error: Error): AppError {
    if (error.name === 'JsonWebTokenError') {
      return this.createError('Invalid token', 401, 'INVALID_TOKEN');
    }
    
    if (error.name === 'TokenExpiredError') {
      return this.createError('Token expired', 401, 'TOKEN_EXPIRED');
    }
    
    return this.createError('Authentication error', 401, 'AUTH_ERROR');
  }

  // Global error handler middleware
  static globalErrorHandler(error: Error, req: Request, res: Response, next: NextFunction): void {
    let appError: AppError;

    // Handle known error types
    if (error instanceof ZodError) {
      appError = ErrorHandler.handleZodError(error);
    } else if ((error as any).code >= 11000) {
      appError = ErrorHandler.handleMongoError(error as MongoError);
    } else if (error.name.includes('Token') || error.name === 'JsonWebTokenError') {
      appError = ErrorHandler.handleJWTError(error);
    } else if ((error as AppError).isOperational) {
      appError = error as AppError;
    } else {
      // Unknown error
      appError = ErrorHandler.createError('Internal server error', 500, 'INTERNAL_ERROR');
    }

    // Log error in production
    if (process.env.NODE_ENV === 'production') {
      console.error('Error:', {
        message: appError.message,
        stack: appError.stack,
        url: req.url,
        method: req.method,
        ip: req.ip,
        userAgent: req.get('User-Agent'),
      });
    } else {
      console.error(error);
    }

    // Send error response
    res.status(appError.statusCode || 500).json({
      error: {
        message: appError.message,
        code: appError.code,
        ...(process.env.NODE_ENV === 'development' && { stack: appError.stack }),
      },
    });
  }

  // Async error wrapper
  static asyncHandler(fn: Function) {
    return (req: Request, res: Response, next: NextFunction) => {
      Promise.resolve(fn(req, res, next)).catch(next);
    };
  }

  // Not found handler
  static notFoundHandler(req: Request, res: Response, next: NextFunction): void {
    const error = ErrorHandler.createError(`Route ${req.originalUrl} not found`, 404, 'NOT_FOUND');
    next(error);
  }

  // Validation error helper
  static validationError(message: string): AppError {
    return ErrorHandler.createError(message, 400, 'VALIDATION_ERROR');
  }

  // Authorization error helper
  static authorizationError(message: string = 'Insufficient permissions'): AppError {
    return ErrorHandler.createError(message, 403, 'AUTHORIZATION_ERROR');
  }

  // Authentication error helper
  static authenticationError(message: string = 'Authentication required'): AppError {
    return ErrorHandler.createError(message, 401, 'AUTHENTICATION_ERROR');
  }

  // Not found error helper
  static notFoundError(resource: string = 'Resource'): AppError {
    return ErrorHandler.createError(`${resource} not found`, 404, 'NOT_FOUND');
  }

  // Conflict error helper
  static conflictError(message: string): AppError {
    return ErrorHandler.createError(message, 409, 'CONFLICT');
  }

  // Rate limit error helper
  static rateLimitError(message: string = 'Rate limit exceeded'): AppError {
    return ErrorHandler.createError(message, 429, 'RATE_LIMIT_EXCEEDED');
  }
}