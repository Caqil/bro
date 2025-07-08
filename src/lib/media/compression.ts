import sharp from 'sharp';
import ffmpeg from 'fluent-ffmpeg';
import { promisify } from 'util';
import fs from 'fs';
import path from 'path';
import os from 'os';

interface CompressionOptions {
  quality?: number;
  maxWidth?: number;
  maxHeight?: number;
  format?: string;
}

interface VideoCompressionOptions {
  quality?: 'low' | 'medium' | 'high';
  maxDuration?: number; // in seconds
  maxSize?: number; // in MB
}

interface AudioCompressionOptions {
  bitrate?: string; // e.g., '128k', '256k'
  maxDuration?: number; // in seconds
}

export class MediaCompressor {
  private static tempDir = os.tmpdir();

  // Compress image using Sharp
  static async compressImage(
    buffer: Buffer,
    options: CompressionOptions = {}
  ): Promise<{ buffer: Buffer; metadata: any }> {
    try {
      const {
        quality = 85,
        maxWidth = 1920,
        maxHeight = 1080,
        format = 'jpeg'
      } = options;

      let sharpInstance = sharp(buffer);
      
      // Get original metadata
      const originalMetadata = await sharpInstance.metadata();
      
      // Resize if needed
      if (originalMetadata.width && originalMetadata.height) {
        if (originalMetadata.width > maxWidth || originalMetadata.height > maxHeight) {
          sharpInstance = sharpInstance.resize(maxWidth, maxHeight, {
            fit: 'inside',
            withoutEnlargement: true,
          });
        }
      }

      // Apply compression based on format
      if (format === 'jpeg') {
        sharpInstance = sharpInstance.jpeg({ quality, progressive: true });
      } else if (format === 'png') {
        sharpInstance = sharpInstance.png({ quality, progressive: true });
      } else if (format === 'webp') {
        sharpInstance = sharpInstance.webp({ quality });
      }

      const compressedBuffer = await sharpInstance.toBuffer();
      const newMetadata = await sharp(compressedBuffer).metadata();

      return {
        buffer: compressedBuffer,
        metadata: {
          originalSize: buffer.length,
          compressedSize: compressedBuffer.length,
          compressionRatio: (1 - compressedBuffer.length / buffer.length) * 100,
          width: newMetadata.width,
          height: newMetadata.height,
          format: newMetadata.format,
        },
      };
    } catch (error) {
      console.error('Image compression error:', error);
      throw new Error('Failed to compress image');
    }
  }

  // Compress video using FFmpeg
  static async compressVideo(
    inputPath: string,
    options: VideoCompressionOptions = {}
  ): Promise<{ outputPath: string; metadata: any }> {
    return new Promise((resolve, reject) => {
      try {
        const {
          quality = 'medium',
          maxDuration = 300, // 5 minutes
          maxSize = 50 // 50MB
        } = options;

        const outputPath = path.join(
          this.tempDir,
          `compressed_${Date.now()}_${Math.random().toString(36).substr(2, 9)}.mp4`
        );

        // Quality settings
        const qualitySettings = {
          low: { crf: 28, preset: 'fast' },
          medium: { crf: 23, preset: 'medium' },
          high: { crf: 18, preset: 'slow' },
        };

        const settings = qualitySettings[quality];

        let ffmpegCommand = ffmpeg(inputPath)
          .videoCodec('libx264')
          .audioCodec('aac')
          .format('mp4')
          .addOptions([
            `-crf ${settings.crf}`,
            `-preset ${settings.preset}`,
            '-movflags +faststart', // Optimize for web streaming
          ]);

        // Limit duration if specified
        if (maxDuration) {
          ffmpegCommand = ffmpegCommand.duration(maxDuration);
        }

        ffmpegCommand
          .on('start', (commandLine) => {
            console.log('FFmpeg started:', commandLine);
          })
          .on('progress', (progress) => {
            console.log('Processing: ' + progress.percent + '% done');
          })
          .on('end', async () => {
            try {
              const stats = await fs.promises.stat(outputPath);
              const inputStats = await fs.promises.stat(inputPath);
              
              resolve({
                outputPath,
                metadata: {
                  originalSize: inputStats.size,
                  compressedSize: stats.size,
                  compressionRatio: (1 - stats.size / inputStats.size) * 100,
                },
              });
            } catch (error) {
              reject(error);
            }
          })
          .on('error', (error) => {
            console.error('FFmpeg error:', error);
            reject(new Error('Video compression failed'));
          })
          .save(outputPath);

      } catch (error) {
        reject(error);
      }
    });
  }

  // Compress audio using FFmpeg
  static async compressAudio(
    inputPath: string,
    options: AudioCompressionOptions = {}
  ): Promise<{ outputPath: string; metadata: any }> {
    return new Promise((resolve, reject) => {
      try {
        const {
          bitrate = '128k',
          maxDuration = 600 // 10 minutes
        } = options;

        const outputPath = path.join(
          this.tempDir,
          `compressed_${Date.now()}_${Math.random().toString(36).substr(2, 9)}.mp3`
        );

        let ffmpegCommand = ffmpeg(inputPath)
          .audioCodec('libmp3lame')
          .audioBitrate(bitrate)
          .format('mp3');

        // Limit duration if specified
        if (maxDuration) {
          ffmpegCommand = ffmpegCommand.duration(maxDuration);
        }

        ffmpegCommand
          .on('end', async () => {
            try {
              const stats = await fs.promises.stat(outputPath);
              const inputStats = await fs.promises.stat(inputPath);
              
              resolve({
                outputPath,
                metadata: {
                  originalSize: inputStats.size,
                  compressedSize: stats.size,
                  compressionRatio: (1 - stats.size / inputStats.size) * 100,
                  bitrate,
                },
              });
            } catch (error) {
              reject(error);
            }
          })
          .on('error', (error) => {
            console.error('Audio compression error:', error);
            reject(new Error('Audio compression failed'));
          })
          .save(outputPath);

      } catch (error) {
        reject(error);
      }
    });
  }

  // Get media metadata
  static async getMediaMetadata(filePath: string): Promise<any> {
    return new Promise((resolve, reject) => {
      ffmpeg.ffprobe(filePath, (error, metadata) => {
        if (error) {
          reject(error);
        } else {
          const videoStream = metadata.streams.find(s => s.codec_type === 'video');
          const audioStream = metadata.streams.find(s => s.codec_type === 'audio');
          
          resolve({
            duration: metadata.format.duration,
            size: metadata.format.size,
            bitRate: metadata.format.bit_rate,
            video: videoStream ? {
              width: videoStream.width,
              height: videoStream.height,
              frameRate: videoStream.avg_frame_rate,
              codec: videoStream.codec_name,
            } : null,
            audio: audioStream ? {
              codec: audioStream.codec_name,
              bitRate: audioStream.bit_rate,
              sampleRate: audioStream.sample_rate,
              channels: audioStream.channels,
            } : null,
          });
        }
      });
    });
  }

  // Clean up temporary files
  static async cleanupTempFiles(filePaths: string[]): Promise<void> {
    const unlinkPromises = filePaths.map(async (filePath) => {
      try {
        await fs.promises.unlink(filePath);
      } catch (error) {
        console.warn(`Failed to delete temp file: ${filePath}`, error);
      }
    });

    await Promise.allSettled(unlinkPromises);
  }
}