<script lang="ts">
	// Horizontal filter pills. Svelte updates them in place, so the row keeps its scroll position (design rule 11).
	let { options, value = $bindable() }: { options: { key: string | number; label: string; count?: number }[]; value: string | number } = $props();
</script>

<div class="no-scrollbar flex gap-2 overflow-x-auto px-4 laptop:px-0">
	{#each options as o (o.key)}
		<button onclick={() => (value = o.key)}
			class="shrink-0 rounded-full px-4 py-1.5 text-sm transition {o.key === value ? 'bg-primary text-white shadow-lg' : 'border border-gray-700/60 bg-gray-800/40 text-gray-300 hover:text-white'}">
			{o.label}{#if o.count !== undefined}<span class="ml-1 text-xs opacity-70">{o.count}</span>{/if}
		</button>
	{/each}
</div>
