import { ApiError } from './api'

export type ErrorView = { title: string; message: string; status?: number; fields: Record<string, string>; retryable: boolean; correlationId?: string }

export function describeError(error: unknown): ErrorView {
  if (!(error instanceof ApiError)) return { title: 'Request failed', message: error instanceof Error ? error.message : 'The server did not return a usable response.', fields: {}, retryable: true }
  if (error.status === 401) return { title: 'Session expired', message: 'Sign in again to continue.', status: 401, fields: error.fields, retryable: false, correlationId: error.correlationId }
  if (error.status === 403) return { title: 'Access denied', message: 'Your permissions do not allow this action.', status: 403, fields: error.fields, retryable: false, correlationId: error.correlationId }
  if (error.status === 404) return { title: 'Not found', message: 'The requested resource no longer exists.', status: 404, fields: error.fields, retryable: false, correlationId: error.correlationId }
  if (error.status === 409) return { title: 'State conflict', message: 'This resource changed. Reload it before saving again.', status: 409, fields: error.fields, retryable: false, correlationId: error.correlationId }
  if (error.status === 422) return { title: 'Check the form', message: error.message, status: 422, fields: error.fields, retryable: false, correlationId: error.correlationId }
  if (error.status === 429) return { title: 'Too many requests', message: 'Wait a moment before trying again.', status: 429, fields: error.fields, retryable: true, correlationId: error.correlationId }
  return { title: 'Server error', message: error.message, status: error.status, fields: error.fields, retryable: error.status >= 500, correlationId: error.correlationId }
}

export function FieldError({ message }: { message?: string }) {
  return message ? <small className="gf-field-error" role="alert">{message}</small> : null
}
