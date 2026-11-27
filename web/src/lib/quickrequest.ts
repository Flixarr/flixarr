// One tap requests with Undo (#283). When there's nothing to choose or show in the request window (a movie,
// no quality to pick, no approval, no weekly limit, out already, one server), Request sends without the window.
// The request waits UNDO_MS so Undo can cancel it before it reaches Radarr. It lives here, not in the page, so
// leaving the page doesn't lose it, and closing the app sends it straight away.
import { api } from './api.ts';
import { notify, undoToast } from './toast.svelte.ts';
import type { Movie, Request } from './types.ts';

export const UNDO_MS = 5000;

type Options = { connected: boolean; approval: boolean; usage?: { limit: number; used: number }; profiles: unknown[]; defaultProfileId?: number; servers?: unknown[] };

// Can this movie be requested in one tap for this person?
export async function oneTap(item: Movie): Promise<{ profileId: number } | null> {
	const today = new Date().toISOString().slice(0, 10);
	if (item.release && item.release > today) return null; // not out yet: the window says so
	const o = await api<Options>('/requests/options?type=movie');
	const simple = o.connected && !o.approval && !o.usage?.limit && !o.profiles.length && (o.servers?.length ?? 1) <= 1;
	return simple ? { profileId: o.defaultProfileId ?? 0 } : null;
}

const waiting = new Map<number, () => void>(); // tmdb id → send now

function body(item: Movie, profileId: number) {
	return JSON.stringify({ type: 'movie', tmdbId: item.id, title: item.title, year: item.year, poster: item.poster ?? '', backdrop: item.backdrop ?? '',
		seasons: [], episodes: [], profileId, genres: item.genres, language: '', seasonCount: 0 });
}

// Request the movie after UNDO_MS unless Undo is pressed. `done` gets the request, or null if it was undone or failed.
export function quickRequest(item: Movie, profileId: number, done: (r: Request | null) => void) {
	let sent = false;
	const send = (keepalive = false) => {
		if (sent) return;
		sent = true;
		clearTimeout(timer);
		waiting.delete(item.id);
		api<Request>('/requests', { method: 'POST', body: body(item, profileId), keepalive })
			.then((r) => {
				done(r);
				if (r.status === 'failed') notify(r.detail || `Unable to add ${item.title}`, 'orange');
			})
			.catch((e) => {
				done(null);
				notify(e instanceof Error ? e.message : `Unable to request ${item.title}`, 'orange');
			});
	};
	const timer = setTimeout(() => send(), UNDO_MS);
	waiting.set(item.id, () => send(true));
	undoToast(`Requested ${item.title}`, UNDO_MS, () => {
		if (sent) return;
		sent = true;
		clearTimeout(timer);
		waiting.delete(item.id);
		done(null);
		notify('Request cancelled', 'gray');
	});
}

// Closing or leaving the app sends whatever is still waiting, so a request is never lost.
if (typeof window !== 'undefined') addEventListener('pagehide', () => [...waiting.values()].forEach((send) => send()));
