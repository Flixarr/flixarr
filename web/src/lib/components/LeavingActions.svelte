<script lang="ts">
	// On a title's page: when Maintainerr removes it, and what you can do about it. People who manage
	// requests keep it, give it more time, or let it go; everyone else can ask to keep it.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { can } from '#lib/session.svelte.ts';
	import { on } from '#lib/live.svelte.ts';
	import Icon from './Icon.svelte';

	type Info = { at?: string; collection?: string; kept?: 'kept' | 'fresh'; keptUntil?: string; asked?: boolean; askedBy?: string[] };
	let { type, id, title }: { type: 'movie' | 'series'; id: number; title: string } = $props();
	let info = $state<Info | null>(null);
	let busy = $state('');
	const key = $derived(`${type}:${id}`);
	const manage = $derived(can('manageRequests'));

	$effect(() => {
		const k = key;
		const load = () => api<{ connected: boolean; titles?: Record<string, Info> }>(`/leaving?keys=${k}`).then((d) => (info = d.titles?.[k] ?? null)).catch(() => {});
		load();
		return on('leaving', (keys: string[]) => keys.includes(k) && load());
	});

	const day = (s?: string) => (s ? new Date(s).toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) : '');
	const days = $derived(info?.at ? Math.ceil((new Date(info.at).getTime() - Date.now()) / 864e5) : 0);
	const when = $derived(days === 0 ? 'today' : days === 1 ? 'tomorrow' : `in ${days} days`);

	async function act(what: 'keep' | 'letgo' | 'postpone' | 'ask', done: string) {
		busy = what;
		try {
			await api(`/leaving/${type}/${id}/${what}`, { method: 'POST', body: JSON.stringify({ days: 30 }) });
			notify(done);
			info = (await api<{ titles?: Record<string, Info> }>(`/leaving?keys=${key}`)).titles?.[key] ?? null;
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not do that.', 'orange');
		} finally {
			busy = '';
		}
	}
</script>

{#if info?.at && !info.kept}
	<div class="flex w-full basis-full flex-wrap items-center gap-2 rounded-lg bg-yellow-500/10 px-3 py-2 text-sm text-yellow-200 ring-1 ring-yellow-500/30">
		<Icon name="clock" class="size-4 shrink-0" />
		<span class="min-w-0 flex-1">{#if days < 0}{manage ? `Was due to leave Plex ${day(info.at)}; Maintainerr hasn't removed it` : 'Leaving Plex any day now'}{:else}Leaves Plex {when} ({day(info.at)}){/if}{#if info.askedBy?.length}<span class="text-yellow-300/80"> · {info.askedBy.join(', ')} asked to keep it</span>{/if}</span>
		{#if manage}
			<button class="btn px-3 py-1 text-xs" disabled={!!busy} onclick={() => act('keep', `${title} stays: Maintainerr's rules skip it now.`)}>{busy === 'keep' ? 'Keeping…' : 'Keep'}</button>
			<button class="btn-ghost px-3 py-1 text-xs" disabled={!!busy} onclick={() => act('postpone', `${title} gets 30 more days.`)}>{busy === 'postpone' ? 'Saving…' : '30 more days'}</button>
		{:else if info.asked}
			<span class="text-xs text-yellow-300/80">You asked to keep it</span>
		{:else if can(type === 'series' ? 'requestTv' : 'requestMovies')}
			<button class="btn-ghost px-3 py-1 text-xs" disabled={!!busy} onclick={() => act('ask', 'Asked. Someone who manages requests decides.')}>{busy === 'ask' ? 'Asking…' : 'Ask to keep'}</button>
		{/if}
	</div>
{:else if info?.kept && manage}
	<div class="flex w-full basis-full flex-wrap items-center gap-2 rounded-lg bg-gray-800/60 px-3 py-2 text-xs text-gray-300 ring-1 ring-gray-700/60">
		<Icon name="check" class="size-4 shrink-0 text-green-400" />
		<span class="flex-1">{info.kept === 'fresh' ? `Kept out of Maintainerr's rules until ${day(info.keptUntil)} (a new request)` : "Kept out of Maintainerr's rules"}</span>
		<button class="btn-ghost px-3 py-1 text-xs" disabled={!!busy} onclick={() => act('letgo', `Maintainerr's rules see ${title} again.`)}>{busy === 'letgo' ? 'Saving…' : 'Let it go'}</button>
	</div>
{/if}
