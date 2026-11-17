<script lang="ts">
	// A section of Settings → Services: a small heading with a note on the right, then its content.
	import type { Snippet } from 'svelte';

	let { title, id = '', right, children }: { title: string; id?: string; right?: Snippet; children: Snippet } = $props();
</script>

<section {id} class="scroll-mt-24">
	<div class="mb-2 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 px-1">
		<h3 class="text-[11px] font-semibold uppercase tracking-wide text-gray-500">{title}</h3>
		{#if right}<span class="text-xs text-gray-500">{@render right()}</span>{/if}
	</div>
	{@render children()}
</section>
