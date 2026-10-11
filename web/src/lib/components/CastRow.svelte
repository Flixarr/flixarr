<script lang="ts">
	import { img } from '#lib/api.ts';
	import type { CastMember } from '#lib/types.ts';
	import Row from './Row.svelte';
	let { cast }: { cast: CastMember[] } = $props();
</script>

<Row title="Cast" items={cast}>
	{#snippet item(c)}
		<a href="/person/{c.id}" class="w-24 shrink-0 tablet:w-28">
			<div class="aspect-[2/3] overflow-hidden rounded-xl border border-gray-700/40 bg-gray-800/40 shadow-xl">
				{#if c.profile}<img src={img(c.profile, 'w185')} alt="" loading="lazy" class="h-full w-full object-cover" />
				{:else}<div class="grid h-full place-items-center text-2xl text-gray-600">{c.name.charAt(0)}</div>{/if}
			</div>
			<div class="mt-2 truncate text-[13px] font-medium">{c.name}</div>
			<div class="truncate text-xs text-gray-400">{c.character}</div>
		</a>
	{/snippet}
</Row>
