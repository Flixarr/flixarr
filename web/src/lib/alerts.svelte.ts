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

async function set(p: Promise<{ alerts: Alert[] }>) {
	try {
		alerts.list = (await p).alerts;
		if (known) for (const a of alerts.list) if (a.new && !a.tapped && !known.has(mark(a))) alertToast(a.title, a.detail, a.link, () => tapAlert(a.id));
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
