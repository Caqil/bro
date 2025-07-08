import admin from 'firebase-admin';
import { IUser } from '../database/models/user';
import { INotification } from '../database/models/notification';

interface PushNotificationConfig {
  projectId: string;
  privateKey: string;
  clientEmail: string;
}

interface PushMessage {
  token: string;
  title: string;
  body: string;
  data?: { [key: string]: string };
  imageUrl?: string;
  sound?: string;
  badge?: number;
  clickAction?: string;
}

interface PushResult {
  success: boolean;
  messageId?: string;
  error?: string;
  token: string;
}

export class PushNotificationService {
  private messaging: admin.messaging.Messaging;

  constructor(config: PushNotificationConfig) {
    if (!admin.apps.length) {
      admin.initializeApp({
        credential: admin.credential.cert({
          projectId: config.projectId,
          privateKey: config.privateKey.replace(/\\n/g, '\n'),
          clientEmail: config.clientEmail,
        }),
      });
    }
    
    this.messaging = admin.messaging();
  }

  // Send push notification to single device
  async sendPushNotification(message: PushMessage): Promise<PushResult> {
    try {
      const payload: admin.messaging.Message = {
        token: message.token,
        notification: {
          title: message.title,
          body: message.body,
          imageUrl: message.imageUrl,
        },
        data: message.data || {},
        android: {
          notification: {
            sound: message.sound || 'default',
            clickAction: message.clickAction,
            channelId: 'chat_messages',
          },
        },
        apns: {
          payload: {
            aps: {
              sound: message.sound || 'default',
              badge: message.badge,
              category: message.clickAction,
            },
          },
        },
        webpush: {
          notification: {
            icon: '/icon-192x192.png',
            badge: '/badge-72x72.png',
            click_action: message.clickAction,
          },
        },
      };

      const messageId = await this.messaging.send(payload);

      return {
        success: true,
        messageId,
        token: message.token,
      };
    } catch (error) {
      console.error('Push notification error:', error);
      return {
        success: false,
        error: error instanceof Error ? error.message : 'Unknown error',
        token: message.token,
      };
    }
  }

  // Send push notifications to multiple devices
  async sendMulticastPushNotification(
    tokens: string[],
    title: string,
    body: string,
    data?: { [key: string]: string },
    options?: {
      imageUrl?: string;
      sound?: string;
      badge?: number;
      clickAction?: string;
    }
  ): Promise<PushResult[]> {
    try {
      const message: admin.messaging.MulticastMessage = {
        tokens,
        notification: {
          title,
          body,
          imageUrl: options?.imageUrl,
        },
        data: data || {},
        android: {
          notification: {
            sound: options?.sound || 'default',
            clickAction: options?.clickAction,
            channelId: 'chat_messages',
          },
        },
        apns: {
          payload: {
            aps: {
              sound: options?.sound || 'default',
              badge: options?.badge,
              category: options?.clickAction,
            },
          },
        },
        webpush: {
          notification: {
            icon: '/icon-192x192.png',
            badge: '/badge-72x72.png',
            click_action: options?.clickAction,
          },
        },
      };

      const response = await this.messaging.sendEachForMulticast(message);

      return response.responses.map((result, index) => ({
        success: result.success,
        messageId: result.messageId,
        error: result.error?.message,
        token: tokens[index],
      }));
    } catch (error) {
      console.error('Multicast push notification error:', error);
      return tokens.map(token => ({
        success: false,
        error: error instanceof Error ? error.message : 'Unknown error',
        token,
      }));
    }
  }

  // Send message notification
  async sendMessageNotification(
    user: IUser,
    senderName: string,
    messageContent: string,
    chatId: string,
    messageId: string
  ): Promise<PushResult[]> {
    const activeTokens = user.devices
      .filter(device => device.pushToken)
      .map(device => device.pushToken!);

    if (activeTokens.length === 0) {
      return [];
    }

    return await this.sendMulticastPushNotification(
      activeTokens,
      senderName,
      messageContent,
      {
        type: 'message',
        chatId,
        messageId,
        senderId: senderName,
      },
      {
        sound: 'message_tone.mp3',
        clickAction: 'OPEN_CHAT',
      }
    );
  }

  // Send call notification
  async sendCallNotification(
    user: IUser,
    callerName: string,
    callType: 'voice' | 'video',
    callId: string
  ): Promise<PushResult[]> {
    const activeTokens = user.devices
      .filter(device => device.pushToken)
      .map(device => device.pushToken!);

    if (activeTokens.length === 0) {
      return [];
    }

    return await this.sendMulticastPushNotification(
      activeTokens,
      `Incoming ${callType} call`,
      `${callerName} is calling you`,
      {
        type: 'call',
        callId,
        callType,
        callerName,
      },
      {
        sound: 'ringtone.mp3',
        clickAction: 'ANSWER_CALL',
      }
    );
  }

  // Send group notification
  async sendGroupNotification(
    user: IUser,
    groupName: string,
    senderName: string,
    messageContent: string,
    chatId: string
  ): Promise<PushResult[]> {
    const activeTokens = user.devices
      .filter(device => device.pushToken)
      .map(device => device.pushToken!);

    if (activeTokens.length === 0) {
      return [];
    }

    return await this.sendMulticastPushNotification(
      activeTokens,
      groupName,
      `${senderName}: ${messageContent}`,
      {
        type: 'group_message',
        chatId,
        groupName,
        senderName,
      },
      {
        sound: 'group_message.mp3',
        clickAction: 'OPEN_GROUP',
      }
    );
  }

  // Validate push token
  async validatePushToken(token: string): Promise<boolean> {
    try {
      await this.messaging.send({
        token,
        data: { test: 'true' },
      }, true); // dry run
      return true;
    } catch (error) {
      return false;
    }
  }

  // Subscribe to topic
  async subscribeToTopic(tokens: string[], topic: string): Promise<void> {
    try {
      await this.messaging.subscribeToTopic(tokens, topic);
    } catch (error) {
      console.error('Topic subscription error:', error);
    }
  }

  // Unsubscribe from topic
  async unsubscribeFromTopic(tokens: string[], topic: string): Promise<void> {
    try {
      await this.messaging.unsubscribeFromTopic(tokens, topic);
    } catch (error) {
      console.error('Topic unsubscription error:', error);
    }
  }
}

// Initialize push notification service
const pushConfig: PushNotificationConfig = {
  projectId: process.env.FIREBASE_PROJECT_ID || '',
  privateKey: process.env.FIREBASE_PRIVATE_KEY || '',
  clientEmail: process.env.FIREBASE_CLIENT_EMAIL || '',
};

export const pushNotificationService = new PushNotificationService(pushConfig);
