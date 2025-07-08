import { S3Client, PutObjectCommand, GetObjectCommand, DeleteObjectCommand, HeadObjectCommand } from '@aws-sdk/client-s3';
import { getSignedUrl } from '@aws-sdk/s3-request-presigner';
import crypto from 'crypto';

interface S3Config {
  region: string;
  bucket: string;
  accessKeyId: string;
  secretAccessKey: string;
  endpoint?: string; // For custom S3-compatible services
}

interface UploadOptions {
  contentType: string;
  metadata?: Record<string, string>;
  acl?: 'private' | 'public-read';
  expiresIn?: number; // For signed URLs
}

interface UploadResult {
  key: string;
  url: string;
  bucket: string;
  etag: string;
  size: number;
}

class S3Service {
  private client: S3Client;
  private bucket: string;

  constructor(config: S3Config) {
    this.client = new S3Client({
      region: config.region,
      credentials: {
        accessKeyId: config.accessKeyId,
        secretAccessKey: config.secretAccessKey,
      },
      endpoint: config.endpoint,
    });
    this.bucket = config.bucket;
  }

  // Generate unique file key
  private generateFileKey(originalName: string, userId: string, type: 'media' | 'avatar' | 'thumbnail'): string {
    const timestamp = Date.now();
    const randomString = crypto.randomBytes(8).toString('hex');
    const extension = originalName.split('.').pop();
    const sanitizedName = originalName.replace(/[^a-zA-Z0-9.-]/g, '_');
    
    return `${type}/${userId}/${timestamp}_${randomString}_${sanitizedName}`;
  }

  // Upload file to S3
  async uploadFile(
    file: Buffer | Uint8Array,
    originalName: string,
    userId: string,
    options: UploadOptions,
    type: 'media' | 'avatar' | 'thumbnail' = 'media'
  ): Promise<UploadResult> {
    try {
      const key = this.generateFileKey(originalName, userId, type);
      
      const command = new PutObjectCommand({
        Bucket: this.bucket,
        Key: key,
        Body: file,
        ContentType: options.contentType,
        Metadata: {
          originalName,
          uploadedBy: userId,
          uploadedAt: new Date().toISOString(),
          ...options.metadata,
        },
        ACL: options.acl || 'private',
      });

      const result = await this.client.send(command);
      const url = await this.getFileUrl(key, options.expiresIn);

      return {
        key,
        url,
        bucket: this.bucket,
        etag: result.ETag || '',
        size: file.length,
      };
    } catch (error) {
      console.error('S3 upload error:', error);
      throw new Error('Failed to upload file to S3');
    }
  }

  // Get file URL (signed if private)
  async getFileUrl(key: string, expiresIn: number = 3600): Promise<string> {
    try {
      const command = new GetObjectCommand({
        Bucket: this.bucket,
        Key: key,
      });

      return await getSignedUrl(this.client, command, { expiresIn });
    } catch (error) {
      console.error('S3 URL generation error:', error);
      throw new Error('Failed to generate file URL');
    }
  }

  // Get file metadata
  async getFileMetadata(key: string): Promise<any> {
    try {
      const command = new HeadObjectCommand({
        Bucket: this.bucket,
        Key: key,
      });

      const result = await this.client.send(command);
      return {
        size: result.ContentLength,
        contentType: result.ContentType,
        lastModified: result.LastModified,
        metadata: result.Metadata,
        etag: result.ETag,
      };
    } catch (error) {
      console.error('S3 metadata error:', error);
      throw new Error('Failed to get file metadata');
    }
  }

  // Delete file from S3
  async deleteFile(key: string): Promise<boolean> {
    try {
      const command = new DeleteObjectCommand({
        Bucket: this.bucket,
        Key: key,
      });

      await this.client.send(command);
      return true;
    } catch (error) {
      console.error('S3 delete error:', error);
      return false;
    }
  }

  // Get download stream
  async getFileStream(key: string): Promise<ReadableStream | undefined> {
    try {
      const command = new GetObjectCommand({
        Bucket: this.bucket,
        Key: key,
      });

      const result = await this.client.send(command);
      return result.Body as ReadableStream;
    } catch (error) {
      console.error('S3 stream error:', error);
      throw new Error('Failed to get file stream');
    }
  }
}

// Initialize S3 service
const s3Config: S3Config = {
  region: process.env.AWS_REGION || 'us-east-1',
  bucket: process.env.AWS_S3_BUCKET || '',
  accessKeyId: process.env.AWS_ACCESS_KEY_ID || '',
  secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY || '',
  endpoint: process.env.AWS_S3_ENDPOINT, // Optional for custom endpoints
};

export const s3Service = new S3Service(s3Config);
export { S3Service };
export type { UploadOptions, UploadResult };