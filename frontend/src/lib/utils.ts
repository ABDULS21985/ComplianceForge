import { type ClassValue, clsx } from 'clsx';
import { twMerge } from 'tailwind-merge';
import { format, formatDistanceToNow, parseISO } from 'date-fns';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatDate(date: string | Date | undefined | null): string {
  if (!date) return '—';
  const d = typeof date === 'string' ? parseISO(date) : date;
  return format(d, 'dd MMM yyyy');
}

export function formatDateTime(date: string | Date | undefined | null): string {
  if (!date) return '—';
  const d = typeof date === 'string' ? parseISO(date) : date;
  return format(d, 'dd MMM yyyy HH:mm');
}

export function formatRelativeTime(date: string | Date | undefined | null): string {
  if (!date) return '—';
  const d = typeof date === 'string' ? parseISO(date) : date;
  return formatDistanceToNow(d, { addSuffix: true });
}

export function formatCurrency(value: number | undefined | null, currency = 'EUR'): string {
  if (value == null) return '—';
  return new Intl.NumberFormat('en-GB', { style: 'currency', currency }).format(value);
}

export function formatPercentage(value: number | undefined | null): string {
  if (value == null) return '—';
  return `${value.toFixed(1)}%`;
}

export function truncate(str: string, length: number): string {
  if (str.length <= length) return str;
  return str.slice(0, length) + '…';
}

export function getInitials(firstName?: string, lastName?: string): string {
  return `${(firstName?.[0] || '').toUpperCase()}${(lastName?.[0] || '').toUpperCase()}`;
}

export function getRiskScoreColor(score: number | undefined | null): string {
  if (score == null) return 'text-muted-foreground';
  if (score >= 20) return 'text-risk-critical';
  if (score >= 12) return 'text-risk-high';
  if (score >= 6) return 'text-risk-medium';
  return 'text-risk-low';
}

export function getRiskLevelColor(level: string | undefined | null): string {
  switch (level) {
    case 'critical': return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400';
    case 'high': return 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400';
    case 'medium': return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400';
    case 'low': return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400';
    case 'very_low': return 'bg-cyan-100 text-cyan-800 dark:bg-cyan-900/30 dark:text-cyan-400';
    default: return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
  }
}

export function getStatusColor(status: string): string {
  switch (status) {
    case 'effective': case 'implemented': case 'completed': case 'published': case 'approved': case 'resolved': case 'closed': case 'attested': case 'certified':
      return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400';
    case 'partial': case 'in_progress': case 'under_review': case 'pending_approval': case 'investigating': case 'contained':
      return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400';
    case 'planned': case 'draft': case 'pending': case 'identified': case 'scheduled':
      return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400';
    case 'not_implemented': case 'not_started': case 'open': case 'overdue': case 'failed':
      return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400';
    case 'not_applicable': case 'archived': case 'retired': case 'cancelled': case 'superseded':
      return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
    default:
      return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400';
  }
}
