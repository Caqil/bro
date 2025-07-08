import { IUser } from '../database/models/user';
import { IChat } from '../database/models/chat';
import { IAdmin } from '../database/models/admin';
import { ChatRepository } from '../database/repositories/chat';
import { GroupRepository } from '../database/repositories/group';
import { UserRepository } from '../database/repositories/user';

// Permission types
export enum Permission {
  // User permissions
  READ_MESSAGES = 'read_messages',
  SEND_MESSAGES = 'send_messages',
  DELETE_MESSAGES = 'delete_messages',
  EDIT_MESSAGES = 'edit_messages',
  
  // Media permissions
  UPLOAD_MEDIA = 'upload_media',
  DELETE_MEDIA = 'delete_media',
  
  // Call permissions
  INITIATE_CALLS = 'initiate_calls',
  ANSWER_CALLS = 'answer_calls',
  
  // Group permissions
  CREATE_GROUPS = 'create_groups',
  ADD_MEMBERS = 'add_members',
  REMOVE_MEMBERS = 'remove_members',
  EDIT_GROUP_INFO = 'edit_group_info',
  PROMOTE_ADMINS = 'promote_admins',
  
  // Admin permissions
  VIEW_USERS = 'view_users',
  MANAGE_USERS = 'manage_users',
  BAN_USERS = 'ban_users',
  VIEW_MESSAGES = 'view_messages',
  DELETE_ANY_MESSAGE = 'delete_any_message',
  VIEW_ANALYTICS = 'view_analytics',
  MANAGE_SYSTEM = 'manage_system',
}

// Role definitions
export const ROLE_PERMISSIONS = {
  user: [
    Permission.READ_MESSAGES,
    Permission.SEND_MESSAGES,
    Permission.DELETE_MESSAGES,
    Permission.EDIT_MESSAGES,
    Permission.UPLOAD_MEDIA,
    Permission.DELETE_MEDIA,
    Permission.INITIATE_CALLS,
    Permission.ANSWER_CALLS,
    Permission.CREATE_GROUPS,
  ],
  
  group_admin: [
    Permission.ADD_MEMBERS,
    Permission.REMOVE_MEMBERS,
    Permission.EDIT_GROUP_INFO,
    Permission.PROMOTE_ADMINS,
  ],
  
  moderator: [
    Permission.VIEW_USERS,
    Permission.VIEW_MESSAGES,
    Permission.DELETE_ANY_MESSAGE,
  ],
  
  admin: [
    Permission.VIEW_USERS,
    Permission.MANAGE_USERS,
    Permission.BAN_USERS,
    Permission.VIEW_MESSAGES,
    Permission.DELETE_ANY_MESSAGE,
    Permission.VIEW_ANALYTICS,
  ],
  
  super_admin: [
    Permission.VIEW_USERS,
    Permission.MANAGE_USERS,
    Permission.BAN_USERS,
    Permission.VIEW_MESSAGES,
    Permission.DELETE_ANY_MESSAGE,
    Permission.VIEW_ANALYTICS,
    Permission.MANAGE_SYSTEM,
  ],
};

export class PermissionService {
  private chatRepository: ChatRepository;
  private groupRepository: GroupRepository;
  private userRepository: UserRepository;

  constructor() {
    this.chatRepository = new ChatRepository();
    this.groupRepository = new GroupRepository();
    this.userRepository = new UserRepository();
  }

  // Check if user has permission
  hasPermission(user: IUser | IAdmin, permission: Permission): boolean {
    // Check if user is banned
    if ('isBanned' in user && user.isBanned) {
      return false;
    }

    // For admin users
    if ('role' in user) {
      const rolePermissions = ROLE_PERMISSIONS[user.role as keyof typeof ROLE_PERMISSIONS] || [];
      return rolePermissions.includes(permission) || user.permissions?.includes(permission.toString());
    }

    // For regular users
    const userPermissions = ROLE_PERMISSIONS.user;
    return userPermissions.includes(permission);
  }

  // Check if user can access chat
  async canAccessChat(userId: string, chatId: string): Promise<boolean> {
    try {
      const chat = await this.chatRepository.findById(chatId);
      return chat ? chat.participants.some(p => p.toString() === userId) : false;
    } catch {
      return false;
    }
  }

  // Check if user can send messages to chat
  async canSendMessage(userId: string, chatId: string): Promise<boolean> {
    try {
      const user = await this.userRepository.findById(userId);
      if (!user || user.isBanned || !this.hasPermission(user, Permission.SEND_MESSAGES)) {
        return false;
      }

      const chat = await this.chatRepository.findById(chatId);
      if (!chat || !chat.participants.some(p => p.toString() === userId)) {
        return false;
      }

      // Check group-specific permissions
      if (chat.type === 'group' && chat.groupInfo?.settings.whoCanSendMessages === 'admins') {
        return await this.groupRepository.isUserAdmin(chatId, userId);
      }

      return true;
    } catch {
      return false;
    }
  }

  // Check if user can delete message
  async canDeleteMessage(userId: string, messageId: string, chatId: string): Promise<boolean> {
    try {
      const user = await this.userRepository.findById(userId);
      if (!user || user.isBanned) {
        return false;
      }

      // Admin can delete any message
      if ('role' in user && this.hasPermission(user as any, Permission.DELETE_ANY_MESSAGE)) {
        return true;
      }

      // User can delete their own messages
      // TODO: Get message and check if user is the sender
      return this.hasPermission(user, Permission.DELETE_MESSAGES);
    } catch {
      return false;
    }
  }

  // Check if user can manage group
  async canManageGroup(userId: string, chatId: string, action: 'add_members' | 'remove_members' | 'edit_info' | 'promote'): Promise<boolean> {
    try {
      const user = await this.userRepository.findById(userId);
      if (!user || user.isBanned) {
        return false;
      }

      const chat = await this.chatRepository.findById(chatId);
      if (!chat || chat.type !== 'group') {
        return false;
      }

      // Check if user is group admin
      const isGroupAdmin = await this.groupRepository.isUserAdmin(chatId, userId);

      // Check group settings
      const settings = chat.groupInfo?.settings;
      switch (action) {
        case 'add_members':
          return isGroupAdmin || settings?.whoCanAddMembers === 'everyone';
        case 'remove_members':
          return isGroupAdmin;
        case 'edit_info':
          return isGroupAdmin || settings?.whoCanEditGroupInfo === 'everyone';
        case 'promote':
          return isGroupAdmin;
        default:
          return false;
      }
    } catch {
      return false;
    }
  }

  // Check if user can upload media
  async canUploadMedia(userId: string, chatId?: string): Promise<boolean> {
    try {
      const user = await this.userRepository.findById(userId);
      if (!user || user.isBanned || !this.hasPermission(user, Permission.UPLOAD_MEDIA)) {
        return false;
      }

      if (chatId) {
        return await this.canAccessChat(userId, chatId);
      }

      return true;
    } catch {
      return false;
    }
  }

  // Check if user can initiate calls
  async canInitiateCall(userId: string, participantIds: string[]): Promise<boolean> {
    try {
      const user = await this.userRepository.findById(userId);
      if (!user || user.isBanned || !this.hasPermission(user, Permission.INITIATE_CALLS)) {
        return false;
      }

      // Check if all participants exist and are not banned
      for (const participantId of participantIds) {
        const participant = await this.userRepository.findById(participantId);
        if (!participant || participant.isBanned) {
          return false;
        }

        // Check if participant allows calls (privacy settings)
        if (!participant.notificationSettings.callNotifications) {
          return false;
        }
      }

      return true;
    } catch {
      return false;
    }
  }

  // Get user permissions
  getUserPermissions(user: IUser | IAdmin): Permission[] {
    if ('role' in user) {
      const rolePermissions = ROLE_PERMISSIONS[user.role as keyof typeof ROLE_PERMISSIONS] || [];
      const customPermissions = user.permissions?.map(p => p as Permission) || [];
      return [...new Set([...rolePermissions, ...customPermissions])];
    }

    return ROLE_PERMISSIONS.user;
  }

  // Check multiple permissions at once
  hasPermissions(user: IUser | IAdmin, permissions: Permission[]): boolean {
    return permissions.every(permission => this.hasPermission(user, permission));
  }

  // Check if user can perform admin action
  canPerformAdminAction(admin: IAdmin, action: string): boolean {
    const actionPermissionMap: Record<string, Permission> = {
      'view_users': Permission.VIEW_USERS,
      'manage_users': Permission.MANAGE_USERS,
      'ban_users': Permission.BAN_USERS,
      'view_messages': Permission.VIEW_MESSAGES,
      'delete_messages': Permission.DELETE_ANY_MESSAGE,
      'view_analytics': Permission.VIEW_ANALYTICS,
      'manage_system': Permission.MANAGE_SYSTEM,
    };

    const requiredPermission = actionPermissionMap[action];
    return requiredPermission ? this.hasPermission(admin, requiredPermission) : false;
  }
}

export const permissionService = new PermissionService();
