import { z } from 'zod';

export const updateProfileSchema = z.object({
  displayName: z.string().min(1).max(50).optional(),
  status: z.string().max(139).optional(),
  username: z.string().min(3).max(30).regex(/^[a-zA-Z0-9_]+$/).optional(),
});

export const privacySettingsSchema = z.object({
  lastSeen: z.enum(['everyone', 'contacts', 'nobody']).optional(),
  profilePhoto: z.enum(['everyone', 'contacts', 'nobody']).optional(),
  status: z.enum(['everyone', 'contacts', 'nobody']).optional(),
  readReceipts: z.boolean().optional(),
  groupInvites: z.enum(['everyone', 'contacts', 'nobody']).optional(),
});

export const notificationSettingsSchema = z.object({
  messageNotifications: z.boolean().optional(),
  groupNotifications: z.boolean().optional(),
  callNotifications: z.boolean().optional(),
  emailNotifications: z.boolean().optional(),
});

export const searchUsersSchema = z.object({
  query: z.string().min(1).max(50),
  limit: z.number().min(1).max(50).default(20),
  offset: z.number().min(0).default(0),
});

export const blockUserSchema = z.object({
  userId: z.string().regex(/^[0-9a-fA-F]{24}$/, 'Invalid user ID'),
});

export type UpdateProfileInput = z.infer<typeof updateProfileSchema>;
export type PrivacySettingsInput = z.infer<typeof privacySettingsSchema>;
export type NotificationSettingsInput = z.infer<typeof notificationSettingsSchema>;
export type SearchUsersInput = z.infer<typeof searchUsersSchema>;
export type BlockUserInput = z.infer<typeof blockUserSchema>;

