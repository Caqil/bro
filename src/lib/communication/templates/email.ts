interface OTPEmailTemplateData {
  userName: string;
  otp: string;
  expiresInMinutes: number;
}

interface WelcomeEmailTemplateData {
  userName: string;
}

interface NotificationEmailTemplateData {
  userName: string;
  notificationTitle: string;
  notificationBody: string;
}

interface PasswordResetEmailTemplateData {
  userName: string;
  resetLink: string;
}

// Generate OTP email template
export function generateOTPEmailTemplate(data: OTPEmailTemplateData): string {
  return `
    <!DOCTYPE html>
    <html lang="en">
    <head>
        <meta charset="UTF-8">
        <meta name="viewport" content="width=device-width, initial-scale=1.0">
        <title>Verification Code</title>
        <style>
            body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; margin: 0; padding: 0; background-color: #f5f5f5; }
            .container { max-width: 600px; margin: 0 auto; background-color: white; }
            .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); padding: 40px 20px; text-align: center; }
            .header h1 { color: white; margin: 0; font-size: 28px; }
            .content { padding: 40px 20px; text-align: center; }
            .otp-code { font-size: 36px; font-weight: bold; color: #667eea; letter-spacing: 8px; margin: 30px 0; padding: 20px; background-color: #f8f9ff; border-radius: 10px; border: 2px dashed #667eea; }
            .warning { background-color: #fff3cd; border: 1px solid #ffeaa7; padding: 15px; border-radius: 8px; margin: 20px 0; color: #856404; }
            .footer { background-color: #f8f9fa; padding: 20px; text-align: center; color: #6c757d; font-size: 14px; }
        </style>
    </head>
    <body>
        <div class="container">
            <div class="header">
                <h1>🔐 Verification Code</h1>
            </div>
            <div class="content">
                <h2>Hello ${data.userName}!</h2>
                <p>Your verification code is:</p>
                <div class="otp-code">${data.otp}</div>
                <p>Enter this code to complete your verification.</p>
                <div class="warning">
                    <strong>⚠️ Security Notice:</strong><br>
                    This code expires in ${data.expiresInMinutes} minutes.<br>
                    Never share this code with anyone.
                </div>
            </div>
            <div class="footer">
                <p>If you didn't request this code, please ignore this email.</p>
                <p>&copy; ${new Date().getFullYear()} Chat App. All rights reserved.</p>
            </div>
        </div>
    </body>
    </html>
  `;
}

// Generate welcome email template
export function generateWelcomeEmailTemplate(data: WelcomeEmailTemplateData): string {
  return `
    <!DOCTYPE html>
    <html lang="en">
    <head>
        <meta charset="UTF-8">
        <meta name="viewport" content="width=device-width, initial-scale=1.0">
        <title>Welcome to Chat App</title>
        <style>
            body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; margin: 0; padding: 0; background-color: #f5f5f5; }
            .container { max-width: 600px; margin: 0 auto; background-color: white; }
            .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); padding: 40px 20px; text-align: center; }
            .header h1 { color: white; margin: 0; font-size: 28px; }
            .content { padding: 40px 20px; }
            .feature { margin: 20px 0; padding: 15px; background-color: #f8f9ff; border-radius: 8px; border-left: 4px solid #667eea; }
            .cta-button { display: inline-block; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 15px 30px; text-decoration: none; border-radius: 25px; margin: 20px 0; }
            .footer { background-color: #f8f9fa; padding: 20px; text-align: center; color: #6c757d; font-size: 14px; }
        </style>
    </head>
    <body>
        <div class="container">
            <div class="header">
                <h1>🎉 Welcome to Chat App!</h1>
            </div>
            <div class="content">
                <h2>Hi ${data.userName}!</h2>
                <p>We're excited to have you join our chat community! Get ready to connect with friends and family like never before.</p>
                
                <div class="feature">
                    <h3>💬 Instant Messaging</h3>
                    <p>Send messages, photos, videos, and voice notes instantly.</p>
                </div>
                
                <div class="feature">
                    <h3>📞 Voice & Video Calls</h3>
                    <p>Make crystal-clear voice and video calls to anyone, anywhere.</p>
                </div>
                
                <div class="feature">
                    <h3>👥 Group Chats</h3>
                    <p>Create groups and chat with multiple people at once.</p>
                </div>
                
                <div class="feature">
                    <h3>🔒 Privacy & Security</h3>
                    <p>Your conversations are protected with end-to-end encryption.</p>
                </div>
                
                <center>
                    <a href="${process.env.FRONTEND_URL}" class="cta-button">Start Chatting Now</a>
                </center>
            </div>
            <div class="footer">
                <p>Need help? Contact our support team anytime.</p>
                <p>&copy; ${new Date().getFullYear()} Chat App. All rights reserved.</p>
            </div>
        </div>
    </body>
    </html>
  `;
}

// Generate notification email template
export function generateNotificationEmailTemplate(data: NotificationEmailTemplateData): string {
  return `
    <!DOCTYPE html>
    <html lang="en">
    <head>
        <meta charset="UTF-8">
        <meta name="viewport" content="width=device-width, initial-scale=1.0">
        <title>${data.notificationTitle}</title>
        <style>
            body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; margin: 0; padding: 0; background-color: #f5f5f5; }
            .container { max-width: 600px; margin: 0 auto; background-color: white; }
            .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); padding: 30px 20px; text-align: center; }
            .header h1 { color: white; margin: 0; font-size: 24px; }
            .content { padding: 30px 20px; }
            .notification-content { background-color: #f8f9ff; padding: 20px; border-radius: 8px; border-left: 4px solid #667eea; margin: 20px 0; }
            .footer { background-color: #f8f9fa; padding: 20px; text-align: center; color: #6c757d; font-size: 14px; }
        </style>
    </head>
    <body>
        <div class="container">
            <div class="header">
                <h1>🔔 ${data.notificationTitle}</h1>
            </div>
            <div class="content">
                <h2>Hi ${data.userName}!</h2>
                <div class="notification-content">
                    <p>${data.notificationBody}</p>
                </div>
                <p>Open the app to see more details and respond.</p>
            </div>
            <div class="footer">
                <p>You can manage your notification preferences in the app settings.</p>
                <p>&copy; ${new Date().getFullYear()} Chat App. All rights reserved.</p>
            </div>
        </div>
    </body>
    </html>
  `;
}

// Generate password reset email template
export function generatePasswordResetEmailTemplate(data: PasswordResetEmailTemplateData): string {
  return `
    <!DOCTYPE html>
    <html lang="en">
    <head>
        <meta charset="UTF-8">
        <meta name="viewport" content="width=device-width, initial-scale=1.0">
        <title>Password Reset</title>
        <style>
            body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; margin: 0; padding: 0; background-color: #f5f5f5; }
            .container { max-width: 600px; margin: 0 auto; background-color: white; }
            .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); padding: 40px 20px; text-align: center; }
            .header h1 { color: white; margin: 0; font-size: 28px; }
            .content { padding: 40px 20px; text-align: center; }
            .reset-button { display: inline-block; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 15px 30px; text-decoration: none; border-radius: 25px; margin: 20px 0; }
            .warning { background-color: #fff3cd; border: 1px solid #ffeaa7; padding: 15px; border-radius: 8px; margin: 20px 0; color: #856404; }
            .footer { background-color: #f8f9fa; padding: 20px; text-align: center; color: #6c757d; font-size: 14px; }
        </style>
    </head>
    <body>
        <div class="container">
            <div class="header">
                <h1>🔑 Password Reset</h1>
            </div>
            <div class="content">
                <h2>Hi ${data.userName}!</h2>
                <p>We received a request to reset your password. Click the button below to create a new password:</p>
                <a href="${data.resetLink}" class="reset-button">Reset Password</a>
                <div class="warning">
                    <strong>⚠️ Security Notice:</strong><br>
                    This link expires in 1 hour for security reasons.<br>
                    If you didn't request this reset, please ignore this email.
                </div>
            </div>
            <div class="footer">
                <p>If the button doesn't work, copy and paste this link:</p>
                <p style="word-break: break-all;">${data.resetLink}</p>
                <p>&copy; ${new Date().getFullYear()} Chat App. All rights reserved.</p>
            </div>
        </div>
    </body>
    </html>
  `;
}