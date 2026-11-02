<script lang="ts">
	// A tick box with a big tap area round it, so a thumb ticks it instead of opening the row under it.
	import Icon from './Icon.svelte';
	let { on, label, onchange, some = false }: { on: boolean; label: string; onchange: () => void; some?: boolean } = $props();
</script>

<button class="-m-3 p-3" role="checkbox" aria-checked={some && !on ? 'mixed' : on} aria-label={label} onclick={(e) => (e.stopPropagation(), onchange())}>
	<span class="grid size-4 place-items-center rounded border transition {on || some ? 'border-primary bg-primary text-white' : 'border-gray-600 hover:border-gray-400'}">
		{#if on}<Icon name="check" class="size-3" />{:else if some}<span class="h-0.5 w-2 rounded bg-white"></span>{/if}
	</span>
</button>
