<script lang="ts">
	// Discover: rotating spotlight + rows (layout D). Recent Requests / Recently Added arrive with requests.
	import { meta } from '#lib/api.ts';
	import type { Card, Page } from '#lib/types.ts';
	import Spotlight from '#lib/components/Spotlight.svelte';
	import Row from '#lib/components/Row.svelte';
	import Poster from '#lib/components/Poster.svelte';
	import SeeMore from '#lib/components/SeeMore.svelte';
	import { LISTS, listHref } from '#lib/lists.ts';

	const ROWS = LISTS.map((l) => [l.key, l.title, listHref(l.key)] as const);

	let lists = $state<Record<string, Card[]>>({});
	let error = $state('');

	$effect(() => {
		let alive = true;
		const fresh = (k: string) => (p: Page<Card>) => alive && (lists = { ...lists, [k]: p.results });
		Promise.all(ROWS.map(([k]) => meta<Page<Card>>('lists/' + k, {}, fresh(k)).then((p) => [k, p.results] as const)))
			.then((all) => alive && (lists = Object.fromEntries(all)))
			.catch((e) => (error = e.message));
		return () => (alive = false);
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
				{#snippet end()}<SeeMore items={lists[k] ?? []} href={more} />{/snippet}
			</Row>
		{/each}
	</div>
{/if}
