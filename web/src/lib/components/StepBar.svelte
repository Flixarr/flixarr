<script lang="ts">
	// Where a request is, as a segmented bar: Requested → Downloading → Available for everyone, or every
	// step (Requested → Grabbed → Downloading → Importing → Available) with full. Steps that are done are
	// green, the current one takes the state's color. A problem (Stuck, Failed) draws the step it broke on
	// red; everyone but managers reads "Hit a problem" there (#299). Labels under it optional (not on phones,
	// where five would be cut to nothing).
	import { stepAt, solidOf, SIMPLE_STEPS, STEPS, type State } from '#lib/requeststatus.ts';
	let { state, progress = 0, brokeAt = '', full = false, labels = false, class: cls = '' }: { state: State; progress?: number; brokeAt?: string; full?: boolean; labels?: boolean; class?: string } = $props();

	const names = $derived(full ? STEPS : SIMPLE_STEPS);
	const pos = $derived(stepAt(state, full, brokeAt));
	const done = $derived(state === 'available' || state === 'partial');
	// The current step's own fill: download progress while downloading, full otherwise.
	const now = $derived(state === 'downloading' ? Math.max(progress, 6) : 100);
	const fill = (i: number) => (i < pos.at || done ? 'bg-green-400' : pos.broke ? 'bg-red-400' : solidOf(state));
	const name = (n: string, i: number) => (pos.broke && i === pos.at && !full ? 'Hit a problem' : n);
</script>

{#if pos.at >= 0}
	<div class="grid gap-1 {cls}" style="grid-template-columns: repeat({names.length}, minmax(0, 1fr))">
		{#each names as n, i (n)}
			<div>
				<div class="h-1.5 overflow-hidden rounded-full bg-gray-700/60"><div class="h-full rounded-full {fill(i)}" style="width:{i < pos.at ? 100 : i === pos.at ? now : 0}%"></div></div>
				{#if labels}<div class="mt-1 hidden truncate text-[11px] tablet:block {pos.broke && i === pos.at ? 'text-red-300' : i <= pos.at ? 'text-white' : 'text-gray-400'}">{name(n, i)}</div>{/if}
			</div>
		{/each}
	</div>
{/if}
