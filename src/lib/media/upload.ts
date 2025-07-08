import { MediaRepository } from '../database/repositories/media';
import { IMedia } from '../database/models/media';
import { s3Service } from './s3';
import { FileValidator, FILE_CONFIGS } from './validation';
import { MediaCompressor } from './compression';
import { ThumbnailGenerator } from './thumbnail';
import crypto from 'crypto';
import { Types } from 'mongoose';

interface UploadFileOptions {
  compress?: boolean;
  generateThumbnail?: boolean;
  chatId?: string;
  messageId?: string;
}

interface UploadResult {
  media: IMedia;
  originalSize: number;
  compressedSize?: number;
  compressionRatio?: number;
}

export class MediaUploadService {
  private mediaRepository: MediaRepository;

  constructor() {
    this.mediaRepository = new MediaRepository();
  }

  // Upload single file
  async uploadFile(
    file: Buffer,
    originalName: string,
    mimeType: string,
    uploadedBy: string,
    type: 'image' | 'video' | 'audio' | 'voice' | 'document',
    options: UploadFileOptions = {}
  ): Promise<UploadResult> {
    try {
      // Validate file
      const validation = FileValidator.validateFile(
        { name: originalName, size: file.length, type: mimeType },
        type
      );

      if (!validation.isValid) {
        throw new Error(`File validation failed: ${validation.errors.join(', ')}`);
      }

      // Check for malicious files
      if (FileValidator.checkMaliciousFile(originalName, mimeType)) {
        throw new Error('File appears to be malicious and cannot be uploaded');
      }

      // Generate file checksum
      const checksum = crypto.createHash('sha256').update(file).digest('hex');

      // Check for duplicate files
      const existingMedia = await this.mediaRepository.findByChecksum(checksum);
      if (existingMedia) {
        // Return existing media instead of uploading duplicate
        return {
          media: existingMedia,
          originalSize: file.length,
        };
      }

      let processedFile = file;
      let metadata: any = {};
      let compressionResult: any = null;

      // Compress file if requested and supported
      if (options.compress && (type === 'image' || type === 'video' || type === 'audio')) {
        compressionResult = await this.compressFile(file, originalName, type);
        processedFile = compressionResult.buffer || file;
        metadata = { ...metadata, ...compressionResult.metadata };
      }

      // Upload to S3
      const uploadResult = await s3Service.uploadFile(
        processedFile,
        validation.sanitizedName,
        uploadedBy,
        {
          contentType: mimeType,
          metadata: {
            originalChecksum: checksum,
            fileType: type,
            compressed: options.compress ? 'true' : 'false',
          },
        }
      );

      let thumbnailUrl: string | undefined;

      // Generate thumbnail if requested
      if (options.generateThumbnail && (type === 'image' || type === 'video')) {
        try {
          thumbnailUrl = await this.generateAndUploadThumbnail(
            processedFile,
            originalName,
            type,
            uploadedBy
          );
        } catch (error) {
          console.warn('Thumbnail generation failed:', error);
          // Continue without thumbnail
        }
      }

      // Create media record in database
      const media = await this.mediaRepository.create({
        filename: uploadResult.key,
        originalName: validation.sanitizedName,
        mimeType,
        size: processedFile.length,
        url: uploadResult.url,
        thumbnailUrl,
        uploadedBy: uploadedBy as any,
        chatId: options.chatId as any,
        messageId: options.messageId as any,
        type,
        metadata,
        isEncrypted: false, // TODO: Implement encryption
        checksumSHA256: checksum,
      });

      return {
        media,
        originalSize: file.length,
        compressedSize: compressionResult?.metadata?.compressedSize,
        compressionRatio: compressionResult?.metadata?.compressionRatio,
      };

    } catch (error) {
      console.error('File upload error:', error);
      throw new Error(`Upload failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  // Compress file based on type
  private async compressFile(
    file: Buffer,
    originalName: string,
    type: 'image' | 'video' | 'audio'
  ): Promise<any> {
    switch (type) {
      case 'image':
        return await MediaCompressor.compressImage(file, {
          quality: 85,
          maxWidth: 1920,
          maxHeight: 1080,
        });

      case 'video':
      case 'audio':
        // For video/audio, we need to write to temp file first
        const tempPath = `/tmp/${Date.now()}_${originalName}`;
        require('fs').writeFileSync(tempPath, file);
        
        try {
          if (type === 'video') {
            const result = await MediaCompressor.compressVideo(tempPath, {
              quality: 'medium',
              maxDuration: 300,
            });
            const compressedBuffer = require('fs').readFileSync(result.outputPath);
            await MediaCompressor.cleanupTempFiles([tempPath, result.outputPath]);
            return {
              buffer: compressedBuffer,
              metadata: result.metadata,
            };
          } else {
            const result = await MediaCompressor.compressAudio(tempPath, {
              bitrate: '128k',
              maxDuration: 600,
            });
            const compressedBuffer = require('fs').readFileSync(result.outputPath);
            await MediaCompressor.cleanupTempFiles([tempPath, result.outputPath]);
            return {
              buffer: compressedBuffer,
              metadata: result.metadata,
            };
          }
        } catch (error) {
          // Cleanup on error
          try {
            require('fs').unlinkSync(tempPath);
          } catch {}
          throw error;
        }

      default:
        return { buffer: file, metadata: {} };
    }
  }

  // Generate and upload thumbnail
  private async generateAndUploadThumbnail(
    file: Buffer,
    originalName: string,
    type: 'image' | 'video',
    uploadedBy: string
  ): Promise<string> {
    if (type === 'image') {
      const thumbnail = await ThumbnailGenerator.generateImageThumbnail(file, {
        width: 300,
        height: 300,
        quality: 80,
      });

      const uploadResult = await s3Service.uploadFile(
        thumbnail.buffer,
        `thumb_${originalName}`,
        uploadedBy,
        {
          contentType: 'image/jpeg',
        },
        'thumbnail'
      );

      return uploadResult.url;
    } else if (type === 'video') {
      // For video, write to temp file first
      const tempPath = `/tmp/${Date.now()}_${originalName}`;
      require('fs').writeFileSync(tempPath, file);

      try {
        const thumbnail = await ThumbnailGenerator.generateVideoThumbnail(tempPath, {
          width: 300,
          height: 300,
        });

        const thumbnailBuffer = require('fs').readFileSync(thumbnail.thumbnailPath);
        
        const uploadResult = await s3Service.uploadFile(
          thumbnailBuffer,
          `thumb_${originalName}.jpg`,
          uploadedBy,
          {
            contentType: 'image/jpeg',
          },
          'thumbnail'
        );

        await ThumbnailGenerator.cleanupThumbnails([tempPath, thumbnail.thumbnailPath]);
        return uploadResult.url;
      } catch (error) {
        try {
          require('fs').unlinkSync(tempPath);
        } catch {}
        throw error;
      }
    }

    throw new Error('Unsupported file type for thumbnail generation');
  }

  // Download file
  async downloadFile(mediaId: string, userId: string): Promise<{ stream: ReadableStream; media: IMedia }> {
    const media = await this.mediaRepository.findById(mediaId);
    
    if (!media) {
      throw new Error('Media file not found');
    }

    // TODO: Add permission check based on chat membership
    
    const stream = await s3Service.getFileStream(media.filename);
    
    if (!stream) {
      throw new Error('Failed to get file stream');
    }

    return { stream, media };
  }

  // Delete file
  async deleteFile(mediaId: string, userId: string): Promise<boolean> {
    const media = await this.mediaRepository.findById(mediaId);
    
    if (!media) {
      throw new Error('Media file not found');
    }

    // Check ownership or admin permissions
    if (media.uploadedBy.toString() !== userId) {
      throw new Error('Not authorized to delete this file');
    }

    // Delete from S3
    const s3Deleted = await s3Service.deleteFile(media.filename);
    
    // Delete thumbnail if exists
    if (media.thumbnailUrl) {
      const thumbnailKey = media.filename.replace('media/', 'thumbnail/');
      await s3Service.deleteFile(thumbnailKey);
    }

    // Delete from database
    const dbDeleted = await this.mediaRepository.delete(mediaId);

    return s3Deleted && dbDeleted;
  }

  // Get file URL with expiration
  async getFileUrl(mediaId: string, expiresIn: number = 3600): Promise<string> {
    const media = await this.mediaRepository.findById(mediaId);
    
    if (!media) {
      throw new Error('Media file not found');
    }

    return await s3Service.getFileUrl(media.filename, expiresIn);
  }

  // Batch upload files
  async uploadFiles(
    files: Array<{
      buffer: Buffer;
      originalName: string;
      mimeType: string;
      type: 'image' | 'video' | 'audio' | 'voice' | 'document';
    }>,
    uploadedBy: string,
    options: UploadFileOptions = {}
  ): Promise<UploadResult[]> {
    const uploadPromises = files.map(file =>
      this.uploadFile(
        file.buffer,
        file.originalName,
        file.mimeType,
        uploadedBy,
        file.type,
        options
      )
    );

    return await Promise.all(uploadPromises);
  }
}

export const mediaUploadService = new MediaUploadService();
