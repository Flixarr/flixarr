// The red numbers on Requests (#141) and Issues (#192): how many requests need attention for the signed-in
// person, and open issues for people who manage them. Asked again when a request changes, Radarr/Sonarr status
// refreshes, the bell changes (issues reported or resolved), or the live connection resyncs.
import { api } from './api.ts';
import { on } from './live.svelte.ts';

let count = $state(0);
let issues = $state(0);
export const attention = () => count;
export const openIssues = () => issues;

let timer: ReturnType<typeof setTimeout> | undefined;
async function load() {
	try {
		({ count, issues = 0 } = await api<{ count: number; issues?: number }>('/requests/attention'));
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
	const offs = [on('request', soon), on('services', soon), on('alerts', soon), on('resync', soon)];
	return () => {
		clearTimeout(timer);
		offs.forEach((off) => off());
	};
}
