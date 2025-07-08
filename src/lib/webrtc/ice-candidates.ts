import { EventEmitter } from 'events';

interface ICECandidate {
  candidate: string;
  sdpMLineIndex?: number | null;
  sdpMid?: string | null;
  usernameFragment?: string | null;
}

interface ICEGatheringState {
  callId: string;
  userId: string;
  state: 'new' | 'gathering' | 'complete';
  candidates: ICECandidate[];
}

export class ICECandidateManager extends EventEmitter {
  private gatheringStates: Map<string, ICEGatheringState> = new Map();
  private batchTimeout = 100; // 100ms batching
  private batchQueues: Map<string, ICECandidate[]> = new Map();
  private batchTimers: Map<string, NodeJS.Timeout> = new Map();

  // Start ICE gathering for a call participant
  startGathering(callId: string, userId: string): void {
    const key = `${callId}:${userId}`;
    
    this.gatheringStates.set(key, {
      callId,
      userId,
      state: 'new',
      candidates: [],
    });

    this.emit('gatheringStarted', { callId, userId });
  }

  // Add ICE candidate (with batching for performance)
  addCandidate(callId: string, userId: string, candidate: ICECandidate): void {
    const key = `${callId}:${userId}`;
    const state = this.gatheringStates.get(key);

    if (!state) {
      console.warn(`No gathering state found for ${key}`);
      return;
    }

    // Update state to gathering if it was new
    if (state.state === 'new') {
      state.state = 'gathering';
    }

    // Add to candidates list
    state.candidates.push(candidate);

    // Add to batch queue
    if (!this.batchQueues.has(key)) {
      this.batchQueues.set(key, []);
    }
    this.batchQueues.get(key)!.push(candidate);

    // Clear existing timer and set new one for batching
    const existingTimer = this.batchTimers.get(key);
    if (existingTimer) {
      clearTimeout(existingTimer);
    }

    const timer = setTimeout(() => {
      this.flushBatch(key);
    }, this.batchTimeout);

    this.batchTimers.set(key, timer);
  }

  // Flush batched candidates
  private flushBatch(key: string): void {
    const candidates = this.batchQueues.get(key);
    if (!candidates || candidates.length === 0) {
      return;
    }

    const [callId, userId] = key.split(':');
    
    this.emit('candidatesBatch', {
      callId,
      userId,
      candidates: [...candidates],
    });

    // Clear batch
    this.batchQueues.set(key, []);
    this.batchTimers.delete(key);
  }

  // Mark ICE gathering as complete
  completeGathering(callId: string, userId: string): void {
    const key = `${callId}:${userId}`;
    const state = this.gatheringStates.get(key);

    if (!state) {
      return;
    }

    state.state = 'complete';
    
    // Flush any remaining candidates
    this.flushBatch(key);

    this.emit('gatheringComplete', {
      callId,
      userId,
      totalCandidates: state.candidates.length,
    });
  }

  // Get ICE candidates for a specific participant
  getCandidates(callId: string, userId: string): ICECandidate[] {
    const key = `${callId}:${userId}`;
    const state = this.gatheringStates.get(key);
    return state ? [...state.candidates] : [];
  }

  // Get gathering state
  getGatheringState(callId: string, userId: string): ICEGatheringState | undefined {
    const key = `${callId}:${userId}`;
    return this.gatheringStates.get(key);
  }

  // Filter and prioritize ICE candidates
  filterCandidates(candidates: ICECandidate[]): ICECandidate[] {
    // Sort candidates by preference:
    // 1. Host candidates (local)
    // 2. Server reflexive (STUN)
    // 3. Relay (TURN)
    return candidates.sort((a, b) => {
      const getPriority = (candidate: ICECandidate): number => {
        const candidateStr = candidate.candidate;
        if (candidateStr.includes('typ host')) return 3;
        if (candidateStr.includes('typ srflx')) return 2;
        if (candidateStr.includes('typ relay')) return 1;
        return 0;
      };

      return getPriority(b) - getPriority(a);
    });
  }

  // Clean up gathering state when call ends
  cleanup(callId: string): void {
    const keysToDelete: string[] = [];
    
    for (const [key, state] of this.gatheringStates.entries()) {
      if (state.callId === callId) {
        keysToDelete.push(key);
        
        // Clear any pending timers
        const timer = this.batchTimers.get(key);
        if (timer) {
          clearTimeout(timer);
          this.batchTimers.delete(key);
        }
      }
    }

    keysToDelete.forEach(key => {
      this.gatheringStates.delete(key);
      this.batchQueues.delete(key);
    });

    this.emit('cleanup', { callId });
  }

  // Get statistics for monitoring
  getStats(): {
    activeGatherings: number;
    totalCandidates: number;
    completedGatherings: number;
  } {
    let totalCandidates = 0;
    let completedGatherings = 0;

    for (const state of this.gatheringStates.values()) {
      totalCandidates += state.candidates.length;
      if (state.state === 'complete') {
        completedGatherings++;
      }
    }

    return {
      activeGatherings: this.gatheringStates.size,
      totalCandidates,
      completedGatherings,
    };
  }
}

export const iceCandidateManager = new ICECandidateManager();
