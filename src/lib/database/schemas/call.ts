
import { z } from 'zod';

export const initiateCallSchema = z.object({
  participantId: z.string().regex(/^[0-9a-fA-F]{24}$/),
  type: z.enum(['voice', 'video']),
  chatId: z.string().regex(/^[0-9a-fA-F]{24}$/).optional(),
});

export const answerCallSchema = z.object({
  callId: z.string(),
  sdp: z.string(),
});

export const endCallSchema = z.object({
  callId: z.string(),
  reason: z.enum(['normal', 'busy', 'failed', 'rejected']).default('normal'),
});

export const iceCandidateSchema = z.object({
  callId: z.string(),
  candidate: z.string(),
});

export const callQualitySchema = z.object({
  callId: z.string(),
  rating: z.number().min(1).max(5),
  feedback: z.string().max(500).optional(),
});

export type InitiateCallInput = z.infer<typeof initiateCallSchema>;
export type AnswerCallInput = z.infer<typeof answerCallSchema>;
export type EndCallInput = z.infer<typeof endCallSchema>;
export type IceCandidateInput = z.infer<typeof iceCandidateSchema>;
export type CallQualityInput = z.infer<typeof callQualitySchema>;
