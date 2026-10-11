<script lang="ts">
	import type { CastMember } from '#lib/types.ts';
	import Row from './Row.svelte';
	import { tilt, tiltEnabled } from '#lib/tilt.ts';
	const tilting = tiltEnabled();
	import Img from './Img.svelte';
	let { cast }: { cast: CastMember[] } = $props();
</script>

<Row title="Cast" items={cast}>
	{#snippet item(c)}
		<a href="/person/{c.id}" class="w-24 shrink-0 tablet:w-28">
			<div use:tilt class="relative aspect-[2/3] overflow-hidden rounded-xl border border-gray-700/40 bg-gray-800/40 shadow-xl">
				{#if c.profile}<Img path={c.profile} kind="profile" sizes="(min-width: 640px) 112px, 96px" class="h-full w-full" />
				{:else}<div class="grid h-full place-items-center text-2xl text-gray-600">{c.name.charAt(0)}</div>{/if}
				{#if tilting}<div data-glare class="pointer-events-none absolute inset-0 opacity-0 mix-blend-soft-light"></div>{/if}
			</div>
			<div class="relative">
				<div class="mt-2 truncate text-[13px] font-medium">{c.name}</div>
				<div class="truncate text-xs text-gray-400">{c.character}</div>
			</div>
		</a>
	{/snippet}
</Row>
