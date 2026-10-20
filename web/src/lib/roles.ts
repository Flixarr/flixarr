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

// The switches as Users and profiles show them: [key, label, what it does].
export const PERM_TEXT: [string, string, string][] = [
	['manageRequests', 'Manage requests', "Approve, decline, retry and remove anyone's requests."],
	['autoApprove', 'Auto-approve', 'Their requests skip approval.'],
	['requestMovies', 'Request movies', ''],
	['requestTv', 'Request series', ''],
	['advanced', 'Choose quality', 'Pick a quality other than the default when requesting.'],
	['reportIssues', 'Report issues', 'Report problems on titles in the library.'],
	['manageIssues', 'Manage issues', "See, resolve and remove anyone's issues."],
	['redownload', 'Fix and redownload', 'Fix missing or bad episodes and movies: search again, or delete the file, block its release and download a new copy. Also needed to delete files from the admin panel.']
];
// Turned on by hand, never by a role.
export const MANUAL_TEXT: [string, string, string][] = [
	['admin', 'Admin', 'Full access to everything, including Settings and Users.'],
	['watchlist', 'Plex Watchlist requests', 'Can switch on requests from their Plex Watchlist (in their Profile).']
];

// A password to hand someone for a new or reset local account.
export function makePassword() {
	const words = crypto.getRandomValues(new Uint32Array(4));
	return Array.from(words, (n) => n.toString(36).slice(0, 5)).join('-');
}
