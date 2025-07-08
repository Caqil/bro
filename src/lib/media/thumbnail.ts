import sharp from 'sharp';
import ffmpeg from 'fluent-ffmpeg';
import path from 'path';
import os from 'os';
import { MediaCompressor } from './compression';

interface ThumbnailOptions {
  width?: number;
  height?: number;
  quality?: number;
  format?: 'jpeg' | 'png' | 'webp';
}

export class ThumbnailGenerator {
  private static tempDir = os.tmpdir();

  // Generate image thumbnail
  static async generateImageThumbnail(
    buffer: Buffer,
    options: ThumbnailOptions = {}
  ): Promise<{ buffer: Buffer; metadata: any }> {
    try {
      const {
        width = 300,
        height = 300,
        quality = 80,
        format = 'jpeg'
      } = options;

      const originalMetadata = await sharp(buffer).metadata();
      
      let thumbnailBuffer: Buffer;
      
      if (format === 'jpeg') {
        thumbnailBuffer = await sharp(buffer)
          .resize(width, height, { fit: 'inside', withoutEnlargement: true })
          .jpeg({ quality })
          .toBuffer();
      } else if (format === 'png') {
        thumbnailBuffer = await sharp(buffer)
          .resize(width, height, { fit: 'inside', withoutEnlargement: true })
          .png({ quality })
          .toBuffer();
      } else {
        thumbnailBuffer = await sharp(buffer)
          .resize(width, height, { fit: 'inside', withoutEnlargement: true })
          .webp({ quality })
          .toBuffer();
      }

      const thumbnailMetadata = await sharp(thumbnailBuffer).metadata();

      return {
        buffer: thumbnailBuffer,
        metadata: {
          originalWidth: originalMetadata.width,
          originalHeight: originalMetadata.height,
          thumbnailWidth: thumbnailMetadata.width,
          thumbnailHeight: thumbnailMetadata.height,
          format: thumbnailMetadata.format,
          size: thumbnailBuffer.length,
        },
      };
    } catch (error) {
      console.error('Image thumbnail generation error:', error);
      throw new Error('Failed to generate image thumbnail');
    }
  }

  // Generate video thumbnail
  static async generateVideoThumbnail(
    videoPath: string,
    options: ThumbnailOptions = {}
  ): Promise<{ thumbnailPath: string; metadata: any }> {
    return new Promise((resolve, reject) => {
      try {
        const {
          width = 300,
          height = 300,
          quality = 80
        } = options;

        const thumbnailPath = path.join(
          this.tempDir,
          `thumb_${Date.now()}_${Math.random().toString(36).substr(2, 9)}.jpg`
        );

        ffmpeg(videoPath)
          .on('end', async () => {
            try {
              // Resize the generated thumbnail
              const thumbnailBuffer = await sharp(thumbnailPath)
                .resize(width, height, { fit: 'inside', withoutEnlargement: true })
                .jpeg({ quality })
                .toBuffer();

              // Save resized thumbnail
              await sharp(thumbnailBuffer).toFile(thumbnailPath);

              const metadata = await sharp(thumbnailPath).metadata();

              resolve({
                thumbnailPath,
                metadata: {
                  width: metadata.width,
                  height: metadata.height,
                  format: metadata.format,
                  size: metadata.size,
                },
              });
            } catch (error) {
              reject(error);
            }
          })
          .on('error', (error) => {
            console.error('Video thumbnail error:', error);
            reject(new Error('Failed to generate video thumbnail'));
          })
          .screenshots({
            timestamps: ['10%'], // Take screenshot at 10% of video duration
            filename: path.basename(thumbnailPath),
            folder: path.dirname(thumbnailPath),
            size: `${width}x${height}`,
          });

      } catch (error) {
        reject(error);
      }
    });
  }

  // Generate multiple thumbnails for video (for preview)
  static async generateVideoThumbnails(
    videoPath: string,
    count: number = 5,
    options: ThumbnailOptions = {}
  ): Promise<{ thumbnailPaths: string[]; metadata: any }> {
    return new Promise((resolve, reject) => {
      try {
        const {
          width = 150,
          height = 100,
        } = options;

        const timestamps: string[] = [];
        for (let i = 1; i <= count; i++) {
          timestamps.push(`${(i * 100 / (count + 1)).toFixed(0)}%`);
        }

        const thumbnailFolder = path.join(this.tempDir, `thumbs_${Date.now()}`);
        
        ffmpeg(videoPath)
          .on('end', () => {
            const thumbnailPaths = timestamps.map((_, index) => 
              path.join(thumbnailFolder, `thumb_${index + 1}.jpg`)
            );

            resolve({
              thumbnailPaths,
              metadata: {
                count: thumbnailPaths.length,
                width,
                height,
              },
            });
          })
          .on('error', (error) => {
            console.error('Multiple video thumbnails error:', error);
            reject(new Error('Failed to generate video thumbnails'));
          })
          .screenshots({
            timestamps,
            filename: 'thumb_%i.jpg',
            folder: thumbnailFolder,
            size: `${width}x${height}`,
          });

      } catch (error) {
        reject(error);
      }
    });
  }

  // Generate audio waveform thumbnail
  static async generateAudioWaveform(
    audioPath: string,
    options: { width?: number; height?: number; color?: string } = {}
  ): Promise<{ waveformPath: string; metadata: any }> {
    return new Promise((resolve, reject) => {
      try {
        const {
          width = 800,
          height = 100,
          color = '#007bff'
        } = options;

        const waveformPath = path.join(
          this.tempDir,
          `waveform_${Date.now()}_${Math.random().toString(36).substr(2, 9)}.png`
        );

        // Generate waveform using ffmpeg
        ffmpeg(audioPath)
          .complexFilter([
            `[0:a]showwavespic=s=${width}x${height}:colors=${color}[v]`
          ])
          .map('[v]')
          .format('image2')
          .on('end', () => {
            resolve({
              waveformPath,
              metadata: {
                width,
                height,
                color,
              },
            });
          })
          .on('error', (error) => {
            console.error('Audio waveform error:', error);
            reject(new Error('Failed to generate audio waveform'));
          })
          .save(waveformPath);

      } catch (error) {
        reject(error);
      }
    });
  }

  // Clean up thumbnail files
  static async cleanupThumbnails(thumbnailPaths: string[]): Promise<void> {
    await MediaCompressor.cleanupTempFiles(thumbnailPaths);
  }
}