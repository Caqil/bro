import mongoose, { Schema, Document, Types } from 'mongoose';

export interface IMessage extends Document {
  _id: Types.ObjectId;
  chatId: Types.ObjectId;
  senderId: Types.ObjectId;
  content: string;
  type: 'text' | 'image' | 'video' | 'audio' | 'document' | 'voice' | 'location' | 'contact' | 'sticker';
  media?: Types.ObjectId;
  replyTo?: Types.ObjectId;
  forwardedFrom?: Types.ObjectId;
  isEdited: boolean;
  editedAt?: Date;
  isDeleted: boolean;
  deletedAt?: Date;
  deletedFor: Types.ObjectId[]; // Users who deleted this message for themselves
  createdAt: Date;
  updatedAt: Date;
  
  // Message Status
  status: 'sent' | 'delivered' | 'read';
  deliveredTo: {
    userId: Types.ObjectId;
    deliveredAt: Date;
  }[];
  readBy: {
    userId: Types.ObjectId;
    readAt: Date;
  }[];
  
  // Reactions
  reactions: {
    userId: Types.ObjectId;
    emoji: string;
    createdAt: Date;
  }[];
  
  // Additional metadata
  metadata?: {
    location?: {
      latitude: number;
      longitude: number;
      address?: string;
    };
    contact?: {
      name: string;
      phoneNumber: string;
      avatar?: string;
    };
    mentions?: Types.ObjectId[];
    links?: string[];
  };
}

const messageSchema = new Schema<IMessage>({
  chatId: { type: Schema.Types.ObjectId, ref: 'Chat', required: true, index: true },
  senderId: { type: Schema.Types.ObjectId, ref: 'User', required: true },
  content: { type: String, required: true },
  type: { 
    type: String, 
    enum: ['text', 'image', 'video', 'audio', 'document', 'voice', 'location', 'contact', 'sticker'],
    default: 'text'
  },
  media: { type: Schema.Types.ObjectId, ref: 'Media' },
  replyTo: { type: Schema.Types.ObjectId, ref: 'Message' },
  forwardedFrom: { type: Schema.Types.ObjectId, ref: 'Message' },
  isEdited: { type: Boolean, default: false },
  editedAt: { type: Date },
  isDeleted: { type: Boolean, default: false },
  deletedAt: { type: Date },
  deletedFor: [{ type: Schema.Types.ObjectId, ref: 'User' }],
  
  status: { type: String, enum: ['sent', 'delivered', 'read'], default: 'sent' },
  deliveredTo: [{
    userId: { type: Schema.Types.ObjectId, ref: 'User' },
    deliveredAt: { type: Date, default: Date.now },
  }],
  readBy: [{
    userId: { type: Schema.Types.ObjectId, ref: 'User' },
    readAt: { type: Date, default: Date.now },
  }],
  
  reactions: [{
    userId: { type: Schema.Types.ObjectId, ref: 'User' },
    emoji: { type: String },
    createdAt: { type: Date, default: Date.now },
  }],
  
  metadata: {
    location: {
      latitude: { type: Number },
      longitude: { type: Number },
      address: { type: String },
    },
    contact: {
      name: { type: String },
      phoneNumber: { type: String },
      avatar: { type: String },
    },
    mentions: [{ type: Schema.Types.ObjectId, ref: 'User' }],
    links: [{ type: String }],
  },
}, {
  timestamps: true,
  versionKey: false,
});

// Indexes
messageSchema.index({ chatId: 1, createdAt: -1 });
messageSchema.index({ senderId: 1 });
messageSchema.index({ content: 'text' });
messageSchema.index({ type: 1 });
messageSchema.index({ isDeleted: 1 });

export const Message = mongoose.models.Message || mongoose.model<IMessage>('Message', messageSchema);

