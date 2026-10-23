<script lang="ts">
	// Last card in a Discover row ("departure board", design 29b on the mockup page seemore.html): four slices that
	// each swap posters every few seconds, "See all →" and how many more there are. The posters come from page 2 of
	// the list, so the card previews titles that aren't already in the row.
	import { meta } from '#lib/api.ts';
	import type { Card, Page } from '#lib/types.ts';
	import { tilt, tiltEnabled } from '#lib/tilt.ts';
	import Img from './Img.svelte';
	import Icon from './Icon.svelte';
	let { path, params = {}, href, remaining = 0 }: { path: string; params?: Record<string, string | number>; href: string; remaining?: number } = $props();

	let later = $state<Card[]>([]);
	$effect(() => {
		let alive = true;
		const show = (p: Page<Card>) => alive && (later = p.results.filter((c) => c.poster).slice(0, 8));
		meta<Page<Card>>(path, { ...params, page: 2 }, show).then(show).catch(() => {});
		return () => (alive = false);
	});
	// slice i shows later[i], then swaps to later[i + 4]
	const slots = $derived(later.length >= 8 ? [0, 1, 2, 3].map((i) => [later[i], later[i + 4]]) : []);
	const tilting = tiltEnabled();
	// Lists are paged by 20, so the count is rounded down to a friendly "380+".
	const label = $derived(remaining >= 20 ? `${Math.floor(remaining / 10) * 10}+ more` : remaining > 0 ? `${remaining} more` : '');
</script>

<a {href} class="group relative block w-28 shrink-0 tablet:w-36" aria-label="See all">
	<div use:tilt class="relative aspect-[2/3] overflow-hidden rounded-xl border border-gray-700/40 bg-gray-800/40 shadow-xl">
		<div class="board absolute inset-0 grid grid-cols-4 gap-px">
			{#each slots as [a, b] (a.type + a.id)}
				<div class="relative overflow-hidden">
					<div class="absolute inset-0"><Img path={a.poster!} kind="poster" sizes="(min-width: 640px) 72px, 56px" class="h-full w-full" /></div>
					<div class="board-swap absolute inset-0"><Img path={b.poster!} kind="poster" sizes="(min-width: 640px) 72px, 56px" class="h-full w-full" /></div>
				</div>
			{/each}
		</div>
		<div class="absolute inset-0 bg-linear-to-t from-gray-950 via-gray-950/60 to-gray-950/10"></div>
		<div class="absolute inset-x-0 bottom-0 p-3">
			<div class="flex items-center gap-1 text-sm font-semibold">See all <Icon name="arrow" class="size-4" /></div>
			{#if label}<div class="text-xs text-gray-400">{label}</div>{/if}
		</div>
		{#if tilting}<div data-glare class="pointer-events-none absolute inset-0 opacity-0 mix-blend-soft-light"></div>{/if}
	</div>
</a>
