import mongoose, { Schema, Document, Types } from 'mongoose';

export interface ICall extends Document {
  _id: Types.ObjectId;
  callId: string;
  initiator: Types.ObjectId;
  participants: Types.ObjectId[];
  type: 'voice' | 'video';
  status: 'initiated' | 'ringing' | 'answered' | 'ended' | 'missed' | 'rejected' | 'busy';
  startTime: Date;
  endTime?: Date;
  duration?: number; // in seconds
  chatId?: Types.ObjectId;
  isGroupCall: boolean;
  
  // WebRTC Data
  signaling: {
    offers: {
      userId: Types.ObjectId;
      sdp: string;
      timestamp: Date;
    }[];
    answers: {
      userId: Types.ObjectId;
      sdp: string;
      timestamp: Date;
    }[];
    iceCandidates: {
      userId: Types.ObjectId;
      candidate: string;
      timestamp: Date;
    }[];
  };
  
  // Call Quality
  quality?: {
    userId: Types.ObjectId;
    rating: number; // 1-5
    feedback?: string;
  }[];
  
  createdAt: Date;
  updatedAt: Date;
}

const callSchema = new Schema<ICall>({
  callId: { type: String, required: true, unique: true },
  initiator: { type: Schema.Types.ObjectId, ref: 'User', required: true },
  participants: [{ type: Schema.Types.ObjectId, ref: 'User', required: true }],
  type: { type: String, enum: ['voice', 'video'], required: true },
  status: { 
    type: String, 
    enum: ['initiated', 'ringing', 'answered', 'ended', 'missed', 'rejected', 'busy'],
    default: 'initiated'
  },
  startTime: { type: Date, default: Date.now },
  endTime: { type: Date },
  duration: { type: Number },
  chatId: { type: Schema.Types.ObjectId, ref: 'Chat' },
  isGroupCall: { type: Boolean, default: false },
  
  signaling: {
    offers: [{
      userId: { type: Schema.Types.ObjectId, ref: 'User' },
      sdp: { type: String },
      timestamp: { type: Date, default: Date.now },
    }],
    answers: [{
      userId: { type: Schema.Types.ObjectId, ref: 'User' },
      sdp: { type: String },
      timestamp: { type: Date, default: Date.now },
    }],
    iceCandidates: [{
      userId: { type: Schema.Types.ObjectId, ref: 'User' },
      candidate: { type: String },
      timestamp: { type: Date, default: Date.now },
    }],
  },
  
  quality: [{
    userId: { type: Schema.Types.ObjectId, ref: 'User' },
    rating: { type: Number, min: 1, max: 5 },
    feedback: { type: String },
  }],
}, {
  timestamps: true,
  versionKey: false,
});

// Indexes
callSchema.index({ callId: 1 });
callSchema.index({ initiator: 1 });
callSchema.index({ participants: 1 });
callSchema.index({ startTime: -1 });
callSchema.index({ status: 1 });

export const Call = mongoose.models.Call || mongoose.model<ICall>('Call', callSchema);
