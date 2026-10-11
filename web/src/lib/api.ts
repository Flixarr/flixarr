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
		headers: { Accept: 'application/json', ...(init.body ? { 'Content-Type': 'application/json' } : {}), ...init.headers }
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
	return res.json() as Promise<T>;
}

/** Data-server lookup through the Flixarr server, e.g. meta('movie/752'). */
export function meta<T>(path: string, params: Record<string, string | number | undefined> = {}): Promise<T> {
	const q = new URLSearchParams();
	for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== '') q.set(k, String(v));
	const qs = q.toString();
	return api<T>('/meta/' + path + (qs ? '?' + qs : ''));
}

const TMDB_IMG = 'https://image.tmdb.org/t/p/';
/** TMDB image URL for a path, e.g. img(poster, 'w342'). */
export const img = (path: string | null | undefined, size = 'w342') => (path ? TMDB_IMG + size + path : '');

/** Detail page URL for a card. */
export const href = (item: { type: string; id: number }) => (item.type === 'person' ? `/person/${item.id}` : `/${item.type}/${item.id}`);

export const fmtDate = (d: string | null | undefined, opts: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric', year: 'numeric' }) =>
	d ? new Date(d.length === 10 ? d + 'T00:00:00' : d).toLocaleDateString('en-US', opts) : 'TBA';

export const runtime = (m: number) => (m ? `${Math.floor(m / 60) ? Math.floor(m / 60) + 'h ' : ''}${m % 60}m` : '');
