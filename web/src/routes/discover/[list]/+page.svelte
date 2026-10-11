<script lang="ts">
	// "See more" page for a Discover row: the whole list as a poster grid, more pages on demand.
	import { page } from '$app/state';
	import { meta } from '#lib/api.ts';
	import { LISTS } from '#lib/lists.ts';
	import type { Card, Page } from '#lib/types.ts';
	import Poster from '#lib/components/Poster.svelte';

	const list = $derived(LISTS.find((l) => l.key === page.params.list));
	let items = $state<Card[]>([]);
	let pageN = $state(1);
	let total = $state(1);
	let busy = $state(false);
	let error = $state('');

	async function load(reset: boolean) {
		if (!list) return;
		busy = true;
		try {
			const p = reset ? 1 : pageN + 1;
			const d = await meta<Page<Card>>('lists/' + list.key, { page: p });
			items = reset ? d.results : [...items, ...d.results.filter((x) => !items.some((y) => y.id === x.id && y.type === x.type))];
			pageN = d.page;
			total = d.totalPages;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load.';
		} finally {
			busy = false;
		}
	}
	$effect(() => {
		if (!list) return;
		document.title = 'Flixarr · ' + list.title;
		items = [];
		error = '';
		load(true);
	});
</script>

<main class="mx-auto max-w-[1600px] space-y-6 pb-28 pt-20 laptop:px-10 laptop:pt-24">
	{#if !list}
		<p class="px-4 pt-10 text-center text-gray-400">That list doesn't exist.</p>
	{:else}
		<div class="px-4 laptop:px-0"><h1 class="text-2xl font-bold laptop:text-3xl">{list.title}</h1></div>
		{#if error && !items.length}<p class="px-4 text-center text-gray-400">{error}</p>{/if}
		<div class="grid grid-cols-3 gap-3 px-4 tablet:grid-cols-4 laptop:grid-cols-6 laptop:px-0 desktop:grid-cols-8">
			{#each items as c (c.type + c.id)}<Poster item={c} class="w-full" />{/each}
		</div>
		{#if pageN < total}
			<div class="flex justify-center"><button class="btn-ghost px-6" disabled={busy} onclick={() => load(false)}>{busy ? 'Loading…' : 'Load more'}</button></div>
		{/if}
	{/if}
</main>
