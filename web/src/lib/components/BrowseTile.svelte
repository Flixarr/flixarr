<script lang="ts">
	// A Browse tile on the empty search page (#286): a smart search shown as the backdrop of its first title,
	// darkened from the left, with what kind of search it is and its name. Picked from mockup searchhelp style A.
	import type { Tile } from '#lib/searchhelp.ts';
	import Img from './Img.svelte';
	let { tile, onpick }: { tile: Tile; onpick: (q: string) => void } = $props();
	// A backdrop path can stop resolving on TMDB: fall back to the tile's first poster, then plain grey.
	let broken = $state(false);
</script>

<button onclick={() => onpick(tile.q)} class="group relative h-28 overflow-hidden rounded-xl bg-gray-900 text-left ring-1 ring-gray-700/50 outline-none transition hover:ring-gray-500 focus-visible:ring-2 focus-visible:ring-primary laptop:h-36">
	{#if !broken && (tile.backdrop || tile.posters[0])}
		<!-- Img's own wrapper is position: relative, so the absolute box is this span. -->
		<span class="absolute inset-0 opacity-60 transition duration-500 group-hover:scale-105 group-hover:opacity-75">
			<Img path={tile.backdrop || tile.posters[0]} kind={tile.backdrop ? 'backdrop' : 'poster'} sizes="(min-width: 1024px) 300px, 50vw" onerror={() => (broken = true)} class="h-full w-full" />
		</span>
	{/if}
	<span class="absolute inset-0 bg-gradient-to-r from-gray-950/95 via-gray-950/60 to-transparent"></span>
	<span class="relative block p-3.5">
		<span class="block text-[10px] font-bold uppercase tracking-wider text-primary">{tile.kind}</span>
		<span class="block max-w-[75%] text-lg font-bold leading-tight text-white laptop:text-xl">{tile.label}</span>
	</span>
</button>
