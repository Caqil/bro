import { z } from 'zod';

export const createGroupSchema = z.object({
  name: z.string().min(1).max(50),
  description: z.string().max(200).optional(),
  participants: z.array(z.string().regex(/^[0-9a-fA-F]{24}$/)).min(1).max(256),
});

export const updateGroupSchema = z.object({
  name: z.string().min(1).max(50).optional(),
  description: z.string().max(200).optional(),
});

export const groupSettingsSchema = z.object({
  whoCanSendMessages: z.enum(['everyone', 'admins']).optional(),
  whoCanEditGroupInfo: z.enum(['everyone', 'admins']).optional(),
  whoCanAddMembers: z.enum(['everyone', 'admins']).optional(),
});

export const addMembersSchema = z.object({
  userIds: z.array(z.string().regex(/^[0-9a-fA-F]{24}$/)).min(1).max(10),
});

export const promoteUserSchema = z.object({
  userId: z.string().regex(/^[0-9a-fA-F]{24}$/),
});

export const generateInviteSchema = z.object({
  expiresIn: z.number().min(3600).max(604800).default(86400), // 1 hour to 1 week, default 1 day
});

export type CreateGroupInput = z.infer<typeof createGroupSchema>;
export type UpdateGroupInput = z.infer<typeof updateGroupSchema>;
export type GroupSettingsInput = z.infer<typeof groupSettingsSchema>;
export type AddMembersInput = z.infer<typeof addMembersSchema>;
export type PromoteUserInput = z.infer<typeof promoteUserSchema>;
export type GenerateInviteInput = z.infer<typeof generateInviteSchema>;

