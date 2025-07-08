import { EventEmitter } from 'events';
import { Request } from 'express';
import { IUser } from '../database/models/user';
import { IMessage } from '../database/models/message';
import { ICall } from '../database/models/call';

interface AnalyticsEvent {
  type: string;
  userId?: string;
  sessionId?: string;
  timestamp: Date;
  properties: Record<string, any>;
  metadata?: {
    ip?: string;
    userAgent?: string;
    platform?: string;
    version?: string;
  };
}

interface UserAnalytics {
  userId: string;
  events: AnalyticsEvent[];
  sessionStart: Date;
  lastActivity: Date;
  totalSessions: number;
  totalEvents: number;
}

interface SystemMetrics {
  users: {
    total: number;
    active: number;
    registered: number;
    online: number;
  };
  messages: {
    total: number;
    sent: number;
    delivered: number;
    read: number;
  };
  calls: {
    total: number;
    completed: number;
    missed: number;
    duration: number;
  };
  system: {
    uptime: number;
    memoryUsage: number;
    cpuUsage: number;
    requests: number;
  };
}

export class AnalyticsService extends EventEmitter {
  private events: AnalyticsEvent[] = [];
  private userSessions = new Map<string, UserAnalytics>();
  private systemMetrics: SystemMetrics = {
    users: { total: 0, active: 0, registered: 0, online: 0 },
    messages: { total: 0, sent: 0, delivered: 0, read: 0 },
    calls: { total: 0, completed: 0, missed: 0, duration: 0 },
    system: { uptime: Date.now(), memoryUsage: 0, cpuUsage: 0, requests: 0 },
  };

  constructor() {
    super();
    this.startMetricsCollection();
  }

  // Track user event
  track(type: string, properties: Record<string, any>, context?: {
    userId?: string;
    sessionId?: string;
    req?: Request;
  }): void {
    const event: AnalyticsEvent = {
      type,
      userId: context?.userId,
      sessionId: context?.sessionId,
      timestamp: new Date(),
      properties,
      metadata: context?.req ? {
        ip: context.req.ip,
        userAgent: context.req.get('User-Agent'),
        platform: this.extractPlatform(context.req.get('User-Agent')),
        version: context.req.get('App-Version'),
      } : undefined,
    };

    this.events.push(event);
    this.updateUserSession(event);
    this.emit('event', event);

    // Keep only recent events in memory (last 10000)
    if (this.events.length > 10000) {
      this.events = this.events.slice(-10000);
    }
  }

  // Track user registration
  trackUserRegistration(user: IUser, req?: Request): void {
    this.track('user_registered', {
      userId: user._id.toString(),
      registrationMethod: user.email ? 'email' : 'phone',
      hasAvatar: !!user.avatar,
    }, { userId: user._id.toString(), req });

    this.systemMetrics.users.registered++;
  }

  // Track user login
  trackUserLogin(user: IUser, method: 'otp' | 'qr', req?: Request): void {
    this.track('user_login', {
      userId: user._id.toString(),
      method,
      deviceCount: user.devices.length,
    }, { userId: user._id.toString(), req });
  }

  // Track message sent
  trackMessageSent(message: IMessage, req?: Request): void {
    this.track('message_sent', {
      messageId: message._id.toString(),
      chatId: message.chatId.toString(),
      type: message.type,
      hasMedia: !!message.media,
      isReply: !!message.replyTo,
      isForwarded: !!message.forwardedFrom,
      contentLength: message.content.length,
    }, { userId: message.senderId.toString(), req });

    this.systemMetrics.messages.total++;
    this.systemMetrics.messages.sent++;
  }

  // Track message delivered
  trackMessageDelivered(messageId: string, userId: string): void {
    this.track('message_delivered', {
      messageId,
      userId,
    }, { userId });

    this.systemMetrics.messages.delivered++;
  }

  // Track message read
  trackMessageRead(messageId: string, userId: string): void {
    this.track('message_read', {
      messageId,
      userId,
    }, { userId });

    this.systemMetrics.messages.read++;
  }

  // Track call initiated
  trackCallInitiated(call: ICall, req?: Request): void {
    this.track('call_initiated', {
      callId: call.callId,
      type: call.type,
      isGroupCall: call.isGroupCall,
      participantCount: call.participants.length,
    }, { userId: call.initiator.toString(), req });

    this.systemMetrics.calls.total++;
  }

  // Track call ended
  trackCallEnded(call: ICall, endReason: string): void {
    const duration = call.duration || 0;
    
    this.track('call_ended', {
      callId: call.callId,
      type: call.type,
      status: call.status,
      duration,
      endReason,
      isGroupCall: call.isGroupCall,
      participantCount: call.participants.length,
    }, { userId: call.initiator.toString() });

    if (call.status === 'ended') {
      this.systemMetrics.calls.completed++;
    } else if (call.status === 'missed') {
      this.systemMetrics.calls.missed++;
    }

    this.systemMetrics.calls.duration += duration;
  }

  // Track API request
  trackAPIRequest(req: Request, responseTime: number, statusCode: number): void {
    this.track('api_request', {
      method: req.method,
      path: req.path,
      statusCode,
      responseTime,
      queryParams: Object.keys(req.query).length,
      bodySize: req.get('Content-Length') || 0,
    }, { req });

    this.systemMetrics.system.requests++;
  }

  // Track file upload
  trackFileUpload(fileType: string, fileSize: number, userId: string, req?: Request): void {
    this.track('file_uploaded', {
      fileType,
      fileSize,
      sizeCategory: this.getFileSizeCategory(fileSize),
    }, { userId, req });
  }

  // Track error
  trackError(error: Error, context?: { userId?: string; req?: Request }): void {
    this.track('error_occurred', {
      errorName: error.name,
      errorMessage: error.message,
      errorStack: error.stack?.split('\n')[0], // First line only
    }, context);
  }

  // Get analytics data
  getAnalytics(timeRange: { start: Date; end: Date }, userId?: string): {
    events: AnalyticsEvent[];
    metrics: any;
  } {
    let filteredEvents = this.events.filter(event => 
      event.timestamp >= timeRange.start && 
      event.timestamp <= timeRange.end
    );

    if (userId) {
      filteredEvents = filteredEvents.filter(event => event.userId === userId);
    }

    return {
      events: filteredEvents,
      metrics: this.generateMetrics(filteredEvents),
    };
  }

  // Get system metrics
  getSystemMetrics(): SystemMetrics {
    // Update system metrics
    const memUsage = process.memoryUsage();
    this.systemMetrics.system.memoryUsage = memUsage.heapUsed;
    this.systemMetrics.system.uptime = Date.now() - this.systemMetrics.system.uptime;

    return { ...this.systemMetrics };
  }

  // Get user session analytics
  getUserSessionAnalytics(userId: string): UserAnalytics | null {
    return this.userSessions.get(userId) || null;
  }

  // Private methods
  private updateUserSession(event: AnalyticsEvent): void {
    if (!event.userId) return;

    const session = this.userSessions.get(event.userId) || {
      userId: event.userId,
      events: [],
      sessionStart: event.timestamp,
      lastActivity: event.timestamp,
      totalSessions: 1,
      totalEvents: 0,
    };

    session.events.push(event);
    session.lastActivity = event.timestamp;
    session.totalEvents++;

    // Consider a new session if gap is > 30 minutes
    const timeSinceLastActivity = event.timestamp.getTime() - session.lastActivity.getTime();
    if (timeSinceLastActivity > 30 * 60 * 1000) {
      session.totalSessions++;
      session.sessionStart = event.timestamp;
    }

    this.userSessions.set(event.userId, session);
  }

  private extractPlatform(userAgent?: string): string {
    if (!userAgent) return 'unknown';
    
    if (userAgent.includes('iPhone')) return 'ios';
    if (userAgent.includes('Android')) return 'android';
    if (userAgent.includes('Windows')) return 'windows';
    if (userAgent.includes('Mac')) return 'mac';
    if (userAgent.includes('Linux')) return 'linux';
    
    return 'web';
  }

  private getFileSizeCategory(size: number): string {
    if (size < 1024 * 1024) return 'small'; // < 1MB
    if (size < 10 * 1024 * 1024) return 'medium'; // < 10MB
    if (size < 100 * 1024 * 1024) return 'large'; // < 100MB
    return 'xlarge';
  }

  private generateMetrics(events: AnalyticsEvent[]): any {
    const metrics = {
      totalEvents: events.length,
      uniqueUsers: new Set(events.map(e => e.userId).filter(Boolean)).size,
      eventTypes: {} as Record<string, number>,
      platforms: {} as Record<string, number>,
      hourlyActivity: {} as Record<string, number>,
    };

    events.forEach(event => {
      // Count event types
      metrics.eventTypes[event.type] = (metrics.eventTypes[event.type] || 0) + 1;

      // Count platforms
      const platform = event.metadata?.platform || 'unknown';
      metrics.platforms[platform] = (metrics.platforms[platform] || 0) + 1;

      // Count hourly activity
      const hour = event.timestamp.getHours().toString().padStart(2, '0');
      metrics.hourlyActivity[hour] = (metrics.hourlyActivity[hour] || 0) + 1;
    });

    return metrics;
  }

  private startMetricsCollection(): void {
    // Collect system metrics every minute
    setInterval(() => {
      const memUsage = process.memoryUsage();
      this.systemMetrics.system.memoryUsage = memUsage.heapUsed;
      
      this.emit('metrics_collected', this.systemMetrics);
    }, 60000);
  }
}

export const analyticsService = new AnalyticsService();