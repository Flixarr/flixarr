<script lang="ts" generics="T">
	import type { Snippet } from 'svelte';
	let { title, items, more = '', item }: { title: string; items: T[]; more?: string; item: Snippet<[T]> } = $props();
</script>

{#if items.length}
	<section class="space-y-3">
		<div class="flex items-center justify-between px-4 laptop:px-0">
			<h2 class="text-base font-semibold">{title}</h2>
			{#if more}<a href={more} class="text-xs text-gray-400 hover:text-white">See all →</a>{/if}
		</div>
		<div class="no-scrollbar flex gap-3 overflow-x-auto px-4 pb-1 laptop:px-0">
			{#each items as it, i (i)}{@render item(it)}{/each}
		</div>
	</section>
{/if}
