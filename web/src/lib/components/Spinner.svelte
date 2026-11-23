<script lang="ts">
	// A loading spinner that shows up on any background (white with a soft shadow).
	let { class: cls = 'size-10' }: { class?: string } = $props();
</script>

<svg viewBox="0 0 24 24" class="{cls} animate-spin text-white drop-shadow-[0_1px_4px_rgba(0,0,0,.8)]" fill="none" role="img" aria-label="Loading">
	<circle cx="12" cy="12" r="9.5" stroke="currentColor" stroke-opacity=".2" stroke-width="2.5" />
	<path d="M21.5 12A9.5 9.5 0 0 0 12 2.5" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" />
</svg>
