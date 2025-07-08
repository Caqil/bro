import { Types } from 'mongoose';
import { Call, ICall } from '../models/call';

export class CallRepository {
  // Create call
  async create(callData: Partial<ICall>): Promise<ICall> {
    const call = new Call(callData);
    return await call.save();
  }

  // Find call by ID
  async findById(id: string | Types.ObjectId): Promise<ICall | null> {
    return await Call.findById(id)
      .populate('initiator', 'displayName avatar')
      .populate('participants', 'displayName avatar')
      .exec();
  }

  // Find call by call ID
  async findByCallId(callId: string): Promise<ICall | null> {
    return await Call.findOne({ callId })
      .populate('initiator', 'displayName avatar')
      .populate('participants', 'displayName avatar')
      .exec();
  }

  // Update call
  async update(id: string | Types.ObjectId, updateData: Partial<ICall>): Promise<ICall | null> {
    return await Call.findByIdAndUpdate(id, updateData, { new: true }).exec();
  }

  // End call
  async endCall(callId: string, status: 'ended' | 'missed' | 'rejected' | 'busy'): Promise<boolean> {
    const endTime = new Date();
    const call = await Call.findOne({ callId }).exec();
    
    if (!call) return false;

    const duration = Math.floor((endTime.getTime() - call.startTime.getTime()) / 1000);

    const result = await Call.findOneAndUpdate(
      { callId },
      {
        status,
        endTime,
        duration: status === 'ended' ? duration : 0
      }
    ).exec();

    return !!result;
  }

  // Get user call history
  async getUserCallHistory(
    userId: string | Types.ObjectId,
    limit: number = 50,
    offset: number = 0
  ): Promise<ICall[]> {
    return await Call.find({
      $or: [
        { initiator: userId },
        { participants: userId }
      ]
    })
    .populate('initiator', 'displayName avatar phoneNumber')
    .populate('participants', 'displayName avatar phoneNumber')
    .sort({ startTime: -1 })
    .limit(limit)
    .skip(offset)
    .exec();
  }

  // Add ICE candidate
  async addIceCandidate(callId: string, userId: string | Types.ObjectId, candidate: string): Promise<boolean> {
    const result = await Call.findOneAndUpdate(
      { callId },
      {
        $push: {
          'signaling.iceCandidates': {
            userId,
            candidate,
            timestamp: new Date()
          }
        }
      }
    ).exec();
    return !!result;
  }

  // Add SDP offer
  async addOffer(callId: string, userId: string | Types.ObjectId, sdp: string): Promise<boolean> {
    const result = await Call.findOneAndUpdate(
      { callId },
      {
        $push: {
          'signaling.offers': {
            userId,
            sdp,
            timestamp: new Date()
          }
        }
      }
    ).exec();
    return !!result;
  }

  // Add SDP answer
  async addAnswer(callId: string, userId: string | Types.ObjectId, sdp: string): Promise<boolean> {
    const result = await Call.findOneAndUpdate(
      { callId },
      {
        $push: {
          'signaling.answers': {
            userId,
            sdp,
            timestamp: new Date()
          }
        }
      }
    ).exec();
    return !!result;
  }

  // Add call quality rating
  async addQualityRating(callId: string, userId: string | Types.ObjectId, rating: number, feedback?: string): Promise<boolean> {
    const result = await Call.findOneAndUpdate(
      { callId },
      {
        $push: {
          quality: {
            userId,
            rating,
            feedback
          }
        }
      }
    ).exec();
    return !!result;
  }

  // Get call analytics
  async getCallAnalytics(startDate: Date, endDate: Date): Promise<any> {
    return await Call.aggregate([
      {
        $match: {
          startTime: { $gte: startDate, $lte: endDate }
        }
      },
      {
        $group: {
          _id: {
            date: { $dateToString: { format: '%Y-%m-%d', date: '$startTime' } },
            type: '$type',
            status: '$status'
          },
          count: { $sum: 1 },
          totalDuration: { $sum: '$duration' }
        }
      },
      {
        $sort: { '_id.date': 1 }
      }
    ]).exec();
  }
}

