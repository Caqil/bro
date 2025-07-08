import nodemailer from 'nodemailer';
import { IUser } from '../database/models/user';
import { INotification } from '../database/models/notification';

interface SMTPConfig {
  host: string;
  port: number;
  secure: boolean;
  auth: {
    user: string;
    pass: string;
  };
}

interface EmailOptions {
  to: string | string[];
  subject: string;
  html?: string;
  text?: string;
  from?: string;
  replyTo?: string;
  attachments?: Array<{
    filename: string;
    content: Buffer;
    contentType: string;
  }>;
}

interface EmailResult {
  success: boolean;
  messageId?: string;
  error?: string;
}

export class SMTPService {
  private transporter: nodemailer.Transporter;
  private defaultFrom: string;

  constructor(config: SMTPConfig, defaultFrom: string) {
    this.transporter = nodemailer.createTransport({
      host: config.host,
      port: config.port,
      secure: config.secure,
      auth: config.auth,
    });
    this.defaultFrom = defaultFrom;
  }

  // Send single email
  async sendEmail(options: EmailOptions): Promise<EmailResult> {
    try {
      const mailOptions = {
        from: options.from || this.defaultFrom,
        to: Array.isArray(options.to) ? options.to.join(', ') : options.to,
        subject: options.subject,
        html: options.html,
        text: options.text,
        replyTo: options.replyTo,
        attachments: options.attachments,
      };

      const result = await this.transporter.sendMail(mailOptions);

      return {
        success: true,
        messageId: result.messageId,
      };
    } catch (error) {
      console.error('SMTP send error:', error);
      return {
        success: false,
        error: error instanceof Error ? error.message : 'Unknown error',
      };
    }
  }

  // Send bulk emails
  async sendBulkEmails(emails: EmailOptions[]): Promise<EmailResult[]> {
    const results = await Promise.allSettled(
      emails.map(email => this.sendEmail(email))
    );

    return results.map(result => 
      result.status === 'fulfilled' 
        ? result.value 
        : { success: false, error: 'Send failed' }
    );
  }

  // Send OTP email
  async sendOTPEmail(
    email: string,
    otp: string,
    userName: string,
    expiresInMinutes: number = 10
  ): Promise<EmailResult> {
    const { generateOTPEmailTemplate } = await import('./templates/email');
    
    const htmlContent = generateOTPEmailTemplate({
      userName,
      otp,
      expiresInMinutes,
    });

    return await this.sendEmail({
      to: email,
      subject: 'Your Verification Code',
      html: htmlContent,
      text: `Your verification code is: ${otp}. This code expires in ${expiresInMinutes} minutes.`,
    });
  }

  // Send welcome email
  async sendWelcomeEmail(user: Pick<IUser, 'email' | 'displayName'>): Promise<EmailResult> {
    if (!user.email) {
      return { success: false, error: 'No email address provided' };
    }

    const { generateWelcomeEmailTemplate } = await import('./templates/email');
    
    const htmlContent = generateWelcomeEmailTemplate({
      userName: user.displayName,
    });

    return await this.sendEmail({
      to: user.email,
      subject: 'Welcome to Our Chat App!',
      html: htmlContent,
    });
  }

  // Send notification email
  async sendNotificationEmail(
    user: Pick<IUser, 'email' | 'displayName'>,
    notification: Pick<INotification, 'title' | 'body'>
  ): Promise<EmailResult> {
    if (!user.email) {
      return { success: false, error: 'No email address provided' };
    }

    const { generateNotificationEmailTemplate } = await import('./templates/email');
    
    const htmlContent = generateNotificationEmailTemplate({
      userName: user.displayName,
      notificationTitle: notification.title,
      notificationBody: notification.body,
    });

    return await this.sendEmail({
      to: user.email,
      subject: notification.title,
      html: htmlContent,
    });
  }

  // Send password reset email (if implementing password auth)
  async sendPasswordResetEmail(
    email: string,
    resetToken: string,
    userName: string
  ): Promise<EmailResult> {
    const { generatePasswordResetEmailTemplate } = await import('./templates/email');
    
    const resetLink = `${process.env.FRONTEND_URL}/reset-password?token=${resetToken}`;
    
    const htmlContent = generatePasswordResetEmailTemplate({
      userName,
      resetLink,
    });

    return await this.sendEmail({
      to: email,
      subject: 'Password Reset Request',
      html: htmlContent,
    });
  }

  // Test connection
  async testConnection(): Promise<boolean> {
    try {
      await this.transporter.verify();
      return true;
    } catch (error) {
      console.error('SMTP connection test failed:', error);
      return false;
    }
  }
}

// Initialize SMTP service
const smtpConfig: SMTPConfig = {
  host: process.env.SMTP_HOST || 'smtp.gmail.com',
  port: parseInt(process.env.SMTP_PORT || '587'),
  secure: process.env.SMTP_SECURE === 'true',
  auth: {
    user: process.env.SMTP_USER || '',
    pass: process.env.SMTP_PASS || '',
  },
};

export const smtpService = new SMTPService(
  smtpConfig,
  process.env.SMTP_FROM || 'noreply@chatapp.com'
);