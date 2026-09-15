/** Read only the stable HTTP status; diagnostic error details stay out of state UI. */
export function resourceErrorStatus(error: unknown): number | null {
  if (!error || typeof error !== 'object' || !('status' in error)) return null;
  return typeof error.status === 'number' ? error.status : null;
}

export function isForbiddenResourceError(error: unknown): boolean {
  return resourceErrorStatus(error) === 403;
}
