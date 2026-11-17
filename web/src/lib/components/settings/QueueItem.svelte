<script lang="ts">
	// One download in a Radarr or Sonarr queue: title, how far along, quality and size, and what's wrong.
	// The app's logo shows on the left when the list mixes both apps; `action` goes beside the problem (Fix).
	import type { Snippet } from 'svelte';
	import { fmtBytes } from '#lib/api.ts';
	import type { QueueRow, ServiceKind } from '#lib/types.ts';

	let { q, kind, color, logo = false, action }: { q: QueueRow; kind: ServiceKind; color: string; logo?: boolean; action?: Snippet } = $props();

	const STATE_TEXT: Record<string, string> = { queued: 'Waiting', paused: 'Paused', importing: 'Importing', warning: 'Needs attention', error: 'Failed' };
	const STATE_TONE: Record<string, string> = { warning: 'text-yellow-400', error: 'text-red-400', paused: 'text-gray-400', queued: 'text-gray-500', importing: 'text-green-400' };
	const bar = $derived(q.state === 'error' ? '#ef4444' : q.state === 'warning' ? '#facc15' : q.state === 'paused' ? '#6b7280' : color);
</script>

<div class="flex gap-3">
	{#if logo}<img src="/logos/{kind}.svg" alt="" class="mt-3 size-4 shrink-0" />{/if}
	<div class="min-w-0 flex-1 py-2.5">
		<div class="flex items-baseline justify-between gap-3 text-xs"><span class="min-w-0 truncate text-gray-200">{q.title}</span>
			<span class="shrink-0 tabular-nums {STATE_TONE[q.state] ?? 'text-gray-400'}">{q.state === 'downloading' ? `${q.percent}%${q.timeLeft ? ` · ${q.timeLeft}` : ''}` : STATE_TEXT[q.state]}</span></div>
		<div class="mt-1.5 h-1 overflow-hidden rounded-full bg-gray-700/60"><div class="h-full rounded-full {q.state === 'importing' ? 'arr-stripes' : ''}" style="width:{q.state === 'queued' ? 0 : Math.max(q.percent, 2)}%; background:{bar}"></div></div>
		<div class="mt-1 text-[11px] text-gray-500">{q.quality}{q.size ? ` · ${fmtBytes(q.size)}` : ''}</div>
		{#if q.message || action}
			<div class="mt-1.5 flex flex-wrap items-center justify-between gap-2">
				{#if q.message}<span class="min-w-0 text-[11px] {STATE_TONE[q.state] ?? 'text-gray-400'}">⚠ {q.message}</span>{/if}
				{#if action}<span class="ml-auto shrink-0">{@render action()}</span>{/if}
			</div>
		{/if}
	</div>
</div>
