export class DateUtils {
  // Format date for display
  static formatDate(date: Date | string, locale: string = 'en-US'): string {
    const d = typeof date === 'string' ? new Date(date) : date;
    return new Intl.DateTimeFormat(locale, {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
    }).format(d);
  }

  // Format time for display
  static formatTime(date: Date | string, locale: string = 'en-US'): string {
    const d = typeof date === 'string' ? new Date(date) : date;
    return new Intl.DateTimeFormat(locale, {
      hour: '2-digit',
      minute: '2-digit',
    }).format(d);
  }

  // Format datetime for display
  static formatDateTime(date: Date | string, locale: string = 'en-US'): string {
    const d = typeof date === 'string' ? new Date(date) : date;
    return new Intl.DateTimeFormat(locale, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    }).format(d);
  }

  // Get relative time (e.g., "2 hours ago")
  static getRelativeTime(date: Date | string, locale: string = 'en-US'): string {
    const d = typeof date === 'string' ? new Date(date) : date;
    const now = new Date();
    const diffInSeconds = Math.floor((now.getTime() - d.getTime()) / 1000);

    if (diffInSeconds < 60) {
      return 'Just now';
    }

    const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });

    const timeUnits: [Intl.RelativeTimeFormatUnit, number][] = [
      ['year', 365 * 24 * 60 * 60],
      ['month', 30 * 24 * 60 * 60],
      ['week', 7 * 24 * 60 * 60],
      ['day', 24 * 60 * 60],
      ['hour', 60 * 60],
      ['minute', 60],
    ];

    for (const [unit, seconds] of timeUnits) {
      const interval = Math.floor(diffInSeconds / seconds);
      if (interval >= 1) {
        return rtf.format(-interval, unit);
      }
    }

    return rtf.format(-diffInSeconds, 'second');
  }

  // Check if date is today
  static isToday(date: Date | string): boolean {
    const d = typeof date === 'string' ? new Date(date) : date;
    const today = new Date();
    return d.toDateString() === today.toDateString();
  }

  // Check if date is yesterday
  static isYesterday(date: Date | string): boolean {
    const d = typeof date === 'string' ? new Date(date) : date;
    const yesterday = new Date();
    yesterday.setDate(yesterday.getDate() - 1);
    return d.toDateString() === yesterday.toDateString();
  }

  // Check if date is this week
  static isThisWeek(date: Date | string): boolean {
    const d = typeof date === 'string' ? new Date(date) : date;
    const now = new Date();
    const startOfWeek = new Date(now.setDate(now.getDate() - now.getDay()));
    const endOfWeek = new Date(now.setDate(now.getDate() - now.getDay() + 6));
    
    return d >= startOfWeek && d <= endOfWeek;
  }

  // Get start of day
  static startOfDay(date: Date | string): Date {
    const d = typeof date === 'string' ? new Date(date) : new Date(date);
    d.setHours(0, 0, 0, 0);
    return d;
  }

  // Get end of day
  static endOfDay(date: Date | string): Date {
    const d = typeof date === 'string' ? new Date(date) : new Date(date);
    d.setHours(23, 59, 59, 999);
    return d;
  }

  // Add days to date
  static addDays(date: Date | string, days: number): Date {
    const d = typeof date === 'string' ? new Date(date) : new Date(date);
    d.setDate(d.getDate() + days);
    return d;
  }

  // Add hours to date
  static addHours(date: Date | string, hours: number): Date {
    const d = typeof date === 'string' ? new Date(date) : new Date(date);
    d.setTime(d.getTime() + (hours * 60 * 60 * 1000));
    return d;
  }

  // Add minutes to date
  static addMinutes(date: Date | string, minutes: number): Date {
    const d = typeof date === 'string' ? new Date(date) : new Date(date);
    d.setTime(d.getTime() + (minutes * 60 * 1000));
    return d;
  }

  // Get difference in days
  static getDaysDifference(date1: Date | string, date2: Date | string): number {
    const d1 = typeof date1 === 'string' ? new Date(date1) : date1;
    const d2 = typeof date2 === 'string' ? new Date(date2) : date2;
    const diffTime = Math.abs(d2.getTime() - d1.getTime());
    return Math.ceil(diffTime / (1000 * 60 * 60 * 24));
  }

  // Format duration (e.g., "2h 30m")
  static formatDuration(milliseconds: number): string {
    const seconds = Math.floor(milliseconds / 1000);
    const minutes = Math.floor(seconds / 60);
    const hours = Math.floor(minutes / 60);
    const days = Math.floor(hours / 24);

    if (days > 0) {
      return `${days}d ${hours % 24}h`;
    } else if (hours > 0) {
      return `${hours}h ${minutes % 60}m`;
    } else if (minutes > 0) {
      return `${minutes}m ${seconds % 60}s`;
    } else {
      return `${seconds}s`;
    }
  }

  // Parse ISO string safely
  static parseISO(dateString: string): Date | null {
    try {
      const date = new Date(dateString);
      return isNaN(date.getTime()) ? null : date;
    } catch {
      return null;
    }
  }

  // Convert to UTC
  static toUTC(date: Date | string): Date {
    const d = typeof date === 'string' ? new Date(date) : date;
    return new Date(d.getTime() + d.getTimezoneOffset() * 60000);
  }

  // Convert from UTC
  static fromUTC(date: Date | string): Date {
    const d = typeof date === 'string' ? new Date(date) : date;
    return new Date(d.getTime() - d.getTimezoneOffset() * 60000);
  }

  // Get timezone offset
  static getTimezoneOffset(): number {
    return new Date().getTimezoneOffset();
  }

  // Format for chat display (smart formatting)
  static formatForChat(date: Date | string): string {
    const d = typeof date === 'string' ? new Date(date) : date;
    
    if (this.isToday(d)) {
      return this.formatTime(d);
    } else if (this.isYesterday(d)) {
      return 'Yesterday';
    } else if (this.isThisWeek(d)) {
      return new Intl.DateTimeFormat('en-US', { weekday: 'short' }).format(d);
    } else {
      return this.formatDate(d);
    }
  }

  // Validate date range
  static isValidDateRange(startDate: Date | string, endDate: Date | string): boolean {
    const start = typeof startDate === 'string' ? new Date(startDate) : startDate;
    const end = typeof endDate === 'string' ? new Date(endDate) : endDate;
    
    return start.getTime() <= end.getTime();
  }
}
