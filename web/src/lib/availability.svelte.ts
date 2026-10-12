// Where titles stand (in Plex, requested, downloading) for the dots on posters. Posters ask for their
// own key once; the asks are batched into one /requests/status call, and live updates keep them current.
import { api } from './api';
import type { RequestBrief } from './types';
import { on } from './live.svelte';

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

// Live: a request changed (use it as sent), something was added to Plex (ask again), or reload all.
on('request', (d: { key: string; brief: RequestBrief | null }) => remember(d.key, d.brief));
on('plex', (keys: string[]) => keys.forEach((k) => k in at && ask(k)));
on('resync', () => Object.keys(at).forEach(ask));

function ask(key: string) {
	if (!waiting.has(key)) {
		waiting.add(key);
		timer ??= setTimeout(flush, 50);
	}
}

// statusOf returns what's known about a title ("movie:603") and asks for it if that's missing or old.
export function statusOf(key: string): RequestBrief | null | undefined {
	if (!(key in at)) ask(key); // after that, live updates keep it current
	return known[key];
}

// remember updates a title right away (after requesting it), so its posters change without waiting.
export function remember(key: string, brief: RequestBrief | null) {
	known[key] = brief;
	at[key] = Date.now();
}
