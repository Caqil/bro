import { Socket } from 'socket.io';
import jwt from 'jsonwebtoken';
import { User } from '../../database/models/user';
import { AuthenticatedSocket } from '../socket';

export const socketAuthMiddleware = async (socket: Socket, next: (err?: Error) => void) => {
  try {
    const token = socket.handshake.auth.token || socket.handshake.headers.authorization;
    
    if (!token) {
      return next(new Error('Authentication token required'));
    }

    // Remove 'Bearer ' prefix if present
    const cleanToken = token.replace('Bearer ', '');
    
    // Verify JWT token
    const decoded = jwt.verify(cleanToken, process.env.JWT_SECRET!) as any;
    
    // Get user from database
    const user = await User.findById(decoded.userId).select('displayName avatar phoneNumber isVerified isBanned').exec();
    
    if (!user) {
      return next(new Error('User not found'));
    }

    if (user.isBanned) {
      return next(new Error('User is banned'));
    }

    // Attach user info to socket
    (socket as AuthenticatedSocket).userId = user._id.toString();
    (socket as AuthenticatedSocket).user = {
      _id: user._id.toString(),
      displayName: user.displayName,
      avatar: user.avatar,
      phoneNumber: user.phoneNumber,
    };

    next();
  } catch (error) {
    next(new Error('Authentication failed'));
  }
};
