import { Server as SocketIOServer } from 'socket.io';
import { AuthenticatedSocket } from '../socket';
import { ChatRepository } from '../../database/repositories/chat';
import { createEventRateLimit } from '../middleware/rate-limit';

const chatRepository = new ChatRepository();
const typingRateLimit = createEventRateLimit({ maxRequests: 60, windowMs: 60000 }); // 60 typing events per minute

// Store typing states
const typingUsers = new Map<string, Set<string>>(); // chatId -> Set of userIds typing

export function registerTypingEvents(socket: AuthenticatedSocket, io: SocketIOServer) {
  // User started typing
  socket.on('typing:start', async (data) => {
    if (!typingRateLimit(socket, 'typing:start')) return;

    try {
      const { chatId } = data;

      // Verify chat membership
      const chat = await chatRepository.findById(chatId);
      if (!chat || !chat.participants.includes(socket.userId as any)) {
        return socket.emit('error', { message: 'Not authorized to access this chat' });
      }

      // Add user to typing set
      if (!typingUsers.has(chatId)) {
        typingUsers.set(chatId, new Set());
      }
      typingUsers.get(chatId)!.add(socket.userId);

      // Broadcast to other chat participants
      socket.to(`chat:${chatId}`).emit('typing:user:start', {
        chatId,
        userId: socket.userId,
        user: {
          displayName: socket.user.displayName,
          avatar: socket.user.avatar,
        },
      });

    } catch (error) {
      console.error('Error handling typing start:', error);
    }
  });

  // User stopped typing
  socket.on('typing:stop', async (data) => {
    try {
      const { chatId } = data;

      // Remove user from typing set
      const chatTypingUsers = typingUsers.get(chatId);
      if (chatTypingUsers) {
        chatTypingUsers.delete(socket.userId);
        if (chatTypingUsers.size === 0) {
          typingUsers.delete(chatId);
        }
      }

      // Broadcast to other chat participants
      socket.to(`chat:${chatId}`).emit('typing:user:stop', {
        chatId,
        userId: socket.userId,
      });

    } catch (error) {
      console.error('Error handling typing stop:', error);
    }
  });

  // Auto-stop typing after timeout
  socket.on('disconnect', () => {
    // Clean up typing state for disconnected user
    for (const [chatId, userSet] of typingUsers.entries()) {
      if (userSet.has(socket.userId)) {
        userSet.delete(socket.userId);
        if (userSet.size === 0) {
          typingUsers.delete(chatId);
        }

        // Broadcast typing stop
        socket.to(`chat:${chatId}`).emit('typing:user:stop', {
          chatId,
          userId: socket.userId,
        });
      }
    }
  });

  // Get current typing users for a chat
  socket.on('typing:get', (data) => {
    const { chatId } = data;
    const currentTypingUsers = Array.from(typingUsers.get(chatId) || []);
    
    socket.emit('typing:current', {
      chatId,
      typingUsers: currentTypingUsers,
    });
  });
}