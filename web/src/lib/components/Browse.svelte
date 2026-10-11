<script lang="ts" module>
	// The Browse tab (phone) opens whichever of Movies/Series was used last.
	const KEY = 'flixarr_browse';
	export const lastBrowse = () => (typeof localStorage !== 'undefined' && localStorage.getItem(KEY) === 'series' ? '/series' : '/movies');
</script>

<script lang="ts">
	// Movies / Series browse: genre pills, sort, a poster grid that keeps loading as you scroll.
	// Going back to it restores the genre, sort, titles loaded so far and scroll position.
	import { tick } from 'svelte';
	import { snapshot } from '$app/navigation';
	import { meta } from '#lib/api.ts';
	import { infinite } from '#lib/infinite.ts';
	import type { Card, Page } from '#lib/types.ts';
	import Poster from './Poster.svelte';
	import Chips from './Chips.svelte';
	import LoadingMore from './LoadingMore.svelte';

	let { type }: { type: 'movie' | 'series' } = $props();
	const label = $derived(type === 'series' ? 'Series' : 'Movies');
	$effect(() => localStorage.setItem(KEY, type));
	let genres = $state<{ id: number; name: string }[]>([]);
	let genre = $state<string | number>(0);
	let sort = $state('popular');
	let items = $state<Card[]>([]);
	let pageN = $state(0);
	let total = $state(1);
	let busy = $state(false);
	let loadedFor = ''; // the type|genre|sort that `items` belongs to
	let gen = 0; // bumped on every reset/restore so a slower, older load can't overwrite newer state
	const key = () => `${type}|${genre}|${sort}`;

	$effect(() => {
		const show = (g: { genres: { id: number; name: string }[] }) => (genres = g.genres);
		meta('genres/' + type, {}, show).then(show);
	});
	async function load(reset: boolean): Promise<boolean> {
		const g = reset ? ++gen : gen;
		const p = reset ? 1 : pageN + 1;
		if (!reset && p > total) return false;
		const k = key();
		busy = true;
		try {
			// No onFresh here: swapping pages while you scroll would jump the grid. The background check still refreshes the saved copy.
			const d = await meta<Page<Card>>('discover/' + type, { genre: genre || undefined, sort, page: p });
			if (g !== gen) return false;
			items = reset ? d.results : [...items, ...d.results.filter((x) => !items.some((y) => y.id === x.id))];
			pageN = d.page;
			total = d.totalPages;
			loadedFor = k;
			return pageN < total;
		} finally {
			if (g === gen) busy = false;
		}
	}
	$effect(() => {
		if (key() === loadedFor) return;
		load(true);
	});

	snapshot({
		id: 'browse',
		capture: () => ({ type, genre, sort, items: $state.snapshot(items), pageN, total, y: scrollY }),
		restore: async (v) => {
			if (v.type !== type) return;
			gen++;
			busy = false;
			genre = v.genre;
			sort = v.sort;
			items = v.items;
			pageN = v.pageN;
			total = v.total;
			loadedFor = key();
			await tick();
			scrollTo(0, v.y);
		}
	});
</script>

<main class="mx-auto max-w-[1600px] space-y-6 pb-28 pt-20 laptop:px-10 laptop:pt-24">
	<div class="flex flex-wrap items-end justify-between gap-4 px-4 laptop:px-0">
		<div>
			<!-- Phone: switch between Movies and Series (the top bar has both links from tablet up). -->
			<div class="mb-3 inline-grid grid-cols-2 gap-1 rounded-xl border border-gray-700/60 p-1 tablet:hidden">
				{#each [['Movies', '/movies', 'movie'], ['Series', '/series', 'series']] as [l, h, t] (h)}
					<a href={h} class="rounded-lg px-5 py-1.5 text-center text-sm {type === t ? 'bg-primary text-white' : 'text-gray-400'}">{l}</a>
				{/each}
			</div>
			<h1 class="text-2xl font-bold laptop:text-3xl">{label}</h1><p class="mt-1 text-sm text-gray-400">Tap anything to see details.</p>
		</div>
		<select bind:value={sort} class="rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-1.5 text-sm">
			<option value="popular">Popular</option><option value="rating">Top rated</option><option value="newest">Newest</option>
		</select>
	</div>
	<Chips bind:value={genre} options={[{ key: 0, label: 'All genres' }, ...genres.map((g) => ({ key: g.id, label: g.name }))]} />
	<div class="grid grid-cols-3 gap-3 px-4 tablet:grid-cols-4 laptop:grid-cols-6 laptop:px-0 desktop:grid-cols-8">
		{#each items as c (c.id)}<Poster item={c} class="w-full" />{/each}
	</div>
	{#if items.length}
		<div use:infinite={() => load(false)}><LoadingMore {busy} done={pageN >= total} /></div>
	{/if}
</main>
