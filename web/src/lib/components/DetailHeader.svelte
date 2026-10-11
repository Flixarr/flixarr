<script lang="ts">
	// Movie/series header: faded backdrop, poster + title artwork, meta line, TMDB score, actions.
	import type { Snippet } from 'svelte';
	import { runtime } from '#lib/api.ts';
	import type { Movie, Series } from '#lib/types.ts';
	import Img, { BACKDROP_SIZES } from './Img.svelte';

	let { item, chip, actions }: { item: Movie | Series; chip?: Snippet; actions?: Snippet } = $props();

	const meta = $derived(
		item.type === 'series'
			? [item.year && item.year + (item.status === 'Ended' && item.last ? '–' + item.last.slice(0, 4) : ''), item.seasons.length + ' season' + (item.seasons.length === 1 ? '' : 's'), item.genres.join(', ')]
			: [item.year, runtime(item.runtime), item.genres.join(', ')]
	);
	let logoBroken = $state(false);
</script>

<section class="relative">
	<div class="absolute inset-x-0 top-0 h-[52dvh] min-h-[340px] [mask-image:linear-gradient(to_bottom,black_30%,transparent_100%)] laptop:h-[70dvh]">
		{#if item.backdrop}<Img path={item.backdrop} kind="backdrop" sizes={BACKDROP_SIZES} eager class="absolute inset-0 h-full w-full opacity-50 saturate-75" />{/if}
		<div class="absolute inset-0 bg-linear-to-r from-gray-950/90 via-gray-950/40 to-transparent"></div>
	</div>
	<div class="relative mx-auto flex max-w-[1600px] flex-row items-end gap-4 px-4 pt-[24dvh] tablet:gap-5 laptop:px-10 laptop:pt-[30dvh]">
		{#if item.poster}<Img path={item.poster} kind="poster" sizes="(min-width: 1024px) 224px, (min-width: 640px) 192px, 96px" eager alt={item.title} class="aspect-[2/3] w-24 shrink-0 self-start rounded-xl border border-gray-700/40 bg-gray-800/40 shadow-2xl tablet:w-48 tablet:self-end laptop:w-56" />{/if}
		<div class="min-w-0 space-y-3 tablet:pb-1">
			<div class="flex flex-wrap items-center gap-2 text-xs">
				<span class="rounded-md px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide {item.type === 'series' ? 'bg-blue-600/80' : 'bg-primary/90'}">{item.type === 'series' ? 'Series' : 'Movie'}</span>
				{@render chip?.()}
			</div>
			{#if item.logo && !logoBroken}
				<h1><Img path={item.logo} kind="logo" sizes="(min-width: 1024px) 420px, (min-width: 640px) 280px, 200px" eager alt={item.title} onerror={() => (logoBroken = true)} class="max-h-12 max-w-[200px] drop-shadow-xl tablet:max-h-20 tablet:max-w-[280px] laptop:max-h-28 laptop:max-w-[420px]" /></h1>
			{:else}
				<h1 class="text-2xl font-bold tablet:text-3xl laptop:text-4xl">{item.title}</h1>
			{/if}
			<div class="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-gray-300">
				{#if item.cert}<span class="rounded border border-gray-500/70 px-1 text-xs font-semibold text-gray-200">{item.cert}</span>{/if}
				{#each meta.filter(Boolean) as m, i (i)}{#if i}<span class="text-gray-600">·</span>{/if}<span>{m}</span>{/each}
			</div>
			<div class="flex items-center gap-2 text-sm">
				<span class="rounded-md bg-tmdb/15 px-1.5 py-0.5 text-xs font-bold text-tmdb ring-1 ring-tmdb/40">TMDB</span>
				<span class="font-semibold">{item.rating || '–'}</span><span class="text-xs text-gray-500">{item.votes.toLocaleString('en-US')} votes</span>
			</div>
			<div class="flex flex-wrap items-center gap-2 pt-1">{@render actions?.()}</div>
		</div>
	</div>
</section>
