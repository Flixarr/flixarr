// Alerts: the bell (#93; inside Flixarr only, notifications are what gets sent out). Red = something new
// since the bell was last opened; the number = alerts not tapped yet.
import { api } from './api';
import { on } from './live.svelte';
import { alertToast } from './toast.svelte';

export interface Alert { id: number; key: string; kind: 'news' | 'todo'; title: string; detail: string; link: string; poster: string; at: string; tapped: boolean; new: boolean }

export const alerts = $state<{ list: Alert[] }>({ list: [] });

export const untapped = () => alerts.list.filter((a) => !a.tapped).length;
export const anyNew = () => alerts.list.some((a) => a.new);
// Radarr/Sonarr problems lead to Settings → Services, so the red-dot trail follows them there.
export const servicesUnread = () => alerts.list.some((a) => !a.tapped && a.key.startsWith('arr:'));

// What the bell held already (id and time), so only alerts that arrive or come back later pop up as a toast
// (#121); null until the first load, which never pops anything.
let known: Set<string> | null = null;
const mark = (a: Alert) => `${a.id}:${a.at}`;

// Alerts of the same kind (same title) that came in close together show as one group (#185): "Left Plex · 12
// titles", opening to the list. A single alert stays as it is. The list is newest first.
export type Shown = { one: Alert } | { title: string; list: Alert[] };
const TOGETHER = 30 * 60 * 1000;
export function grouped(list: Alert[] = alerts.list): Shown[] {
	// Other alerts can land in between; a group takes every alert of its kind within half an hour of its last.
	const out: Alert[][] = [];
	const open = new Map<string, Alert[]>();
	for (const a of list) {
		const k = `${a.kind}|${a.title}`;
		const g = open.get(k);
		if (g && Math.abs(Date.parse(g[g.length - 1].at) - Date.parse(a.at)) <= TOGETHER) g.push(a);
		else {
			const fresh = [a];
			open.set(k, fresh);
			out.push(fresh);
		}
	}
	return out.map((g) => (g.length === 1 ? { one: g[0] } : { title: g[0].title, list: g }));
}
// "Heat, Alien and 10 more"
export const names = (list: Alert[]) => {
	const t = [...new Set(list.map((a) => a.detail.split(' · ')[0]))];
	return t.slice(0, 2).join(', ') + (t.length > 2 ? ` and ${t.length - 2} more` : '');
};

async function set(p: Promise<{ alerts: Alert[] }>) {
	try {
		alerts.list = (await p).alerts;
		// New ones pop up as toasts, one per group, not one per alert.
		const fresh = known ? alerts.list.filter((a) => a.new && !a.tapped && !known!.has(mark(a))) : [];
		for (const g of grouped(fresh)) {
			if ('one' in g) alertToast(g.one.title, g.one.detail, g.one.link, () => tapAlert(g.one.id));
			else alertToast(`${g.title} · ${g.list.length}`, names(g.list), '', () => tapAlerts(g.list.map((a) => a.id)));
		}
		known = new Set(alerts.list.map(mark));
	} catch {
		/* signed out or offline: keep what's shown */
	}
	show();
}

export const loadAlerts = () => set(api('/alerts'));
// Opening the bell: nothing in it is new any more (the number stays).
export function openedBell() {
	if (anyNew()) set(api('/alerts/opened', { method: 'POST' }));
}
export const tapAlert = (id: number) => set(api(`/alerts/${id}/tap`, { method: 'POST' }));
// Tapping a group taps every alert in it, then reads the bell once.
export async function tapAlerts(ids: number[]) {
	for (const id of ids) await api(`/alerts/${id}/tap`, { method: 'POST' }).catch(() => {});
	loadAlerts();
}
export const readAllAlerts = () => set(api('/alerts/read-all', { method: 'POST' }));
// Arriving somewhere by another way counts as tapping its alerts (Settings → Services for server problems).
export function tapWhere(prefix: string) {
	for (const a of alerts.list) if (!a.tapped && a.key.startsWith(prefix)) tapAlert(a.id);
}

// The number in the tab title (pages set their own title, so this runs after each navigation too) ("(2) Flixarr") and on the home-screen app icon where phones support it.
export function show() {
	const n = untapped();
	if (typeof document === 'undefined') return;
	document.title = document.title.replace(/^\(\d+\+?\) /, '');
	if (n) document.title = `(${n > 99 ? '99+' : n}) ${document.title}`;
	const nav = navigator as Navigator & { setAppBadge?: (n?: number) => Promise<void>; clearAppBadge?: () => Promise<void> };
	(n ? nav.setAppBadge?.(n) : nav.clearAppBadge?.())?.catch(() => {});
}

// Load now, then again whenever the bell changes (pushed over the live connection); returns a stop function.
export function watchAlerts() {
	loadAlerts();
	const a = on('alerts', loadAlerts);
	const b = on('resync', loadAlerts);
	return () => { a(); b(); };
}
