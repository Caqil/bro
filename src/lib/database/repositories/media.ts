import { Types } from 'mongoose';
import { Media, IMedia } from '../models/media';

export class MediaRepository {
  // Create media record
  async create(mediaData: Partial<IMedia>): Promise<IMedia> {
    const media = new Media(mediaData);
    return await media.save();
  }

  // Find media by ID
  async findById(id: string | Types.ObjectId): Promise<IMedia | null> {
    return await Media.findById(id).exec();
  }

  // Get chat media
  async getChatMedia(
    chatId: string | Types.ObjectId,
    type?: string,
    limit: number = 20,
    offset: number = 0
  ): Promise<IMedia[]> {
    const query: any = { chatId };
    if (type) {
      query.type = type;
    }

    return await Media.find(query)
      .sort({ createdAt: -1 })
      .limit(limit)
      .skip(offset)
      .exec();
  }

  // Get user media
  async getUserMedia(
    userId: string | Types.ObjectId,
    type?: string,
    limit: number = 20,
    offset: number = 0
  ): Promise<IMedia[]> {
    const query: any = { uploadedBy: userId };
    if (type) {
      query.type = type;
    }

    return await Media.find(query)
      .sort({ createdAt: -1 })
      .limit(limit)
      .skip(offset)
      .exec();
  }

  // Delete media
  async delete(id: string | Types.ObjectId): Promise<boolean> {
    const result = await Media.findByIdAndDelete(id).exec();
    return !!result;
  }

  // Get storage statistics
  async getStorageStats(): Promise<any> {
    return await Media.aggregate([
      {
        $group: {
          _id: '$type',
          totalSize: { $sum: '$size' },
          count: { $sum: 1 }
        }
      }
    ]).exec();
  }

  // Get unused media (not linked to any message)
  async getUnusedMedia(limit: number = 100): Promise<IMedia[]> {
    return await Media.find({
      messageId: { $exists: false }
    })
    .limit(limit)
    .exec();
  }

  // Cleanup unused media
  async cleanupUnused(): Promise<number> {
    const result = await Media.deleteMany({
      messageId: { $exists: false },
      createdAt: { $lt: new Date(Date.now() - 24 * 60 * 60 * 1000) } // Older than 24 hours
    }).exec();
    return result.deletedCount;
  }

  // Get media by checksum (for deduplication)
  async findByChecksum(checksum: string): Promise<IMedia | null> {
    return await Media.findOne({ checksumSHA256: checksum }).exec();
  }
}
