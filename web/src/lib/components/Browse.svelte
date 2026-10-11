<script lang="ts">
	// Movies / Series browse: genre pills, sort, poster grid, "Load more".
	import { meta } from '#lib/api.ts';
	import type { Card, Page } from '#lib/types.ts';
	import Poster from './Poster.svelte';
	import Chips from './Chips.svelte';

	let { type }: { type: 'movie' | 'series' } = $props();
	const label = $derived(type === 'series' ? 'Series' : 'Movies');
	let genres = $state<{ id: number; name: string }[]>([]);
	let genre = $state<string | number>(0);
	let sort = $state('popular');
	let items = $state<Card[]>([]);
	let pageN = $state(1);
	let total = $state(1);
	let busy = $state(false);

	$effect(() => {
		const show = (g: { genres: { id: number; name: string }[] }) => (genres = g.genres);
		meta('genres/' + type, {}, show).then(show);
	});
	async function load(reset: boolean) {
		busy = true;
		const p = reset ? 1 : pageN + 1;
		// No onFresh here: swapping pages under "Load more" would jump the grid. The background check still refreshes the saved copy.
		const d = await meta<Page<Card>>('discover/' + type, { genre: genre || undefined, sort, page: p });
		items = reset ? d.results : [...items, ...d.results.filter((x) => !items.some((y) => y.id === x.id))];
		pageN = d.page;
		total = d.totalPages;
		busy = false;
	}
	$effect(() => {
		void genre; void sort;
		load(true);
	});
</script>

<main class="mx-auto max-w-[1600px] space-y-6 pt-20 laptop:px-10 laptop:pt-24">
	<div class="flex flex-wrap items-end justify-between gap-4 px-4 laptop:px-0">
		<div><h1 class="text-2xl font-bold laptop:text-3xl">{label}</h1><p class="mt-1 text-sm text-gray-400">Tap anything to see details.</p></div>
		<select bind:value={sort} class="rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-1.5 text-sm">
			<option value="popular">Popular</option><option value="rating">Top rated</option><option value="newest">Newest</option>
		</select>
	</div>
	<Chips bind:value={genre} options={[{ key: 0, label: 'All genres' }, ...genres.map((g) => ({ key: g.id, label: g.name }))]} />
	<div class="grid grid-cols-3 gap-3 px-4 tablet:grid-cols-4 laptop:grid-cols-6 laptop:px-0 desktop:grid-cols-8">
		{#each items as c (c.id)}<Poster item={c} class="w-full" />{/each}
	</div>
	{#if pageN < total}
		<div class="flex justify-center"><button class="btn-ghost" disabled={busy} onclick={() => load(false)}>{busy ? 'Loading…' : 'Load more'}</button></div>
	{/if}
</main>
