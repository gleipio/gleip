import { theme } from './theme';

export const getMethodColor = (method: string): string => {
  switch (method.toUpperCase()) {
    case 'GET':
      return `var(--color-method-get)`;
    case 'POST':
      return `var(--color-method-post)`;
    case 'PUT':
      return `var(--color-method-put)`;
    case 'DELETE':
      return `var(--color-method-delete)`;
    case 'PATCH':
      return `var(--color-method-patch)`;
    default:
      return `var(--color-method-other)`;
  }
};

export const getStatusColor = (statusCode: number): string => {
  if (statusCode < 100) return `var(--color-status-other)`;
  if (statusCode < 200) return `var(--color-status-1xx)`;
  if (statusCode < 300) return `var(--color-status-2xx)`;
  if (statusCode < 400) return `var(--color-status-3xx)`;
  if (statusCode < 500) return `var(--color-status-4xx)`;
  if (statusCode < 600) return `var(--color-status-5xx)`;
  return `var(--color-status-other)`;
}; 