<script lang="ts">
	// A copy is in the library but hasn't reached the quality its profile aims for. Shown as a gray badge,
	// or just the icon where a row has no room, with the explanation in a tooltip.
	import Icon from './Icon.svelte';
	import Tooltip from './Tooltip.svelte';

	let { text = "The downloaded quality doesn't meet the selected quality. It'll be replaced automatically when a better copy is available.", icon = false }: { text?: string; icon?: boolean } = $props();
</script>

<!-- Inside a season's summary row, a tap on the tooltip must not open the season. -->
<span role="presentation" onclick={(e) => e.preventDefault()}>
	<Tooltip {text}>
		{#if icon}
			<span class="grid size-5 shrink-0 place-items-center text-gray-400" aria-label="Quality unmet"><Icon name="upgrade" class="size-4" /></span>
		{:else}
			<span class="rounded-md bg-gray-900/70 px-1.5 py-0.5 text-gray-300 ring-1 ring-gray-700/60">Quality unmet</span>
		{/if}
	</Tooltip>
</span>
