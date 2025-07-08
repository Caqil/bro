import mongoose, { Schema, Document, Types } from 'mongoose';

export interface IChat extends Document {
  _id: Types.ObjectId;
  participants: Types.ObjectId[];
  type: 'direct' | 'group';
  lastMessage?: Types.ObjectId;
  lastActivity: Date;
  isArchived: boolean;
  isPinned: boolean;
  mutedUntil?: Date;
  createdAt: Date;
  updatedAt: Date;
  
  // Group-specific fields (when type is 'group')
  groupInfo?: {
    name: string;
    description?: string;
    avatar?: string;
    admins: Types.ObjectId[];
    settings: {
      whoCanSendMessages: 'everyone' | 'admins';
      whoCanEditGroupInfo: 'everyone' | 'admins';
      whoCanAddMembers: 'everyone' | 'admins';
    };
  };
  
  // Direct chat specific
  participantSettings: {
    userId: Types.ObjectId;
    nickname?: string;
    customNotifications: boolean;
    wallpaper?: string;
  }[];
}

const chatSchema = new Schema<IChat>({
  participants: [{ type: Schema.Types.ObjectId, ref: 'User', required: true }],
  type: { type: String, enum: ['direct', 'group'], required: true },
  lastMessage: { type: Schema.Types.ObjectId, ref: 'Message' },
  lastActivity: { type: Date, default: Date.now },
  isArchived: { type: Boolean, default: false },
  isPinned: { type: Boolean, default: false },
  mutedUntil: { type: Date },
  
  groupInfo: {
    name: { type: String },
    description: { type: String },
    avatar: { type: String },
    admins: [{ type: Schema.Types.ObjectId, ref: 'User' }],
    settings: {
      whoCanSendMessages: { type: String, enum: ['everyone', 'admins'], default: 'everyone' },
      whoCanEditGroupInfo: { type: String, enum: ['everyone', 'admins'], default: 'admins' },
      whoCanAddMembers: { type: String, enum: ['everyone', 'admins'], default: 'admins' },
    },
  },
  
  participantSettings: [{
    userId: { type: Schema.Types.ObjectId, ref: 'User', required: true },
    nickname: { type: String },
    customNotifications: { type: Boolean, default: false },
    wallpaper: { type: String },
  }],
}, {
  timestamps: true,
  versionKey: false,
});

// Indexes
chatSchema.index({ participants: 1 });
chatSchema.index({ lastActivity: -1 });
chatSchema.index({ type: 1 });
chatSchema.index({ 'groupInfo.name': 'text' });

export const Chat = mongoose.models.Chat || mongoose.model<IChat>('Chat', chatSchema);
