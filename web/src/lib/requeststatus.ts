// How a request's status reads everywhere (button, Requests page, request window, posters). The server
// decides the state and the line under it (#301); this only says how each state looks. Colors follow
// the palette: yellow waiting, blue downloading or almost there, green done, red a problem, grey neutral.
import type { Request } from './types';

type Status = Request['status'] | 'blocked';
// A state is the status as shown: Requested splits into Not out yet (#300).
export type State = Exclude<Status, 'approved' | 'sent'> | 'notout' | 'requested';

// The state of a request or brief: the server's when it sent one.
export const stateOf = (r: { status: string; state?: string; unreleased?: boolean }): State =>
	(r.state || (r.status === 'sent' || r.status === 'approved' ? (r.unreleased ? 'notout' : 'requested') : r.status)) as State;

// Names: managers see every step (exact); everyone else the short set.
const EXACT: Record<State, string> = {
	pending: 'Waiting for approval', notout: 'Not out yet', requested: 'Requested', grabbed: 'Grabbed', downloading: 'Downloading', importing: 'Importing', stuck: 'Stuck',
	partial: 'Partially available', available: 'Available', failed: 'Failed', declined: 'Declined', expired: 'Expired', removed: 'Removed', blocked: "Can't be requested"
};
const SHORT: Partial<Record<State, string>> = { grabbed: 'Downloading', importing: 'Almost ready' };

// Pill shades (palette: faint background, light text, soft ring).
const C = {
	yellow: 'bg-yellow-500/15 text-yellow-300 ring-yellow-500/40', blue: 'bg-blue-500/15 text-blue-300 ring-blue-500/40', green: 'bg-green-500/15 text-green-300 ring-green-500/40',
	red: 'bg-red-500/15 text-red-300 ring-red-500/40', gray: 'bg-gray-500/15 text-gray-300 ring-gray-500/40'
};
const COLOR: Record<State, keyof typeof C> = {
	pending: 'yellow', notout: 'gray', requested: 'gray', grabbed: 'blue', downloading: 'blue', importing: 'blue', stuck: 'red',
	partial: 'green', available: 'green', failed: 'red', declined: 'gray', expired: 'gray', removed: 'gray', blocked: 'gray'
};
export const NEUTRAL = C.gray;
export const toneOf = (s: State) => C[COLOR[s] ?? 'gray'];
// Solid shade of a state (dots, bars).
export const solidOf = (s: State) => ({ yellow: 'bg-yellow-400', blue: 'bg-blue-400', green: 'bg-green-400', red: 'bg-red-400', gray: 'bg-gray-400' })[COLOR[s] ?? 'gray'];

// The pill: its words and shades. Movies downloading show their percent (series don't, #216).
export function pill(s: State, progress = 0, series = false, exact = false) {
	const label = (exact ? EXACT[s] : (SHORT[s] ?? EXACT[s])) + (s === 'downloading' && progress && !series ? ` ${progress}%` : '');
	return { label, tone: toneOf(s) };
}

// The step bar. Managers see every step, everyone else three. A problem (Stuck, Failed) breaks on a step:
// the ones before it went fine (green), that one is red, and everyone else reads "Hit a problem" there.
export const STEPS = ['Requested', 'Grabbed', 'Downloading', 'Importing', 'Available'];
export const SIMPLE_STEPS = ['Requested', 'Downloading', 'Available'];
const AT_FULL: Partial<Record<State, number>> = { pending: 0, notout: 0, requested: 0, grabbed: 1, downloading: 2, importing: 3, partial: 4, available: 4 };
const AT_SHORT: Partial<Record<State, number>> = { pending: 0, notout: 0, requested: 0, grabbed: 1, downloading: 1, importing: 1, partial: 2, available: 2 };
const BROKE_FULL: Record<string, number> = { send: 1, download: 2, import: 3 };
const BROKE_SHORT: Record<string, number> = { send: 1, download: 1, import: 2 };
export function stepAt(s: State, full: boolean, brokeAt = ''): { at: number; broke: boolean } {
	if ((s === 'stuck' || s === 'failed') && brokeAt) return { at: (full ? BROKE_FULL : BROKE_SHORT)[brokeAt] ?? -1, broke: true };
	return { at: (full ? AT_FULL : AT_SHORT)[s] ?? -1, broke: false };
}

// Still moving: worth watching for changes.
export const ACTIVE: string[] = ['pending', 'approved', 'sent', 'grabbed', 'downloading', 'importing', 'stuck', 'partial'];
export const isActive = (s?: string) => !!s && ACTIVE.includes(s);

// The one line under a request, as the server wrote it; its color only when someone has to act.
export const hintClass = (tone?: string) => (tone === 'red' ? 'text-red-300' : tone === 'yellow' ? 'text-yellow-300' : 'text-gray-400');

// The timeline's words for each step.
export const EVENT_TEXT: Record<string, string> = {
	requested: 'Requested', redownload: 'Redownload asked for', approved: 'Approved', declined: 'Declined', sent: 'Looking for a download',
	grabbed: 'Found a download', downloading: 'Downloading', importing: 'Downloaded, adding to Plex', partial: 'First episode in Plex', available: 'In Plex',
	stuck: 'Hit a problem', failed: 'Failed', expired: 'Removed from Plex', removed: 'Removed', pending: 'Waiting for approval',
	imported: 'Brought over from Seerr as ended' // #186: the import's own "expired", not a removal
};

// What a request asked for: "Whole show", "Season 2", "Seasons 1, 3", "4 episodes" ('' for movies).
export const askedFor = (r: Pick<Request, 'type' | 'mode' | 'seasons' | 'episodes'>) =>
	r.type !== 'series' ? '' : r.mode === 'all' ? 'Whole show' : r.mode === 'episodes' ? `${r.episodes.length} episodes` : r.seasons.length === 1 ? `Season ${r.seasons[0]}` : `Seasons ${r.seasons.join(', ')}`;

// A timeline dot's colour for a request step (yellow: someone pressed Fix).
export const eventDot = (s: string) => (s === 'failed' || s === 'stuck' ? 'bg-red-500' : s === 'fix' ? 'bg-yellow-500' : s === 'available' || s === 'partial' ? 'bg-green-500' : ['declined', 'expired', 'removed', 'imported'].includes(s) ? 'bg-gray-500' : 'bg-primary');
