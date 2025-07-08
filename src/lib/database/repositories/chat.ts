import { Types } from 'mongoose';
import { Chat, IChat } from '../models/chat';
import { Message } from '../models/message';

export class ChatRepository {
  // Create chat
  async create(chatData: Partial<IChat>): Promise<IChat> {
    const chat = new Chat(chatData);
    return await chat.save();
  }

  // Find chat by ID
  async findById(id: string | Types.ObjectId): Promise<IChat | null> {
    return await Chat.findById(id)
      .populate('participants', 'displayName avatar phoneNumber isOnline lastSeen')
      .populate('lastMessage')
      .exec();
  }

  // Get user chats
  async getUserChats(userId: string | Types.ObjectId, limit: number = 20, offset: number = 0): Promise<IChat[]> {
    return await Chat.find({
      participants: userId,
      isArchived: false
    })
    .populate('participants', 'displayName avatar phoneNumber isOnline lastSeen')
    .populate('lastMessage')
    .sort({ lastActivity: -1 })
    .limit(limit)
    .skip(offset)
    .exec();
  }

  // Find direct chat between two users
  async findDirectChat(user1Id: string | Types.ObjectId, user2Id: string | Types.ObjectId): Promise<IChat | null> {
    return await Chat.findOne({
      type: 'direct',
      participants: { $all: [user1Id, user2Id], $size: 2 }
    })
    .populate('participants', 'displayName avatar phoneNumber isOnline lastSeen')
    .exec();
  }

  // Update chat
  async update(id: string | Types.ObjectId, updateData: Partial<IChat>): Promise<IChat | null> {
    return await Chat.findByIdAndUpdate(id, updateData, { new: true })
      .populate('participants', 'displayName avatar phoneNumber isOnline lastSeen')
      .exec();
  }

  // Delete chat
  async delete(id: string | Types.ObjectId): Promise<boolean> {
    const result = await Chat.findByIdAndDelete(id).exec();
    return !!result;
  }

  // Update last activity
  async updateLastActivity(chatId: string | Types.ObjectId, messageId?: string | Types.ObjectId): Promise<void> {
    const updateData: any = { lastActivity: new Date() };
    if (messageId) {
      updateData.lastMessage = messageId;
    }
    await Chat.findByIdAndUpdate(chatId, updateData).exec();
  }

  // Archive/Unarchive chat
  async archiveChat(chatId: string | Types.ObjectId, isArchived: boolean): Promise<boolean> {
    const result = await Chat.findByIdAndUpdate(chatId, { isArchived }).exec();
    return !!result;
  }

  // Pin/Unpin chat
  async pinChat(chatId: string | Types.ObjectId, isPinned: boolean): Promise<boolean> {
    const result = await Chat.findByIdAndUpdate(chatId, { isPinned }).exec();
    return !!result;
  }

  // Mute chat
  async muteChat(chatId: string | Types.ObjectId, mutedUntil?: Date): Promise<boolean> {
    const result = await Chat.findByIdAndUpdate(chatId, { mutedUntil }).exec();
    return !!result;
  }

  // Clear chat history
  async clearHistory(chatId: string | Types.ObjectId): Promise<boolean> {
    // Delete all messages in the chat
    await Message.deleteMany({ chatId }).exec();
    
    // Update chat last message
    const result = await Chat.findByIdAndUpdate(chatId, {
      $unset: { lastMessage: 1 },
      lastActivity: new Date()
    }).exec();
    
    return !!result;
  }

  // Add participants to group
  async addParticipants(chatId: string | Types.ObjectId, participantIds: (string | Types.ObjectId)[]): Promise<boolean> {
    const result = await Chat.findByIdAndUpdate(
      chatId,
      { $addToSet: { participants: { $each: participantIds } } }
    ).exec();
    return !!result;
  }

  // Remove participant from group
  async removeParticipant(chatId: string | Types.ObjectId, participantId: string | Types.ObjectId): Promise<boolean> {
    const result = await Chat.findByIdAndUpdate(
      chatId,
      { $pull: { participants: participantId } }
    ).exec();
    return !!result;
  }

  // Promote user to admin
  async promoteToAdmin(chatId: string | Types.ObjectId, userId: string | Types.ObjectId): Promise<boolean> {
    const result = await Chat.findByIdAndUpdate(
      chatId,
      { $addToSet: { 'groupInfo.admins': userId } }
    ).exec();
    return !!result;
  }

  // Demote admin
  async demoteAdmin(chatId: string | Types.ObjectId, userId: string | Types.ObjectId): Promise<boolean> {
    const result = await Chat.findByIdAndUpdate(
      chatId,
      { $pull: { 'groupInfo.admins': userId } }
    ).exec();
    return !!result;
  }

  // Search chats
  async searchChats(userId: string | Types.ObjectId, query: string): Promise<IChat[]> {
    const searchRegex = new RegExp(query, 'i');
    
    return await Chat.find({
      participants: userId,
      $or: [
        { 'groupInfo.name': { $regex: searchRegex } },
        { 'groupInfo.description': { $regex: searchRegex } }
      ]
    })
    .populate('participants', 'displayName avatar phoneNumber')
    .limit(20)
    .exec();
  }
}
