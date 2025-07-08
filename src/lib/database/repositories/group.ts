import { Types } from 'mongoose';
import { Chat, IChat } from '../models/chat';
import { User } from '../models/user';
import { ChatRepository } from './chat';

export class GroupRepository extends ChatRepository {
  // Create group
  async createGroup(groupData: {
    name: string;
    description?: string;
    participants: (string | Types.ObjectId)[];
    createdBy: string | Types.ObjectId;
    avatar?: string;
  }): Promise<IChat> {
    const group = await this.create({
      type: 'group',
      participants: [
        ...groupData.participants.map(id => typeof id === 'string' ? new Types.ObjectId(id) : id),
        typeof groupData.createdBy === 'string' ? new Types.ObjectId(groupData.createdBy) : groupData.createdBy
      ],
      groupInfo: {
        name: groupData.name,
        description: groupData.description,
        avatar: groupData.avatar,
        admins: [
          typeof groupData.createdBy === 'string'
            ? new Types.ObjectId(groupData.createdBy)
            : groupData.createdBy
        ],
        settings: {
          whoCanSendMessages: 'everyone',
          whoCanEditGroupInfo: 'admins',
          whoCanAddMembers: 'admins',
        },
      },
    });

    return group;
  }

  // Get all groups for a user
  async getUserGroups(userId: string | Types.ObjectId, limit: number = 20, offset: number = 0): Promise<IChat[]> {
    return await Chat.find({
      type: 'group',
      participants: userId,
      isArchived: false
    })
    .populate('participants', 'displayName avatar phoneNumber isOnline lastSeen')
    .populate('lastMessage')
    .populate('groupInfo.admins', 'displayName avatar')
    .sort({ lastActivity: -1 })
    .limit(limit)
    .skip(offset)
    .exec();
  }

  // Update group info
  async updateGroupInfo(
    groupId: string | Types.ObjectId, 
    updates: {
      name?: string;
      description?: string;
      avatar?: string;
    }
  ): Promise<IChat | null> {
    const updateData: any = {};
    
    if (updates.name) updateData['groupInfo.name'] = updates.name;
    if (updates.description !== undefined) updateData['groupInfo.description'] = updates.description;
    if (updates.avatar !== undefined) updateData['groupInfo.avatar'] = updates.avatar;

    return await Chat.findByIdAndUpdate(groupId, updateData, { new: true })
      .populate('participants', 'displayName avatar phoneNumber isOnline lastSeen')
      .populate('groupInfo.admins', 'displayName avatar')
      .exec();
  }

  // Update group settings
  async updateGroupSettings(
    groupId: string | Types.ObjectId,
    settings: {
      whoCanSendMessages?: 'everyone' | 'admins';
      whoCanEditGroupInfo?: 'everyone' | 'admins';
      whoCanAddMembers?: 'everyone' | 'admins';
    }
  ): Promise<IChat | null> {
    const updateData: any = {};
    
    Object.entries(settings).forEach(([key, value]) => {
      if (value !== undefined) {
        updateData[`groupInfo.settings.${key}`] = value;
      }
    });

    return await Chat.findByIdAndUpdate(groupId, updateData, { new: true }).exec();
  }

  // Check if user is admin
  async isUserAdmin(groupId: string | Types.ObjectId, userId: string | Types.ObjectId): Promise<boolean> {
    const group = await Chat.findById(groupId).exec();
    return group?.groupInfo?.admins.includes(userId as Types.ObjectId) || false;
  }

  // Get group admins
  async getGroupAdmins(groupId: string | Types.ObjectId): Promise<any[]> {
    const group = await Chat.findById(groupId)
      .populate('groupInfo.admins', 'displayName avatar phoneNumber')
      .exec();
    
    return group?.groupInfo?.admins || [];
  }

  // Generate invite link
  async generateInviteLink(groupId: string | Types.ObjectId, expiresIn: number = 86400): Promise<string> {
    const inviteCode = require('crypto').randomUUID();
    const expiresAt = new Date(Date.now() + expiresIn * 1000);
    
    // Store invite in a separate collection or cache
    // For now, we'll return the invite code
    return `https://chat.app/invite/${inviteCode}`;
  }

  // Join group via invite
  async joinGroupByInvite(inviteCode: string, userId: string | Types.ObjectId): Promise<IChat | null> {
    // In real implementation, you'd validate the invite code and get the group ID
    // For now, this is a placeholder
    throw new Error('Not implemented - requires invite system');
  }

  // Leave group
  async leaveGroup(groupId: string | Types.ObjectId, userId: string | Types.ObjectId): Promise<boolean> {
    // Remove user from participants
    await this.removeParticipant(groupId, userId);
    
    // If user was admin, remove from admins
    await Chat.findByIdAndUpdate(groupId, {
      $pull: { 'groupInfo.admins': userId }
    }).exec();

    return true;
  }

  // Get group member count
  async getMemberCount(groupId: string | Types.ObjectId): Promise<number> {
    const group = await Chat.findById(groupId).exec();
    return group?.participants.length || 0;
  }

  // Search groups
  async searchGroups(query: string, userId: string | Types.ObjectId, limit: number = 20): Promise<IChat[]> {
    const searchRegex = new RegExp(query, 'i');
    
    return await Chat.find({
      type: 'group',
      participants: userId,
      'groupInfo.name': { $regex: searchRegex }
    })
    .populate('participants', 'displayName avatar')
    .populate('groupInfo.admins', 'displayName avatar')
    .limit(limit)
    .exec();
  }
}