// Small fetch wrapper for the Flixarr API.

export class ApiError extends Error {
	status: number;
	constructor(status: number, message: string) {
		super(message);
		this.status = status;
	}
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
	const res = await fetch('/api' + path, {
		credentials: 'same-origin',
		...init,
		headers: { Accept: 'application/json', ...(typeof init.body === 'string' ? { 'Content-Type': 'application/json' } : {}), ...init.headers }
	});
	if (!res.ok) {
		let msg = res.statusText;
		try {
			msg = (await res.json()).error || msg;
		} catch {
			/* not JSON */
		}
		throw new ApiError(res.status, msg);
	}
	// 202/204 and other empty answers have no JSON to read.
	const text = await res.text();
	return (text ? JSON.parse(text) : undefined) as T;
}

// Recent lookups kept in memory while the app is open (stale-while-revalidate, like Seerr's web app):
// a revisit shows the saved copy instantly, then asks the server again in the background. The server
// still sees every visit, so its cache counters stay honest. Copies older than MAX_AGE aren't shown.
const recent = new Map<string, { json: string; data: unknown; at: number }>();
const MAX_ENTRIES = 300;
const MAX_AGE = 30 * 60 * 1000;

function remember(key: string, data: unknown, json: string) {
	recent.delete(key); // re-insert so the Map's order is least-recently-used first
	recent.set(key, { json, data, at: Date.now() });
	if (recent.size > MAX_ENTRIES) recent.delete(recent.keys().next().value!);
}

// forgetMeta drops the saved lookups (after changing your region or language, so pages load in the new one).
export function forgetMeta() {
	recent.clear();
}

/**
 * Flixarr API lookup through this server, e.g. meta('movie/752').
 * With a saved copy it resolves instantly; if the background check finds newer data, onFresh gets it.
 */
export function meta<T>(path: string, params: Record<string, string | number | undefined> = {}, onFresh?: (data: T) => void): Promise<T> {
	const q = new URLSearchParams();
	for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== '') q.set(k, String(v));
	const qs = q.toString();
	const key = '/meta/' + path + (qs ? '?' + qs : '');
	const fetchFresh = () =>
		api<T>(key).then((data) => {
			const json = JSON.stringify(data);
			const changed = recent.get(key)?.json !== json;
			remember(key, data, json);
			return { data, changed };
		});

	const saved = recent.get(key);
	if (saved && Date.now() - saved.at < MAX_AGE) {
		fetchFresh()
			.then(({ data, changed }) => changed && onFresh?.(data))
			.catch(() => {}); // the saved copy is already on screen
		return Promise.resolve(saved.data as T);
	}
	return fetchFresh().then(({ data }) => data);
}

const TMDB_IMG = 'https://image.tmdb.org/t/p/';
/** Set from /api/status: when image caching is on, images load through this server (Settings → Cache). */
export const prefs = { imageCache: false };
/** TMDB image URL for a path, e.g. img(poster, 'w342'). */
export const img = (path: string | null | undefined, size = 'w342') =>
	path ? (prefs.imageCache ? '/api/image/' + size : TMDB_IMG + size) + path : '';

/** Detail page URL for a card. */
export const href = (item: { type: string; id: number }) => (item.type === 'person' ? `/person/${item.id}` : `/${item.type}/${item.id}`);

export const fmtDate = (d: string | null | undefined, opts: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric', year: 'numeric' }) =>
	d ? new Date(d.length === 10 ? d + 'T00:00:00' : d).toLocaleDateString('en-US', opts) : 'TBA';

export const runtime = (m: number) => (m ? `${Math.floor(m / 60) ? Math.floor(m / 60) + 'h ' : ''}${m % 60}m` : '');

export const fmtBytes = (n: number) =>
	n < 1024 ? `${n} B` : n < 1024 ** 2 ? `${(n / 1024).toFixed(0)} KB` : n < 1024 ** 3 ? `${(n / 1024 ** 2).toFixed(1)} MB` : n < 1024 ** 4 ? `${(n / 1024 ** 3).toFixed(n < 100 * 1024 ** 3 ? 2 : 1)} GB` : `${(n / 1024 ** 4).toFixed(1)} TB`;

// How long ago: "just now", "42 sec ago", "5 min ago", "3 hr ago", "2 days ago". now is passed by pages
// that tick, so the text keeps moving.
export function ago(iso: string | null | undefined, now = Date.now()) {
	if (!iso) return 'never';
	const s = Math.max(0, Math.round((now - Date.parse(iso)) / 1000));
	if (s < 10) return 'just now';
	if (s < 60) return `${s} sec ago`;
	if (s < 3600) return `${Math.round(s / 60)} min ago`;
	if (s < 86400) return `${Math.round(s / 3600)} hr ago`;
	const d = Math.round(s / 86400);
	return `${d} day${d === 1 ? '' : 's'} ago`;
}
// "1 h 12 min left" from Radarr / Sonarr's "01:12:05".
export function timeLeft(t?: string) {
	if (!t) return '';
	const [h, m] = t.split(':').map(Number);
	return h ? `${h} h ${m} min left` : m ? `${m} min left` : 'under a minute left';
}
export const fmtNum = (n: number) => n.toLocaleString('en-US');
