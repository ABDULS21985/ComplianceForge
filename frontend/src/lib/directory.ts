import type { DirectoryUser } from '@/types/directory';

export const DIRECTORY_ROUTES = {
  users: '/directory/users/',
} as const;

export const directoryKeys = {
  activeUserSearch: (search: string) => ['directory', 'users', 'active-search', search] as const,
  userSearch: (search: string, status: string) =>
    ['directory', 'users', 'search', status, search] as const,
};

export function directoryUserName(user: Pick<DirectoryUser, 'email' | 'first_name' | 'last_name'>): string {
  return [user.first_name, user.last_name].map((part) => part.trim()).filter(Boolean).join(' ') || user.email;
}

export function directoryUserInitials(user: Pick<DirectoryUser, 'email' | 'first_name' | 'last_name'>): string {
  const initials = [user.first_name, user.last_name]
    .map((part) => part.trim().charAt(0))
    .filter(Boolean)
    .join('')
    .toUpperCase();
  return initials || user.email.charAt(0).toUpperCase();
}
