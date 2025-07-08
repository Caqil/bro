import { z } from 'zod';

// File type configurations
export const FILE_CONFIGS = {
  image: {
    maxSize: 10 * 1024 * 1024, // 10MB
    allowedTypes: ['image/jpeg', 'image/png', 'image/gif', 'image/webp'],
    allowedExtensions: ['.jpg', '.jpeg', '.png', '.gif', '.webp'],
  },
  video: {
    maxSize: 100 * 1024 * 1024, // 100MB
    allowedTypes: ['video/mp4', 'video/mpeg', 'video/quicktime', 'video/webm'],
    allowedExtensions: ['.mp4', '.mov', '.avi', '.webm'],
  },
  audio: {
    maxSize: 50 * 1024 * 1024, // 50MB
    allowedTypes: ['audio/mpeg', 'audio/wav', 'audio/ogg', 'audio/aac'],
    allowedExtensions: ['.mp3', '.wav', '.ogg', '.aac'],
  },
  voice: {
    maxSize: 10 * 1024 * 1024, // 10MB
    allowedTypes: ['audio/ogg', 'audio/wav', 'audio/webm'],
    allowedExtensions: ['.ogg', '.wav', '.webm'],
  },
  document: {
    maxSize: 50 * 1024 * 1024, // 50MB
    allowedTypes: [
      'application/pdf',
      'application/msword',
      'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
      'application/vnd.ms-excel',
      'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      'text/plain',
    ],
    allowedExtensions: ['.pdf', '.doc', '.docx', '.xls', '.xlsx', '.txt'],
  },
};

export interface FileValidationResult {
  isValid: boolean;
  errors: string[];
  sanitizedName: string;
  detectedType: keyof typeof FILE_CONFIGS;
}

export class FileValidator {
  // Validate file based on type
  static validateFile(
    file: File | { name: string; size: number; type: string },
    expectedType: keyof typeof FILE_CONFIGS
  ): FileValidationResult {
    const errors: string[] = [];
    const config = FILE_CONFIGS[expectedType];
    
    // Sanitize filename
    const sanitizedName = this.sanitizeFilename(file.name);
    
    // Check file size
    if (file.size > config.maxSize) {
      errors.push(`File size exceeds ${config.maxSize / (1024 * 1024)}MB limit`);
    }
    
    if (file.size === 0) {
      errors.push('File is empty');
    }

    // Check MIME type
    if (!config.allowedTypes.includes(file.type)) {
      errors.push(`File type ${file.type} is not allowed for ${expectedType}`);
    }

    // Check file extension
    const extension = this.getFileExtension(file.name).toLowerCase();
    if (!config.allowedExtensions.includes(extension)) {
      errors.push(`File extension ${extension} is not allowed for ${expectedType}`);
    }

    // Detect actual type based on MIME and extension
    const detectedType = this.detectFileType(file.type, extension);

    return {
      isValid: errors.length === 0,
      errors,
      sanitizedName,
      detectedType,
    };
  }

  // Sanitize filename
  static sanitizeFilename(filename: string): string {
    // Remove path traversal attempts
    let sanitized = filename.replace(/[/\\]/g, '');
    
    // Remove or replace dangerous characters
    sanitized = sanitized.replace(/[<>:"|?*]/g, '_');
    
    // Limit filename length
    const maxLength = 255;
    if (sanitized.length > maxLength) {
      const extension = this.getFileExtension(sanitized);
      const nameWithoutExt = sanitized.substring(0, sanitized.lastIndexOf('.'));
      const truncatedName = nameWithoutExt.substring(0, maxLength - extension.length - 1);
      sanitized = truncatedName + extension;
    }

    return sanitized;
  }

  // Get file extension
  static getFileExtension(filename: string): string {
    const lastDotIndex = filename.lastIndexOf('.');
    return lastDotIndex !== -1 ? filename.substring(lastDotIndex) : '';
  }

  // Detect file type based on MIME type and extension
  static detectFileType(mimeType: string, extension: string): keyof typeof FILE_CONFIGS {
    for (const [type, config] of Object.entries(FILE_CONFIGS)) {
      if (config.allowedTypes.includes(mimeType) && config.allowedExtensions.includes(extension)) {
        return type as keyof typeof FILE_CONFIGS;
      }
    }
    return 'document'; // Default fallback
  }

  // Validate multiple files
  static validateFiles(
    files: (File | { name: string; size: number; type: string })[],
    expectedType: keyof typeof FILE_CONFIGS
  ): { valid: FileValidationResult[]; invalid: FileValidationResult[] } {
    const results = files.map(file => this.validateFile(file, expectedType));
    
    return {
      valid: results.filter(result => result.isValid),
      invalid: results.filter(result => !result.isValid),
    };
  }

  // Check if file is potentially malicious
  static checkMaliciousFile(filename: string, mimeType: string): boolean {
    const dangerousExtensions = [
      '.exe', '.bat', '.cmd', '.scr', '.pif', '.com', '.jar',
      '.js', '.jse', '.vbs', '.vbe', '.ws', '.wsf', '.wsc'
    ];
    
    const extension = this.getFileExtension(filename).toLowerCase();
    
    // Check for dangerous extensions
    if (dangerousExtensions.includes(extension)) {
      return true;
    }
    
    // Check for MIME type mismatches (potential attack)
    if (mimeType.includes('application/x-msdownload') || 
        mimeType.includes('application/x-executable')) {
      return true;
    }
    
    return false;
  }
}

// Zod schemas for API validation
export const uploadFileSchema = z.object({
  type: z.enum(['image', 'video', 'audio', 'voice', 'document']),
  chatId: z.string().regex(/^[0-9a-fA-F]{24}$/).optional(),
});

export const fileQuerySchema = z.object({
  type: z.enum(['image', 'video', 'audio', 'voice', 'document']).optional(),
  limit: z.number().min(1).max(50).default(20),
  offset: z.number().min(0).default(0),
});

export type UploadFileInput = z.infer<typeof uploadFileSchema>;
export type FileQueryInput = z.infer<typeof fileQuerySchema>;