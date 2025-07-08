import twilio from 'twilio';
import { IUser } from '../database/models/user';

interface SMSConfig {
  accountSid: string;
  authToken: string;
  fromNumber: string;
}

interface SMSOptions {
  to: string;
  body: string;
  from?: string;
}

interface SMSResult {
  success: boolean;
  messageSid?: string;
  error?: string;
}

export class SMSService {
  private client: twilio.Twilio;
  private fromNumber: string;

  constructor(config: SMSConfig) {
    this.client = twilio(config.accountSid, config.authToken);
    this.fromNumber = config.fromNumber;
  }

  // Send single SMS
  async sendSMS(options: SMSOptions): Promise<SMSResult> {
    try {
      const message = await this.client.messages.create({
        body: options.body,
        from: options.from || this.fromNumber,
        to: options.to,
      });

      return {
        success: true,
        messageSid: message.sid,
      };
    } catch (error) {
      console.error('SMS send error:', error);
      return {
        success: false,
        error: error instanceof Error ? error.message : 'Unknown error',
      };
    }
  }

  // Send bulk SMS
  async sendBulkSMS(messages: SMSOptions[]): Promise<SMSResult[]> {
    const results = await Promise.allSettled(
      messages.map(message => this.sendSMS(message))
    );

    return results.map(result => 
      result.status === 'fulfilled' 
        ? result.value 
        : { success: false, error: 'Send failed' }
    );
  }

  // Send OTP SMS
  async sendOTPSMS(
    phoneNumber: string,
    otp: string,
    expiresInMinutes: number = 10
  ): Promise<SMSResult> {
    const { generateOTPSMSTemplate } = await import('./templates/sms');
    
    const message = generateOTPSMSTemplate({
      otp,
      expiresInMinutes,
    });

    return await this.sendSMS({
      to: phoneNumber,
      body: message,
    });
  }

  // Send welcome SMS
  async sendWelcomeSMS(user: Pick<IUser, 'phoneNumber' | 'displayName'>): Promise<SMSResult> {
    const { generateWelcomeSMSTemplate } = await import('./templates/sms');
    
    const message = generateWelcomeSMSTemplate({
      userName: user.displayName,
    });

    return await this.sendSMS({
      to: user.phoneNumber,
      body: message,
    });
  }

  // Send notification SMS
  async sendNotificationSMS(
    phoneNumber: string,
    title: string,
    body: string
  ): Promise<SMSResult> {
    const { generateNotificationSMSTemplate } = await import('./templates/sms');
    
    const message = generateNotificationSMSTemplate({
      title,
      body,
    });

    return await this.sendSMS({
      to: phoneNumber,
      body: message,
    });
  }

  // Validate phone number format
  validatePhoneNumber(phoneNumber: string): boolean {
    // Basic international phone number validation
    const phoneRegex = /^\+?[1-9]\d{1,14}$/;
    return phoneRegex.test(phoneNumber);
  }

  // Format phone number for Twilio
  formatPhoneNumber(phoneNumber: string): string {
    // Remove any spaces, dashes, or parentheses
    let formatted = phoneNumber.replace(/[\s\-\(\)]/g, '');
    
    // Add + if not present
    if (!formatted.startsWith('+')) {
      formatted = '+' + formatted;
    }
    
    return formatted;
  }
}

// Initialize SMS service
const smsConfig: SMSConfig = {
  accountSid: process.env.TWILIO_ACCOUNT_SID || '',
  authToken: process.env.TWILIO_AUTH_TOKEN || '',
  fromNumber: process.env.TWILIO_FROM_NUMBER || '',
};

export const smsService = new SMSService(smsConfig);
