import type { ArrStatus, QueueRow, ServiceKind, ServiceView } from '#lib/types.ts';
import { ago } from '#lib/api.ts';

// What the Services page knows about each kind of server. Connections are stored as a list; for now each
// kind takes one.
export type Kind = { kind: ServiceKind; name: string; what: string; port: number; about: string; color: string; rgb: string };

export const KINDS: Kind[] = [
	{ kind: 'radarr', name: 'Radarr', what: 'movies', port: 7878, about: 'Movie requests are sent here.', color: '#ffc230', rgb: '255 194 48' },
	{ kind: 'sonarr', name: 'Sonarr', what: 'series', port: 8989, about: 'Series requests are sent here.', color: '#35c5f4', rgb: '53 197 244' }
];

export type Services = Record<ServiceKind, ServiceView | null>;

// Maintainerr as Settings → Services reads it (GET /api/maintainerr).
export type MaintainerrStat = { id: number; title: string; type: string; active: boolean; deleteAfterDays: number; leaving: number; handled: number; handledBytes: number };
export type MaintainerrInfo = {
	connected: boolean; url?: string; keySet?: boolean; version?: string; error?: string; webhook?: boolean; protectDays: number; stats: MaintainerrStat[]; leaving?: number; keeps?: number;
};
/** Maintainerr has been set up (it may still be failing to answer). */
export const maintainerrOn = (m: MaintainerrInfo | null) => !!m?.url;

// "connected", "issues", "down" or "checking": one word for how a server is doing.
export function health(v: ServiceView): 'checking' | 'down' | 'issues' | 'ok' {
	const st = v.status;
	if (!st) return 'checking';
	if (!st.connected) return 'down';
	return st.health.length ? 'issues' : 'ok';
}

export const isStuck = (q: QueueRow) => q.state === 'warning' || q.state === 'error';
/** Sonarr couldn't import it automatically because it matched the series by ID: only a manual import in Sonarr helps. */
export const needsManualImport = (q: QueueRow) => /by ID/i.test(q.message ?? '');
export const shortVersion = (x?: string) => (x ? 'v' + x.split('.').slice(0, 2).join('.') : '');
export const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`;
export const lowerFirst = (s: string) => s.charAt(0).toLowerCase() + s.slice(1);

// "Next update in 42 sec" normally; "Updates paused" when the job is off; "Last updated X ago" (warn) when the
// last read is overdue. title is the other wording, for the tooltip.
export function updateText(v: ServiceView, now: number): { text: string; title: string; warn: boolean } {
	const st = v.status!;
	const age = `Updated ${ago(st.updatedAt, now)}`;
	if (v.updatesPaused) return { text: 'Updates paused', title: age, warn: false };
	if (now - Date.parse(st.updatedAt) > 420000) return { text: `Last updated ${ago(st.updatedAt, now)}, overdue`, title: 'The refresh-status job is overdue', warn: true }; // idle servers are read every 5 min
	const secs = v.nextUpdate ? Math.round((Date.parse(v.nextUpdate) - now) / 1000) : -1;
	if (secs <= 0) return { text: 'Updating…', title: age, warn: false };
	const left = secs < 60 ? `${secs} sec` : secs < 3600 ? `${Math.round(secs / 60)} min` : `${Math.round(secs / 3600)} hr`;
	return { text: `Next update in ${left}`, title: age, warn: false };
}

// Storage by drive, not by app. Containers mount the same disk at different paths, so folders are matched
// by their free space (read in the same refresh, within about 1 GB) instead of their path. A drive whose size
// nobody reports shows free space only.
export type Drive = { kinds: ServiceKind[]; folders: string[]; free: number; total: number; used: number };
const SAME_DRIVE = 1e9;
export function drives(saved: Services): Drive[] {
	const out: Drive[] = [];
	for (const k of KINDS) {
		const st: ArrStatus | null | undefined = saved[k.kind]?.status;
		if (!st?.connected || !st.folder) continue;
		const same = out.filter((d) => Math.abs(d.free - st.freeSpace) <= SAME_DRIVE);
		if (same.length === 1) {
			const d = same[0];
			d.kinds.push(k.kind);
			d.folders.push(st.folder);
			if (!d.total && st.totalSpace) d.total = st.totalSpace;
		} else out.push({ kinds: [k.kind], folders: [st.folder], free: st.freeSpace, total: st.totalSpace, used: 0 });
	}
	for (const d of out) d.used = d.total ? Math.round((1 - d.free / d.total) * 100) : 0;
	return out;
}
export const driveName = (d: Drive) => (d.kinds.length > 1 ? 'Movies and series' : d.kinds[0] === 'radarr' ? 'Movies' : 'Series');
