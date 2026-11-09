import type { QueueRow, ServiceKind, ServiceView } from '#lib/types.ts';

// What the Services page knows about each kind of server. Connections are stored as a list; for now each
// kind takes one.
export type Kind = { kind: ServiceKind; name: string; what: string; port: number; about: string; color: string; rgb: string };

export const KINDS: Kind[] = [
	{ kind: 'radarr', name: 'Radarr', what: 'movies', port: 7878, about: 'Movie requests are sent here.', color: '#ffc230', rgb: '255 194 48' },
	{ kind: 'sonarr', name: 'Sonarr', what: 'series', port: 8989, about: 'Series requests are sent here.', color: '#35c5f4', rgb: '53 197 244' }
];

export type Services = Record<ServiceKind, ServiceView | null>;

// "connected", "issues", "down" or "checking": one word for how a server is doing.
export function health(v: ServiceView): 'checking' | 'down' | 'issues' | 'ok' {
	const st = v.status;
	if (!st) return 'checking';
	if (!st.connected) return 'down';
	return st.health.length ? 'issues' : 'ok';
}

// Queue summary parts, e.g. 2 downloading · 1 waiting · 3 need attention (that part in yellow, red when something failed).
export function queueLabel(q: QueueRow[]): { text: string; tone: string }[] {
	if (q.length && q.every((r) => r.state === 'paused')) return [{ text: `${q.length} paused`, tone: '' }];
	const n = (f: (r: QueueRow) => boolean) => q.filter(f).length;
	const failed = n((r) => r.state === 'error');
	return ([[n((r) => r.state === 'downloading'), 'downloading', ''], [n((r) => r.state === 'importing'), 'importing', ''], [n((r) => r.state === 'queued'), 'waiting', ''],
		[n((r) => r.state === 'warning' || r.state === 'error'), 'need attention', failed ? 'text-red-400' : 'text-yellow-400'], [n((r) => r.state === 'paused'), 'paused', '']] as [number, string, string][])
		.filter(([c]) => c).map(([c, w, tone]) => ({ text: `${c} ${w === 'need attention' && c === 1 ? 'needs attention' : w}`, tone }));
}
