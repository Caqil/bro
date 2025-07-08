import { Server as SocketIOServer } from 'socket.io';
import { AuthenticatedSocket } from '../socket';
import { CallRepository } from '../../database/repositories/call';
import { ChatRepository } from '../../database/repositories/chat';
import { socketManager } from '../socket';

const callRepository = new CallRepository();
const chatRepository = new ChatRepository();

export function registerCallEvents(socket: AuthenticatedSocket, io: SocketIOServer) {
  // Initiate call
  socket.on('call:initiate', async (data) => {
    try {
      const { participantId, type, chatId } = data; // type: 'voice' | 'video'

      // Check if participant is online
      if (!socketManager.isUserOnline(participantId)) {
        return socket.emit('call:error', { message: 'User is offline' });
      }

      // Generate unique call ID
      const callId = require('crypto').randomUUID();

      // Create call record
      const call = await callRepository.create({
        callId,
        initiator: socket.userId as any,
        participants: [socket.userId as any, participantId],
        type,
        status: 'initiated',
        chatId: chatId || undefined,
        isGroupCall: false,
      });

      // Join call room
      socket.join(`call:${callId}`);

      // Emit to initiator
      socket.emit('call:initiated', {
        callId,
        type,
        participant: participantId,
      });

      // Emit incoming call to participant
      socketManager.emitToUser(participantId, 'call:incoming', {
        callId,
        type,
        initiator: {
          userId: socket.userId,
          displayName: socket.user.displayName,
          avatar: socket.user.avatar,
        },
        chatId,
      });

    } catch (error) {
      console.error('Error initiating call:', error);
      socket.emit('call:error', { message: 'Failed to initiate call' });
    }
  });

  // Answer call
  socket.on('call:answer', async (data) => {
    try {
      const { callId } = data;

      // Get call details
      const call = await callRepository.findByCallId(callId);
      if (!call || !call.participants.includes(socket.userId as any)) {
        return socket.emit('call:error', { message: 'Call not found or unauthorized' });
      }

      // Update call status
      await callRepository.update(call._id, { status: 'answered' });

      // Join call room
      socket.join(`call:${callId}`);

      // Notify all participants
      io.to(`call:${callId}`).emit('call:answered', {
        callId,
        answeredBy: socket.userId,
      });

    } catch (error) {
      console.error('Error answering call:', error);
      socket.emit('call:error', { message: 'Failed to answer call' });
    }
  });

  // Reject call
  socket.on('call:reject', async (data) => {
    try {
      const { callId } = data;

      // End call with rejected status
      await callRepository.endCall(callId, 'rejected');

      // Notify all participants
      io.to(`call:${callId}`).emit('call:rejected', {
        callId,
        rejectedBy: socket.userId,
      });

      // Clean up call room
      io.in(`call:${callId}`).socketsLeave(`call:${callId}`);

    } catch (error) {
      console.error('Error rejecting call:', error);
      socket.emit('call:error', { message: 'Failed to reject call' });
    }
  });

  // End call
  socket.on('call:end', async (data) => {
    try {
      const { callId } = data;

      // End call
      await callRepository.endCall(callId, 'ended');

      // Notify all participants
      io.to(`call:${callId}`).emit('call:ended', {
        callId,
        endedBy: socket.userId,
      });

      // Clean up call room
      io.in(`call:${callId}`).socketsLeave(`call:${callId}`);

    } catch (error) {
      console.error('Error ending call:', error);
      socket.emit('call:error', { message: 'Failed to end call' });
    }
  });

  // WebRTC Signaling Events
  // ICE Candidate
  socket.on('call:ice-candidate', async (data) => {
    try {
      const { callId, candidate } = data;

      // Store ICE candidate
      await callRepository.addIceCandidate(callId, socket.userId as any, candidate);

      // Forward to other participants
      socket.to(`call:${callId}`).emit('call:ice-candidate', {
        callId,
        candidate,
        from: socket.userId,
      });

    } catch (error) {
      console.error('Error handling ICE candidate:', error);
    }
  });

  // SDP Offer
  socket.on('call:offer', async (data) => {
    try {
      const { callId, sdp } = data;

      // Store offer
      await callRepository.addOffer(callId, socket.userId as any, sdp);

      // Forward to other participants
      socket.to(`call:${callId}`).emit('call:offer', {
        callId,
        sdp,
        from: socket.userId,
      });

    } catch (error) {
      console.error('Error handling call offer:', error);
    }
  });

  // SDP Answer
  socket.on('call:answer-sdp', async (data) => {
    try {
      const { callId, sdp } = data;

      // Store answer
      await callRepository.addAnswer(callId, socket.userId as any, sdp);

      // Forward to other participants
      socket.to(`call:${callId}`).emit('call:answer-sdp', {
        callId,
        sdp,
        from: socket.userId,
      });

    } catch (error) {
      console.error('Error handling call answer:', error);
    }
  });

  // Call quality feedback
  socket.on('call:quality', async (data) => {
    try {
      const { callId, rating, feedback } = data;

      await callRepository.addQualityRating(callId, socket.userId as any, rating, feedback);

      socket.emit('call:quality:saved', { callId });

    } catch (error) {
      console.error('Error saving call quality:', error);
    }
  });
}