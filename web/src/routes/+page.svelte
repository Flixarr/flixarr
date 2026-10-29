<script lang="ts">
	// Discover: rotating spotlight + rows (layout D). Recently Added (from Plex) and Recent Requests come first.
	import { tick } from 'svelte';
	import { restoreScroll } from '#lib/scroll.ts';
	import { snapshot } from '$app/navigation';
	import { api, meta } from '#lib/api.ts';
	import type { Card, Page } from '#lib/types.ts';
	import Spotlight from '#lib/components/Spotlight.svelte';
	import Row from '#lib/components/Row.svelte';
	import Poster from '#lib/components/Poster.svelte';
	import SeeMore from '#lib/components/SeeMore.svelte';
	import { LISTS, listHref, rowSource, rowTitle, type DiscoverRow } from '#lib/lists.ts';

	const LOCAL = ['recently-added', 'recent-requests', 'leaving-soon'];

	let lists = $state<Record<string, Card[]>>({});
	let pages = $state<Record<string, number>>({}); // total pages per list (20 titles each), for the See all count
	let error = $state('');
	let local = $state<{ added: Card[]; requests: Card[]; leaving: Card[] }>({ added: [], requests: [], leaving: [] });
	let localDone = $state({ added: false, requests: false, leaving: false }); // their answers arrived
	// Row order and on/off from Settings → Discover; until it arrives, the order this device saw last time
	// (so rows don't shuffle), else every row in the default order.
	const ORDER_KEY = 'flixarr:discover-rows';
	let order = $state<DiscoverRow[] | null>(JSON.parse(localStorage.getItem(ORDER_KEY) ?? 'null'));
	const shown = $derived((order ?? [...LOCAL, ...LISTS.slice(0, 5).map((l) => l.key)].map((key) => ({ key, on: true }))).filter((r) => r.on));

	// A row's titles (and how many pages there are, for the See all count). Trending always loads: the
	// spotlight shows it.
	function fill(r: DiscoverRow, alive: () => boolean) {
		const { path, params } = rowSource(r);
		const got = (p: Page<Card>) => alive() && ((lists = { ...lists, [r.key]: p.results }), (pages = { ...pages, [r.key]: p.totalPages }));
		return meta<Page<Card>>(path, params, got).then(got);
	}
	$effect(() => {
		let alive = true;
		const live = () => alive;
		fill({ key: 'trending', on: true }, live).catch((e) => (error = e.message));
		api<DiscoverRow[]>('/discover/rows')
			.then((rows) => {
				if (!alive) return;
				order = rows;
				localStorage.setItem(ORDER_KEY, JSON.stringify(rows));
				for (const r of rows) if (r.on && !LOCAL.includes(r.key) && r.key !== 'trending') fill(r, live).catch(() => alive && (lists = { ...lists, [r.key]: [] }));
			})
			.catch(() => {});
		// Flixarr's own rows: what just landed in Plex, and the newest requests.
		api<Page<Card>>('/discover/recently-added').then((p) => alive && (local.added = p.results)).catch(() => {}).finally(() => (localDone.added = true));
		api<Page<Card>>('/discover/recent-requests').then((p) => alive && (local.requests = p.results)).catch(() => {}).finally(() => (localDone.requests = true));
		api<{ results: { card: Card }[] }>('/leaving').then((d) => alive && (local.leaving = d.results.map((r) => r.card))).catch(() => {}).finally(() => (localDone.leaving = true));
		return () => (alive = false);
	});
	// Going back to Discover: same rows, each row scrolled sideways where you left it (design rule 11), same page scroll.
	snapshot({
		id: 'discover',
		capture: () => ({
			lists: $state.snapshot(lists),
			local: $state.snapshot(local),
			order: $state.snapshot(order),
			pages: $state.snapshot(pages),
			rows: Object.fromEntries([...document.querySelectorAll<HTMLElement>('[data-row]')].map((r) => [r.dataset.row, r.scrollLeft])),
			y: scrollY
		}),
		restore: async (v) => {
			if (!Object.keys(v.lists).length) return;
			lists = v.lists;
			local = v.local ?? local;
			order = v.order ?? order;
			pages = v.pages;
			await tick();
			for (const r of document.querySelectorAll<HTMLElement>('[data-row]')) r.scrollLeft = v.rows[r.dataset.row!] ?? 0;
			restoreScroll(v.y);
		}
	});
	const spot = $derived((lists.trending ?? []).filter((c) => c.backdrop).slice(0, 5));
</script>

{#if error}
	<div class="px-4 pt-28 text-center text-gray-400">{error}</div>
{:else}
	{#if lists.trending}<Spotlight items={spot} />{:else}<div class="h-[40dvh] min-h-[300px] animate-pulse bg-gray-900/40"></div>{/if}
	<div class="relative z-10 mx-auto -mt-5 max-w-[1600px] space-y-9 laptop:-mt-7 laptop:px-10">
		{#each shown as r (r.key)}
			{@const k = r.key}
			{#if k === 'recently-added'}
				<Row title="Recently Added" items={local.added} loading={!localDone.added}>
					{#snippet item(c)}<Poster item={c} />{/snippet}
				</Row>
			{:else if k === 'leaving-soon'}
				<Row title="Leaving Soon" items={local.leaving} more="/leaving" loading={!localDone.leaving}>
					{#snippet item(c)}<Poster item={c} />{/snippet}
				</Row>
			{:else if k === 'recent-requests'}
				<Row title="Recent Requests" items={local.requests} more="/requests" loading={!localDone.requests}>
					{#snippet item(c)}<Poster item={c} />{/snippet}
				</Row>
			{:else if !lists[k] || lists[k].length}
				{@const more = listHref(k)}
				{@const src = rowSource(r)}
				<Row title={rowTitle(r)} items={lists[k] ?? []} {more} loading={!lists[k]}>
					{#snippet item(c)}<Poster item={c} />{/snippet}
					{#snippet end()}<SeeMore path={src.path} params={src.params} href={more} remaining={Math.max(0, (pages[k] ?? 0) * 20 - (lists[k]?.length ?? 0))} />{/snippet}
				</Row>
			{/if}
		{/each}
	</div>
{/if}
