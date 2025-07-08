import { EventEmitter } from 'events';
import { performance } from 'perf_hooks';

interface Metric {
  name: string;
  value: number;
  timestamp: Date;
  tags?: Record<string, string>;
}

interface Timer {
  start: number;
  name: string;
  tags?: Record<string, string>;
}

interface PerformanceMetrics {
  responseTime: {
    avg: number;
    p50: number;
    p95: number;
    p99: number;
  };
  throughput: {
    requestsPerSecond: number;
    requestsPerMinute: number;
  };
  errors: {
    rate: number;
    count: number;
  };
  system: {
    memoryUsage: number;
    cpuUsage: number;
    uptime: number;
  };
}

export class MetricsCollector extends EventEmitter {
  private metrics: Metric[] = [];
  private timers = new Map<string, Timer>();
  private counters = new Map<string, number>();
  private gauges = new Map<string, number>();
  private histograms = new Map<string, number[]>();

  constructor() {
    super();
    this.startSystemMetricsCollection();
  }

  // Start a timer
  startTimer(name: string, tags?: Record<string, string>): string {
    const timerId = `${name}-${Date.now()}-${Math.random()}`;
    this.timers.set(timerId, {
      start: performance.now(),
      name,
      tags,
    });
    return timerId;
  }

  // End a timer and record the duration
  endTimer(timerId: string): number {
    const timer = this.timers.get(timerId);
    if (!timer) {
      throw new Error(`Timer ${timerId} not found`);
    }

    const duration = performance.now() - timer.start;
    this.timers.delete(timerId);

    this.recordHistogram(timer.name, duration, timer.tags);
    return duration;
  }

  // Record a counter metric
  incrementCounter(name: string, value: number = 1, tags?: Record<string, string>): void {
    const key = this.getMetricKey(name, tags);
    const currentValue = this.counters.get(key) || 0;
    this.counters.set(key, currentValue + value);

    this.recordMetric({
      name,
      value: currentValue + value,
      timestamp: new Date(),
      tags,
    });
  }

  // Record a gauge metric
  recordGauge(name: string, value: number, tags?: Record<string, string>): void {
    const key = this.getMetricKey(name, tags);
    this.gauges.set(key, value);

    this.recordMetric({
      name,
      value,
      timestamp: new Date(),
      tags,
    });
  }

  // Record a histogram metric
  recordHistogram(name: string, value: number, tags?: Record<string, string>): void {
    const key = this.getMetricKey(name, tags);
    const values = this.histograms.get(key) || [];
    values.push(value);
    
    // Keep only recent values (last 1000)
    if (values.length > 1000) {
      values.splice(0, values.length - 1000);
    }
    
    this.histograms.set(key, values);

    this.recordMetric({
      name,
      value,
      timestamp: new Date(),
      tags,
    });
  }

  // Time a function execution
  async timeFunction<T>(name: string, fn: () => Promise<T>, tags?: Record<string, string>): Promise<T> {
    const timerId = this.startTimer(name, tags);
    try {
      const result = await fn();
      this.endTimer(timerId);
      return result;
    } catch (error) {
      this.endTimer(timerId);
      this.incrementCounter(`${name}_errors`, 1, tags);
      throw error;
    }
  }

  // Get performance metrics
  getPerformanceMetrics(): PerformanceMetrics {
    const responseTimes = this.histograms.get('response_time') || [];
    const sortedResponseTimes = [...responseTimes].sort((a, b) => a - b);
    
    const requestsPerSecond = this.counters.get('http_requests') || 0;
    const errors = this.counters.get('http_errors') || 0;
    
    return {
      responseTime: {
        avg: this.calculateAverage(responseTimes),
        p50: this.calculatePercentile(sortedResponseTimes, 50),
        p95: this.calculatePercentile(sortedResponseTimes, 95),
        p99: this.calculatePercentile(sortedResponseTimes, 99),
      },
      throughput: {
        requestsPerSecond: requestsPerSecond / 60, // Approximate
        requestsPerMinute: requestsPerSecond,
      },
      errors: {
        rate: requestsPerSecond > 0 ? (errors / requestsPerSecond) * 100 : 0,
        count: errors,
      },
      system: {
        memoryUsage: this.gauges.get('memory_usage') || 0,
        cpuUsage: this.gauges.get('cpu_usage') || 0,
        uptime: process.uptime(),
      },
    };
  }

  // Get all metrics for a time range
  getMetrics(timeRange?: { start: Date; end: Date }): Metric[] {
    if (!timeRange) {
      return [...this.metrics];
    }

    return this.metrics.filter(metric => 
      metric.timestamp >= timeRange.start && metric.timestamp <= timeRange.end
    );
  }

  // Get metric summary
  getMetricSummary(metricName: string): {
    count: number;
    sum: number;
    avg: number;
    min: number;
    max: number;
  } | null {
    const metricValues = this.metrics
      .filter(m => m.name === metricName)
      .map(m => m.value);

    if (metricValues.length === 0) {
      return null;
    }

    return {
      count: metricValues.length,
      sum: metricValues.reduce((sum, val) => sum + val, 0),
      avg: this.calculateAverage(metricValues),
      min: Math.min(...metricValues),
      max: Math.max(...metricValues),
    };
  }

  // Clear old metrics (keep last 24 hours)
  cleanupMetrics(): void {
    const oneDayAgo = new Date(Date.now() - 24 * 60 * 60 * 1000);
    this.metrics = this.metrics.filter(metric => metric.timestamp >= oneDayAgo);
  }

  // Private methods
  private recordMetric(metric: Metric): void {
    this.metrics.push(metric);
    this.emit('metric', metric);

    // Auto-cleanup if too many metrics in memory
    if (this.metrics.length > 50000) {
      this.cleanupMetrics();
    }
  }

  private getMetricKey(name: string, tags?: Record<string, string>): string {
    if (!tags) return name;
    const tagString = Object.entries(tags)
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([key, value]) => `${key}=${value}`)
      .join(',');
    return `${name}{${tagString}}`;
  }

  private calculateAverage(values: number[]): number {
    if (values.length === 0) return 0;
    return values.reduce((sum, val) => sum + val, 0) / values.length;
  }

  private calculatePercentile(sortedValues: number[], percentile: number): number {
    if (sortedValues.length === 0) return 0;
    const index = Math.ceil((percentile / 100) * sortedValues.length) - 1;
    return sortedValues[Math.max(0, index)];
  }

  private startSystemMetricsCollection(): void {
    setInterval(() => {
      // Memory usage
      const memUsage = process.memoryUsage();
      this.recordGauge('memory_usage', memUsage.heapUsed);
      this.recordGauge('memory_total', memUsage.heapTotal);

      // Event loop lag (approximate CPU usage)
      const start = process.hrtime.bigint();
      setImmediate(() => {
        const lag = Number(process.hrtime.bigint() - start) / 1000000; // Convert to milliseconds
        this.recordGauge('event_loop_lag', lag);
      });

      this.emit('system_metrics_collected');
    }, 10000); // Every 10 seconds
  }
}

export const metricsCollector = new MetricsCollector();
