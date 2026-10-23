// The red number on Requests (#141): how many requests need attention for the signed-in person. Asked again
// when a request changes, Radarr/Sonarr status refreshes, or the live connection resyncs.
import { api } from './api.ts';
import { on } from './live.svelte.ts';

let count = $state(0);
export const attention = () => count;

let timer: ReturnType<typeof setTimeout> | undefined;
async function load() {
	try {
		count = (await api<{ count: number }>('/requests/attention')).count;
	} catch {
		/* keep the last number */
	}
}
function soon() {
	clearTimeout(timer);
	timer = setTimeout(load, 600);
}

// watchAttention starts keeping the number current while signed in; returns a stop function.
export function watchAttention() {
	load();
	const offs = [on('request', soon), on('services', soon), on('resync', soon)];
	return () => {
		clearTimeout(timer);
		offs.forEach((off) => off());
	};
}
