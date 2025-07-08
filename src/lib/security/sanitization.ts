import DOMPurify from 'isomorphic-dompurify';
import validator from 'validator';

export class DataSanitizer {
  // Sanitize HTML content
  static sanitizeHTML(input: string): string {
    return DOMPurify.sanitize(input, {
      ALLOWED_TAGS: ['b', 'i', 'em', 'strong', 'a', 'br'],
      ALLOWED_ATTR: ['href'],
      ALLOW_DATA_ATTR: false,
    });
  }

  // Sanitize plain text (remove HTML)
  static sanitizePlainText(input: string): string {
    return validator.stripLow(validator.escape(input));
  }

  // Sanitize filename
  static sanitizeFilename(filename: string): string {
    // Remove path traversal attempts
    let sanitized = filename.replace(/[/\\]/g, '');
    
    // Remove dangerous characters
    sanitized = sanitized.replace(/[<>:"|?*]/g, '_');
    
    // Remove leading/trailing dots and spaces
    sanitized = sanitized.replace(/^[.\s]+|[.\s]+$/g, '');
    
    // Limit length
    if (sanitized.length > 255) {
      const ext = sanitized.split('.').pop();
      const name = sanitized.substring(0, sanitized.lastIndexOf('.'));
      sanitized = name.substring(0, 255 - ext!.length - 1) + '.' + ext;
    }
    
    return sanitized || 'unnamed_file';
  }

  // Sanitize phone number
  static sanitizePhoneNumber(phone: string): string {
    // Remove all non-digit characters except +
    let sanitized = phone.replace(/[^\d+]/g, '');
    
    // Ensure it starts with + if it doesn't
    if (!sanitized.startsWith('+')) {
      sanitized = '+' + sanitized;
    }
    
    return sanitized;
  }

  // Sanitize email
  static sanitizeEmail(email: string): string {
    return validator.normalizeEmail(email.toLowerCase().trim()) || '';
  }

  // Sanitize URL
  static sanitizeURL(url: string): string {
    try {
      const sanitized = new URL(url.trim());
      // Only allow http and https protocols
      if (!['http:', 'https:'].includes(sanitized.protocol)) {
        throw new Error('Invalid protocol');
      }
      return sanitized.toString();
    } catch {
      return '';
    }
  }

  // Remove SQL injection patterns
  static sanitizeSQL(input: string): string {
    const sqlPatterns = [
      /(\b(SELECT|INSERT|UPDATE|DELETE|DROP|CREATE|ALTER|EXEC|UNION|SCRIPT)\b)/gi,
      /(--|#|\/\*|\*\/)/g,
      /(\bOR\b|\bAND\b)\s+\d+\s*=\s*\d+/gi,
    ];

    let sanitized = input;
    sqlPatterns.forEach(pattern => {
      sanitized = sanitized.replace(pattern, '');
    });

    return sanitized.trim();
  }

  // Sanitize user input for search
  static sanitizeSearchQuery(query: string): string {
    let sanitized = query.trim();
    
    // Remove special regex characters
    sanitized = sanitized.replace(/[.*+?^${}()|[\]\\]/g, '\\export const callManager = new CallManager();');
    
    // Limit length
    sanitized = sanitized.substring(0, 100);
    
    return sanitized;
  }

  // Sanitize message content
  static sanitizeMessageContent(content: string): string {
    let sanitized = content.trim();
    
    // Remove null bytes
    sanitized = sanitized.replace(/\0/g, '');
    
    // Normalize whitespace
    sanitized = sanitized.replace(/\s+/g, ' ');
    
    // Limit length
    if (sanitized.length > 4096) {
      sanitized = sanitized.substring(0, 4096);
    }
    
    return sanitized;
  }

  // Sanitize object recursively
  static sanitizeObject(obj: any): any {
    if (typeof obj === 'string') {
      return this.sanitizePlainText(obj);
    }
    
    if (Array.isArray(obj)) {
      return obj.map(item => this.sanitizeObject(item));
    }
    
    if (obj && typeof obj === 'object') {
      const sanitized: any = {};
      for (const [key, value] of Object.entries(obj)) {
        const sanitizedKey = this.sanitizePlainText(key);
        sanitized[sanitizedKey] = this.sanitizeObject(value);
      }
      return sanitized;
    }
    
    return obj;
  }

  // Remove XSS patterns
  static removeXSS(input: string): string {
    const xssPatterns = [
      /<script\b[^<]*(?:(?!<\/script>)<[^<]*)*<\/script>/gi,
      /<iframe\b[^<]*(?:(?!<\/iframe>)<[^<]*)*<\/iframe>/gi,
      /javascript:/gi,
      /on\w+\s*=/gi,
      /<object\b[^<]*(?:(?!<\/object>)<[^<]*)*<\/object>/gi,
      /<embed\b[^>]*>/gi,
      /<applet\b[^<]*(?:(?!<\/applet>)<[^<]*)*<\/applet>/gi,
    ];

    let sanitized = input;
    xssPatterns.forEach(pattern => {
      sanitized = sanitized.replace(pattern, '');
    });

    return sanitized;
  }
}

// Middleware for request sanitization
export function sanitizeRequest() {
  return (req: any, res: any, next: any) => {
    if (req.body) {
      req.body = DataSanitizer.sanitizeObject(req.body);
    }
    if (req.query) {
      req.query = DataSanitizer.sanitizeObject(req.query);
    }
    if (req.params) {
      req.params = DataSanitizer.sanitizeObject(req.params);
    }
    next();
  };
}