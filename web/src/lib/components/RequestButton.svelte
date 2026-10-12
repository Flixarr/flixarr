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
	import { TONE, NEUTRAL, text, isActive } from '#lib/requeststatus.ts';
	import RequestModal from './RequestModal.svelte';
	import Modal from './Modal.svelte';
	import { notify } from '#lib/toast.svelte.ts';

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
		let alive = true;
		let t: ReturnType<typeof setTimeout>;
		// Live: every 4 seconds while it's on its way, so the button moves from Requested to Available by itself.
		const load = () =>
			api<Record<string, RequestBrief>>(`/requests/status?keys=${k}`)
				.then((d) => alive && (req = d[k] ?? null))
				.catch(() => alive && req === undefined && (req = null))
				.finally(() => alive && (t = setTimeout(load, isActive(req?.status) ? 4000 : 60000)));
		load();
		return () => { alive = false; clearTimeout(t); };
	});

	// Series in Plex: offer Request more only while some season that has aired isn't fully there.
	let plexSeasons = $state<Record<number, number> | null>(null);
	const hasPlex = $derived(!!req?.plex);
	$effect(() => {
		plexSeasons = null;
		if (item.type === 'series' && hasPlex) api<Record<number, number>>(`/plex/seasons/${item.id}`).then((d) => (plexSeasons = d)).catch(() => {});
	});
	const missingSeasons = $derived.by(() => {
		if (item.type !== 'series' || !plexSeasons) return false;
		const today = new Date().toISOString().slice(0, 10);
		return (item as Series).seasons.some((s) => s.n > 0 && s.episodes > 0 && (!s.air || s.air <= today) && (plexSeasons![s.n] ?? 0) < s.episodes);
	});

	let blockOpen = $state(false);
	async function block() {
		blockOpen = false;
		try {
			await api('/blocklist', { method: 'POST', body: JSON.stringify({ type: item.type, tmdbId: item.id, title: item.title, year: item.year, poster: item.poster ?? '' }) });
			req = { id: 0, status: 'blocked', progress: 0, blocked: true };
			remember(key, req);
			notify(`${item.title} is blocked.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not block it.', 'orange');
		}
	}
	async function unblock() {
		try {
			await api(`/blocklist/${item.type}/${item.id}`, { method: 'DELETE' });
			req = null;
			remember(key, null);
			notify(`${item.title} can be requested again.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not unblock it.', 'orange');
		}
	}


</script>

{#snippet pill(r: RequestBrief)}
	<a href="/requests" class="act inline-flex items-center rounded-lg font-medium ring-1 {TONE[r.status] ?? NEUTRAL}">
		{#if r.status === 'available'}<Icon name="check" class="size-4" />{/if}{r.status === 'pending' ? 'Requested · waiting' : text(r.status, r.progress)}
	</a>
{/snippet}

{#if req?.plex}
	<a href={req.plex} target="_blank" rel="noopener" class="btn-plex act"><PlexMark class="size-4" color="#111827" />Play on Plex</a>
	{#if item.type === 'series'}
		{#if req.status !== 'available'}
			{@render pill(req)}
		{:else if missingSeasons && can('requestTv')}
			<button class="btn-ghost act" onclick={() => (open = true)}><Icon name="plus" class="size-4" />Request more</button>
		{/if}
	{/if}
{:else if req?.blocked}
	<span class="btn-ghost act cursor-default text-gray-400">Can't be requested</span>
	{#if can('admin')}<button class="btn-ghost act" onclick={unblock}>Unblock</button>{/if}
{:else if req && req.status !== 'declined' && req.status !== 'failed'}
	{@render pill(req)}
{:else if req !== undefined && !can(item.type === 'series' ? 'requestTv' : 'requestMovies')}
	<span class="btn-ghost act cursor-default text-gray-400">You can't request {item.type === 'series' ? 'series' : 'movies'}</span>
{:else if req !== undefined}
	<button class="btn act" onclick={() => (open = true)}><Icon name="plus" class="size-4" />Request</button>
	{#if can('admin') && req === null}<button class="btn-ghost act" onclick={() => (blockOpen = true)}>Block</button>{/if}
{/if}
<Modal bind:open={blockOpen} title="Block {item.title}?">
	<p class="p-5 text-sm text-gray-300">Nobody can request it until it's unblocked (Settings → Blocklist).</p>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (blockOpen = false)}>Cancel</button>
		<button class="btn px-6" onclick={block}>Block</button>
	{/snippet}
</Modal>
<RequestModal bind:open {item} onrequested={(r) => { req = { id: r.id, status: r.status, progress: r.progress, plex: req?.plex }; remember(key, req); }} />
