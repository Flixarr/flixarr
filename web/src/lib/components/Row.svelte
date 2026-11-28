<script lang="ts" generics="T">
	import type { Snippet } from 'svelte';
	import { rowWidth } from '#lib/posters.ts';
	import Icon from './Icon.svelte';
	// loading: the row's titles haven't arrived yet, so it holds its place with pulsing grey posters.
	let { title, items, more = '', item, end, loading = false }: { title: string; items: T[]; more?: string; item: Snippet<[T]>; end?: Snippet; loading?: boolean } = $props();

	// Which ends have more row past them: that side fades out and shows its arrow.
	let el = $state<HTMLElement>();
	let atStart = $state(true);
	let atEnd = $state(true);
	const edges = () => {
		if (!el) return;
		atStart = el.scrollLeft <= 2;
		atEnd = el.scrollLeft >= el.scrollWidth - el.clientWidth - 2;
	};
	$effect(() => {
		items.length;
		loading;
		edges();
	});
	$effect(() => {
		if (!el) return;
		const ro = new ResizeObserver(edges);
		ro.observe(el);
		return () => ro.disconnect();
	});
	// An arrow moves about a screen's worth, keeping a poster or so in view for context.
	const page = (dir: number) => {
		if (!el) return;
		const smooth = !matchMedia('(prefers-reduced-motion: reduce)').matches;
		el.scrollBy({ left: dir * Math.max(el.clientWidth - 160, el.clientWidth / 2), behavior: smooth ? 'smooth' : 'auto' });
	};
	// Shown only while a mouse is over the row (hover: devices), or when the arrow has keyboard focus.
	const arrowCls = 'absolute top-[38%] z-20 hidden size-10 -translate-y-1/2 place-items-center rounded-full bg-gray-900/90 text-white opacity-0 shadow-xl ring-1 ring-gray-700/60 backdrop-blur transition hover:bg-gray-800 pointer-events-none focus-visible:pointer-events-auto focus-visible:opacity-100 group-hover/row:pointer-events-auto group-hover/row:opacity-100 laptop:grid';
</script>

{#if items.length || loading}
	<section class="space-y-3">
		<div class="flex items-center justify-between px-4 laptop:px-0">
			<h2 class="text-base font-semibold">{#if more}<a href={more} class="hover:text-primary-light">{title}</a>{:else}{title}{/if}</h2>
			{#if more}<a href={more} class="text-xs text-gray-400 hover:text-white">See all →</a>{/if}
		</div>
		<div class="group/row relative">
			{#if items.length}
				<button type="button" aria-label="Scroll {title} back" onclick={() => page(-1)} class="{arrowCls} left-0 laptop:-left-5 {atStart ? 'invisible' : ''}"><Icon name="chevron" class="size-5 rotate-180" /></button>
				<button type="button" aria-label="Scroll {title} forward" onclick={() => page(1)} class="{arrowCls} right-0 laptop:-right-5 {atEnd ? 'invisible' : ''}"><Icon name="chevron" class="size-5" /></button>
			{/if}
			<!-- Extra room on every side (cancelled by negative margins) so a hovered poster's lift and shadow aren't clipped.
			     On laptops the row pulls 1rem out into the page padding, so the first poster still lines up with the heading.
			     row-fade fades each end that has more row past it (app.css). -->
			<div
				bind:this={el}
				onscroll={edges}
				data-row={title}
				class="row-fade no-scrollbar -mb-9 -mt-3 flex gap-3 overflow-x-auto px-4 pb-10 pt-3 laptop:-mx-4"
				style:--fade-l={atStart ? '0px' : '3rem'}
				style:--fade-r={atEnd ? '0px' : '3rem'}
			>
				{#if items.length}
					{#each items as it, i (i)}{@render item(it)}{/each}
					{@render end?.()}
				{:else}
					{#each { length: 8 } as _, i (i)}
						<div class="shrink-0 animate-pulse {rowWidth}" aria-hidden="true">
							<div class="aspect-[2/3] rounded-xl bg-gray-800/70"></div>
							<div class="mt-2 h-3.5 w-3/4 rounded bg-gray-800/70"></div>
							<div class="mt-1.5 h-3 w-1/2 rounded bg-gray-800/50"></div>
						</div>
					{/each}
				{/if}
			</div>
		</div>
	</section>
{/if}
