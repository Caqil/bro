import { Types } from 'mongoose';
import { User, IUser } from '../models/user';

export class UserRepository {
  // Create user
  async create(userData: Partial<IUser>): Promise<IUser> {
    const user = new User(userData);
    return await user.save();
  }

  // Find user by ID
  async findById(id: string | Types.ObjectId): Promise<IUser | null> {
    return await User.findById(id).exec();
  }

  // Find user by phone number
  async findByPhoneNumber(phoneNumber: string): Promise<IUser | null> {
    return await User.findOne({ phoneNumber }).exec();
  }

  // Find user by email
  async findByEmail(email: string): Promise<IUser | null> {
    return await User.findOne({ email }).exec();
  }

  // Find user by username
  async findByUsername(username: string): Promise<IUser | null> {
    return await User.findOne({ username }).exec();
  }

  // Update user
  async update(id: string | Types.ObjectId, updateData: Partial<IUser>): Promise<IUser | null> {
    return await User.findByIdAndUpdate(id, updateData, { new: true }).exec();
  }

  // Delete user (soft delete by marking as banned)
  async delete(id: string | Types.ObjectId): Promise<boolean> {
    const result = await User.findByIdAndUpdate(id, { 
      isBanned: true, 
      banReason: 'Account deleted' 
    }).exec();
    return !!result;
  }

  // Search users
  async searchUsers(query: string, limit: number = 20, offset: number = 0): Promise<IUser[]> {
    const searchRegex = new RegExp(query, 'i');
    return await User.find({
      $or: [
        { displayName: { $regex: searchRegex } },
        { username: { $regex: searchRegex } },
        { phoneNumber: { $regex: searchRegex } }
      ],
      isBanned: false
    })
    .limit(limit)
    .skip(offset)
    .select('-__v -devices')
    .exec();
  }

  // Get user contacts
  async getUserContacts(userId: string | Types.ObjectId): Promise<IUser[]> {
    const user = await User.findById(userId).populate('contacts', 'displayName avatar phoneNumber isOnline lastSeen').exec();
    return user?.contacts as IUser[] || [];
  }

  // Add contact
  async addContact(userId: string | Types.ObjectId, contactId: string | Types.ObjectId): Promise<boolean> {
    const result = await User.findByIdAndUpdate(
      userId,
      { $addToSet: { contacts: contactId } },
      { new: true }
    ).exec();
    return !!result;
  }

  // Remove contact
  async removeContact(userId: string | Types.ObjectId, contactId: string | Types.ObjectId): Promise<boolean> {
    const result = await User.findByIdAndUpdate(
      userId,
      { $pull: { contacts: contactId } },
      { new: true }
    ).exec();
    return !!result;
  }

  // Block user
  async blockUser(userId: string | Types.ObjectId, blockedUserId: string | Types.ObjectId): Promise<boolean> {
    const result = await User.findByIdAndUpdate(
      userId,
      { $addToSet: { blockedUsers: blockedUserId } },
      { new: true }
    ).exec();
    return !!result;
  }

  // Unblock user
  async unblockUser(userId: string | Types.ObjectId, unblockedUserId: string | Types.ObjectId): Promise<boolean> {
    const result = await User.findByIdAndUpdate(
      userId,
      { $pull: { blockedUsers: unblockedUserId } },
      { new: true }
    ).exec();
    return !!result;
  }

  // Update online status
  async updateOnlineStatus(userId: string | Types.ObjectId, isOnline: boolean): Promise<void> {
    await User.findByIdAndUpdate(userId, {
      isOnline,
      lastSeen: new Date()
    }).exec();
  }

  // Get blocked users
  async getBlockedUsers(userId: string | Types.ObjectId): Promise<IUser[]> {
    const user = await User.findById(userId).populate('blockedUsers', 'displayName avatar phoneNumber').exec();
    return user?.blockedUsers as IUser[] || [];
  }

  // Verify user
  async verifyUser(userId: string | Types.ObjectId): Promise<boolean> {
    const result = await User.findByIdAndUpdate(userId, { isVerified: true }).exec();
    return !!result;
  }

  // Ban user
  async banUser(userId: string | Types.ObjectId, reason: string, expiresAt?: Date): Promise<boolean> {
    const result = await User.findByIdAndUpdate(userId, {
      isBanned: true,
      banReason: reason,
      banExpiresAt: expiresAt
    }).exec();
    return !!result;
  }

  // Unban user
  async unbanUser(userId: string | Types.ObjectId): Promise<boolean> {
    const result = await User.findByIdAndUpdate(userId, {
      isBanned: false,
      $unset: { banReason: 1, banExpiresAt: 1 }
    }).exec();
    return !!result;
  }

  // Get users with pagination (admin)
  async getUsers(limit: number = 20, offset: number = 0, filters?: any): Promise<{ users: IUser[], total: number }> {
    const query = filters || {};
    const [users, total] = await Promise.all([
      User.find(query).limit(limit).skip(offset).select('-__v').exec(),
      User.countDocuments(query).exec()
    ]);
    return { users, total };
  }
}
