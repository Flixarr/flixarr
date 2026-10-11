<script lang="ts">
	// Last card in a Discover row: four of the row's posters as tall slices fading to dark, "See all →" and how many
	// more there are (design #29 on the mockup page seemore.html, which has the other candidates).
	import type { Card } from '#lib/types.ts';
	import { tilt, tiltEnabled } from '#lib/tilt.ts';
	import Img from './Img.svelte';
	import Icon from './Icon.svelte';
	let { items, href, remaining = 0 }: { items: Card[]; href: string; remaining?: number } = $props();
	const four = $derived(items.filter((c) => c.poster).slice(0, 4));
	const tilting = tiltEnabled();
	// Lists are paged by 20, so the count is rounded down to a friendly "380+".
	const label = $derived(remaining >= 20 ? `${Math.floor(remaining / 10) * 10}+ more` : remaining > 0 ? `${remaining} more` : '');
</script>

<a {href} class="group relative block w-28 shrink-0 tablet:w-36" aria-label="See all">
	<div use:tilt class="relative aspect-[2/3] overflow-hidden rounded-xl border border-gray-700/40 bg-gray-800/40 shadow-xl">
		<div class="absolute inset-0 grid grid-cols-4 gap-px transition duration-500 {tilting ? '' : 'group-hover:scale-105'}">
			{#each four as c (c.id)}<Img path={c.poster!} kind="poster" sizes="(min-width: 640px) 72px, 56px" class="h-full w-full" />{/each}
		</div>
		<div class="absolute inset-0 bg-linear-to-t from-gray-950 via-gray-950/60 to-gray-950/10"></div>
		<div class="absolute inset-x-0 bottom-0 p-3">
			<div class="flex items-center gap-1 text-sm font-semibold">See all <Icon name="arrow" class="size-4" /></div>
			{#if label}<div class="text-xs text-gray-400">{label}</div>{/if}
		</div>
		{#if tilting}<div data-glare class="pointer-events-none absolute inset-0 opacity-0 mix-blend-soft-light"></div>{/if}
	</div>
</a>
