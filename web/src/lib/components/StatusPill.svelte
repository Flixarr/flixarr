<script lang="ts">
	// Where a request is, as a pill. Everyone sees the short set (Downloading, Almost ready…); exact (people
	// who manage requests) names every step. The state comes from the server (#301).
	import { pill as pillOf, type State } from '#lib/requeststatus.ts';
	let { state, progress = 0, series = false, exact = false, class: cls = 'px-2 py-0.5 text-[11px]' }: { state: State; progress?: number; series?: boolean; exact?: boolean; class?: string } = $props();
	const pill = $derived(pillOf(state, progress, series, exact));
</script>

<span class="inline-flex shrink-0 whitespace-nowrap rounded-lg font-medium ring-1 {pill.tone} {cls}">{pill.label}</span>
