<script lang="ts">
	let { facts, keywords = [] }: { facts: [string, string | number | null | undefined][]; keywords?: string[] } = $props();
</script>

<div class="card divide-y divide-gray-700/40">
	{#each facts.filter(([, v]) => v) as [k, v] (k)}
		<div class="flex justify-between gap-4 px-4 py-2.5 text-sm"><span class="text-gray-400">{k}</span><span class="text-right">{v}</span></div>
	{/each}
	{#if keywords.length}
		<div class="flex flex-wrap gap-1.5 px-4 py-3">
			{#each keywords as k (k)}<span class="rounded-full border border-gray-700/60 bg-gray-800/50 px-2 py-0.5 text-[11px] text-gray-300">{k}</span>{/each}
		</div>
	{/if}
</div>
