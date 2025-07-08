import { CallRepository } from '../database/repositories/call';
import { ICall } from '../database/models/call';
import { socketManager } from '../realtime/socket';
import { Types } from 'mongoose';

interface SignalingMessage {
  type: 'offer' | 'answer' | 'ice-candidate' | 'call-end';
  callId: string;
  senderId: string;
  targetId?: string;
  data: any;
}

interface CallSession {
  callId: string;
  initiator: string;
  participants: string[];
  status: 'initiating' | 'ringing' | 'connected' | 'ended';
  startTime: Date;
  signaling: {
    offers: Map<string, any>;
    answers: Map<string, any>;
    iceCandidates: Map<string, any[]>;
  };
}

export class WebRTCSignalingService {
  private callRepository: CallRepository;
  private activeCalls: Map<string, CallSession> = new Map();

  constructor() {
    this.callRepository = new CallRepository();
  }

  // Initiate a new call
  async initiateCall(
    callId: string,
    initiatorId: string,
    participantIds: string[],
    type: 'voice' | 'video',
    chatId?: string
  ): Promise<CallSession> {
    try {
      // Create call in database
      await this.callRepository.create({
        callId,
        initiator: initiatorId as any,
        participants: [initiatorId, ...participantIds].map(id => id as any),
        type,
        status: 'initiated',
        chatId: chatId as any,
        isGroupCall: participantIds.length > 1,
      });

      // Create local session
      const session: CallSession = {
        callId,
        initiator: initiatorId,
        participants: [initiatorId, ...participantIds],
        status: 'initiating',
        startTime: new Date(),
        signaling: {
          offers: new Map(),
          answers: new Map(),
          iceCandidates: new Map(),
        },
      };

      this.activeCalls.set(callId, session);

      // Notify participants via Socket.IO
      participantIds.forEach(participantId => {
        socketManager.emitToUser(participantId, 'call:incoming', {
          callId,
          type,
          initiator: initiatorId,
          chatId,
        });
      });

      return session;
    } catch (error) {
      console.error('Error initiating call:', error);
      throw new Error('Failed to initiate call');
    }
  }

  // Handle WebRTC offer
  async handleOffer(callId: string, senderId: string, offer: RTCSessionDescriptionInit): Promise<void> {
    try {
      const session = this.activeCalls.get(callId);
      if (!session) {
        throw new Error('Call session not found');
      }

      // Store offer in session
      session.signaling.offers.set(senderId, offer);

      // Store in database
      await this.callRepository.addOffer(callId, senderId as any, JSON.stringify(offer));

      // Forward offer to other participants
      session.participants
        .filter(id => id !== senderId)
        .forEach(participantId => {
          socketManager.emitToUser(participantId, 'call:offer', {
            callId,
            offer,
            senderId,
          });
        });

    } catch (error) {
      console.error('Error handling offer:', error);
      throw new Error('Failed to handle offer');
    }
  }

  // Handle WebRTC answer
  async handleAnswer(callId: string, senderId: string, answer: RTCSessionDescriptionInit): Promise<void> {
    try {
      const session = this.activeCalls.get(callId);
      if (!session) {
        throw new Error('Call session not found');
      }

      // Store answer in session
      session.signaling.answers.set(senderId, answer);

      // Store in database
      await this.callRepository.addAnswer(callId, senderId as any, JSON.stringify(answer));

      // Update call status to connected
      session.status = 'connected';
      await this.callRepository.update(
        (await this.callRepository.findByCallId(callId))?._id!,
        { status: 'answered' }
      );

      // Forward answer to other participants
      session.participants
        .filter(id => id !== senderId)
        .forEach(participantId => {
          socketManager.emitToUser(participantId, 'call:answer', {
            callId,
            answer,
            senderId,
          });
        });

    } catch (error) {
      console.error('Error handling answer:', error);
      throw new Error('Failed to handle answer');
    }
  }

  // Handle ICE candidate
  async handleIceCandidate(callId: string, senderId: string, candidate: RTCIceCandidateInit): Promise<void> {
    try {
      const session = this.activeCalls.get(callId);
      if (!session) {
        throw new Error('Call session not found');
      }

      // Store ICE candidate in session
      if (!session.signaling.iceCandidates.has(senderId)) {
        session.signaling.iceCandidates.set(senderId, []);
      }
      session.signaling.iceCandidates.get(senderId)!.push(candidate);

      // Store in database
      await this.callRepository.addIceCandidate(callId, senderId as any, JSON.stringify(candidate));

      // Forward ICE candidate to other participants
      session.participants
        .filter(id => id !== senderId)
        .forEach(participantId => {
          socketManager.emitToUser(participantId, 'call:ice-candidate', {
            callId,
            candidate,
            senderId,
          });
        });

    } catch (error) {
      console.error('Error handling ICE candidate:', error);
      throw new Error('Failed to handle ICE candidate');
    }
  }

  // End call
  async endCall(callId: string, endedBy: string, reason: 'normal' | 'busy' | 'missed' | 'rejected' = 'normal'): Promise<void> {
    try {
      const session = this.activeCalls.get(callId);
      if (!session) {
        throw new Error('Call session not found');
      }

      // Update session status
      session.status = 'ended';

      // Update database
      const status = reason === 'normal' ? 'ended' : reason;
      await this.callRepository.endCall(callId, status as any);

      // Notify all participants
      session.participants.forEach(participantId => {
        socketManager.emitToUser(participantId, 'call:ended', {
          callId,
          endedBy,
          reason,
        });
      });

      // Clean up session
      this.activeCalls.delete(callId);

    } catch (error) {
      console.error('Error ending call:', error);
      throw new Error('Failed to end call');
    }
  }

  // Get active call session
  getCallSession(callId: string): CallSession | undefined {
    return this.activeCalls.get(callId);
  }

  // Get all active calls for a user
  getUserActiveCalls(userId: string): CallSession[] {
    return Array.from(this.activeCalls.values())
      .filter(session => session.participants.includes(userId));
  }

  // Check if user is in any active call
  isUserInCall(userId: string): boolean {
    return this.getUserActiveCalls(userId).length > 0;
  }

  // Clean up expired calls (should be called periodically)
  cleanupExpiredCalls(): void {
    const now = new Date();
    const maxCallDuration = 4 * 60 * 60 * 1000; // 4 hours

    for (const [callId, session] of this.activeCalls.entries()) {
      const callDuration = now.getTime() - session.startTime.getTime();
      
      if (callDuration > maxCallDuration) {
        console.log(`Cleaning up expired call: ${callId}`);
        this.endCall(callId, 'system', 'normal');
      }
    }
  }
}

export const webrtcSignalingService = new WebRTCSignalingService();
