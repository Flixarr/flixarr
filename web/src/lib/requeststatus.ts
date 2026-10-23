// How a request's status reads everywhere (button, Requests page, posters).
import type { Request } from './types';

type Status = Request['status'] | 'blocked';

export const LABEL: Record<Status, string> = {
	pending: 'Waiting for approval', approved: 'Requested', sent: 'Requested', notout: 'Not out yet', grabbed: 'Grabbed', downloading: 'Downloading',
	importing: 'Importing', available: 'Available', failed: 'Failed', declined: 'Declined', expired: 'Expired', removed: 'Removed', blocked: "Can't be requested"
};

// Pill colours: [background, text, ring].
export const TONE: Partial<Record<Status, string>> = {
	pending: 'bg-yellow-500/15 text-yellow-300 ring-yellow-500/40',
	notout: 'bg-sky-500/15 text-sky-300 ring-sky-500/40',
	grabbed: 'bg-blue-500/15 text-blue-300 ring-blue-500/40',
	downloading: 'bg-blue-500/15 text-blue-300 ring-blue-500/40',
	importing: 'bg-purple-500/15 text-purple-300 ring-purple-500/40',
	available: 'bg-green-500/15 text-green-300 ring-green-500/40',
	failed: 'bg-red-500/15 text-red-300 ring-red-500/40'
};
export const NEUTRAL = 'bg-gray-800/70 text-gray-200 ring-gray-700/60';

// Step bar: Requested → Grabbed → Downloading → Importing → Available.
export const STEPS = ['Requested', 'Grabbed', 'Downloading', 'Importing', 'Available'];
export const step = (s: Status) => ({ pending: 0, approved: 0, sent: 0, notout: 0, grabbed: 1, downloading: 2, importing: 3, available: 4 } as Record<string, number>)[s] ?? -1;

// Still moving: worth watching for changes every few seconds.
export const ACTIVE: Status[] = ['pending', 'approved', 'sent', 'notout', 'grabbed', 'downloading', 'importing'];
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
		case 'notout': return { label: 'Not out yet', tone: TONE.notout!, step: 0 };
		case 'grabbed': return { label: 'Downloading', tone: TONE.downloading!, step: 1 };
		case 'downloading': return { label: progress ? `Downloading ${progress}%` : 'Downloading', tone: TONE.downloading!, step: 1 };
		case 'importing': return { label: 'Almost ready', tone: TONE.downloading!, step: 1 };
		case 'available': return { label: 'Available', tone: TONE.available!, step: 2 };
		case 'failed': return { label: 'Failed', tone: TONE.failed!, step: -1 };
		case 'declined': return { label: 'Declined', tone: NEUTRAL, step: -1 };
		case 'removed': return { label: 'Removed', tone: NEUTRAL, step: -1 };
	}
	return null;
}
export const SIMPLE_STEPS = ['Requested', 'Downloading', 'Available'];

// The timeline's words for each step.
export const EVENT_TEXT: Record<string, string> = {
	requested: 'Requested', redownload: 'Redownload asked for', approved: 'Approved', declined: 'Declined', sent: 'Looking for a download',
	grabbed: 'Found a download', downloading: 'Downloading', importing: 'Downloaded, adding to Plex', available: 'In Plex',
	failed: 'Failed', expired: 'Removed from Plex', removed: 'Removed', pending: 'Waiting for approval', notout: 'Not out yet'
};

// What a request asked for: "Whole show", "Season 2", "Seasons 1, 3", "4 episodes" ('' for movies).
export const askedFor = (r: Pick<Request, 'type' | 'mode' | 'seasons' | 'episodes'>) =>
	r.type !== 'series' ? '' : r.mode === 'all' ? 'Whole show' : r.mode === 'episodes' ? `${r.episodes.length} episodes` : r.seasons.length === 1 ? `Season ${r.seasons[0]}` : `Seasons ${r.seasons.join(', ')}`;

// A timeline dot's colour for a request step (yellow: someone pressed Fix).
export const eventDot = (s: string) => (s === 'failed' ? 'bg-red-500' : s === 'fix' ? 'bg-yellow-500' : s === 'notout' ? 'bg-sky-500' : s === 'available' ? 'bg-green-500' : ['declined', 'expired', 'removed'].includes(s) ? 'bg-gray-500' : 'bg-primary');

// When a Not out yet request comes out, in plain words (#151). rel is "kind:date" from the server;
// seasons (one picked season) makes a series read "Season 3 starts Jan 8".
export function releaseText(rel?: string, seasons: number[] = [], now = new Date()): string {
	if (!rel) return '';
	const [kind, d] = rel.split(':');
	const day = d ? new Date(d + 'T12:00:00') : null;
	const when = day ? day.toLocaleDateString('en-US', { month: 'short', day: 'numeric', ...(day.getFullYear() !== now.getFullYear() ? { year: 'numeric' } : {}) }) : '';
	const past = !!day && day.getTime() < now.getTime();
	switch (kind) {
		case 'digital': return `Out on digital ${when}`;
		case 'disc': return `Out on disc ${when}`;
		case 'theaters': return past ? `In theaters since ${when}. Movies usually reach digital 1 to 3 months later.` : `In theaters ${when}`;
		case 'air': return seasons.length === 1 ? `Season ${seasons[0]} starts ${when}` : `Starts ${when}`;
	}
	return 'No release date yet';
}

// Before requesting a movie: when it comes out, if it isn't out yet ("" once it's out or unknown).
export function upcomingText(dates?: { theaters: string | null; digital: string | null; disc: string | null } | null, now = new Date()): string {
	if (!dates) return '';
	const today = now.toISOString().slice(0, 10);
	const fmt = (d: string) => new Date(d + 'T12:00:00').toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
	if (dates.digital && dates.digital >= today) return `Comes out on digital ${fmt(dates.digital)}. It downloads then on its own.`;
	if (dates.digital || (dates.disc && dates.disc < today)) return '';
	if (dates.disc && dates.disc >= today) return `Comes out on disc ${fmt(dates.disc)}. It downloads then on its own.`;
	if (dates.theaters && dates.theaters >= today) return `In theaters ${fmt(dates.theaters)}. It downloads once it's out on digital, usually 1 to 3 months later.`;
	// In theaters lately with no digital date yet; long ago with no dates is just missing data.
	if (dates.theaters && now.getTime() - new Date(dates.theaters + 'T12:00:00').getTime() < 120 * 86400e3) return `In theaters now. It downloads once it's out on digital, usually 1 to 3 months after theaters.`;
	return '';
}
