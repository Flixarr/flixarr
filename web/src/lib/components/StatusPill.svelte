<script lang="ts">
	// Where a request is, as a pill. Everyone sees the short set (Requested, Downloading, Available…);
	// exact (people who manage requests, in the request window) names every step.
	import { simple, text, TONE, NEUTRAL } from '#lib/requeststatus.ts';
	import type { Request } from '#lib/types.ts';
	let { status, progress = 0, exact = false, class: cls = 'px-2 py-0.5 text-[11px]' }: { status: Request['status']; progress?: number; exact?: boolean; class?: string } = $props();
	const pill = $derived(exact ? { label: text(status, progress), tone: TONE[status] ?? NEUTRAL } : simple(status, progress));
</script>

{#if pill}<span class="inline-flex shrink-0 whitespace-nowrap rounded-lg font-medium ring-1 {pill.tone} {cls}">{pill.label}</span>{/if}
