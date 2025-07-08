import { webrtcSignalingService } from './signaling';
import { iceCandidateManager } from './ice-candidates';
import { coturnManager } from './coturn';
import { CallRepository } from '../database/repositories/call';
import { UserRepository } from '../database/repositories/user';
import { socketManager } from '../realtime/socket';

interface CallOptions {
  type: 'voice' | 'video';
  chatId?: string;
  groupCall?: boolean;
  maxParticipants?: number;
}

interface CallQuality {
  audioLevel: number;
  videoQuality: 'low' | 'medium' | 'high';
  connectionQuality: 'poor' | 'fair' | 'good' | 'excellent';
  packetLoss: number;
  latency: number;
}

export class CallManager {
  private callRepository: CallRepository;
  private userRepository: UserRepository;
  private activeCallChecks: Map<string, NodeJS.Timeout> = new Map();

  constructor() {
    this.callRepository = new CallRepository();
    this.userRepository = new UserRepository();
    
    // Set up ICE candidate event handlers
    this.setupICEEventHandlers();
    
    // Start periodic cleanup
    this.startPeriodicCleanup();
  }

  // Initiate a new call
  async initiateCall(
    initiatorId: string,
    participantIds: string[],
    options: CallOptions
  ): Promise<{ callId: string; iceServers: RTCIceServer[] }> {
    try {
      // Validate participants
      await this.validateCallParticipants(initiatorId, participantIds);

      // Check if any participant is already in a call
      const busyParticipants = await this.checkBusyParticipants([initiatorId, ...participantIds]);
      if (busyParticipants.length > 0) {
        throw new Error(`Participants are busy: ${busyParticipants.join(', ')}`);
      }

      // Generate unique call ID
      const callId = `call_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;

      // Get ICE servers for the initiator
      const iceServers = coturnManager.getICEServers(initiatorId);

      // Start the call session
      await webrtcSignalingService.initiateCall(
        callId,
        initiatorId,
        participantIds,
        options.type,
        options.chatId
      );

      // Start ICE gathering for initiator
      iceCandidateManager.startGathering(callId, initiatorId);

      // Set up call quality monitoring
      this.startCallQualityMonitoring(callId);

      return { callId, iceServers };

    } catch (error) {
      console.error('Error initiating call:', error);
      throw error;
    }
  }

  // Answer an incoming call
  async answerCall(callId: string, userId: string): Promise<{ iceServers: RTCIceServer[] }> {
    try {
      const session = webrtcSignalingService.getCallSession(callId);
      if (!session) {
        throw new Error('Call not found');
      }

      if (!session.participants.includes(userId)) {
        throw new Error('User not authorized to answer this call');
      }

      // Get ICE servers for the user
      const iceServers = coturnManager.getICEServers(userId);

      // Start ICE gathering for answering user
      iceCandidateManager.startGathering(callId, userId);

      // Update call status
      await this.callRepository.update(
        (await this.callRepository.findByCallId(callId))?._id!,
        { status: 'answered' }
      );

      // Notify other participants
      session.participants
        .filter(id => id !== userId)
        .forEach(participantId => {
          socketManager.emitToUser(participantId, 'call:participant-joined', {
            callId,
            userId,
          });
        });

      return { iceServers };

    } catch (error) {
      console.error('Error answering call:', error);
      throw error;
    }
  }

  // Reject a call
  async rejectCall(callId: string, userId: string, reason: string = 'rejected'): Promise<void> {
    try {
      await webrtcSignalingService.endCall(callId, userId, 'rejected');
      
      // Clean up ICE gathering
      iceCandidateManager.cleanup(callId);
      
      // Stop quality monitoring
      this.stopCallQualityMonitoring(callId);

    } catch (error) {
      console.error('Error rejecting call:', error);
      throw error;
    }
  }

  // End a call
  async endCall(callId: string, userId: string): Promise<void> {
    try {
      await webrtcSignalingService.endCall(callId, userId, 'normal');
      
      // Clean up ICE gathering
      iceCandidateManager.cleanup(callId);
      
      // Stop quality monitoring
      this.stopCallQualityMonitoring(callId);

    } catch (error) {
      console.error('Error ending call:', error);
      throw error;
    }
  }

  // Handle WebRTC offer
  async handleOffer(callId: string, userId: string, offer: RTCSessionDescriptionInit): Promise<void> {
    return webrtcSignalingService.handleOffer(callId, userId, offer);
  }

  // Handle WebRTC answer
  async handleAnswer(callId: string, userId: string, answer: RTCSessionDescriptionInit): Promise<void> {
    return webrtcSignalingService.handleAnswer(callId, userId, answer);
  }

  // Handle ICE candidate
  async handleIceCandidate(callId: string, userId: string, candidate: RTCIceCandidateInit): Promise<void> {
    // Add to ICE candidate manager for batching
    // Ensure candidate.candidate is a string
    if (typeof candidate.candidate !== 'string') {
      throw new Error('ICE candidate is missing the candidate string');
    }
    iceCandidateManager.addCandidate(callId, userId, candidate as RTCIceCandidateInit & { candidate: string });
    
    // Also handle through signaling service
    return webrtcSignalingService.handleIceCandidate(callId, userId, candidate);
  }

  // Update call quality metrics
  async updateCallQuality(callId: string, userId: string, quality: CallQuality): Promise<void> {
    try {
      // Store quality metrics (could be stored in Redis for real-time monitoring)
      const qualityData = {
        callId,
        userId,
        timestamp: new Date(),
        ...quality,
      };

      // Emit quality update to monitoring systems
      socketManager.emitToUser(userId, 'call:quality-update', qualityData);

      // If quality is poor, suggest quality improvements
      if (quality.connectionQuality === 'poor' || quality.packetLoss > 0.05) {
        this.suggestQualityImprovements(callId, userId, quality);
      }

    } catch (error) {
      console.error('Error updating call quality:', error);
    }
  }

  // Private helper methods

  private async validateCallParticipants(initiatorId: string, participantIds: string[]): Promise<void> {
    // Check if initiator exists
    const initiator = await this.userRepository.findById(initiatorId);
    if (!initiator || initiator.isBanned) {
      throw new Error('Initiator not found or banned');
    }

    // Check if all participants exist and are not banned
    for (const participantId of participantIds) {
      const participant = await this.userRepository.findById(participantId);
      if (!participant || participant.isBanned) {
        throw new Error(`Participant ${participantId} not found or banned`);
      }
    }
  }

  private async checkBusyParticipants(userIds: string[]): Promise<string[]> {
    const busyUsers: string[] = [];
    
    for (const userId of userIds) {
      if (webrtcSignalingService.isUserInCall(userId)) {
        busyUsers.push(userId);
      }
    }
    
    return busyUsers;
  }

  private setupICEEventHandlers(): void {
    iceCandidateManager.on('candidatesBatch', async ({ callId, userId, candidates }) => {
      // Handle batched ICE candidates
      try {
        await webrtcSignalingService.handleIceCandidate(callId, userId, candidates[0]);
      } catch (error) {
        console.error('Error handling ICE candidate batch:', error);
      }
    });

    iceCandidateManager.on('gatheringComplete', ({ callId, userId }) => {
      // Notify that ICE gathering is complete
      const session = webrtcSignalingService.getCallSession(callId);
      if (session) {
        session.participants
          .filter(id => id !== userId)
          .forEach(participantId => {
            socketManager.emitToUser(participantId, 'call:ice-gathering-complete', {
              callId,
              userId,
            });
          });
      }
    });
  }

  private startCallQualityMonitoring(callId: string): void {
    const interval = setInterval(() => {
      const session = webrtcSignalingService.getCallSession(callId);
      if (!session) {
        clearInterval(interval);
        return;
      }

      // Request quality updates from all participants
      session.participants.forEach(participantId => {
        socketManager.emitToUser(participantId, 'call:quality-request', { callId });
      });
    }, 10000); // Every 10 seconds

    this.activeCallChecks.set(callId, interval);
  }

  private stopCallQualityMonitoring(callId: string): void {
    const interval = this.activeCallChecks.get(callId);
    if (interval) {
      clearInterval(interval);
      this.activeCallChecks.delete(callId);
    }
  }

  private suggestQualityImprovements(callId: string, userId: string, quality: CallQuality): void {
    const suggestions: string[] = [];

    if (quality.packetLoss > 0.05) {
      suggestions.push('Check your internet connection stability');
    }

    if (quality.latency > 200) {
      suggestions.push('Try moving closer to your router');
    }

    if (quality.connectionQuality === 'poor') {
      suggestions.push('Consider switching to audio-only mode');
    }

    if (suggestions.length > 0) {
      socketManager.emitToUser(userId, 'call:quality-suggestions', {
        callId,
        suggestions,
      });
    }
  }

  private startPeriodicCleanup(): void {
    // Clean up expired calls every 5 minutes
    setInterval(() => {
      webrtcSignalingService.cleanupExpiredCalls();
    }, 5 * 60 * 1000);
  }

  // Get call statistics
  async getCallStats(): Promise<any> {
    const iceStats = iceCandidateManager.getStats();
    const activeCalls = Array.from(webrtcSignalingService['activeCalls'].values());

    return {
      activeCalls: activeCalls.length,
      totalParticipants: activeCalls.reduce((total, call) => total + call.participants.length, 0),
      iceGathering: iceStats,
      callsByType: {
        voice: activeCalls.filter(call => call.callId.includes('voice')).length,
        video: activeCalls.filter(call => call.callId.includes('video')).length,
      },
    };
  }
}

export const callManager = new CallManager();
