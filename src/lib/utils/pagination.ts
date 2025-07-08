export interface PaginationOptions {
  page?: number;
  limit?: number;
  sortBy?: string;
  sortOrder?: 'asc' | 'desc';
}

export interface PaginationResult<T> {
  data: T[];
  pagination: {
    currentPage: number;
    totalPages: number;
    totalCount: number;
    hasNextPage: boolean;
    hasPreviousPage: boolean;
    limit: number;
  };
}

export class PaginationUtils {
  // Calculate skip value for database queries
  static getSkip(page: number, limit: number): number {
    return (page - 1) * limit;
  }

  // Validate and normalize pagination options
  static normalizePaginationOptions(options: PaginationOptions): Required<PaginationOptions> {
    return {
      page: Math.max(1, options.page || 1),
      limit: Math.min(100, Math.max(1, options.limit || 20)),
      sortBy: options.sortBy || 'createdAt',
      sortOrder: options.sortOrder || 'desc',
    };
  }

  // Create pagination result
  static createPaginationResult<T>(
    data: T[],
    totalCount: number,
    options: PaginationOptions
  ): PaginationResult<T> {
    const normalizedOptions = this.normalizePaginationOptions(options);
    const totalPages = Math.ceil(totalCount / normalizedOptions.limit);

    return {
      data,
      pagination: {
        currentPage: normalizedOptions.page,
        totalPages,
        totalCount,
        hasNextPage: normalizedOptions.page < totalPages,
        hasPreviousPage: normalizedOptions.page > 1,
        limit: normalizedOptions.limit,
      },
    };
  }

  // Generate MongoDB sort object
  static generateSortObject(sortBy: string, sortOrder: 'asc' | 'desc'): Record<string, 1 | -1> {
    return { [sortBy]: sortOrder === 'asc' ? 1 : -1 };
  }

  // Calculate pagination metadata only
  static calculatePaginationMeta(totalCount: number, page: number, limit: number) {
    const totalPages = Math.ceil(totalCount / limit);
    
    return {
      currentPage: page,
      totalPages,
      totalCount,
      hasNextPage: page < totalPages,
      hasPreviousPage: page > 1,
      limit,
    };
  }

  // Generate pagination links (for APIs)
  static generatePaginationLinks(
    baseUrl: string,
    currentPage: number,
    totalPages: number,
    limit: number
  ): {
    first?: string;
    prev?: string;
    next?: string;
    last?: string;
  } {
    const links: any = {};

    if (currentPage > 1) {
      links.first = `${baseUrl}?page=1&limit=${limit}`;
      links.prev = `${baseUrl}?page=${currentPage - 1}&limit=${limit}`;
    }

    if (currentPage < totalPages) {
      links.next = `${baseUrl}?page=${currentPage + 1}&limit=${limit}`;
      links.last = `${baseUrl}?page=${totalPages}&limit=${limit}`;
    }

    return links;
  }

  // Get page numbers for pagination UI
  static getPageNumbers(currentPage: number, totalPages: number, maxVisible: number = 5): number[] {
    const pages: number[] = [];
    
    if (totalPages <= maxVisible) {
      for (let i = 1; i <= totalPages; i++) {
        pages.push(i);
      }
    } else {
      const half = Math.floor(maxVisible / 2);
      let start = Math.max(1, currentPage - half);
      let end = Math.min(totalPages, start + maxVisible - 1);
      
      if (end - start + 1 < maxVisible) {
        start = Math.max(1, end - maxVisible + 1);
      }
      
      for (let i = start; i <= end; i++) {
        pages.push(i);
      }
    }
    
    return pages;
  }
}
