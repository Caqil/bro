import { z } from 'zod';
import { Request, Response, NextFunction } from 'express';

// Common validation schemas
export const commonSchemas = {
  objectId: z.string().regex(/^[0-9a-fA-F]{24}$/, 'Invalid ObjectId'),
  phoneNumber: z.string().regex(/^\+?[1-9]\d{1,14}$/, 'Invalid phone number'),
  email: z.string().email('Invalid email address'),
  password: z.string().min(8, 'Password must be at least 8 characters').max(128, 'Password too long'),
  username: z.string().min(3, 'Username too short').max(30, 'Username too long').regex(/^[a-zA-Z0-9_]+$/, 'Invalid username format'),
  url: z.string().url('Invalid URL'),
  uuid: z.string().uuid('Invalid UUID'),
};

// Pagination schema
export const paginationSchema = z.object({
  page: z.number().min(1).default(1),
  limit: z.number().min(1).max(100).default(20),
  sortBy: z.string().optional(),
  sortOrder: z.enum(['asc', 'desc']).default('desc'),
});

// Request validation middleware
export function validateRequest(schema: z.ZodSchema) {
  return (req: Request, res: Response, next: NextFunction) => {
    try {
      const result = schema.safeParse({
        body: req.body,
        query: req.query,
        params: req.params,
      });

      if (!result.success) {
        return res.status(400).json({
          error: 'Validation failed',
          details: result.error.errors.map(err => ({
            field: err.path.join('.'),
            message: err.message,
            code: err.code,
          })),
        });
      }

      // Attach validated data to request
      req.validatedData = result.data;
      next();
    } catch (error) {
      console.error('Validation error:', error);
      res.status(500).json({ error: 'Internal validation error' });
    }
  };
}

// Body validation
export function validateBody(schema: z.ZodSchema) {
  return (req: Request, res: Response, next: NextFunction) => {
    try {
      const result = schema.safeParse(req.body);

      if (!result.success) {
        return res.status(400).json({
          error: 'Invalid request body',
          details: result.error.errors,
        });
      }

      req.body = result.data;
      next();
    } catch (error) {
      console.error('Body validation error:', error);
      res.status(500).json({ error: 'Internal validation error' });
    }
  };
}

// Query validation
export function validateQuery(schema: z.ZodSchema) {
  return (req: Request, res: Response, next: NextFunction) => {
    try {
      const result = schema.safeParse(req.query);

      if (!result.success) {
        return res.status(400).json({
          error: 'Invalid query parameters',
          details: result.error.errors,
        });
      }

      req.query = result.data;
      next();
    } catch (error) {
      console.error('Query validation error:', error);
      res.status(500).json({ error: 'Internal validation error' });
    }
  };
}

// Params validation
export function validateParams(schema: z.ZodSchema) {
  return (req: Request, res: Response, next: NextFunction) => {
    try {
      const result = schema.safeParse(req.params);

      if (!result.success) {
        return res.status(400).json({
          error: 'Invalid URL parameters',
          details: result.error.errors,
        });
      }

      req.params = result.data;
      next();
    } catch (error) {
      console.error('Params validation error:', error);
      res.status(500).json({ error: 'Internal validation error' });
    }
  };
}

// Custom validators
export const customValidators = {
  // Validate file upload
  fileUpload: z.object({
    mimetype: z.string(),
    size: z.number().max(100 * 1024 * 1024, 'File too large'), // 100MB
    originalname: z.string(),
  }),

  // Validate date range
  dateRange: z.object({
    startDate: z.string().datetime().optional(),
    endDate: z.string().datetime().optional(),
  }).refine(data => {
    if (data.startDate && data.endDate) {
      return new Date(data.startDate) < new Date(data.endDate);
    }
    return true;
  }, 'Start date must be before end date'),

  // Validate coordinates
  coordinates: z.object({
    latitude: z.number().min(-90).max(90),
    longitude: z.number().min(-180).max(180),
  }),

  // Validate color hex code
  hexColor: z.string().regex(/^#([A-Fa-f0-9]{6}|[A-Fa-f0-9]{3})$/, 'Invalid hex color'),

  // Validate JSON string
  jsonString: z.string().refine(val => {
    try {
      JSON.parse(val);
      return true;
    } catch {
      return false;
    }
  }, 'Invalid JSON string'),
};
