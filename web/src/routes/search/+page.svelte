<script lang="ts">
	import { tick } from 'svelte';
	import { replaceState, snapshot } from '$app/navigation';
	import { page } from '$app/state';
	import { meta } from '#lib/api.ts';
	import type { Card, Page, SearchHit } from '#lib/types.ts';
	import Poster from '#lib/components/Poster.svelte';
	import Chips from '#lib/components/Chips.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import Img from '#lib/components/Img.svelte';

	let q = $state(page.url.searchParams.get('q') ?? '');
	let tab = $state<string | number>('all');
	let hits = $state<SearchHit[]>([]);
	let busy = $state(false);
	let trending = $state<Card[]>([]);
	let input = $state<HTMLInputElement>();

	$effect(() => {
		const showTrending = (p: Page<Card>) => (trending = p.results.slice(0, 16));
		meta<Page<Card>>('lists/trending', {}, showTrending).then(showTrending);
		if (!q) input?.focus();
	});

	// Debounced live search.
	$effect(() => {
		const term = q.trim();
		replaceState(term ? `/search?q=${encodeURIComponent(term)}` : '/search', {});
		if (!term) { hits = []; return; }
		busy = true;
		let alive = true;
		const t = setTimeout(() => {
			const show = (p: Page<SearchHit>) => alive && (hits = p.results);
			meta<Page<SearchHit>>('search', { q: term }, show).then(show).finally(() => (busy = false));
		}, 250);
		return () => { clearTimeout(t); alive = false; };
	});
	// Going back to search brings back the results, the tab you picked and where you'd scrolled to.
	snapshot({
		id: 'search',
		capture: () => ({ q, tab, hits: $state.snapshot(hits), y: scrollY }),
		restore: async (v) => {
			if (v.q !== q) return;
			tab = v.tab;
			hits = v.hits;
			await tick();
			scrollTo(0, v.y);
		}
	});
	const shown = $derived(hits.filter((h) => tab === 'all' || h.type === tab));
	const count = (t: string) => hits.filter((h) => h.type === t).length;
</script>

<main class="mx-auto max-w-[1600px] space-y-6 pt-20 laptop:px-10 laptop:pt-24">
	<div class="px-4 laptop:px-0">
		<label class="card flex items-center gap-3 px-4 py-3.5 text-gray-400 focus-within:ring-2 focus-within:ring-primary/60">
			<Icon name="search" class="size-5 shrink-0" />
			<input bind:this={input} bind:value={q} autocomplete="off" class="w-full bg-transparent text-base text-white placeholder-gray-500 outline-none" placeholder="Search movies, series, people" />
			{#if q}<button class="text-xs text-gray-400 hover:text-white" onclick={() => { q = ''; input?.focus(); }}>Clear</button>{/if}
		</label>
	</div>
	{#if q.trim()}
		<Chips bind:value={tab} options={[{ key: 'all', label: 'All', count: hits.length }, { key: 'movie', label: 'Movies', count: count('movie') }, { key: 'series', label: 'Series', count: count('series') }, { key: 'person', label: 'People', count: count('person') }]} />
		{#if !shown.length && !busy}
			<p class="px-4 py-20 text-center text-sm text-gray-400">No results for “{q.trim()}”.</p>
		{:else}
			<div class="grid grid-cols-3 gap-3 px-4 tablet:grid-cols-4 laptop:grid-cols-6 laptop:px-0 desktop:grid-cols-8">
				{#each shown as h (h.type + h.id)}
					{#if h.type === 'person'}
						<a href="/person/{h.id}" class="group block text-center">
							<div class="mx-auto aspect-square w-full overflow-hidden rounded-full border border-gray-700/40 bg-gray-800/40 shadow-xl">
								{#if h.poster}<Img path={h.poster} kind="profile" sizes="(min-width: 1280px) 190px, (min-width: 1024px) 16vw, (min-width: 640px) 25vw, 33vw" class="h-full w-full" />{:else}<div class="grid h-full place-items-center text-3xl text-gray-600">{h.title.charAt(0)}</div>{/if}
							</div>
							<div class="mt-2 truncate text-[13px] font-medium">{h.title}</div>
							<div class="text-xs text-gray-400">{h.dept ?? ''}</div>
						</a>
					{:else}
						<Poster item={h as Card} class="w-full" />
					{/if}
				{/each}
			</div>
		{/if}
	{:else}
		<section class="space-y-3">
			<h2 class="px-4 text-base font-semibold laptop:px-0">Trending now</h2>
			<div class="grid grid-cols-3 gap-3 px-4 tablet:grid-cols-4 laptop:grid-cols-6 laptop:px-0 desktop:grid-cols-8">
				{#each trending as c (c.type + c.id)}<Poster item={c} class="w-full" />{/each}
			</div>
		</section>
	{/if}
</main>
