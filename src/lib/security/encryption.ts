import crypto from 'crypto';
import { promisify } from 'util';

interface EncryptionResult {
  encrypted: string;
  iv: string;
  tag?: string;
}

interface KeyPair {
  publicKey: string;
  privateKey: string;
}

export class EncryptionService {
  private algorithm = 'aes-256-gcm';
  private keyLength = 32; // 256 bits
  private ivLength = 16;  // 128 bits

  // Generate random encryption key
  generateKey(): string {
    return crypto.randomBytes(this.keyLength).toString('hex');
  }

  // Generate RSA key pair for end-to-end encryption
  generateKeyPair(): KeyPair {
    const { publicKey, privateKey } = crypto.generateKeyPairSync('rsa', {
      modulusLength: 2048,
      publicKeyEncoding: {
        type: 'spki',
        format: 'pem'
      },
      privateKeyEncoding: {
        type: 'pkcs8',
        format: 'pem'
      }
    });

    return { publicKey, privateKey };
  }

  // Encrypt data using AES-256-GCM
  encrypt(data: string, key: string): EncryptionResult {
    try {
      const keyBuffer = Buffer.from(key, 'hex');
      const iv = crypto.randomBytes(this.ivLength);
      const cipher = crypto.createCipher(this.algorithm, keyBuffer);
      cipher.setAAD(Buffer.from('chatapp', 'utf8'));

      let encrypted = cipher.update(data, 'utf8', 'hex');
      encrypted += cipher.final('hex');

      const tag = cipher.getAuthTag().toString('hex');

      return {
        encrypted,
        iv: iv.toString('hex'),
        tag
      };
    } catch (error) {
      console.error('Encryption error:', error);
      throw new Error('Failed to encrypt data');
    }
  }

  // Decrypt data using AES-256-GCM
  decrypt(encryptedData: EncryptionResult, key: string): string {
    try {
      const keyBuffer = Buffer.from(key, 'hex');
      const iv = Buffer.from(encryptedData.iv, 'hex');
      const tag = Buffer.from(encryptedData.tag!, 'hex');

      const decipher = crypto.createDecipher(this.algorithm, keyBuffer);
      decipher.setAAD(Buffer.from('chatapp', 'utf8'));
      decipher.setAuthTag(tag);

      let decrypted = decipher.update(encryptedData.encrypted, 'hex', 'utf8');
      decrypted += decipher.final('utf8');

      return decrypted;
    } catch (error) {
      console.error('Decryption error:', error);
      throw new Error('Failed to decrypt data');
    }
  }

  // Encrypt with RSA public key (for key exchange)
  encryptWithPublicKey(data: string, publicKey: string): string {
    try {
      const encrypted = crypto.publicEncrypt(
        {
          key: publicKey,
          padding: crypto.constants.RSA_PKCS1_OAEP_PADDING,
          oaepHash: 'sha256'
        },
        Buffer.from(data, 'utf8')
      );

      return encrypted.toString('base64');
    } catch (error) {
      console.error('RSA encryption error:', error);
      throw new Error('Failed to encrypt with public key');
    }
  }

  // Decrypt with RSA private key
  decryptWithPrivateKey(encryptedData: string, privateKey: string): string {
    try {
      const decrypted = crypto.privateDecrypt(
        {
          key: privateKey,
          padding: crypto.constants.RSA_PKCS1_OAEP_PADDING,
          oaepHash: 'sha256'
        },
        Buffer.from(encryptedData, 'base64')
      );

      return decrypted.toString('utf8');
    } catch (error) {
      console.error('RSA decryption error:', error);
      throw new Error('Failed to decrypt with private key');
    }
  }

  // Generate PBKDF2 hash for passwords
  async hashPassword(password: string, salt?: string): Promise<{ hash: string; salt: string }> {
    try {
      const saltBuffer = salt ? Buffer.from(salt, 'hex') : crypto.randomBytes(16);
      const hashBuffer = await promisify(crypto.pbkdf2)(password, saltBuffer, 100000, 32, 'sha256');

      return {
        hash: hashBuffer.toString('hex'),
        salt: saltBuffer.toString('hex')
      };
    } catch (error) {
      console.error('Password hashing error:', error);
      throw new Error('Failed to hash password');
    }
  }

  // Verify password hash
  async verifyPassword(password: string, hash: string, salt: string): Promise<boolean> {
    try {
      const { hash: computedHash } = await this.hashPassword(password, salt);
      return crypto.timingSafeEqual(Buffer.from(hash, 'hex'), Buffer.from(computedHash, 'hex'));
    } catch (error) {
      console.error('Password verification error:', error);
      return false;
    }
  }

  // Generate secure random token
  generateSecureToken(length: number = 32): string {
    return crypto.randomBytes(length).toString('hex');
  }

  // Hash data with SHA-256
  hashData(data: string): string {
    return crypto.createHash('sha256').update(data, 'utf8').digest('hex');
  }

  // Generate HMAC signature
  generateSignature(data: string, secret: string): string {
    return crypto.createHmac('sha256', secret).update(data, 'utf8').digest('hex');
  }

  // Verify HMAC signature
  verifySignature(data: string, signature: string, secret: string): boolean {
    const expectedSignature = this.generateSignature(data, secret);
    return crypto.timingSafeEqual(Buffer.from(signature, 'hex'), Buffer.from(expectedSignature, 'hex'));
  }
}

export const encryptionService = new EncryptionService();
