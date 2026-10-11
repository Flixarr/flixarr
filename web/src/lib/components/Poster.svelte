<script lang="ts">
	import { href } from '#lib/api.ts';
	import { tilt, tiltEnabled } from '#lib/tilt.ts';
	import type { Card } from '#lib/types.ts';
	import Img from './Img.svelte';
	let { item, class: cls = 'w-28 tablet:w-36', sub = '' }: { item: Card; class?: string; sub?: string } = $props();
	// Rows show posters at a fixed width; grids (class w-full) at 3/4/6/8 per row.
	const sizes = $derived(cls.includes('w-full') ? '(min-width: 1280px) 190px, (min-width: 1024px) 16vw, (min-width: 640px) 25vw, 33vw' : '(min-width: 640px) 144px, 112px');
	// Mouse + motion allowed: the tilt replaces the plain hover zoom.
	const tilting = tiltEnabled();
</script>

<a href={href(item)} class="group relative block shrink-0 {cls}">
	<div use:tilt class="relative aspect-[2/3] overflow-hidden rounded-xl border border-gray-700/40 bg-gray-800/40 shadow-xl">
		{#if item.poster}
			<Img path={item.poster} kind="poster" {sizes} alt={item.title} class="h-full w-full {tilting ? '' : 'transition duration-300 group-hover:scale-105'}" />
		{/if}
		{#if tilting}
			<div data-glare class="pointer-events-none absolute inset-0 opacity-0 mix-blend-soft-light"></div>
		{/if}
		<div class="absolute left-1.5 top-1.5 z-10">
			<span class="rounded-md px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide {item.type === 'series' ? 'bg-blue-600/80' : 'bg-primary/90'}">{item.type === 'series' ? 'Series' : 'Movie'}</span>
		</div>
	</div>
	<!-- relative: a tilted poster paints in its own layer, so its shadow would otherwise cover this text. -->
	<div class="relative">
		<div class="mt-2 truncate text-[13px] font-medium">{item.title}</div>
		{#if sub}<div class="truncate text-xs text-gray-500">{sub}</div>{/if}
		<div class="text-xs text-gray-400">{item.year || 'TBA'} · ★ {item.rating || '–'}</div>
	</div>
</a>

