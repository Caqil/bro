import mongoose, { Schema, Document, Types } from 'mongoose';

export interface INotification extends Document {
  _id: Types.ObjectId;
  userId: Types.ObjectId;
  type: 'message' | 'call' | 'group_invite' | 'contact_request' | 'system' | 'status_view';
  title: string;
  body: string;
  data?: any;
  isRead: boolean;
  readAt?: Date;
  
  // Related entities
  relatedChat?: Types.ObjectId;
  relatedMessage?: Types.ObjectId;
  relatedCall?: Types.ObjectId;
  relatedUser?: Types.ObjectId;
  
  // Delivery status
  deliveryStatus: 'pending' | 'sent' | 'failed';
  sentAt?: Date;
  failureReason?: string;
  
  createdAt: Date;
  updatedAt: Date;
}

const notificationSchema = new Schema<INotification>({
  userId: { type: Schema.Types.ObjectId, ref: 'User', required: true },
  type: { 
    type: String, 
    enum: ['message', 'call', 'group_invite', 'contact_request', 'system', 'status_view'],
    required: true 
  },
  title: { type: String, required: true },
  body: { type: String, required: true },
  data: { type: Schema.Types.Mixed },
  isRead: { type: Boolean, default: false },
  readAt: { type: Date },
  
  relatedChat: { type: Schema.Types.ObjectId, ref: 'Chat' },
  relatedMessage: { type: Schema.Types.ObjectId, ref: 'Message' },
  relatedCall: { type: Schema.Types.ObjectId, ref: 'Call' },
  relatedUser: { type: Schema.Types.ObjectId, ref: 'User' },
  
  deliveryStatus: { type: String, enum: ['pending', 'sent', 'failed'], default: 'pending' },
  sentAt: { type: Date },
  failureReason: { type: String },
}, {
  timestamps: true,
  versionKey: false,
});

// Indexes
notificationSchema.index({ userId: 1, createdAt: -1 });
notificationSchema.index({ type: 1 });
notificationSchema.index({ isRead: 1 });
notificationSchema.index({ deliveryStatus: 1 });

export const Notification = mongoose.models.Notification || mongoose.model<INotification>('Notification', notificationSchema);
