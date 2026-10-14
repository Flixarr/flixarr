// How a request's status reads everywhere (button, Requests page, posters).
import type { Request } from './types';

type Status = Request['status'] | 'blocked';

export const LABEL: Record<Status, string> = {
	pending: 'Waiting for approval', approved: 'Requested', sent: 'Requested', grabbed: 'Grabbed', downloading: 'Downloading',
	importing: 'Importing', available: 'Available', failed: 'Failed', declined: 'Declined', expired: 'Expired', blocked: "Can't be requested"
};

// Pill colours: [background, text, ring].
export const TONE: Partial<Record<Status, string>> = {
	pending: 'bg-yellow-500/15 text-yellow-300 ring-yellow-500/40',
	grabbed: 'bg-blue-500/15 text-blue-300 ring-blue-500/40',
	downloading: 'bg-blue-500/15 text-blue-300 ring-blue-500/40',
	importing: 'bg-purple-500/15 text-purple-300 ring-purple-500/40',
	available: 'bg-green-500/15 text-green-300 ring-green-500/40',
	failed: 'bg-red-500/15 text-red-300 ring-red-500/40'
};
export const NEUTRAL = 'bg-gray-800/70 text-gray-200 ring-gray-700/60';

// Step bar: Requested → Grabbed → Downloading → Importing → Available.
export const STEPS = ['Requested', 'Grabbed', 'Downloading', 'Importing', 'Available'];
export const step = (s: Status) => ({ pending: 0, approved: 0, sent: 0, grabbed: 1, downloading: 2, importing: 3, available: 4 } as Record<string, number>)[s] ?? -1;

// Still moving: worth watching for changes every few seconds.
export const ACTIVE: Status[] = ['pending', 'approved', 'sent', 'grabbed', 'downloading', 'importing'];
export const isActive = (s?: string) => !!s && (ACTIVE as string[]).includes(s);

export const text = (s: Status, progress = 0) => LABEL[s] + (s === 'downloading' && progress ? ` ${progress}%` : '');

// What everyone sees on title pages and on other people's cards: a short set of states. Grabbed and
// Importing fold into Downloading (starting / almost ready); the exact state is for people who manage
// requests (the admin panel and the details window). Expired reads as nothing (Request again).
export type Simple = { label: string; tone: string; step: number } | null;
export function simple(s: Status | string | undefined, progress = 0): Simple {
	switch (s) {
		case 'pending': return { label: 'Requested · waiting for approval', tone: TONE.pending!, step: 0 };
		case 'approved': case 'sent': return { label: 'Requested', tone: NEUTRAL, step: 0 };
		case 'grabbed': return { label: 'Downloading', tone: TONE.downloading!, step: 1 };
		case 'downloading': return { label: progress ? `Downloading ${progress}%` : 'Downloading', tone: TONE.downloading!, step: 1 };
		case 'importing': return { label: 'Almost ready', tone: TONE.downloading!, step: 1 };
		case 'available': return { label: 'Available', tone: TONE.available!, step: 2 };
		case 'failed': return { label: 'Failed', tone: TONE.failed!, step: -1 };
		case 'declined': return { label: 'Declined', tone: NEUTRAL, step: -1 };
	}
	return null;
}
export const SIMPLE_STEPS = ['Requested', 'Downloading', 'Available'];

// The timeline's words for each step.
export const EVENT_TEXT: Record<string, string> = {
	requested: 'Requested', redownload: 'Redownload asked for', approved: 'Approved', declined: 'Declined', sent: 'Looking for a download',
	grabbed: 'Found a download', downloading: 'Downloading', importing: 'Downloaded, adding to Plex', available: 'In Plex',
	failed: 'Failed', expired: 'Removed by Maintainerr', pending: 'Waiting for approval'
};
