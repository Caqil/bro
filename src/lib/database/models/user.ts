import mongoose, { Schema, Document, Types } from 'mongoose';

export interface IUser extends Document {
  _id: Types.ObjectId;
  phoneNumber: string;
  email?: string;
  username?: string;
  displayName: string;
  avatar?: string;
  status: string;
  isOnline: boolean;
  lastSeen: Date;
  isVerified: boolean;
  isBanned: boolean;
  banReason?: string;
  banExpiresAt?: Date;
  createdAt: Date;
  updatedAt: Date;
  
  // Privacy Settings
  privacySettings: {
    lastSeen: 'everyone' | 'contacts' | 'nobody';
    profilePhoto: 'everyone' | 'contacts' | 'nobody';
    status: 'everyone' | 'contacts' | 'nobody';
    readReceipts: boolean;
    groupInvites: 'everyone' | 'contacts' | 'nobody';
  };
  
  // Notification Settings
  notificationSettings: {
    messageNotifications: boolean;
    groupNotifications: boolean;
    callNotifications: boolean;
    emailNotifications: boolean;
    pushToken?: string;
  };
  
  // Contact Lists
  contacts: Types.ObjectId[];
  blockedUsers: Types.ObjectId[];
  
  // Device Information
  devices: {
    deviceId: string;
    platform: 'ios' | 'android' | 'web';
    lastActive: Date;
    pushToken?: string;
  }[];
}

const userSchema = new Schema<IUser>({
  phoneNumber: { type: String, required: true, unique: true, index: true },
  email: { type: String, sparse: true, unique: true },
  username: { type: String, sparse: true, unique: true },
  displayName: { type: String, required: true },
  avatar: { type: String },
  status: { type: String, default: 'Hey there! I am using WhatsApp.' },
  isOnline: { type: Boolean, default: false },
  lastSeen: { type: Date, default: Date.now },
  isVerified: { type: Boolean, default: false },
  isBanned: { type: Boolean, default: false },
  banReason: { type: String },
  banExpiresAt: { type: Date },
  
  privacySettings: {
    lastSeen: { type: String, enum: ['everyone', 'contacts', 'nobody'], default: 'everyone' },
    profilePhoto: { type: String, enum: ['everyone', 'contacts', 'nobody'], default: 'everyone' },
    status: { type: String, enum: ['everyone', 'contacts', 'nobody'], default: 'everyone' },
    readReceipts: { type: Boolean, default: true },
    groupInvites: { type: String, enum: ['everyone', 'contacts', 'nobody'], default: 'everyone' },
  },
  
  notificationSettings: {
    messageNotifications: { type: Boolean, default: true },
    groupNotifications: { type: Boolean, default: true },
    callNotifications: { type: Boolean, default: true },
    emailNotifications: { type: Boolean, default: true },
    pushToken: { type: String },
  },
  
  contacts: [{ type: Schema.Types.ObjectId, ref: 'User' }],
  blockedUsers: [{ type: Schema.Types.ObjectId, ref: 'User' }],
  
  devices: [{
    deviceId: { type: String, required: true },
    platform: { type: String, enum: ['ios', 'android', 'web'], required: true },
    lastActive: { type: Date, default: Date.now },
    pushToken: { type: String },
  }],
}, {
  timestamps: true,
  versionKey: false,
});

// Indexes
userSchema.index({ phoneNumber: 1 });
userSchema.index({ email: 1 });
userSchema.index({ username: 1 });
userSchema.index({ isOnline: 1 });
userSchema.index({ lastSeen: 1 });

export const User = mongoose.models.User || mongoose.model<IUser>('User', userSchema);

