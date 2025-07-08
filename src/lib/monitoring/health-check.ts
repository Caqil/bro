import { logger } from './logging';
import connectDB from '../database/mongodb';
import { User } from '../database/models/user';

interface HealthStatus {
  status: 'healthy' | 'degraded' | 'unhealthy';
  timestamp: Date;
  uptime: number;
  version: string;
  checks: HealthCheck[];
}

interface HealthCheck {
  name: string;
  status: 'pass' | 'fail' | 'warn';
  duration: number;
  message?: string;
  details?: any;
}

export class HealthMonitor {
  private checks: Map<string, () => Promise<HealthCheck>> = new Map();
  private cachedStatus?: HealthStatus;
  private cacheExpiry?: Date;
  private cacheTimeout = 30000; // 30 seconds

  constructor() {
    this.registerDefaultChecks();
  }

  // Register a health check
  registerCheck(name: string, checkFn: () => Promise<HealthCheck>): void {
    this.checks.set(name, checkFn);
  }

  // Get current health status
  async getHealthStatus(): Promise<HealthStatus> {
    // Return cached status if still valid
    if (this.cachedStatus && this.cacheExpiry && new Date() < this.cacheExpiry) {
      return this.cachedStatus;
    }

    const checks: HealthCheck[] = [];
    const startTime = Date.now();

    // Run all health checks
    for (const [name, checkFn] of this.checks) {
      try {
        const checkStartTime = Date.now();
        const result = await Promise.race([
          checkFn(),
          this.timeoutPromise(10000), // 10 second timeout
        ]);
        
        checks.push({
          ...result,
          duration: Date.now() - checkStartTime,
        });
      } catch (error) {
        checks.push({
          name,
          status: 'fail',
          duration: Date.now() - startTime,
          message: error instanceof Error ? error.message : 'Unknown error',
        });
      }
    }

    // Determine overall status
    const overallStatus = this.determineOverallStatus(checks);

    const healthStatus: HealthStatus = {
      status: overallStatus,
      timestamp: new Date(),
      uptime: process.uptime(),
      version: process.env.APP_VERSION || '1.0.0',
      checks,
    };

    // Cache the result
    this.cachedStatus = healthStatus;
    this.cacheExpiry = new Date(Date.now() + this.cacheTimeout);

    return healthStatus;
  }

  // Get simplified health check (for load balancers)
  async getSimpleHealthCheck(): Promise<{ status: string; timestamp: Date }> {
    const health = await this.getHealthStatus();
    return {
      status: health.status,
      timestamp: health.timestamp,
    };
  }

  // Register default health checks
  private registerDefaultChecks(): void {
    // Database connectivity check
    this.registerCheck('database', async (): Promise<HealthCheck> => {
      try {
        await connectDB();
        await User.findOne().limit(1).lean().exec();
        
        return {
          name: 'database',
          status: 'pass',
          duration: 0,
          message: 'Database connection successful',
        };
      } catch (error) {
        return {
          name: 'database',
          status: 'fail',
          duration: 0,
          message: `Database connection failed: ${error instanceof Error ? error.message : 'Unknown error'}`,
        };
      }
    });

    // Memory usage check
    this.registerCheck('memory', async (): Promise<HealthCheck> => {
      const memUsage = process.memoryUsage();
      const heapUsedMB = memUsage.heapUsed / 1024 / 1024;
      const heapTotalMB = memUsage.heapTotal / 1024 / 1024;
      const usagePercentage = (heapUsedMB / heapTotalMB) * 100;

      let status: 'pass' | 'warn' | 'fail' = 'pass';
      let message = `Memory usage: ${heapUsedMB.toFixed(2)}MB / ${heapTotalMB.toFixed(2)}MB (${usagePercentage.toFixed(1)}%)`;

      if (usagePercentage > 90) {
        status = 'fail';
        message += ' - Critical memory usage';
      } else if (usagePercentage > 80) {
        status = 'warn';
        message += ' - High memory usage';
      }

      return {
        name: 'memory',
        status,
        duration: 0,
        message,
        details: {
          heapUsed: heapUsedMB,
          heapTotal: heapTotalMB,
          usagePercentage,
        },
      };
    });

    // Disk space check (if applicable)
    this.registerCheck('disk', async (): Promise<HealthCheck> => {
      try {
        const fs = require('fs').promises;
        const stats = await fs.statfs('./');
        const freeSpace = stats.bavail * stats.bsize;
        const totalSpace = stats.blocks * stats.bsize;
        const usedPercentage = ((totalSpace - freeSpace) / totalSpace) * 100;

        let status: 'pass' | 'warn' | 'fail' = 'pass';
        let message = `Disk usage: ${usedPercentage.toFixed(1)}%`;

        if (usedPercentage > 95) {
          status = 'fail';
          message += ' - Critical disk usage';
        } else if (usedPercentage > 85) {
          status = 'warn';
          message += ' - High disk usage';
        }

        return {
          name: 'disk',
          status,
          duration: 0,
          message,
          details: {
            freeSpace: Math.round(freeSpace / 1024 / 1024 / 1024), // GB
            totalSpace: Math.round(totalSpace / 1024 / 1024 / 1024), // GB
            usedPercentage,
          },
        };
      } catch (error) {
        return {
          name: 'disk',
          status: 'warn',
          duration: 0,
          message: 'Unable to check disk space',
        };
      }
    });

    // Redis connectivity check (if Redis is configured)
    if (process.env.REDIS_URL) {
      this.registerCheck('redis', async (): Promise<HealthCheck> => {
        try {
          // Import Redis client and test connection
          const Redis = require('ioredis');
          const redis = new Redis(process.env.REDIS_URL);
          
          await redis.ping();
          await redis.disconnect();

          return {
            name: 'redis',
            status: 'pass',
            duration: 0,
            message: 'Redis connection successful',
          };
        } catch (error) {
          return {
            name: 'redis',
            status: 'fail',
            duration: 0,
            message: `Redis connection failed: ${error instanceof Error ? error.message : 'Unknown error'}`,
          };
        }
      });
    }

    // External service checks
    this.registerCheck('external_services', async (): Promise<HealthCheck> => {
      const services: Array<{ name: string; status: 'pass' | 'fail'; error?: string }> = [];
      
      // Check SMTP if configured
      if (process.env.SMTP_HOST) {
        try {
          const nodemailer = require('nodemailer');
          const transporter = nodemailer.createTransporter({
            host: process.env.SMTP_HOST,
            port: parseInt(process.env.SMTP_PORT || '587'),
            secure: process.env.SMTP_SECURE === 'true',
            auth: {
              user: process.env.SMTP_USER,
              pass: process.env.SMTP_PASS,
            },
          });
          
          await transporter.verify();
          services.push({ name: 'SMTP', status: 'pass' });
        } catch (error) {
          services.push({ name: 'SMTP', status: 'fail', error: error instanceof Error ? error.message : 'Unknown' });
        }
      }

      // Check S3 if configured
      if (process.env.AWS_ACCESS_KEY_ID) {
        try {
          const { S3Client, HeadBucketCommand } = require('@aws-sdk/client-s3');
          const s3 = new S3Client({
            region: process.env.AWS_REGION,
            credentials: {
              accessKeyId: process.env.AWS_ACCESS_KEY_ID,
              secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY,
            },
          });
          
          await s3.send(new HeadBucketCommand({ Bucket: process.env.AWS_S3_BUCKET }));
          services.push({ name: 'S3', status: 'pass' });
        } catch (error) {
          services.push({ name: 'S3', status: 'fail', error: error instanceof Error ? error.message : 'Unknown' });
        }
      }

      const failedServices = services.filter(s => s.status === 'fail');
      const status = failedServices.length === 0 ? 'pass' : failedServices.length === services.length ? 'fail' : 'warn';

      return {
        name: 'external_services',
        status,
        duration: 0,
        message: `External services check: ${services.length - failedServices.length}/${services.length} healthy`,
        details: services,
      };
    });
  }

  private determineOverallStatus(checks: HealthCheck[]): 'healthy' | 'degraded' | 'unhealthy' {
    const failedChecks = checks.filter(check => check.status === 'fail');
    const warnChecks = checks.filter(check => check.status === 'warn');

    if (failedChecks.length > 0) {
      // If critical checks fail, mark as unhealthy
      const criticalChecks = ['database'];
      const criticalFailures = failedChecks.filter(check => criticalChecks.includes(check.name));
      
      if (criticalFailures.length > 0) {
        return 'unhealthy';
      }
      
      return 'degraded';
    }

    if (warnChecks.length > 0) {
      return 'degraded';
    }

    return 'healthy';
  }

  private timeoutPromise(timeout: number): Promise<never> {
    return new Promise((_, reject) => {
      setTimeout(() => reject(new Error('Health check timeout')), timeout);
    });
  }
}

export const healthMonitor = new HealthMonitor();
