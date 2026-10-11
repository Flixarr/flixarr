<script lang="ts">
	// "See all" page for a Discover row: the whole list as a poster grid that keeps loading as you scroll.
	// Going back to it puts you exactly where you were (titles loaded so far + scroll position).
	import { tick } from 'svelte';
	import { page } from '$app/state';
	import { snapshot } from '$app/navigation';
	import { meta } from '#lib/api.ts';
	import { infinite } from '#lib/infinite.ts';
	import { LISTS } from '#lib/lists.ts';
	import type { Card, Page } from '#lib/types.ts';
	import Poster from '#lib/components/Poster.svelte';
	import LoadingMore from '#lib/components/LoadingMore.svelte';

	const list = $derived(LISTS.find((l) => l.key === page.params.list));
	let items = $state<Card[]>([]);
	let pageN = $state(0);
	let total = $state(1);
	let busy = $state(false);
	let error = $state('');
	let loadedFor = ''; // which list `items` belongs to
	let gen = 0; // bumped on every reset/restore so a slower, older load can't overwrite newer state

	async function load(reset: boolean): Promise<boolean> {
		if (!list) return false;
		const g = reset ? ++gen : gen;
		const p = reset ? 1 : pageN + 1;
		if (!reset && p > total) return false;
		busy = true;
		try {
			const d = await meta<Page<Card>>('lists/' + list.key, { page: p });
			if (g !== gen) return false;
			items = reset ? d.results : [...items, ...d.results.filter((x) => !items.some((y) => y.id === x.id && y.type === x.type))];
			pageN = d.page;
			total = d.totalPages;
			loadedFor = list.key;
			return pageN < total;
		} catch (e) {
			if (g === gen) error = e instanceof Error ? e.message : 'Could not load.';
			return false;
		} finally {
			if (g === gen) busy = false;
		}
	}

	$effect(() => {
		if (!list) return;
		document.title = 'Flixarr · ' + list.title;
		if (loadedFor === list.key) return;
		items = [];
		error = '';
		load(true);
	});

	snapshot({
		id: 'list-page',
		capture: () => ({ key: list?.key ?? '', items: $state.snapshot(items), pageN, total, y: scrollY }),
		restore: async (v) => {
			if (v.key !== list?.key) return;
			gen++;
			busy = false;
			items = v.items;
			pageN = v.pageN;
			total = v.total;
			loadedFor = v.key;
			await tick();
			scrollTo(0, v.y);
		}
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
		{#if items.length}
			<div use:infinite={() => load(false)}><LoadingMore {busy} done={pageN >= total} /></div>
		{/if}
	{/if}
</main>
