<script lang="ts">
	// The Request button on movie and series pages. Once requested it shows where the request is
	// (waiting, requested, downloading, available) instead, and titles already in Plex link straight to it.
	import { api } from '#lib/api.ts';
	import type { Movie, Series, RequestBrief } from '#lib/types.ts';
	import Icon from './Icon.svelte';
	import PlexMark from './PlexMark.svelte';
	import RequestModal from './RequestModal.svelte';

	// req: undefined while loading, null when never requested and not in Plex. Bindable so the page can show "In your library".
	let { item, req = $bindable() }: { item: Movie | Series; req?: RequestBrief | null } = $props();
	let open = $state(false);
	const key = $derived(`${item.type}:${item.id}`);

	$effect(() => {
		const k = key;
		req = undefined;
		api<Record<string, RequestBrief>>(`/requests/status?keys=${k}`).then((d) => (req = d[k] ?? null)).catch(() => (req = null));
	});

	const TEXT: Record<string, string> = { pending: 'Requested · waiting for approval', approved: 'Requested', sent: 'Requested', downloading: 'Downloading', available: 'Available', failed: 'Request failed', declined: 'Declined' };
	const TONE: Record<string, string> = { available: 'bg-green-500/15 text-green-300 ring-green-500/40', downloading: 'bg-primary/15 text-primary-light ring-primary/40', failed: 'bg-red-500/15 text-red-300 ring-red-500/40' };
</script>

{#if req?.plex}
	<a href={req.plex} target="_blank" rel="noopener" class="btn-plex"><PlexMark class="size-4" color="#111827" />Play on Plex</a>
	{#if item.type === 'series'}
		{#if req.status === 'available'}
			<button class="btn-ghost px-4" onclick={() => (open = true)}><Icon name="plus" class="size-4" />Request more</button>
		{:else}
			<a href="/requests" class="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium ring-1 {TONE[req.status] ?? 'bg-gray-800/70 text-gray-200 ring-gray-700/60'}">{TEXT[req.status]}{req.status === 'downloading' && req.progress ? ` ${req.progress}%` : ''}</a>
		{/if}
	{/if}
{:else if req && req.status !== 'declined' && req.status !== 'failed'}
	<a href="/requests" class="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium ring-1 {TONE[req.status] ?? 'bg-gray-800/70 text-gray-200 ring-gray-700/60'}">
		{#if req.status === 'available'}<Icon name="check" class="size-4" />{/if}{TEXT[req.status]}{req.status === 'downloading' && req.progress ? ` ${req.progress}%` : ''}
	</a>
{:else}
	<button class="btn px-5" onclick={() => (open = true)}><Icon name="plus" class="size-4" />Request</button>
{/if}
<RequestModal bind:open {item} onrequested={(r) => (req = { id: r.id, status: r.status, progress: r.progress, plex: req?.plex })} />
