<script lang="ts">
	// The Request button on movie and series pages. Once requested it shows where the request is
	// (waiting, requested, downloading, available) instead, and titles already in Plex link straight to it.
	import { api } from '#lib/api.ts';
	import type { Movie, Series, RequestBrief } from '#lib/types.ts';
	import Icon from './Icon.svelte';
	import PlexMark from './PlexMark.svelte';
	import { can } from '#lib/session.svelte.ts';
	import { page } from '$app/state';
	import { remember } from '#lib/availability.svelte.ts';
	import { simple } from '#lib/requeststatus.ts';
	import { on } from '#lib/live.svelte.ts';
	import RequestModal from './RequestModal.svelte';

	// req: undefined while loading, null when never requested and not in Plex. Bindable so the page can show "In your library".
	let { item, req = $bindable() }: { item: Movie | Series; req?: RequestBrief | null } = $props();
	// ?request=1 (the Discover spotlight's Request) opens the window once the status says it can be requested.
	let autoOpened = false;
	$effect(() => {
		if (req === null && !autoOpened && page.url.searchParams.get('request') === '1' && can(item.type === 'series' ? 'requestTv' : 'requestMovies')) {
			autoOpened = true;
			open = true;
		}
	});
	let open = $state(false);
	const key = $derived(`${item.type}:${item.id}`);

	$effect(() => {
		const k = key;
		req = undefined;
		const load = () => api<Record<string, RequestBrief>>(`/requests/status?keys=${k}`).then((d) => (req = d[k] ?? null)).catch(() => req === undefined && (req = null));
		load();
		// Live: the request moves along (Grabbed, Downloading %, Available) without asking again.
		const offs = [
			on('request', (d: { key: string; brief: RequestBrief | null }) => d.key === k && (req = d.brief ? { ...d.brief, plex: d.brief.plex ?? req?.plex } : null)),
			on('plex', (keys: string[]) => keys.includes(k) && load()),
			on('resync', load)
		];
		return () => offs.forEach((off) => off());
	});

	// Series: offer Request more while some season that has aired is neither fully in Plex nor asked for.
	let plexSeasons = $state<Record<number, number>>({});
	const hasPlex = $derived(!!req?.plex);
	$effect(() => {
		plexSeasons = {};
		if (item.type === 'series' && hasPlex) api<Record<number, number>>(`/plex/seasons/${item.id}`).then((d) => (plexSeasons = d)).catch(() => {});
	});
	const asked = $derived(req?.asked);
	const moreSeasons = $derived.by(() => {
		if (item.type !== 'series' || asked?.whole || (!asked && !hasPlex)) return false;
		const today = new Date().toISOString().slice(0, 10);
		return (item as Series).seasons.some((s) => s.n > 0 && s.episodes > 0 && (!s.air || s.air <= today) && (plexSeasons[s.n] ?? 0) < s.episodes && !(asked?.seasons ?? []).includes(s.n));
	});

</script>

{#snippet pill(r: RequestBrief)}
	{@const st = simple(r.status, r.progress)}
	{#if st}
		<a href="/requests" class="act inline-flex items-center rounded-lg font-medium ring-1 {st.tone}">
			{#if r.status === 'available'}<Icon name="check" class="size-4" />{/if}{st.label}
		</a>
	{/if}
{/snippet}

{#snippet more()}
	<button class="btn-ghost act" onclick={() => (open = true)}><Icon name="plus" class="size-4" />Request more</button>
{/snippet}

{#if req?.plex}
	<a href={req.plex} target="_blank" rel="noopener" class="btn-plex act"><PlexMark class="size-4" color="#111827" />Play on Plex</a>
	{#if item.type === 'series'}
		{#if req.status !== 'available'}{@render pill(req)}{/if}
		{#if moreSeasons && can('requestTv')}{@render more()}{/if}
	{/if}
{:else if req?.blocked}
	<span class="btn-ghost act cursor-default text-gray-400">Can't be requested</span>
{:else if req && req.status !== 'declined' && req.status !== 'failed' && req.status !== 'expired'}
	{@render pill(req)}
	{#if moreSeasons && can('requestTv')}{@render more()}{/if}
{:else if req !== undefined && !can(item.type === 'series' ? 'requestTv' : 'requestMovies')}
	<span class="btn-ghost act cursor-default text-gray-400">You can't request {item.type === 'series' ? 'series' : 'movies'}</span>
{:else if req !== undefined}
	{#if req && (req.status === 'failed' || req.status === 'declined')}{@render pill(req)}{/if}
	<button class="btn act" onclick={() => (open = true)}><Icon name="plus" class="size-4" />{req && ['failed', 'declined', 'expired'].includes(req.status) ? 'Request again' : 'Request'}</button>
{/if}
<RequestModal bind:open {item} asked={req?.asked} onrequested={(r) => {
	// The live update brings the title's full state (all its requests) right after; this is the stopgap.
	const seasons = r.mode === 'all' ? [] : [...(req?.asked?.seasons ?? []), ...r.seasons];
	req = { id: r.id, status: r.status, progress: r.progress, plex: req?.plex, asked: item.type === 'series' ? { whole: r.mode === 'all' || !!req?.asked?.whole, seasons } : undefined };
	remember(key, req);
}} />
