import crypto from 'crypto';

export class CryptoUtils {
  // Generate secure random bytes
  static generateRandomBytes(length: number): Buffer {
    return crypto.randomBytes(length);
  }

  // Generate secure random string
  static generateRandomString(length: number, charset?: string): string {
    const defaultCharset = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    const chars = charset || defaultCharset;
    
    let result = '';
    const randomBytes = this.generateRandomBytes(length);
    
    for (let i = 0; i < length; i++) {
      result += chars[randomBytes[i] % chars.length];
    }
    
    return result;
  }

  // Generate UUID v4
  static generateUUID(): string {
    return crypto.randomUUID();
  }

  // Generate OTP
  static generateOTP(length: number = 6): string {
    const digits = '0123456789';
    return this.generateRandomString(length, digits);
  }

  // Hash string with SHA-256
  static hash(data: string, algorithm: string = 'sha256'): string {
    return crypto.createHash(algorithm).update(data, 'utf8').digest('hex');
  }

  // Generate HMAC
  static hmac(data: string, secret: string, algorithm: string = 'sha256'): string {
    return crypto.createHmac(algorithm, secret).update(data, 'utf8').digest('hex');
  }

  // Verify HMAC
  static verifyHmac(data: string, secret: string, signature: string, algorithm: string = 'sha256'): boolean {
    const expectedSignature = this.hmac(data, secret, algorithm);
    return crypto.timingSafeEqual(Buffer.from(signature, 'hex'), Buffer.from(expectedSignature, 'hex'));
  }

  // Generate salt
  static generateSalt(length: number = 16): string {
    return this.generateRandomBytes(length).toString('hex');
  }

  // Hash password with salt
  static async hashPassword(password: string, salt?: string): Promise<{ hash: string; salt: string }> {
    const saltToUse = salt || this.generateSalt();
    const hash = await new Promise<string>((resolve, reject) => {
      crypto.pbkdf2(password, saltToUse, 100000, 32, 'sha256', (err, derivedKey) => {
        if (err) reject(err);
        else resolve(derivedKey.toString('hex'));
      });
    });
    
    return { hash, salt: saltToUse };
  }

  // Verify password
  static async verifyPassword(password: string, hash: string, salt: string): Promise<boolean> {
    const { hash: computedHash } = await this.hashPassword(password, salt);
    return crypto.timingSafeEqual(Buffer.from(hash, 'hex'), Buffer.from(computedHash, 'hex'));
  }

  // Generate JWT-like token
  static generateToken(payload: object, secret: string, expiresIn?: number): string {
    const header = { alg: 'HS256', typ: 'JWT' };
    const now = Math.floor(Date.now() / 1000);
    
    const claims = {
      ...payload,
      iat: now,
      ...(expiresIn && { exp: now + expiresIn }),
    };

    const encodedHeader = Buffer.from(JSON.stringify(header)).toString('base64url');
    const encodedPayload = Buffer.from(JSON.stringify(claims)).toString('base64url');
    const signature = this.hmac(`${encodedHeader}.${encodedPayload}`, secret);
    
    return `${encodedHeader}.${encodedPayload}.${signature}`;
  }

  // Verify JWT-like token
  static verifyToken(token: string, secret: string): object | null {
    try {
      const [encodedHeader, encodedPayload, signature] = token.split('.');
      
      if (!encodedHeader || !encodedPayload || !signature) {
        return null;
      }

      // Verify signature
      const expectedSignature = this.hmac(`${encodedHeader}.${encodedPayload}`, secret);
      if (!crypto.timingSafeEqual(Buffer.from(signature, 'hex'), Buffer.from(expectedSignature, 'hex'))) {
        return null;
      }

      // Decode payload
      const payload = JSON.parse(Buffer.from(encodedPayload, 'base64url').toString('utf8'));
      
      // Check expiration
      if (payload.exp && payload.exp < Math.floor(Date.now() / 1000)) {
        return null;
      }

      return payload;
    } catch {
      return null;
    }
  }

  // Generate file checksum
  static generateFileChecksum(buffer: Buffer, algorithm: string = 'sha256'): string {
    return crypto.createHash(algorithm).update(buffer).digest('hex');
  }

  // Encrypt string (simple AES-256-GCM)
  static encrypt(text: string, key: string): { encrypted: string; iv: string; tag: string } {
    const iv = crypto.randomBytes(16);
    const cipher = crypto.createCipher('aes-256-gcm', Buffer.from(key, 'hex'));
    
    let encrypted = cipher.update(text, 'utf8', 'hex');
    encrypted += cipher.final('hex');
    
    const tag = cipher.getAuthTag();
    
    return {
      encrypted,
      iv: iv.toString('hex'),
      tag: tag.toString('hex'),
    };
  }

  // Decrypt string
  static decrypt(encryptedData: { encrypted: string; iv: string; tag: string }, key: string): string {
    const decipher = crypto.createDecipher('aes-256-gcm', Buffer.from(key, 'hex'));
    decipher.setAuthTag(Buffer.from(encryptedData.tag, 'hex'));
    
    let decrypted = decipher.update(encryptedData.encrypted, 'hex', 'utf8');
    decrypted += decipher.final('utf8');
    
    return decrypted;
  }

  // Constant time string comparison
  static constantTimeEqual(a: string, b: string): boolean {
    if (a.length !== b.length) {
      return false;
    }
    
    return crypto.timingSafeEqual(Buffer.from(a), Buffer.from(b));
  }
}