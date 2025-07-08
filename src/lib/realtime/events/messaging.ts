import { Server as SocketIOServer } from 'socket.io';
import { AuthenticatedSocket } from '../socket';
import { MessageRepository } from '../../database/repositories/message';
import { ChatRepository } from '../../database/repositories/chat';
import { createEventRateLimit } from '../middleware/rate-limit';

const messageRepository = new MessageRepository();
const chatRepository = new ChatRepository();

// Rate limits for messaging events
const messageRateLimit = createEventRateLimit({ maxRequests: 30, windowMs: 60000 }); // 30 messages per minute
const reactionRateLimit = createEventRateLimit({ maxRequests: 60, windowMs: 60000 }); // 60 reactions per minute

export function registerMessagingEvents(socket: AuthenticatedSocket, io: SocketIOServer) {
  // Send message
  socket.on('message:send', async (data) => {
    if (!messageRateLimit(socket, 'message:send')) return;

    try {
      const { chatId, content, type = 'text', replyTo, mediaId, metadata } = data;

      // Validate chat membership
      const chat = await chatRepository.findById(chatId);
      if (!chat || !chat.participants.includes(socket.userId as any)) {
        return socket.emit('error', { message: 'Not authorized to send message to this chat' });
      }

      // Create message
      const message = await messageRepository.create({
        chatId,
        senderId: socket.userId as any,
        content,
        type,
        replyTo,
        media: mediaId,
        metadata,
      });

      // Update chat last activity
      await chatRepository.updateLastActivity(chatId, message._id);

      // Populate message for response
      const populatedMessage = await messageRepository.findById(message._id);

      // Emit to all chat participants
      io.to(`chat:${chatId}`).emit('message:new', populatedMessage);

      // Send delivery confirmations to sender
      socket.emit('message:sent', { messageId: message._id, tempId: data.tempId });

    } catch (error) {
      console.error('Error sending message:', error);
      socket.emit('error', { message: 'Failed to send message' });
    }
  });

  // Edit message
  socket.on('message:edit', async (data) => {
    if (!messageRateLimit(socket, 'message:edit')) return;

    try {
      const { messageId, content } = data;

      // Get message and verify ownership
      const message = await messageRepository.findById(messageId);
      if (!message || message.senderId.toString() !== socket.userId) {
        return socket.emit('error', { message: 'Not authorized to edit this message' });
      }

      // Update message
      const updatedMessage = await messageRepository.update(messageId, {
        content,
        isEdited: true,
        editedAt: new Date(),
      });

      // Emit to chat participants
      io.to(`chat:${message.chatId}`).emit('message:edited', updatedMessage);

    } catch (error) {
      console.error('Error editing message:', error);
      socket.emit('error', { message: 'Failed to edit message' });
    }
  });

  // Delete message
  socket.on('message:delete', async (data) => {
    try {
      const { messageId, deleteForEveryone = false } = data;

      // Get message and verify ownership
      const message = await messageRepository.findById(messageId);
      if (!message || message.senderId.toString() !== socket.userId) {
        return socket.emit('error', { message: 'Not authorized to delete this message' });
      }

      if (deleteForEveryone) {
        // Delete for everyone (within time limit)
        const timeDiff = Date.now() - message.createdAt.getTime();
        const deleteTimeLimit = 7 * 60 * 1000; // 7 minutes

        if (timeDiff > deleteTimeLimit) {
          return socket.emit('error', { message: 'Time limit exceeded for deleting for everyone' });
        }

        await messageRepository.delete(messageId);
        io.to(`chat:${message.chatId}`).emit('message:deleted', { messageId, deletedForEveryone: true });
      } else {
        // Delete for sender only
        await messageRepository.delete(messageId, socket.userId as any);
        socket.emit('message:deleted', { messageId, deletedForEveryone: false });
      }

    } catch (error) {
      console.error('Error deleting message:', error);
      socket.emit('error', { message: 'Failed to delete message' });
    }
  });

  // Add reaction
  socket.on('message:react', async (data) => {
    if (!reactionRateLimit(socket, 'message:react')) return;

    try {
      const { messageId, emoji } = data;

      // Get message to verify chat membership
      const message = await messageRepository.findById(messageId);
      if (!message) {
        return socket.emit('error', { message: 'Message not found' });
      }

      // Verify chat membership
      const chat = await chatRepository.findById(message.chatId);
      if (!chat || !chat.participants.includes(socket.userId as any)) {
        return socket.emit('error', { message: 'Not authorized to react to this message' });
      }

      // Add reaction
      await messageRepository.addReaction(messageId, socket.userId as any, emoji);

      // Emit to chat participants
      io.to(`chat:${message.chatId}`).emit('message:reaction:added', {
        messageId,
        userId: socket.userId,
        emoji,
        timestamp: new Date(),
      });

    } catch (error) {
      console.error('Error adding reaction:', error);
      socket.emit('error', { message: 'Failed to add reaction' });
    }
  });

  // Remove reaction
  socket.on('message:unreact', async (data) => {
    try {
      const { messageId } = data;

      // Get message to verify chat membership
      const message = await messageRepository.findById(messageId);
      if (!message) {
        return socket.emit('error', { message: 'Message not found' });
      }

      // Remove reaction
      await messageRepository.removeReaction(messageId, socket.userId as any);

      // Emit to chat participants
      io.to(`chat:${message.chatId}`).emit('message:reaction:removed', {
        messageId,
        userId: socket.userId,
      });

    } catch (error) {
      console.error('Error removing reaction:', error);
      socket.emit('error', { message: 'Failed to remove reaction' });
    }
  });

  // Mark messages as read
  socket.on('message:read', async (data) => {
    try {
      const { messageIds } = data;

      // Mark messages as read
      const readCount = await messageRepository.markMultipleAsRead(messageIds, socket.userId as any);

      if (readCount > 0) {
        // Get first message to determine chat
        const firstMessage = await messageRepository.findById(messageIds[0]);
        if (firstMessage) {
          // Emit read receipt to chat participants (except sender)
          socket.to(`chat:${firstMessage.chatId}`).emit('message:read:receipt', {
            messageIds,
            readBy: socket.userId,
            readAt: new Date(),
          });
        }
      }

    } catch (error) {
      console.error('Error marking messages as read:', error);
      socket.emit('error', { message: 'Failed to mark messages as read' });
    }
  });

  // Join chat room (for real-time updates)
  socket.on('chat:join', async (data) => {
    try {
      const { chatId } = data;

      // Verify chat membership
      const chat = await chatRepository.findById(chatId);
      if (!chat || !chat.participants.includes(socket.userId as any)) {
        return socket.emit('error', { message: 'Not authorized to join this chat' });
      }

      // Join chat room
      socket.join(`chat:${chatId}`);
      socket.emit('chat:joined', { chatId });

    } catch (error) {
      console.error('Error joining chat:', error);
      socket.emit('error', { message: 'Failed to join chat' });
    }
  });

  // Leave chat room
  socket.on('chat:leave', (data) => {
    const { chatId } = data;
    socket.leave(`chat:${chatId}`);
    socket.emit('chat:left', { chatId });
  });
}