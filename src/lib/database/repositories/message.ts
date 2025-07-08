import { Types } from 'mongoose';
import { Message, IMessage } from '../models/message';

export class MessageRepository {
  // Create message
  async create(messageData: Partial<IMessage>): Promise<IMessage> {
    const message = new Message(messageData);
    return await message.save();
  }

  // Find message by ID
  async findById(id: string | Types.ObjectId): Promise<IMessage | null> {
    return await Message.findById(id)
      .populate('senderId', 'displayName avatar')
      .populate('replyTo')
      .populate('media')
      .exec();
  }

  // Get chat messages
  async getChatMessages(
    chatId: string | Types.ObjectId, 
    limit: number = 50, 
    before?: Date,
    userId?: string | Types.ObjectId
  ): Promise<IMessage[]> {
    const query: any = { 
      chatId,
      isDeleted: false
    };

    // Filter out messages deleted for this user
    if (userId) {
      query.deletedFor = { $ne: userId };
    }

    if (before) {
      query.createdAt = { $lt: before };
    }

    return await Message.find(query)
      .populate('senderId', 'displayName avatar')
      .populate('replyTo')
      .populate('media')
      .sort({ createdAt: -1 })
      .limit(limit)
      .exec();
  }

  // Update message
  async update(id: string | Types.ObjectId, updateData: Partial<IMessage>): Promise<IMessage | null> {
    return await Message.findByIdAndUpdate(id, updateData, { new: true })
      .populate('senderId', 'displayName avatar')
      .exec();
  }

  // Delete message (soft delete)
  async delete(id: string | Types.ObjectId, userId?: string | Types.ObjectId): Promise<boolean> {
    if (userId) {
      // Delete for specific user
      const result = await Message.findByIdAndUpdate(id, {
        $addToSet: { deletedFor: userId }
      }).exec();
      return !!result;
    } else {
      // Delete for everyone
      const result = await Message.findByIdAndUpdate(id, {
        isDeleted: true,
        deletedAt: new Date()
      }).exec();
      return !!result;
    }
  }

  // Mark message as delivered
  async markAsDelivered(messageId: string | Types.ObjectId, userId: string | Types.ObjectId): Promise<boolean> {
    const result = await Message.findByIdAndUpdate(messageId, {
      $addToSet: {
        deliveredTo: {
          userId,
          deliveredAt: new Date()
        }
      }
    }).exec();
    return !!result;
  }

  // Mark message as read
  async markAsRead(messageId: string | Types.ObjectId, userId: string | Types.ObjectId): Promise<boolean> {
    const result = await Message.findByIdAndUpdate(messageId, {
      $addToSet: {
        readBy: {
          userId,
          readAt: new Date()
        }
      },
      status: 'read'
    }).exec();
    return !!result;
  }

  // Mark multiple messages as read
  async markMultipleAsRead(messageIds: (string | Types.ObjectId)[], userId: string | Types.ObjectId): Promise<number> {
    const result = await Message.updateMany(
      { 
        _id: { $in: messageIds },
        'readBy.userId': { $ne: userId }
      },
      {
        $addToSet: {
          readBy: {
            userId,
            readAt: new Date()
          }
        },
        status: 'read'
      }
    ).exec();
    return result.modifiedCount;
  }

  // Add reaction
  async addReaction(messageId: string | Types.ObjectId, userId: string | Types.ObjectId, emoji: string): Promise<boolean> {
    // Remove existing reaction from this user first
    await Message.findByIdAndUpdate(messageId, {
      $pull: { reactions: { userId } }
    }).exec();

    // Add new reaction
    const result = await Message.findByIdAndUpdate(messageId, {
      $addToSet: {
        reactions: {
          userId,
          emoji,
          createdAt: new Date()
        }
      }
    }).exec();
    return !!result;
  }

  // Remove reaction
  async removeReaction(messageId: string | Types.ObjectId, userId: string | Types.ObjectId): Promise<boolean> {
    const result = await Message.findByIdAndUpdate(messageId, {
      $pull: { reactions: { userId } }
    }).exec();
    return !!result;
  }

  // Get unread count for chat
  async getUnreadCount(chatId: string | Types.ObjectId, userId: string | Types.ObjectId): Promise<number> {
    return await Message.countDocuments({
      chatId,
      senderId: { $ne: userId },
      'readBy.userId': { $ne: userId },
      isDeleted: false,
      deletedFor: { $ne: userId }
    }).exec();
  }

  // Search messages
  async searchMessages(
    query: string, 
    chatId?: string | Types.ObjectId, 
    limit: number = 20, 
    offset: number = 0
  ): Promise<IMessage[]> {
    const searchRegex = new RegExp(query, 'i');
    const searchQuery: any = {
      content: { $regex: searchRegex },
      isDeleted: false
    };

    if (chatId) {
      searchQuery.chatId = chatId;
    }

    return await Message.find(searchQuery)
      .populate('senderId', 'displayName avatar')
      .populate('chatId', 'type groupInfo.name participants')
      .sort({ createdAt: -1 })
      .limit(limit)
      .skip(offset)
      .exec();
  }

  // Get message analytics (admin)
  async getMessageAnalytics(startDate: Date, endDate: Date): Promise<any> {
    return await Message.aggregate([
      {
        $match: {
          createdAt: { $gte: startDate, $lte: endDate },
          isDeleted: false
        }
      },
      {
        $group: {
          _id: {
            date: { $dateToString: { format: '%Y-%m-%d', date: '$createdAt' } },
            type: '$type'
          },
          count: { $sum: 1 }
        }
      },
      {
        $sort: { '_id.date': 1 }
      }
    ]).exec();
  }

  // Get recent messages (admin)
  async getRecentMessages(limit: number = 50): Promise<IMessage[]> {
    return await Message.find({ isDeleted: false })
      .populate('senderId', 'displayName phoneNumber')
      .populate('chatId', 'type groupInfo.name')
      .sort({ createdAt: -1 })
      .limit(limit)
      .exec();
  }
}

