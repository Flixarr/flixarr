<script lang="ts">
	// Leaving soon: every title Maintainerr is about to remove, soonest first. People who manage requests
	// see who asked to keep what (those first) and decide right here.
	import { api } from '#lib/api.ts';
	import type { Card } from '#lib/types.ts';
	import Img from '#lib/components/Img.svelte';
	import LeavingActions from '#lib/components/LeavingActions.svelte';
	import { can } from '#lib/session.svelte.ts';
	import { on } from '#lib/live.svelte.ts';

	type Item = { card: Card; leaving: { at: string; collection: string; askedBy?: string[] } };
	let items = $state<Item[] | null>(null);
	let connected = $state(true);
	const load = () => api<{ connected: boolean; results: Item[] }>('/leaving').then((d) => ((connected = d.connected), (items = d.results)));
	$effect(() => {
		document.title = 'Flixarr · Leaving soon';
		load().catch(() => (items = []));
		return on('leaving', () => load().catch(() => {}));
	});
	const asked = $derived(items?.filter((i) => i.leaving.askedBy?.length).length ?? 0);
</script>

<main class="mx-auto max-w-[1200px] px-4 pb-28 pt-20 laptop:px-10 laptop:pt-24">
	<h1 class="text-2xl font-bold laptop:text-3xl">Leaving soon</h1>
	<p class="text-sm text-gray-400">
		{#if !connected}Maintainerr isn't connected, so Flixarr doesn't know what's leaving.{:else}Titles Maintainerr removes from Plex soon, soonest first.{can('manageRequests') && asked ? ` ${asked} ${asked === 1 ? 'has' : 'have'} been asked to stay.` : ''}{/if}
	</p>
	{#if !items}
		<p class="mt-8 text-sm text-gray-400">Loading…</p>
	{:else if connected && !items.length}
		<div class="card mt-6 p-8 text-center text-sm text-gray-400">Nothing is leaving right now.</div>
	{:else}
		<div class="mt-5 space-y-3">
			{#each items as it (it.card.type + it.card.id)}
				<div class="card flex gap-4 p-3">
					<a href="/{it.card.type}/{it.card.id}" class="block w-16 shrink-0 self-start overflow-hidden rounded-lg ring-1 ring-gray-700/50 tablet:w-20">{#if it.card.poster}<Img path={it.card.poster} kind="poster" sizes="80px" class="aspect-[2/3] w-full" />{:else}<div class="aspect-[2/3] bg-gray-800"></div>{/if}</a>
					<div class="min-w-0 flex-1 space-y-2">
						<div>
							<a href="/{it.card.type}/{it.card.id}" class="font-semibold hover:underline">{it.card.title}</a>
							<span class="text-sm text-gray-400">{it.card.year ?? ''}</span>
							<div class="text-xs text-gray-500">{it.leaving.collection}</div>
						</div>
						<div class="flex flex-wrap"><LeavingActions type={it.card.type as 'movie' | 'series'} id={it.card.id} title={it.card.title} /></div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</main>
