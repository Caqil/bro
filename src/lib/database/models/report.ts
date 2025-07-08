import mongoose, { Schema, Document, Types } from 'mongoose';

export interface IReport extends Document {
  _id: Types.ObjectId;
  reporterId: Types.ObjectId;
  reportedUserId?: Types.ObjectId;
  reportedMessageId?: Types.ObjectId;
  reportedChatId?: Types.ObjectId;
  type: 'spam' | 'harassment' | 'inappropriate_content' | 'fake_account' | 'other';
  reason: string;
  description?: string;
  status: 'pending' | 'under_review' | 'resolved' | 'dismissed';
  priority: 'low' | 'medium' | 'high' | 'critical';
  
  // Admin handling
  assignedTo?: Types.ObjectId;
  adminNotes?: string;
  resolution?: string;
  resolvedAt?: Date;
  
  // Evidence
  evidence: {
    type: 'screenshot' | 'message' | 'other';
    url?: string;
    content?: string;
  }[];
  
  createdAt: Date;
  updatedAt: Date;
}

const reportSchema = new Schema<IReport>({
  reporterId: { type: Schema.Types.ObjectId, ref: 'User', required: true },
  reportedUserId: { type: Schema.Types.ObjectId, ref: 'User' },
  reportedMessageId: { type: Schema.Types.ObjectId, ref: 'Message' },
  reportedChatId: { type: Schema.Types.ObjectId, ref: 'Chat' },
  type: { 
    type: String, 
    enum: ['spam', 'harassment', 'inappropriate_content', 'fake_account', 'other'],
    required: true 
  },
  reason: { type: String, required: true },
  description: { type: String },
  status: { type: String, enum: ['pending', 'under_review', 'resolved', 'dismissed'], default: 'pending' },
  priority: { type: String, enum: ['low', 'medium', 'high', 'critical'], default: 'medium' },
  
  assignedTo: { type: Schema.Types.ObjectId, ref: 'Admin' },
  adminNotes: { type: String },
  resolution: { type: String },
  resolvedAt: { type: Date },
  
  evidence: [{
    type: { type: String, enum: ['screenshot', 'message', 'other'] },
    url: { type: String },
    content: { type: String },
  }],
}, {
  timestamps: true,
  versionKey: false,
});

// Indexes
reportSchema.index({ reporterId: 1 });
reportSchema.index({ reportedUserId: 1 });
reportSchema.index({ status: 1 });
reportSchema.index({ priority: 1 });
reportSchema.index({ createdAt: -1 });

export const Report = mongoose.models.Report || mongoose.model<IReport>('Report', reportSchema);

