// Edge Runtime compatible logger
interface LogContext {
  userId?: string;
  requestId?: string;
  ip?: string;
  userAgent?: string;
  method?: string;
  url?: string;
  statusCode?: number;
  responseTime?: number;
}

export class EdgeLogger {
  private serviceName: string;
  private environment: string;

  constructor(serviceName: string = 'chatapp-edge') {
    this.serviceName = serviceName;
    this.environment = process.env.NODE_ENV || 'development';
  }

  private formatLog(level: string, message: string, context?: LogContext): string {
    const timestamp = new Date().toISOString();
    const logEntry = {
      timestamp,
      level,
      message,
      service: this.serviceName,
      environment: this.environment,
      ...context,
    };
    return JSON.stringify(logEntry);
  }

  info(message: string, context?: LogContext): void {
    console.info(this.formatLog('info', message, context));
  }

  warn(message: string, context?: LogContext): void {
    console.warn(this.formatLog('warn', message, context));
  }

  error(message: string, error?: Error, context?: LogContext): void {
    const errorContext = {
      ...context,
      error: error ? {
        name: error.name,
        message: error.message,
        stack: error.stack,
      } : undefined,
    };
    console.error(this.formatLog('error', message, errorContext));
  }

  debug(message: string, context?: LogContext): void {
    if (this.environment === 'development') {
      console.debug(this.formatLog('debug', message, context));
    }
  }

  logRequest(req: any, statusCode: number, responseTime: number): void {
    const context = {
      requestId: req.headers.get('x-request-id'),
      ip: this.getClientIP(req),
      userAgent: req.headers.get('user-agent'),
      method: req.method,
      url: req.url,
      statusCode,
      responseTime,
    };

    const level = statusCode >= 400 ? 'warn' : 'info';
    const message = `${req.method} ${req.url} ${statusCode} - ${responseTime}ms`;
    
    if (level === 'warn') {
      this.warn(message, context);
    } else {
      this.info(message, context);
    }
  }

  private getClientIP(request: any): string {
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
    
    return '127.0.0.1';
  }
}

export const edgeLogger = new EdgeLogger();