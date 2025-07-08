import { Server as SocketIOServer } from 'socket.io';
import { AuthenticatedSocket } from '../socket';
import { UserRepository } from '../../database/repositories/user';

const userRepository = new UserRepository();

export function registerPresenceEvents(socket: AuthenticatedSocket, io: SocketIOServer) {
  // Update user online status on connection
  userRepository.updateOnlineStatus(socket.userId as any, true);

  // Handle explicit presence updates
  socket.on('presence:update', async (data) => {
    try {
      const { status } = data; // 'online', 'away', 'busy', 'offline'

      const isOnline = status === 'online';
      await userRepository.updateOnlineStatus(socket.userId as any, isOnline);

      // Broadcast presence to contacts
      socket.broadcast.emit('presence:changed', {
        userId: socket.userId,
        status,
        lastSeen: new Date(),
      });

    } catch (error) {
      console.error('Error updating presence:', error);
    }
  });

  // Handle user going offline on disconnect
  socket.on('disconnect', async () => {
    try {
      await userRepository.updateOnlineStatus(socket.userId as any, false);

      // Broadcast offline status
      socket.broadcast.emit('presence:changed', {
        userId: socket.userId,
        status: 'offline',
        lastSeen: new Date(),
      });

    } catch (error) {
      console.error('Error updating offline status:', error);
    }
  });

  // Handle ping/heartbeat for active status
  socket.on('presence:ping', () => {
    socket.emit('presence:pong');
    userRepository.updateOnlineStatus(socket.userId as any, true);
  });
}
