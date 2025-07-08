import mongoose, { Schema, Document, Types } from 'mongoose';

export interface IMedia extends Document {
  _id: Types.ObjectId;
  filename: string;
  originalName: string;
  mimeType: string;
  size: number;
  url: string;
  thumbnailUrl?: string;
  uploadedBy: Types.ObjectId;
  chatId?: Types.ObjectId;
  messageId?: Types.ObjectId;
  type: 'image' | 'video' | 'audio' | 'document' | 'voice';
  metadata?: {
    duration?: number; // for audio/video
    dimensions?: {
      width: number;
      height: number;
    };
    format?: string;
    bitrate?: number;
  };
  isEncrypted: boolean;
  encryptionKey?: string;
  checksumSHA256: string;
  createdAt: Date;
  updatedAt: Date;
}

const mediaSchema = new Schema<IMedia>({
  filename: { type: String, required: true },
  originalName: { type: String, required: true },
  mimeType: { type: String, required: true },
  size: { type: Number, required: true },
  url: { type: String, required: true },
  thumbnailUrl: { type: String },
  uploadedBy: { type: Schema.Types.ObjectId, ref: 'User', required: true },
  chatId: { type: Schema.Types.ObjectId, ref: 'Chat' },
  messageId: { type: Schema.Types.ObjectId, ref: 'Message' },
  type: { 
    type: String, 
    enum: ['image', 'video', 'audio', 'document', 'voice'],
    required: true 
  },
  metadata: {
    duration: { type: Number },
    dimensions: {
      width: { type: Number },
      height: { type: Number },
    },
    format: { type: String },
    bitrate: { type: Number },
  },
  isEncrypted: { type: Boolean, default: false },
  encryptionKey: { type: String },
  checksumSHA256: { type: String, required: true },
}, {
  timestamps: true,
  versionKey: false,
});

// Indexes
mediaSchema.index({ uploadedBy: 1 });
mediaSchema.index({ chatId: 1 });
mediaSchema.index({ messageId: 1 });
mediaSchema.index({ type: 1 });
mediaSchema.index({ createdAt: -1 });

export const Media = mongoose.models.Media || mongoose.model<IMedia>('Media', mediaSchema);
