<script lang="ts" generics="T">
	import type { Snippet } from 'svelte';
	let { title, items, more = '', item, end }: { title: string; items: T[]; more?: string; item: Snippet<[T]>; end?: Snippet } = $props();
</script>

{#if items.length}
	<section class="space-y-3">
		<div class="flex items-center justify-between px-4 laptop:px-0">
			<h2 class="text-base font-semibold">{#if more}<a href={more} class="hover:text-primary-light">{title}</a>{:else}{title}{/if}</h2>
			{#if more}<a href={more} class="text-xs text-gray-400 hover:text-white">See all →</a>{/if}
		</div>
		<!-- Extra top/bottom room (cancelled by negative margins) so a hovered poster's lift and shadow aren't clipped. -->
		<div class="no-scrollbar -mb-9 -mt-3 flex gap-3 overflow-x-auto px-4 pb-10 pt-3 laptop:px-0">
			{#each items as it, i (i)}{@render item(it)}{/each}
			{@render end?.()}
		</div>
	</section>
{/if}
