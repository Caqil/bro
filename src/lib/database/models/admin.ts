import mongoose, { Schema, Document, Types } from 'mongoose';

export interface IAdmin extends Document {
  _id: Types.ObjectId;
  email: string;
  username: string;
  displayName: string;
  avatar?: string;
  role: 'super_admin' | 'admin' | 'moderator' | 'support';
  permissions: string[];
  isActive: boolean;
  lastLogin?: Date;
  loginHistory: {
    ip: string;
    userAgent: string;
    timestamp: Date;
  }[];
  
  // Two-factor authentication
  twoFactorEnabled: boolean;
  twoFactorSecret?: string;
  
  createdAt: Date;
  updatedAt: Date;
}

const adminSchema = new Schema<IAdmin>({
  email: { type: String, required: true, unique: true },
  username: { type: String, required: true, unique: true },
  displayName: { type: String, required: true },
  avatar: { type: String },
  role: { 
    type: String, 
    enum: ['super_admin', 'admin', 'moderator', 'support'],
    required: true 
  },
  permissions: [{ type: String }],
  isActive: { type: Boolean, default: true },
  lastLogin: { type: Date },
  loginHistory: [{
    ip: { type: String },
    userAgent: { type: String },
    timestamp: { type: Date, default: Date.now },
  }],
  
  twoFactorEnabled: { type: Boolean, default: false },
  twoFactorSecret: { type: String },
}, {
  timestamps: true,
  versionKey: false,
});

// Indexes
adminSchema.index({ email: 1 });
adminSchema.index({ username: 1 });
adminSchema.index({ role: 1 });
adminSchema.index({ isActive: 1 });

export const Admin = mongoose.models.Admin || mongoose.model<IAdmin>('Admin', adminSchema);
