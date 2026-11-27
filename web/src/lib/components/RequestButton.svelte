<script lang="ts">
	// The Request button on movie and series pages. Once requested it shows where the request is
	// (waiting, requested, downloading, available) instead, and titles already in Plex link straight to it.
	import { api } from '#lib/api.ts';
	import type { Movie, Series, RequestBrief, Request } from '#lib/types.ts';
	import Icon from './Icon.svelte';
	import PlexMark from './PlexMark.svelte';
	import { can } from '#lib/session.svelte.ts';
	import { page } from '$app/state';
	import { remember } from '#lib/availability.svelte.ts';
	import { simple, waitingRelease, isActive } from '#lib/requeststatus.ts';
	import { on } from '#lib/live.svelte.ts';
	import { loadSeasons, noSeasons, episodeCount, type PlexSeasons } from '#lib/seasons.ts';
	import RequestModal from './RequestModal.svelte';
	import RequestDetail from './RequestDetail.svelte';
	import { oneTap, quickRequest } from '#lib/quickrequest.ts';

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
	let detailOpen = $state(false); // the request's own window, opened from the status button
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
	// Once it's in Plex, only after Plex's counts have loaded: until then every season looks missing (#257).
	let plexSeasons = $state<PlexSeasons | null>(null);
	const hasPlex = $derived(!!req?.plex);
	$effect(() => {
		const id = item.id;
		plexSeasons = null;
		if (item.type === 'series' && hasPlex) loadSeasons(id).then((d) => id === item.id && (plexSeasons = d));
	});
	const asked = $derived(req?.asked);
	const moreSeasons = $derived.by(() => {
		if (item.type !== 'series' || asked?.whole || (!asked && !hasPlex)) return false;
		if (hasPlex && (!plexSeasons || plexSeasons.failed)) return false;
		const counts = plexSeasons ?? noSeasons();
		const today = new Date().toISOString().slice(0, 10);
		return (item as Series).seasons.some((s) => s.n > 0 && episodeCount(counts, s) > 0 && (!s.air || s.air <= today) && (counts.inPlex[s.n] ?? 0) < episodeCount(counts, s) && !(asked?.seasons ?? []).includes(s.n));
	});


	// Request: a movie with nothing to choose or show is sent in one tap with Undo (#283); anything else opens the window.
	let checking = $state(false);
	const brief = (r: Request): RequestBrief => ({ id: r.id, status: r.status, progress: r.progress, plex: req?.plex });
	async function start() {
		if (item.type !== 'movie') return void (open = true);
		checking = true;
		const ok = await oneTap(item as Movie).catch(() => null);
		checking = false;
		if (!ok) return void (open = true);
		const before = req;
		req = { id: 0, status: 'approved', progress: 0, plex: req?.plex };
		quickRequest(item as Movie, ok.profileId, (r) => {
			req = r ? brief(r) : before; // a failed one shows Failed with Request again, like the window
			remember(key, req ?? null);
		});
	}
</script>

{#snippet pill(r: RequestBrief)}
	{@const st = simple(r.status, r.progress, item.type === 'series')}
	{#if st}
		<button class="act inline-flex items-center rounded-lg font-medium ring-1 {st.tone}" onclick={() => (detailOpen = true)}>
			{#if r.status === 'available' || r.status === 'partial'}<Icon name="check" class="size-4" />{/if}{st.label}{#if waitingRelease(r)} · not released yet{/if}
		</button>
	{/if}
{/snippet}

{#snippet more()}
	<button class="btn-ghost act" onclick={() => (open = true)}><Icon name="plus" class="size-4" />Request more</button>
{/snippet}

{#if req?.plex}
	<a href={req.plex} target="_blank" rel="noopener" class="btn-plex act"><PlexMark class="size-4" color="#111827" />Play on Plex</a>
	{#if item.type === 'series'}
		{#if isActive(req.status)}{@render pill(req)}{/if}
		{#if moreSeasons && can('requestTv')}{@render more()}{/if}
	{/if}
{:else if req?.blocked}
	<span class="btn-ghost act cursor-default text-gray-400">Can't be requested</span>
{:else if req && req.status !== 'declined' && req.status !== 'failed' && req.status !== 'expired' && req.status !== 'removed'}
	{@render pill(req)}
	{#if moreSeasons && can('requestTv')}{@render more()}{/if}
{:else if req !== undefined && !can(item.type === 'series' ? 'requestTv' : 'requestMovies')}
	<span class="btn-ghost act cursor-default text-gray-400">You can't request {item.type === 'series' ? 'series' : 'movies'}</span>
{:else if req !== undefined}
	{#if req && (req.status === 'failed' || req.status === 'declined')}{@render pill(req)}{/if}
	<button class="btn act" disabled={checking} onclick={start}><Icon name="plus" class="size-4" />{req && ['failed', 'declined', 'expired', 'removed'].includes(req.status) ? 'Request again' : 'Request'}</button>
{/if}
<RequestModal bind:open {item} asked={req?.asked} onrequested={(r) => {
	// The live update brings the title's full state (all its requests) right after; this is the stopgap.
	const seasons = r.mode === 'all' ? [] : [...(req?.asked?.seasons ?? []), ...r.seasons];
	req = { id: r.id, status: r.status, progress: r.progress, plex: req?.plex, asked: item.type === 'series' ? { whole: r.mode === 'all' || !!req?.asked?.whole, seasons } : undefined };
	remember(key, req);
}} />
{#if req?.id}<RequestDetail bind:open={detailOpen} id={req.id} />{/if}
