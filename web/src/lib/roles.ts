// Roles are presets (User, Trusted, Manager). Admin and Plex Watchlist requests are switches turned on by
// hand that never belong to a preset (#146); anyone with admin shows as Admin, whatever their role.
import type { User } from '#lib/types.ts';

export const ROLES = ['User', 'Trusted', 'Manager'];
export const MANUAL_PERMS = ['admin', 'watchlist'];

// The label people see for someone: Admin, or their role.
export const roleLabel = (u: Pick<User, 'role' | 'perms'>) => (u.perms.includes('admin') ? 'Admin' : u.role);

export const ROLE_TONE: Record<string, string> = {
	Admin: 'bg-primary/15 text-primary-light ring-primary/40', Manager: 'bg-sky-500/15 text-sky-300 ring-sky-500/40',
	Trusted: 'bg-green-500/15 text-green-400 ring-green-500/40', Custom: 'bg-purple-500/15 text-purple-300 ring-purple-500/40',
	User: 'bg-gray-800 text-gray-300 ring-gray-700/60'
};
