import { Server as HTTPServer } from 'http';
import { Server as SocketIOServer, Socket } from 'socket.io';
import { socketAuthMiddleware } from './middleware/auth';
import { socketRateLimitMiddleware } from './middleware/rate-limit';
import { registerMessagingEvents } from './events/messaging';
import { registerPresenceEvents } from './events/presence';
import { registerTypingEvents } from './events/typing';
import { registerCallEvents } from './events/calls';
import { registerGroupEvents } from './events/groups';

export interface AuthenticatedSocket extends Socket {
  userId: string;
  user: {
    _id: string;
    displayName: string;
    avatar?: string;
    phoneNumber: string;
  };
}

class SocketManager {
  private io: SocketIOServer | null = null;
  private userSockets: Map<string, Set<string>> = new Map(); // userId -> Set of socketIds
  private socketUsers: Map<string, string> = new Map(); // socketId -> userId

  initialize(httpServer: HTTPServer): SocketIOServer {
    this.io = new SocketIOServer(httpServer, {
      cors: {
        origin: process.env.FRONTEND_URL || "http://localhost:3000",
        methods: ["GET", "POST"],
        credentials: true
      },
      pingTimeout: 60000,
      pingInterval: 25000,
    });

    // Apply middleware
    this.io.use(socketAuthMiddleware);
    this.io.use(socketRateLimitMiddleware);

    // Handle connections
    this.io.on('connection', (socket) => {
      this.handleConnection(socket as AuthenticatedSocket);
    });

    return this.io;
  }

  private handleConnection(socket: AuthenticatedSocket) {
    const userId = socket.userId;
    
    console.log(`User ${userId} connected with socket ${socket.id}`);

    // Track user connections
    if (!this.userSockets.has(userId)) {
      this.userSockets.set(userId, new Set());
    }
    this.userSockets.get(userId)!.add(socket.id);
    this.socketUsers.set(socket.id, userId);

    // Join user to their personal room
    socket.join(`user:${userId}`);

    // Register event handlers
    registerMessagingEvents(socket, this.io!);
    registerPresenceEvents(socket, this.io!);
    registerTypingEvents(socket, this.io!);
    registerCallEvents(socket, this.io!);
    registerGroupEvents(socket, this.io!);

    // Handle disconnection
    socket.on('disconnect', () => {
      this.handleDisconnection(socket);
    });

    // Emit user online status
    this.broadcastUserPresence(userId, true);
  }

  private handleDisconnection(socket: AuthenticatedSocket) {
    const userId = socket.userId;
    
    console.log(`User ${userId} disconnected from socket ${socket.id}`);

    // Remove socket tracking
    this.socketUsers.delete(socket.id);
    const userSocketSet = this.userSockets.get(userId);
    if (userSocketSet) {
      userSocketSet.delete(socket.id);
      if (userSocketSet.size === 0) {
        this.userSockets.delete(userId);
        // User is fully offline, broadcast presence
        this.broadcastUserPresence(userId, false);
      }
    }
  }

  // Public methods for emitting to users
  emitToUser(userId: string, event: string, data: any) {
    if (this.io) {
      this.io.to(`user:${userId}`).emit(event, data);
    }
  }

  emitToChat(chatId: string, event: string, data: any, excludeUserId?: string) {
    if (this.io) {
      const emitter = this.io.to(`chat:${chatId}`);
      if (excludeUserId) {
        emitter.except(`user:${excludeUserId}`);
      }
      emitter.emit(event, data);
    }
  }

  isUserOnline(userId: string): boolean {
    return this.userSockets.has(userId);
  }

  getUserSocketCount(userId: string): number {
    return this.userSockets.get(userId)?.size || 0;
  }

  private broadcastUserPresence(userId: string, isOnline: boolean) {
    if (this.io) {
      this.io.emit('user:presence:changed', {
        userId,
        isOnline,
        lastSeen: new Date()
      });
    }
  }

  getIO(): SocketIOServer | null {
    return this.io;
  }
}

export const socketManager = new SocketManager();
