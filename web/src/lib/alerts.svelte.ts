// Alerts: what the bell shows (inside Flixarr only; notifications are what gets sent out).
import { api } from './api';
import { on } from './live.svelte';

export interface Alert { id: number; key: string; severity: 'info' | 'warning' | 'error'; title: string; detail: string; link: string; startedAt: string; changedAt: string; unread: boolean }

export const alerts = $state<{ list: Alert[] }>({ list: [] });

export const unreadCount = () => alerts.list.filter((a) => a.unread).length;
// Radarr/Sonarr alerts lead to Settings → Services, so the red-dot trail follows them there.
export const servicesUnread = () => alerts.list.some((a) => a.unread && a.key.startsWith('arr:'));

export async function loadAlerts() {
	try {
		alerts.list = (await api<{ alerts: Alert[] }>('/alerts')).alerts;
	} catch {
		/* signed out or offline: keep what's shown */
	}
}

export async function markAlertsSeen() {
	if (!alerts.list.some((a) => a.unread)) return;
	alerts.list = (await api<{ alerts: Alert[] }>('/alerts/seen', { method: 'POST' })).alerts;
}

// Load now, then again whenever the bell changes (pushed over the live connection); returns a stop function.
export function watchAlerts() {
	loadAlerts();
	const a = on('alerts', loadAlerts);
	const b = on('resync', loadAlerts);
	return () => { a(); b(); };
}
