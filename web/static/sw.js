// Flixarr's service worker: shows Web Push notifications and opens the right page when one is tapped, and
// shows a short message instead of the browser's error page when Flixarr can't be reached. It caches nothing.
self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (e) => e.waitUntil(self.clients.claim()));

// Opening a page: straight from the server; only if that fails, a plain "can't be reached" page.
const OFFLINE = '<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Flixarr</title>' +
	'<style>body{margin:0;min-height:100vh;display:grid;place-items:center;background:#030712;color:#e5e7eb;font:16px system-ui,sans-serif;text-align:center;padding:24px}' +
	'b{display:block;font-size:22px;margin-bottom:8px;color:#fff}button{margin-top:20px;padding:10px 22px;border:0;border-radius:10px;background:#ea580c;color:#fff;font:inherit}</style></head>' +
	'<body><div><b>Flixarr can\'t be reached</b>Check your connection and try again.<br><button onclick="location.reload()">Try again</button></div></body></html>';
self.addEventListener('fetch', (e) => {
	if (e.request.mode !== 'navigate') return;
	e.respondWith(fetch(e.request).catch(() => new Response(OFFLINE, { status: 503, headers: { 'Content-Type': 'text/html; charset=utf-8' } })));
});

self.addEventListener('push', (e) => {
	let d = {};
	try {
		d = e.data ? e.data.json() : {};
	} catch {
		d = { title: 'Flixarr', body: e.data ? e.data.text() : '' };
	}
	e.waitUntil(
		self.registration.showNotification(d.title || 'Flixarr', {
			body: d.body || '',
			icon: '/icon-192.png',
			badge: '/icon-192.png',
			image: d.image,
			tag: d.tag,
			data: { url: d.url || '/' }
		})
	);
});

self.addEventListener('notificationclick', (e) => {
	e.notification.close();
	const url = new URL(e.notification.data?.url || '/', self.location.origin).href;
	e.waitUntil(
		self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((list) => {
			for (const c of list) if (c.url.startsWith(self.location.origin)) return c.navigate(url).then((w) => w?.focus());
			return self.clients.openWindow(url);
		})
	);
});
