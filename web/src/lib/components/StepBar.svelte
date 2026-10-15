<script lang="ts">
	// Where a request is, as a segmented bar: Requested → Downloading → Available for everyone, or every
	// step (Requested → Grabbed → Downloading → Importing → Available) with full. Labels under it optional.
	import { simple, step, SIMPLE_STEPS, STEPS } from '#lib/requeststatus.ts';
	import type { Request } from '#lib/types.ts';
	let { status, progress = 0, full = false, labels = false, class: cls = '' }: { status: Request['status']; progress?: number; full?: boolean; labels?: boolean; class?: string } = $props();

	const names = $derived(full ? STEPS : SIMPLE_STEPS);
	const at = $derived(full ? step(status) : (simple(status, progress)?.step ?? -1));
	const done = $derived(status === 'available');
	// The current step's own fill: download progress while downloading, full once it's past downloading.
	const now = $derived(status === 'downloading' ? progress : status === 'importing' || done ? 100 : 15);
</script>

{#if at >= 0}
	<div class="grid gap-1 {cls}" style="grid-template-columns: repeat({names.length}, minmax(0, 1fr))">
		{#each names as n, i (n)}
			<div>
				<div class="h-1.5 overflow-hidden rounded-full bg-gray-700/60"><div class="h-full rounded-full {done ? 'bg-green-500' : 'bg-primary'}" style="width:{i < at ? 100 : i === at ? now : 0}%"></div></div>
				{#if labels}<div class="mt-1 truncate text-[11px] {i <= at ? 'text-gray-300' : 'text-gray-500'}">{n}</div>{/if}
			</div>
		{/each}
	</div>
{/if}
