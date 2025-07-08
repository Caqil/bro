import { z } from 'zod';

export const registerSchema = z.object({
  phoneNumber: z.string().regex(/^\+?[1-9]\d{1,14}$/, 'Invalid phone number format'),
  email: z.string().email('Invalid email format').optional(),
  displayName: z.string().min(1, 'Display name is required').max(50, 'Display name too long'),
});

export const verifyOTPSchema = z.object({
  phoneNumber: z.string().regex(/^\+?[1-9]\d{1,14}$/),
  otp: z.string().length(6, 'OTP must be 6 digits'),
});

export const loginSchema = z.object({
  phoneNumber: z.string().regex(/^\+?[1-9]\d{1,14}$/),
});

export const refreshTokenSchema = z.object({
  refreshToken: z.string().min(1, 'Refresh token is required'),
});

export const qrLoginSchema = z.object({
  qrCode: z.string().min(1, 'QR code is required'),
  deviceInfo: z.object({
    platform: z.enum(['web', 'desktop']),
    userAgent: z.string(),
    ip: z.string().ip().optional(),
  }),
});

export type RegisterInput = z.infer<typeof registerSchema>;
export type VerifyOTPInput = z.infer<typeof verifyOTPSchema>;
export type LoginInput = z.infer<typeof loginSchema>;
export type RefreshTokenInput = z.infer<typeof refreshTokenSchema>;
export type QRLoginInput = z.infer<typeof qrLoginSchema>;

