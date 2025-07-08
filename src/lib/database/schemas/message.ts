import { z } from 'zod';

export const sendMessageSchema = z.object({
  chatId: z.string().regex(/^[0-9a-fA-F]{24}$/, 'Invalid chat ID'),
  content: z.string().min(1).max(4096),
  type: z.enum(['text', 'image', 'video', 'audio', 'document', 'voice', 'location', 'contact']).default('text'),
  replyTo: z.string().regex(/^[0-9a-fA-F]{24}$/).optional(),
  mediaId: z.string().regex(/^[0-9a-fA-F]{24}$/).optional(),
  metadata: z.object({
    location: z.object({
      latitude: z.number().min(-90).max(90),
      longitude: z.number().min(-180).max(180),
      address: z.string().optional(),
    }).optional(),
    contact: z.object({
      name: z.string(),
      phoneNumber: z.string(),
      avatar: z.string().optional(),
    }).optional(),
    mentions: z.array(z.string().regex(/^[0-9a-fA-F]{24}$/)).optional(),
  }).optional(),
});

export const editMessageSchema = z.object({
  content: z.string().min(1).max(4096),
});

export const addReactionSchema = z.object({
  emoji: z.string().regex(/^[\u{1F600}-\u{1F64F}]|[\u{1F300}-\u{1F5FF}]|[\u{1F680}-\u{1F6FF}]|[\u{1F1E0}-\u{1F1FF}]|[\u{2600}-\u{26FF}]|[\u{2700}-\u{27BF}]/u),
});

export const markAsReadSchema = z.object({
  messageIds: z.array(z.string().regex(/^[0-9a-fA-F]{24}$/)),
});

export const searchMessagesSchema = z.object({
  query: z.string().min(1).max(100),
  chatId: z.string().regex(/^[0-9a-fA-F]{24}$/).optional(),
  limit: z.number().min(1).max(50).default(20),
  offset: z.number().min(0).default(0),
});

export type SendMessageInput = z.infer<typeof sendMessageSchema>;
export type EditMessageInput = z.infer<typeof editMessageSchema>;
export type AddReactionInput = z.infer<typeof addReactionSchema>;
export type MarkAsReadInput = z.infer<typeof markAsReadSchema>;
export type SearchMessagesInput = z.infer<typeof searchMessagesSchema>;
