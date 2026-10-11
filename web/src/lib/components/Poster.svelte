<script lang="ts">
	import { img, href } from '#lib/api.ts';
	import type { Card } from '#lib/types.ts';
	let { item, class: cls = 'w-28 tablet:w-36', sub = '' }: { item: Card; class?: string; sub?: string } = $props();
</script>

<a href={href(item)} class="group relative block shrink-0 {cls}">
	<div class="relative aspect-[2/3] overflow-hidden rounded-xl border border-gray-700/40 bg-gray-800/40 shadow-xl">
		{#if item.poster}
			<img src={img(item.poster)} alt={item.title} loading="lazy" class="h-full w-full object-cover transition duration-300 group-hover:scale-105" />
		{/if}
		<div class="absolute left-1.5 top-1.5">
			<span class="rounded-md px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide {item.type === 'series' ? 'bg-blue-600/80' : 'bg-primary/90'}">{item.type === 'series' ? 'Series' : 'Movie'}</span>
		</div>
	</div>
	<div class="mt-2 truncate text-[13px] font-medium">{item.title}</div>
	{#if sub}<div class="truncate text-xs text-gray-500">{sub}</div>{/if}
	<div class="text-xs text-gray-400">{item.year || 'TBA'} · ★ {item.rating || '–'}</div>
</a>
