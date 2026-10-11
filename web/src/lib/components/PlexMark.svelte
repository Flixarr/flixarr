<script lang="ts">
	// Plex chevron mark (design rule 7).
	let { class: cls = 'size-5', color = '#e5a00d' }: { class?: string; color?: string } = $props();
</script>

<svg class={cls} viewBox="0 0 24 24" aria-hidden="true"><path fill={color} d="M4.5 1.5h7.4L20 12l-8.1 10.5H4.5L12.6 12z" /></svg>
