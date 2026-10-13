<script lang="ts">
	import { statusOf } from '#lib/availability.svelte.ts';
	import { can } from '#lib/session.svelte.ts';
	import PlexMark from './PlexMark.svelte';
	// Discover banner: rotates trending titles every 5s, fades into the page, pauses on hover.
	import { href } from '#lib/api.ts';
	import type { Card } from '#lib/types.ts';
	import Icon from './Icon.svelte';
	import Img, { BACKDROP_SIZES } from './Img.svelte';

	let { items }: { items: Card[] } = $props();
	let cur = $state(0);
	let paused = $state(false);
	// Backdrops load one slide ahead instead of all five at once.
	const seen = $derived(new Set([0, 1, cur, (cur + 1) % Math.max(items.length, 1)]));

	$effect(() => {
		if (paused || items.length < 2) return;
		const t = setInterval(() => (cur = (cur + 1) % items.length), 5000);
		return () => clearInterval(t);
	});
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<section class="relative h-[40dvh] min-h-[300px] w-full overflow-hidden laptop:h-[44dvh]" onmouseenter={() => (paused = true)} onmouseleave={() => (paused = false)}>
	{#each items as h, i (h.id)}
		<div class="absolute inset-0 transition-opacity duration-1000 {i === cur ? 'opacity-100' : 'pointer-events-none opacity-0'}">
			<div class="absolute inset-0 [mask-image:linear-gradient(to_bottom,black_35%,transparent_100%)]">
				{#if seen.has(i) && h.backdrop}<Img path={h.backdrop} kind="backdrop" sizes={BACKDROP_SIZES} eager={i === 0} class="absolute inset-0 h-full w-full opacity-55 saturate-75" />{/if}
				<div class="absolute inset-0 bg-linear-to-t from-gray-950/60 via-gray-950/40 to-gray-950/30"></div>
				<div class="absolute inset-0 bg-linear-to-r from-gray-950/90 via-gray-950/30 to-transparent"></div>
			</div>
			<div class="absolute inset-x-0 bottom-14 mx-auto max-w-[1600px] px-4 laptop:bottom-16 laptop:px-10">
				<div class="max-w-lg space-y-2.5">
					<div class="flex items-center gap-2 text-xs">
						<span class="rounded-md px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide {h.type === 'series' ? 'bg-blue-600/80' : 'bg-primary/90'}">{h.type === 'series' ? 'Series' : 'Movie'}</span>
						<span class="rounded-md bg-gray-900/70 px-1.5 py-0.5 text-gray-300 ring-1 ring-gray-700/60">Trending this week</span>
					</div>
					<h1 class="text-2xl font-bold leading-tight drop-shadow-xl laptop:text-3xl">{h.title}</h1>
					<p class="line-clamp-1 text-sm text-gray-400 laptop:line-clamp-2">{h.overview}</p>
					<div class="flex items-center gap-3 pt-1">
						{#if statusOf(`${h.type}:${h.id}`)?.plex}
							<a href={statusOf(`${h.type}:${h.id}`)?.plex} target="_blank" rel="noopener" class="btn-plex px-4 py-1.5 text-sm"><PlexMark class="size-4" color="#111827" />Play on Plex</a>
						{:else if statusOf(`${h.type}:${h.id}`) && !['failed', 'declined', 'expired'].includes(statusOf(`${h.type}:${h.id}`)!.status)}
							<a href="/requests" class="btn-ghost px-4 py-1.5 text-sm text-yellow-300">Requested</a>
						{:else if can(h.type === 'series' ? 'requestTv' : 'requestMovies')}
							<a href="{href(h)}?request=1" class="btn px-4 py-1.5 text-sm"><Icon name="plus" class="size-4" />Request</a>
						{/if}
						<a href={href(h)} class="btn-ghost px-4 py-1.5 text-sm">Details</a>
						<span class="ml-1 text-xs text-gray-400">{h.year} · ★ {h.rating}</span>
					</div>
				</div>
			</div>
		</div>
	{/each}
	{#if items.length > 1}
		<div class="absolute inset-x-0 bottom-14 mx-auto flex max-w-[1600px] justify-end px-4 laptop:bottom-16 laptop:px-10">
			<div class="flex gap-1.5">
				{#each items as _, i (i)}
					<button aria-label="Show slide {i + 1}" onclick={() => (cur = i)} class="h-1.5 rounded-full transition-all {i === cur ? 'w-6 bg-primary' : 'w-1.5 bg-gray-500/70'}"></button>
				{/each}
			</div>
		</div>
	{/if}
</section>
