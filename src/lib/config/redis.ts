import Redis from 'ioredis';
import { environmentConfig } from './environment';
import { logger } from '../monitoring/logging';

export interface RedisConfig {
  url?: string;
  host?: string;
  port?: number;
  password?: string;
  db?: number;
  maxRetriesPerRequest: number;
  retryDelayOnFailover: number;
  enableReadyCheck: boolean;
  lazyConnect: boolean;
  keepAlive: number;
  connectTimeout: number;
  commandTimeout: number;
}

class RedisConfiguration {
  private config: RedisConfig | null;
  private client: Redis | null = null;

  constructor() {
    this.config = this.createConfig();
  }

  private createConfig(): RedisConfig | null {
    const env = environmentConfig.get();
    
    if (!env.REDIS_URL) {
      logger.warn('Redis URL not configured, Redis features will be disabled');
      return null;
    }

    return {
      url: env.REDIS_URL,
      maxRetriesPerRequest: 3,
      retryDelayOnFailover: 100,
      enableReadyCheck: false,
      lazyConnect: true,
      keepAlive: 30000,
      connectTimeout: 10000,
      commandTimeout: 5000,
    };
  }

  getConfig(): RedisConfig | null {
    return this.config;
  }

  // Get Redis client instance
  getClient(): Redis | null {
    if (!this.config) {
      return null;
    }

    if (!this.client) {
      this.client = new Redis(this.config);
      
      this.client.on('connect', () => {
        logger.info('Redis connected successfully');
      });

      this.client.on('error', (error) => {
        logger.error('Redis connection error', error);
      });

      this.client.on('close', () => {
        logger.warn('Redis connection closed');
      });

      this.client.on('reconnecting', () => {
        logger.info('Redis reconnecting...');
      });
    }

    return this.client;
  }

  // Test Redis connection
  async testConnection(): Promise<boolean> {
    const client = this.getClient();
    
    if (!client) {
      return false;
    }

    try {
      await client.ping();
      return true;
    } catch (error) {
      logger.error(
        'Redis connection test failed',
        error instanceof Error ? error : new Error(String(error))
      );
      return false;
    }
  }

  // Get Redis info
  async getInfo(): Promise<any> {
    const client = this.getClient();
    
    if (!client) {
      return null;
    }

    try {
      const info = await client.info();
      return this.parseRedisInfo(info);
    } catch (error) {
      logger.error(
        'Failed to get Redis info',
        error instanceof Error ? error : new Error(String(error))
      );
      return null;
    }
  }

  // Close Redis connection
  async close(): Promise<void> {
    if (this.client) {
      await this.client.quit();
      this.client = null;
    }
  }

  private parseRedisInfo(info: string): any {
    const parsed: any = {};
    const sections = info.split('\r\n\r\n');
    
    sections.forEach(section => {
      const lines = section.split('\r\n');
      const sectionName = lines[0].replace('# ', '');
      parsed[sectionName] = {};
      
      lines.slice(1).forEach(line => {
        if (line && !line.startsWith('#')) {
          const [key, value] = line.split(':');
          if (key && value) {
            parsed[sectionName][key] = value;
          }
        }
      });
    });
    
    return parsed;
  }
}

export const redisConfig = new RedisConfiguration();
