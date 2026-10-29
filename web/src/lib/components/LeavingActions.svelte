<script lang="ts">
	// When Maintainerr removes a title, and what you can do about it. People who manage requests keep it,
	// give it more time, or let it go; everyone else can ask to keep it. On title pages it's a small chip
	// that opens a menu (chip, #169); on the Leaving soon page a box.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { can } from '#lib/session.svelte.ts';
	import { on } from '#lib/live.svelte.ts';
	import Icon from './Icon.svelte';

	type Info = { at?: string; collection?: string; kept?: 'kept' | 'fresh'; keptUntil?: string; asked?: boolean; askedBy?: string[] };
	let { type, id, title, chip = false }: { type: 'movie' | 'series'; id: number; title: string; chip?: boolean } = $props();
	let menu = $state(false);
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
	const longDay = (s?: string) => (s ? new Date(s).toLocaleDateString(undefined, { weekday: 'long', month: 'long', day: 'numeric' }) : '');

	async function act(what: 'keep' | 'letgo' | 'postpone' | 'ask', done: string) {
		busy = what;
		menu = false;
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

<svelte:window onclick={() => (menu = false)} />

{#snippet item(label: string, what: 'keep' | 'letgo' | 'postpone' | 'ask', done: string)}
	<button class="w-full rounded-lg px-3 py-2 text-left hover:bg-white/5 disabled:opacity-50" disabled={!!busy} onclick={() => act(what, done)}>{busy === what ? 'Working…' : label}</button>
{/snippet}

{#if chip && info?.at && !info.kept}
	<span class="relative">
		<button class="inline-flex items-center gap-1 rounded-md bg-yellow-500/15 px-1.5 py-0.5 font-medium text-yellow-200 ring-1 ring-yellow-500/40" onclick={(e) => (e.stopPropagation(), (menu = !menu))}>
			<Icon name="clock" class="size-3" />{days < 0 ? 'Leaving any day' : `Leaves ${day(info.at)}`}<Icon name="chevron" class="size-3 rotate-90" />
		</button>
		{#if menu}
			<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
			<div class="absolute left-0 top-full z-30 mt-2 w-60 rounded-xl border border-gray-700/60 bg-gray-900 p-1.5 text-sm shadow-2xl" onclick={(e) => e.stopPropagation()}>
				<div class="px-3 pb-1.5 pt-1 text-[11px] text-gray-400">{days < 0 ? 'Leaves Plex any day now' : `Leaves Plex on ${longDay(info.at)}`}{#if info.askedBy?.length}. {info.askedBy.join(', ')} asked to keep it{/if}</div>
				{#if manage}
					{@render item('Keep forever', 'keep', `${title} stays: Maintainerr's rules skip it now.`)}
					{@render item('Extend 30 days', 'postpone', `${title} gets 30 more days.`)}
				{:else if info.asked}
					<div class="px-3 py-2 text-gray-400">You asked to keep it</div>
				{:else if can(type === 'series' ? 'requestTv' : 'requestMovies')}
					{@render item('Ask to keep it', 'ask', 'Asked. Someone who manages requests decides.')}
				{/if}
			</div>
		{/if}
	</span>
{:else if chip && info?.kept && manage}
	<span class="relative">
		<button class="inline-flex items-center gap-1 rounded-md bg-gray-900/70 px-1.5 py-0.5 text-gray-300 ring-1 ring-gray-700/60" onclick={(e) => (e.stopPropagation(), (menu = !menu))}>
			<Icon name="check" class="size-3 text-green-400" />Kept<Icon name="chevron" class="size-3 rotate-90" />
		</button>
		{#if menu}
			<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
			<div class="absolute left-0 top-full z-30 mt-2 w-60 rounded-xl border border-gray-700/60 bg-gray-900 p-1.5 text-sm shadow-2xl" onclick={(e) => e.stopPropagation()}>
				<div class="px-3 pb-1.5 pt-1 text-[11px] text-gray-400">{info.kept === 'fresh' ? `Kept out of Maintainerr's rules until ${day(info.keptUntil)} (a new request)` : "Kept out of Maintainerr's rules"}</div>
				{@render item('Let it go', 'letgo', `Maintainerr's rules see ${title} again.`)}
			</div>
		{/if}
	</span>
{:else if chip}
	<!-- nothing leaving -->
{:else if info?.at && !info.kept}
	<div class="flex w-full basis-full flex-wrap items-center gap-2 rounded-lg bg-yellow-500/10 px-3 py-2 text-sm text-yellow-200 ring-1 ring-yellow-500/30">
		<Icon name="clock" class="size-4 shrink-0" />
		<span class="min-w-0 flex-1">{#if days < 0}Leaving Plex any day now{:else}Leaves Plex {when} ({day(info.at)}){/if}{#if info.askedBy?.length}<span class="text-yellow-300/80"> · {info.askedBy.join(', ')} asked to keep it</span>{/if}</span>
		{#if manage}
			<button class="btn px-3 py-1 text-xs" disabled={!!busy} onclick={() => act('keep', `${title} stays: Maintainerr's rules skip it now.`)}>{busy === 'keep' ? 'Keeping…' : 'Keep forever'}</button>
			<button class="btn-ghost px-3 py-1 text-xs" disabled={!!busy} onclick={() => act('postpone', `${title} gets 30 more days.`)}>{busy === 'postpone' ? 'Saving…' : 'Extend 30 days'}</button>
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
