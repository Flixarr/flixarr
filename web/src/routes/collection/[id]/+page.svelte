<script lang="ts">
	import { page } from '$app/state';
	import { meta } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { Collection } from '#lib/types.ts';
	import Poster from '#lib/components/Poster.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import Img, { BACKDROP_SIZES } from '#lib/components/Img.svelte';

	let c = $state<Collection | null>(null);
	let error = $state('');
	$effect(() => {
		const id = page.params.id;
		c = null;
		let alive = true;
		const show = (d: Collection) => { if (!alive) return; c = d; document.title = 'Flixarr · ' + d.name; };
		meta<Collection>('collection/' + id, {}, show).then(show).catch((e) => (error = e.message));
		return () => (alive = false);
	});
</script>

{#if error}<p class="px-4 pt-28 text-center text-gray-400">{error}</p>
{:else if c}
	<section class="relative">
		<div class="absolute inset-x-0 top-0 h-[52dvh] min-h-[340px] [mask-image:linear-gradient(to_bottom,black_30%,transparent_100%)] laptop:h-[64dvh]">
			{#if c.backdrop}<Img path={c.backdrop} kind="backdrop" sizes={BACKDROP_SIZES} eager class="absolute inset-0 h-full w-full opacity-50 saturate-75" />{/if}
			<div class="absolute inset-0 bg-linear-to-r from-gray-950/90 via-gray-950/40 to-transparent"></div>
		</div>
		<div class="relative mx-auto flex max-w-[1600px] flex-row items-end gap-4 px-4 pt-[24dvh] tablet:gap-5 laptop:px-10 laptop:pt-[28dvh]">
			{#if c.poster}<Img path={c.poster} kind="poster" sizes="(min-width: 640px) 176px, 96px" eager class="aspect-[2/3] w-24 shrink-0 self-start rounded-xl border border-gray-700/40 bg-gray-800/40 shadow-2xl tablet:w-44 tablet:self-end" />{/if}
			<div class="min-w-0 space-y-3">
				<span class="rounded-md bg-primary/90 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide">Collection</span>
				<h1 class="text-2xl font-bold tablet:text-3xl laptop:text-4xl">{c.name}</h1>
				<div class="text-sm text-gray-300">{c.parts.length} movies</div>
				<button class="btn px-5" onclick={() => notify('Requests arrive in the next Flixarr update.', 'orange')}><Icon name="plus" class="size-4" />Request collection</button>
			</div>
		</div>
	</section>
	<main class="relative z-10 mx-auto max-w-[1600px] space-y-9 pt-9 laptop:px-10">
		{#if c.overview}<section class="space-y-3 px-4 laptop:px-0"><h2 class="text-base font-semibold">Overview</h2><p class="max-w-3xl leading-relaxed text-gray-300">{c.overview}</p></section>{/if}
		<section class="space-y-3">
			<h2 class="px-4 text-base font-semibold laptop:px-0">Movies</h2>
			<div class="grid grid-cols-3 gap-3 px-4 tablet:grid-cols-4 laptop:grid-cols-6 laptop:px-0 desktop:grid-cols-8">
				{#each c.parts as p (p.id)}<Poster item={p} class="w-full" />{/each}
			</div>
		</section>
	</main>
{/if}
