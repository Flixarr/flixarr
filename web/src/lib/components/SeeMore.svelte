<script lang="ts">
	// Last card in a Discover row (like Seerr): poster-sized, a 2×2 of the row's posters behind an arrow.
	import type { Card } from '#lib/types.ts';
	import { tilt } from '#lib/tilt.ts';
	import Img from './Img.svelte';
	import Icon from './Icon.svelte';
	let { items, href }: { items: Card[]; href: string } = $props();
	const four = $derived(items.filter((c) => c.poster).slice(0, 4));
</script>

<a {href} class="group relative block w-28 shrink-0 tablet:w-36" aria-label="See more">
	<div use:tilt class="relative grid aspect-[2/3] grid-cols-2 grid-rows-2 gap-0.5 overflow-hidden rounded-xl border border-gray-700/40 bg-gray-800/40 shadow-xl">
		{#each four as c (c.id)}<Img path={c.poster!} kind="poster" sizes="(min-width: 640px) 72px, 56px" class="h-full w-full" />{/each}
		<div class="absolute inset-0 grid place-items-center bg-gray-950/65 transition group-hover:bg-gray-950/50">
			<div class="flex flex-col items-center gap-2">
				<span class="grid size-10 place-items-center rounded-full bg-white text-gray-950 shadow-lg transition group-hover:scale-110"><Icon name="arrow" class="size-5" /></span>
				<span class="text-sm font-semibold">See more</span>
			</div>
		</div>
		<div data-glare class="pointer-events-none absolute inset-0 opacity-0 mix-blend-soft-light"></div>
	</div>
</a>
