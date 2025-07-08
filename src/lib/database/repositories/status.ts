import { Types } from 'mongoose';
import { Status, IStatus } from '../models/status';

export class StatusRepository {
  // Create status
  async create(statusData: Partial<IStatus>): Promise<IStatus> {
    const status = new Status(statusData);
    return await status.save();
  }

  // Find status by ID
  async findById(id: string | Types.ObjectId): Promise<IStatus | null> {
    return await Status.findById(id)
      .populate('userId', 'displayName avatar phoneNumber')
      .populate('media')
      .exec();
  }

  // Get user's active status
  async getUserActiveStatus(userId: string | Types.ObjectId): Promise<IStatus[]> {
    return await Status.find({
      userId,
      isActive: true,
      expiresAt: { $gt: new Date() }
    })
    .populate('media')
    .sort({ createdAt: -1 })
    .exec();
  }

  // Get recent status updates from contacts
  async getContactsStatus(contactIds: (string | Types.ObjectId)[], userId: string | Types.ObjectId): Promise<IStatus[]> {
    return await Status.find({
      userId: { $in: contactIds },
      isActive: true,
      expiresAt: { $gt: new Date() },
      $or: [
        { 'privacySettings.viewableBy': 'everyone' },
        { 'privacySettings.viewableBy': 'contacts' },
        { 
          'privacySettings.viewableBy': 'specific',
          'privacySettings.allowedUsers': userId
        }
      ],
      'privacySettings.blockedUsers': { $ne: userId }
    })
    .populate('userId', 'displayName avatar phoneNumber')
    .populate('media')
    .sort({ createdAt: -1 })
    .exec();
  }

  // Mark status as viewed
  async markAsViewed(statusId: string | Types.ObjectId, userId: string | Types.ObjectId): Promise<boolean> {
    const result = await Status.findByIdAndUpdate(statusId, {
      $addToSet: {
        viewers: {
          userId,
          viewedAt: new Date()
        }
      }
    }).exec();
    return !!result;
  }

  // Delete status
  async delete(id: string | Types.ObjectId): Promise<boolean> {
    const result = await Status.findByIdAndUpdate(id, {
      isActive: false
    }).exec();
    return !!result;
  }

  // Get status viewers
  async getStatusViewers(statusId: string | Types.ObjectId): Promise<any[]> {
    const status = await Status.findById(statusId)
      .populate('viewers.userId', 'displayName avatar phoneNumber')
      .exec();
    
    return status?.viewers || [];
  }

  // Cleanup expired status
  async cleanupExpiredStatus(): Promise<number> {
    const result = await Status.updateMany(
      {
        expiresAt: { $lte: new Date() },
        isActive: true
      },
      {
        isActive: false
      }
    ).exec();
    return result.modifiedCount;
  }

  // Get status analytics
  async getStatusAnalytics(userId: string | Types.ObjectId, startDate: Date, endDate: Date): Promise<any> {
    return await Status.aggregate([
      {
        $match: {
          userId: new Types.ObjectId(userId as string),
          createdAt: { $gte: startDate, $lte: endDate }
        }
      },
      {
        $group: {
          _id: { $dateToString: { format: '%Y-%m-%d', date: '$createdAt' } },
          totalStatus: { $sum: 1 },
          totalViews: { $sum: { $size: '$viewers' } }
        }
      },
      {
        $sort: { '_id': 1 }
      }
    ]).exec();
  }
}