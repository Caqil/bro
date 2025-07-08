interface OTPSMSTemplateData {
  otp: string;
  expiresInMinutes: number;
}

interface WelcomeSMSTemplateData {
  userName: string;
}

interface NotificationSMSTemplateData {
  title: string;
  body: string;
}

// Generate OTP SMS template
export function generateOTPSMSTemplate(data: OTPSMSTemplateData): string {
  return `Your Chat App verification code is: ${data.otp}

This code expires in ${data.expiresInMinutes} minutes. Do not share this code with anyone.

If you didn't request this code, please ignore this message.`;
}

// Generate welcome SMS template
export function generateWelcomeSMSTemplate(data: WelcomeSMSTemplateData): string {
  return `Welcome to Chat App, ${data.userName}! 🎉

Start chatting with friends and family instantly. Download our app and enjoy:
- Instant messaging
- Voice & video calls  
- Group chats
- Secure & private

Happy chatting!`;
}

// Generate notification SMS template
export function generateNotificationSMSTemplate(data: NotificationSMSTemplateData): string {
  return `${data.title}

${data.body}

Open Chat App to see more details.`;
}

// Generate call notification SMS template
export function generateCallNotificationSMSTemplate(data: {
  callerName: string;
  callType: 'voice' | 'video';
}): string {
  return `📞 Missed ${data.callType} call from ${data.callerName}

Open Chat App to call back or see more details.`;
}

// Generate group invitation SMS template
export function generateGroupInvitationSMSTemplate(data: {
  inviterName: string;
  groupName: string;
  inviteLink: string;
}): string {
  return `${data.inviterName} invited you to join "${data.groupName}" on Chat App!

Join here: ${data.inviteLink}

Download Chat App if you don't have it yet.`;
}