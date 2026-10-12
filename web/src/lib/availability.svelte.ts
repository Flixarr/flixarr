// Where titles stand (in Plex, requested, downloading) for the dots on posters. Posters ask for their
// own key; the asks are batched into one /requests/status call and kept for a minute.
import { api } from './api';
import type { RequestBrief } from './types';

const known = $state<Record<string, RequestBrief | null>>({});
const at: Record<string, number> = {};
let waiting = new Set<string>();
let timer: ReturnType<typeof setTimeout> | null = null;

async function flush() {
	timer = null;
	const keys = [...waiting];
	waiting = new Set();
	for (let i = 0; i < keys.length; i += 100) {
		const chunk = keys.slice(i, i + 100);
		try {
			const d = await api<Record<string, RequestBrief>>('/requests/status?keys=' + chunk.join(','));
			const now = Date.now();
			for (const k of chunk) {
				known[k] = d[k] ?? null;
				at[k] = now;
			}
		} catch {
			// no dots this time; the next ask tries again
		}
	}
}

// statusOf returns what's known about a title ("movie:603") and asks for it if that's missing or old.
export function statusOf(key: string): RequestBrief | null | undefined {
	if (!(key in at) || Date.now() - at[key] > 60_000) {
		if (!waiting.has(key)) {
			waiting.add(key);
			timer ??= setTimeout(flush, 50);
		}
	}
	return known[key];
}

// remember updates a title right away (after requesting it), so its posters change without waiting.
export function remember(key: string, brief: RequestBrief | null) {
	known[key] = brief;
	at[key] = Date.now();
}
