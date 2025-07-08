export const APP_CONFIG = {
  NAME: 'ChatApp',
  VERSION: '1.0.0',
  DESCRIPTION: 'WhatsApp-like messaging application',
  SUPPORT_EMAIL: 'support@chatapp.com',
  WEBSITE_URL: process.env.WEBSITE_URL || 'https://chatapp.com',
  FRONTEND_URL: process.env.FRONTEND_URL || 'http://localhost:3000',
  API_URL: process.env.API_URL || 'http://localhost:3000/api',
} as const;

// Database constants
export const DB_CONSTANTS = {
  MAX_RETRIES: 3,
  RETRY_DELAY: 1000,
  CONNECTION_TIMEOUT: 30000,
  QUERY_TIMEOUT: 15000,
} as const;

// Authentication constants
export const AUTH_CONSTANTS = {
  JWT_EXPIRES_IN: '7d',
  REFRESH_TOKEN_EXPIRES_IN: '30d',
  OTP_EXPIRES_IN: 10 * 60 * 1000, // 10 minutes in milliseconds
  OTP_LENGTH: 6,
  MAX_LOGIN_ATTEMPTS: 5,
  LOCKOUT_DURATION: 15 * 60 * 1000, // 15 minutes
  PASSWORD_MIN_LENGTH: 8,
  PASSWORD_MAX_LENGTH: 128,
} as const;

// Message constants
export const MESSAGE_CONSTANTS = {
  MAX_LENGTH: 4096,
  MAX_MEDIA_SIZE: 100 * 1024 * 1024, // 100MB
  SUPPORTED_IMAGE_TYPES: ['image/jpeg', 'image/png', 'image/gif', 'image/webp'],
  SUPPORTED_VIDEO_TYPES: ['video/mp4', 'video/mpeg', 'video/quicktime', 'video/webm'],
  SUPPORTED_AUDIO_TYPES: ['audio/mpeg', 'audio/wav', 'audio/ogg', 'audio/aac'],
  SUPPORTED_DOCUMENT_TYPES: [
    'application/pdf',
    'application/msword',
    'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    'application/vnd.ms-excel',
    'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    'text/plain',
  ],
  DELETE_FOR_EVERYONE_TIME_LIMIT: 7 * 60 * 1000, // 7 minutes
  EDIT_TIME_LIMIT: 15 * 60 * 1000, // 15 minutes
} as const;

// Group constants
export const GROUP_CONSTANTS = {
  MAX_PARTICIPANTS: 256,
  MAX_ADMINS: 10,
  NAME_MAX_LENGTH: 50,
  DESCRIPTION_MAX_LENGTH: 200,
  INVITE_LINK_EXPIRES_IN: 24 * 60 * 60 * 1000, // 24 hours
} as const;

// Call constants
export const CALL_CONSTANTS = {
  MAX_DURATION: 4 * 60 * 60 * 1000, // 4 hours
  MAX_GROUP_PARTICIPANTS: 8,
  ICE_GATHERING_TIMEOUT: 10000, // 10 seconds
  TURN_CREDENTIALS_TTL: 24 * 60 * 60, // 24 hours in seconds
  QUALITY_CHECK_INTERVAL: 10000, // 10 seconds
} as const;

// Status constants
export const STATUS_CONSTANTS = {
  EXPIRES_IN: 24 * 60 * 60 * 1000, // 24 hours
  MAX_LENGTH: 139, // Like Twitter
  MAX_MEDIA_SIZE: 50 * 1024 * 1024, // 50MB
} as const;

// Rate limiting constants
export const RATE_LIMIT_CONSTANTS = {
  GENERAL_REQUESTS_PER_WINDOW: 100,
  GENERAL_WINDOW_MS: 15 * 60 * 1000, // 15 minutes
  AUTH_REQUESTS_PER_WINDOW: 5,
  AUTH_WINDOW_MS: 15 * 60 * 1000, // 15 minutes
  MESSAGE_REQUESTS_PER_WINDOW: 30,
  MESSAGE_WINDOW_MS: 60 * 1000, // 1 minute
  UPLOAD_REQUESTS_PER_WINDOW: 10,
  UPLOAD_WINDOW_MS: 60 * 1000, // 1 minute
} as const;

// File upload constants
export const UPLOAD_CONSTANTS = {
  TEMP_DIR: '/tmp/chatapp',
  THUMBNAIL_SIZE: { width: 300, height: 300 },
  COMPRESSION_QUALITY: 85,
  MAX_FILENAME_LENGTH: 255,
  ALLOWED_EXTENSIONS: {
    image: ['.jpg', '.jpeg', '.png', '.gif', '.webp'],
    video: ['.mp4', '.mov', '.avi', '.webm'],
    audio: ['.mp3', '.wav', '.ogg', '.aac'],
    document: ['.pdf', '.doc', '.docx', '.xls', '.xlsx', '.txt'],
  },
} as const;

// Email constants
export const EMAIL_CONSTANTS = {
  FROM_ADDRESS: process.env.SMTP_FROM || 'noreply@chatapp.com',
  REPLY_TO: process.env.SMTP_REPLY_TO || 'support@chatapp.com',
  TEMPLATES_PATH: './templates',
  ATTACHMENT_SIZE_LIMIT: 25 * 1024 * 1024, // 25MB
} as const;

// Push notification constants
export const PUSH_CONSTANTS = {
  MAX_PAYLOAD_SIZE: 4096,
  BATCH_SIZE: 500,
  RETRY_ATTEMPTS: 3,
  TTL: 24 * 60 * 60, // 24 hours
  CHANNELS: {
    MESSAGES: 'chat_messages',
    CALLS: 'voice_calls',
    GROUPS: 'group_updates',
    SYSTEM: 'system_notifications',
  },
} as const;

// Pagination constants
export const PAGINATION_CONSTANTS = {
  DEFAULT_PAGE_SIZE: 20,
  MAX_PAGE_SIZE: 100,
  DEFAULT_PAGE: 1,
} as const;

// Cache constants
export const CACHE_CONSTANTS = {
  USER_CACHE_TTL: 60 * 60, // 1 hour
  MESSAGE_CACHE_TTL: 5 * 60, // 5 minutes
  CHAT_CACHE_TTL: 30 * 60, // 30 minutes
  MEDIA_CACHE_TTL: 24 * 60 * 60, // 24 hours
  DEFAULT_TTL: 15 * 60, // 15 minutes
} as const;

// Error codes
export const ERROR_CODES = {
  // Authentication errors
  INVALID_CREDENTIALS: 'INVALID_CREDENTIALS',
  EXPIRED_TOKEN: 'EXPIRED_TOKEN',
  INVALID_OTP: 'INVALID_OTP',
  OTP_EXPIRED: 'OTP_EXPIRED',
  ACCOUNT_LOCKED: 'ACCOUNT_LOCKED',
  USER_BANNED: 'USER_BANNED',
  
  // Authorization errors
  INSUFFICIENT_PERMISSIONS: 'INSUFFICIENT_PERMISSIONS',
  ACCESS_DENIED: 'ACCESS_DENIED',
  RESOURCE_NOT_FOUND: 'RESOURCE_NOT_FOUND',
  
  // Validation errors
  INVALID_INPUT: 'INVALID_INPUT',
  MISSING_REQUIRED_FIELD: 'MISSING_REQUIRED_FIELD',
  INVALID_FILE_TYPE: 'INVALID_FILE_TYPE',
  FILE_TOO_LARGE: 'FILE_TOO_LARGE',
  
  // Rate limiting
  RATE_LIMIT_EXCEEDED: 'RATE_LIMIT_EXCEEDED',
  
  // Server errors
  INTERNAL_SERVER_ERROR: 'INTERNAL_SERVER_ERROR',
  DATABASE_ERROR: 'DATABASE_ERROR',
  EXTERNAL_SERVICE_ERROR: 'EXTERNAL_SERVICE_ERROR',
  
  // Business logic errors
  USER_ALREADY_EXISTS: 'USER_ALREADY_EXISTS',
  USER_NOT_FOUND: 'USER_NOT_FOUND',
  CHAT_NOT_FOUND: 'CHAT_NOT_FOUND',
  MESSAGE_NOT_FOUND: 'MESSAGE_NOT_FOUND',
  CALL_NOT_FOUND: 'CALL_NOT_FOUND',
  GROUP_FULL: 'GROUP_FULL',
  USER_ALREADY_IN_GROUP: 'USER_ALREADY_IN_GROUP',
  USER_NOT_IN_GROUP: 'USER_NOT_IN_GROUP',
} as const;

// Socket events
export const SOCKET_EVENTS = {
  // Connection
  CONNECT: 'connect',
  DISCONNECT: 'disconnect',
  
  // Authentication
  AUTHENTICATE: 'authenticate',
  AUTHENTICATED: 'authenticated',
  
  // Presence
  USER_ONLINE: 'user:online',
  USER_OFFLINE: 'user:offline',
  PRESENCE_UPDATE: 'presence:update',
  
  // Messaging
  MESSAGE_SEND: 'message:send',
  MESSAGE_RECEIVE: 'message:receive',
  MESSAGE_DELIVERED: 'message:delivered',
  MESSAGE_READ: 'message:read',
  MESSAGE_TYPING: 'message:typing',
  MESSAGE_TYPING_STOP: 'message:typing:stop',
  
  // Calls
  CALL_INITIATE: 'call:initiate',
  CALL_INCOMING: 'call:incoming',
  CALL_ACCEPT: 'call:accept',
  CALL_REJECT: 'call:reject',
  CALL_END: 'call:end',
  CALL_ICE_CANDIDATE: 'call:ice-candidate',
  CALL_OFFER: 'call:offer',
  CALL_ANSWER: 'call:answer',
  
  // Groups
  GROUP_CREATED: 'group:created',
  GROUP_MEMBER_ADDED: 'group:member:added',
  GROUP_MEMBER_REMOVED: 'group:member:removed',
  GROUP_UPDATED: 'group:updated',
  
  // Errors
  ERROR: 'error',
} as const;

// Status messages
export const STATUS_MESSAGES = {
  SUCCESS: 'Operation completed successfully',
  CREATED: 'Resource created successfully',
  UPDATED: 'Resource updated successfully',
  DELETED: 'Resource deleted successfully',
  NOT_FOUND: 'Resource not found',
  UNAUTHORIZED: 'Unauthorized access',
  FORBIDDEN: 'Insufficient permissions',
  BAD_REQUEST: 'Invalid request',
  INTERNAL_ERROR: 'Internal server error',
  RATE_LIMITED: 'Rate limit exceeded',
} as const;

// Regular expressions
export const REGEX_PATTERNS = {
  PHONE_NUMBER: /^\+?[1-9]\d{1,14}$/,
  EMAIL: /^[^\s@]+@[^\s@]+\.[^\s@]+$/,
  USERNAME: /^[a-zA-Z0-9_]{3,30}$/,
  HEX_COLOR: /^#([A-Fa-f0-9]{6}|[A-Fa-f0-9]{3})$/,
  UUID: /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i,
  OBJECT_ID: /^[0-9a-fA-F]{24}$/,
  JWT_TOKEN: /^[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+\.[A-Za-z0-9-_]*$/,
} as const;