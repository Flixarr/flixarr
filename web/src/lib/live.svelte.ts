// One live connection to Flixarr (/api/live) for the whole app: request status, bell alerts, titles added
// to Plex, job runs. Pages listen with on() instead of asking on a timer. The browser reconnects by
// itself and the server replays what was missed; "resync" means reload everything once.
type Handler = (data: any) => void; // eslint-disable-line @typescript-eslint/no-explicit-any

const handlers = new Map<string, Set<Handler>>();
let es: EventSource | null = null;
const TYPES = ['request', 'alerts', 'plex', 'job', 'services', 'resync', 'leaving'];

export function on(type: string, fn: Handler) {
	if (!handlers.has(type)) handlers.set(type, new Set());
	handlers.get(type)!.add(fn);
	return () => handlers.get(type)!.delete(fn);
}

function emit(type: string, data: unknown) {
	for (const fn of handlers.get(type) ?? []) fn(data);
}

// startLive connects (once) while signed in; returns a stop function.
export function startLive() {
	if (es) return () => {};
	es = new EventSource('/api/live');
	for (const t of TYPES) es.addEventListener(t, (m) => emit(t, JSON.parse((m as MessageEvent).data || 'null')));
	// Back from a long sleep (phone locked, laptop closed): the stream may have been gone a while.
	const wake = () => document.visibilityState === 'visible' && es?.readyState === EventSource.CLOSED && restart();
	document.addEventListener('visibilitychange', wake);
	return () => {
		document.removeEventListener('visibilitychange', wake);
		es?.close();
		es = null;
	};
}

function restart() {
	es?.close();
	es = null;
	startLive();
	emit('resync', null);
}
