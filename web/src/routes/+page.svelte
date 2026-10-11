<script lang="ts">
	// Discover: rotating spotlight + rows (layout D). Recent Requests / Recently Added arrive with requests.
	import { meta } from '#lib/api.ts';
	import type { Card, Page } from '#lib/types.ts';
	import Spotlight from '#lib/components/Spotlight.svelte';
	import Row from '#lib/components/Row.svelte';
	import Poster from '#lib/components/Poster.svelte';

	const ROWS = [
		['trending', 'Trending', '/movies'],
		['popular-movies', 'Popular Movies', '/movies'],
		['upcoming-movies', 'Upcoming Movies', '/movies'],
		['popular-series', 'Popular Series', '/series'],
		['upcoming-series', 'On the Air', '/series']
	] as const;

	let lists = $state<Record<string, Card[]>>({});
	let error = $state('');

	$effect(() => {
		Promise.all(ROWS.map(([k]) => meta<Page<Card>>('lists/' + k).then((p) => [k, p.results] as const)))
			.then((all) => (lists = Object.fromEntries(all)))
			.catch((e) => (error = e.message));
	});
	const spot = $derived((lists.trending ?? []).filter((c) => c.backdrop).slice(0, 5));
</script>

{#if error}
	<div class="px-4 pt-28 text-center text-gray-400">{error}</div>
{:else if !lists.trending}
	<div class="h-[40dvh] min-h-[300px] animate-pulse bg-gray-900/40"></div>
{:else}
	<Spotlight items={spot} />
	<div class="relative z-10 mx-auto -mt-5 max-w-[1600px] space-y-9 laptop:-mt-7 laptop:px-10">
		{#each ROWS as [k, title, more] (k)}
			<Row {title} items={lists[k] ?? []} {more}>
				{#snippet item(c)}<Poster item={c} />{/snippet}
			</Row>
		{/each}
	</div>
{/if}
