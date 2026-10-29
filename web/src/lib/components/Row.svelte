<script lang="ts" generics="T">
	import type { Snippet } from 'svelte';
	// loading: the row's titles haven't arrived yet, so it holds its place with pulsing grey posters.
	let { title, items, more = '', item, end, loading = false }: { title: string; items: T[]; more?: string; item: Snippet<[T]>; end?: Snippet; loading?: boolean } = $props();
</script>

{#if items.length || loading}
	<section class="space-y-3">
		<div class="flex items-center justify-between px-4 laptop:px-0">
			<h2 class="text-base font-semibold">{#if more}<a href={more} class="hover:text-primary-light">{title}</a>{:else}{title}{/if}</h2>
			{#if more}<a href={more} class="text-xs text-gray-400 hover:text-white">See all →</a>{/if}
		</div>
		<!-- Extra top/bottom room (cancelled by negative margins) so a hovered poster's lift and shadow aren't clipped. -->
		<div data-row={title} class="no-scrollbar -mb-9 -mt-3 flex gap-3 overflow-x-auto px-4 pb-10 pt-3 laptop:px-0">
			{#if items.length}
				{#each items as it, i (i)}{@render item(it)}{/each}
				{@render end?.()}
			{:else}
				{#each { length: 8 } as _, i (i)}
					<div class="w-28 shrink-0 animate-pulse tablet:w-36" aria-hidden="true">
						<div class="aspect-[2/3] rounded-xl bg-gray-800/70"></div>
						<div class="mt-2 h-3.5 w-3/4 rounded bg-gray-800/70"></div>
						<div class="mt-1.5 h-3 w-1/2 rounded bg-gray-800/50"></div>
					</div>
				{/each}
			{/if}
		</div>
	</section>
{/if}
