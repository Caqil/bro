import mongoose, { Schema, Document, Types } from 'mongoose';

export interface IStatus extends Document {
  _id: Types.ObjectId;
  userId: Types.ObjectId;
  content: string;
  media?: Types.ObjectId;
  type: 'text' | 'image' | 'video';
  backgroundColor?: string;
  textColor?: string;
  font?: string;
  expiresAt: Date;
  isActive: boolean;
  
  // Privacy
  privacySettings: {
    viewableBy: 'everyone' | 'contacts' | 'specific';
    allowedUsers?: Types.ObjectId[];
    blockedUsers?: Types.ObjectId[];
  };
  
  // Viewers
  viewers: {
    userId: Types.ObjectId;
    viewedAt: Date;
  }[];
  
  createdAt: Date;
  updatedAt: Date;
}

const statusSchema = new Schema<IStatus>({
  userId: { type: Schema.Types.ObjectId, ref: 'User', required: true },
  content: { type: String, required: true },
  media: { type: Schema.Types.ObjectId, ref: 'Media' },
  type: { type: String, enum: ['text', 'image', 'video'], default: 'text' },
  backgroundColor: { type: String },
  textColor: { type: String },
  font: { type: String },
  expiresAt: { 
    type: Date, 
    default: () => new Date(Date.now() + 24 * 60 * 60 * 1000) // 24 hours
  },
  isActive: { type: Boolean, default: true },
  
  privacySettings: {
    viewableBy: { type: String, enum: ['everyone', 'contacts', 'specific'], default: 'contacts' },
    allowedUsers: [{ type: Schema.Types.ObjectId, ref: 'User' }],
    blockedUsers: [{ type: Schema.Types.ObjectId, ref: 'User' }],
  },
  
  viewers: [{
    userId: { type: Schema.Types.ObjectId, ref: 'User' },
    viewedAt: { type: Date, default: Date.now },
  }],
}, {
  timestamps: true,
  versionKey: false,
});

// Indexes
statusSchema.index({ userId: 1 });
statusSchema.index({ expiresAt: 1 });
statusSchema.index({ isActive: 1 });
statusSchema.index({ createdAt: -1 });

export const Status = mongoose.models.Status || mongoose.model<IStatus>('Status', statusSchema);
