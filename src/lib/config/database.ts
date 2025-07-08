import { logger } from '../monitoring/logging';
import { environmentConfig } from './environment';

export interface DatabaseConfig {
  uri: string;
  options: {
    maxPoolSize: number;
    serverSelectionTimeoutMS: number;
    socketTimeoutMS: number;
    bufferCommands: boolean;
    bufferMaxEntries: number;
    retryWrites: boolean;
    w: string | number;
    readPreference: string;
  };
}

class DatabaseConfiguration {
  private config: DatabaseConfig;

  constructor() {
    this.config = this.createConfig();
  }

  private createConfig(): DatabaseConfig {
    const env = environmentConfig.get();
    
    return {
      uri: env.MONGODB_URI,
      options: {
        maxPoolSize: env.NODE_ENV === 'production' ? 20 : 10,
        serverSelectionTimeoutMS: 5000,
        socketTimeoutMS: 45000,
        bufferCommands: false,
        bufferMaxEntries: 0,
        retryWrites: true,
        w: 'majority',
        readPreference: 'primary',
      },
    };
  }

  getConfig(): DatabaseConfig {
    return this.config;
  }

  // Get connection string with options
  getConnectionString(): string {
    const { uri, options } = this.config;
    const params = new URLSearchParams();
    
    Object.entries(options).forEach(([key, value]) => {
      if (value !== undefined && value !== null) {
        params.append(key, value.toString());
      }
    });

    return uri.includes('?') 
      ? `${uri}&${params.toString()}`
      : `${uri}?${params.toString()}`;
  }

  // Database connection health check
  async testConnection(): Promise<boolean> {
    try {
      const { default: connectDB } = await import('../database/mongodb');
      const mongoose = await connectDB();
      
      // Test with a simple operation
      if (!mongoose.connection.db) {
        throw new Error('Database connection is not established.');
      }
      await mongoose.connection.db.admin().ping();
      return true;
    } catch (error) {
      logger.error(
        'Database connection test failed',
        error instanceof Error ? error : new Error(String(error))
      );
      return false;
    }
  }

  // Get database connection statistics
  getConnectionStats() {
    const mongoose = require('mongoose');
    
    if (mongoose.connection.readyState === 1) {
      return {
        state: 'connected',
        host: mongoose.connection.host,
        port: mongoose.connection.port,
        name: mongoose.connection.name,
        collections: Object.keys(mongoose.connection.collections),
      };
    }
    
    return {
      state: 'disconnected',
    };
  }
}

export const databaseConfig = new DatabaseConfiguration();
