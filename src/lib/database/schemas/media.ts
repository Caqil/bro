import { z } from 'zod';

export const uploadMediaSchema = z.object({
  file: z.any(), // File object
  chatId: z.string().regex(/^[0-9a-fA-F]{24}$/).optional(),
  type: z.enum(['image', 'video', 'audio', 'document', 'voice']),
});

export const mediaQuerySchema = z.object({
  chatId: z.string().regex(/^[0-9a-fA-F]{24}$/).optional(),
  type: z.enum(['image', 'video', 'audio', 'document', 'voice']).optional(),
  limit: z.number().min(1).max(50).default(20),
  offset: z.number().min(0).default(0),
});

export type UploadMediaInput = z.infer<typeof uploadMediaSchema>;
export type MediaQueryInput = z.infer<typeof mediaQuerySchema>;
