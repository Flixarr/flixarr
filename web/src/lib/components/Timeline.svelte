<script lang="ts">
	// A vertical timeline, oldest first: a coloured dot, what happened, when, and an optional note.
	import { fmtDate } from '#lib/api.ts';
	export type Step = { text: string; at: string; dot: string; note?: string };
	let { steps, year = false }: { steps: Step[]; year?: boolean } = $props();
</script>

<ol class="relative ml-1.5 space-y-3 border-l border-gray-700/60 pl-4">
	{#each steps as s, i (i)}
		<li class="relative">
			<span class="absolute -left-[21px] top-1 size-2.5 rounded-full ring-2 ring-gray-900 {s.dot}"></span>
			<div class="text-xs text-gray-200">{s.text}</div>
			<div class="text-[11px] text-gray-500">{fmtDate(s.at, { month: 'short', day: 'numeric', ...(year ? { year: 'numeric' } : {}), hour: 'numeric', minute: '2-digit' })}</div>
			{#if s.note}<div class="text-[11px] text-gray-400">{s.note}</div>{/if}
		</li>
	{/each}
</ol>
