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

// The switches as profiles and Edit permissions show them, grouped by what they're for (#158). Each group
// goes from what everyone gets to what fewer people get. [key, label, what it does].
export type Perm = [string, string, string];
export const PERM_GROUPS: [string, Perm[]][] = [
	['Requests', [
		['requestMovies', 'Request movies', ''],
		['requestTv', 'Request series', ''],
		['advanced', 'Choose quality', 'Pick a quality other than the default when requesting.'],
		['autoApprove', 'Auto-approve', 'Their requests skip approval.'],
		['watchlist', 'Plex Watchlist requests', 'Can switch on requests from their Plex Watchlist (in their Profile).']
	]],
	['Issues', [
		['reportIssues', 'Report issues', 'Report problems on titles in the library.']
	]],
	['Managing', [
		['manageRequests', 'Manage requests', "Approve, decline, retry and remove anyone's requests."],
		['manageIssues', 'Manage issues', "See, resolve and remove anyone's issues."],
		['redownload', 'Fix and redownload', 'Fix missing or bad episodes and movies: search again, or delete the file, block its release and download a new copy. Also needed to delete files from the admin panel.'],
		['manageUsers', 'Manage users', 'See everyone on the Users page and change their roles, switches and request limits, up to Manager. Never admins.'],
		['admin', 'Admin', 'Full access to everything, including Settings and Users.']
	]]
];
// Auto-approve does nothing while approvals are off (Settings, Requests); the switch says so instead.
export const NO_APPROVALS = "Approvals are off (Settings, Requests), so everyone's requests go straight through. This only matters if they're turned on.";


// A password to hand someone for a new or reset local account (#122): 16 characters with upper and lower
// case letters, numbers and symbols, leaving out ones easy to misread (l, 1, I, O, 0).
export function makePassword(length = 16) {
	const sets = ['abcdefghijkmnopqrstuvwxyz', 'ABCDEFGHJKLMNPQRSTUVWXYZ', '23456789', '!@#$%&*?'];
	const all = sets.join('');
	const pick = (from: string) => {
		const max = 256 - (256 % from.length); // no bias toward the first characters
		for (;;) {
			const [n] = crypto.getRandomValues(new Uint8Array(1));
			if (n < max) return from[n % from.length];
		}
	};
	const out = [...sets.map(pick), ...Array.from({ length: length - sets.length }, () => pick(all))];
	for (let i = out.length - 1; i > 0; i--) {
		const j = crypto.getRandomValues(new Uint32Array(1))[0] % (i + 1);
		[out[i], out[j]] = [out[j], out[i]];
	}
	return out.join('');
}
