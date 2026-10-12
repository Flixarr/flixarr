// Signed-in user + server status, shared across the app.
import { api, ApiError, prefs } from './api';
import type { Status, User } from './types';

export const session = $state<{ ready: boolean; status: Status | null; me: User | null }>({ ready: false, status: null, me: null });

export async function loadSession() {
	session.status = await api<Status>('/status');
	prefs.imageCache = session.status.imageCache;
	if (!session.status.setupRequired) {
		try {
			session.me = await api<User>('/me');
		} catch (e) {
			if (!(e instanceof ApiError && e.status === 401)) throw e;
			session.me = null;
		}
	}
	session.ready = true;
}

export async function signOut() {
	await api('/auth/logout', { method: 'POST' });
	session.me = null;
}

// can: does the signed-in user have a permission? Admins can do everything.
export const can = (perm: string) => !!session.me && (session.me.perms.includes('admin') || session.me.perms.includes(perm));
