// Web Push on this device: the service worker, and subscribing or unsubscribing with the server's key.
import { api } from './api';

export const pushSupported = () => typeof window !== 'undefined' && 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window;

// iPhone and iPad only allow push once Flixarr is added to the home screen.
export const needsHomeScreen = () =>
	typeof window !== 'undefined' && /iPhone|iPad|iPod/.test(navigator.userAgent) && !(navigator as Navigator & { standalone?: boolean }).standalone && !matchMedia('(display-mode: standalone)').matches;

async function registration() {
	return (await navigator.serviceWorker.getRegistration('/')) ?? navigator.serviceWorker.register('/sw.js', { scope: '/' });
}

function keyBytes(b64: string) {
	const s = atob(b64.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - (b64.length % 4)) % 4));
	return Uint8Array.from(s, (c) => c.charCodeAt(0));
}

// subscribed: is this device getting notifications?
export async function subscribed() {
	if (!pushSupported()) return false;
	const reg = await navigator.serviceWorker.getRegistration('/');
	return !!(await reg?.pushManager.getSubscription());
}

export async function subscribe() {
	if ((await Notification.requestPermission()) !== 'granted') throw new Error("Notifications are blocked for Flixarr in this browser's settings.");
	const { key } = await api<{ key: string }>('/push');
	const reg = await registration();
	await navigator.serviceWorker.ready;
	const sub = (await reg.pushManager.getSubscription()) ?? (await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: keyBytes(key) }));
	const j = sub.toJSON();
	await api('/push', { method: 'POST', body: JSON.stringify({ endpoint: j.endpoint, keys: j.keys, device: navigator.userAgent }) });
}

export async function unsubscribe() {
	const reg = await navigator.serviceWorker.getRegistration('/');
	const sub = await reg?.pushManager.getSubscription();
	if (!sub) return;
	const j = sub.toJSON();
	await api('/push', { method: 'DELETE', body: JSON.stringify({ endpoint: j.endpoint, keys: j.keys }) }).catch(() => {});
	await sub.unsubscribe();
}
