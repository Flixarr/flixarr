<script lang="ts">
	// "See all" page for a Discover row: the whole list as a poster grid that keeps loading as you scroll.
	// Going back to it puts you exactly where you were (titles loaded so far + scroll position).
	import { tick } from 'svelte';
	import { restoreScroll } from '#lib/scroll.ts';
	import { page } from '$app/state';
	import { snapshot } from '$app/navigation';
	import { api, meta } from '#lib/api.ts';
	import { infinite } from '#lib/infinite.ts';
	import { LISTS, isCustom, rowSource, rowTitle, type DiscoverRow } from '#lib/lists.ts';
	import type { Card, Page } from '#lib/types.ts';
	import Poster from '#lib/components/Poster.svelte';
	import LoadingMore from '#lib/components/LoadingMore.svelte';

	// A built-in list, or a custom row from Settings → Discover (looked up by its key).
	let customs = $state<DiscoverRow[]>([]);
	$effect(() => {
		if (page.params.list?.startsWith('custom-') && !customs.length) api<DiscoverRow[]>('/discover/rows').then((r) => (customs = r.filter(isCustom)));
	});
	const row = $derived<DiscoverRow | undefined>(LISTS.some((l) => l.key === page.params.list) ? { key: page.params.list!, on: true } : customs.find((r) => r.key === page.params.list));
	const list = $derived(row ? { key: row.key, title: rowTitle(row), ...rowSource(row) } : undefined);
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
			const d = await meta<Page<Card>>(list.path, { ...list.params, page: p });
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
			restoreScroll(v.y);
		}
	});
</script>

<main class="mx-auto max-w-[1600px] space-y-6 pb-28 pt-20 laptop:px-10 laptop:pt-24">
	{#if !list}
		{#if !page.params.list?.startsWith('custom-') || customs.length}<p class="px-4 pt-10 text-center text-gray-400">That list doesn't exist.</p>{/if}
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
