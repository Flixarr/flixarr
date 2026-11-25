<script lang="ts">
	// The poster hover in one place: the frame tilts toward the mouse with a soft glare, and without a mouse (or
	// with reduce motion) the picture zooms a little instead. Its parent must not move: the mouse is measured
	// against it. Every poster, profile photo and collection cover in a browsing view uses this or Poster.
	import type { Snippet } from 'svelte';
	import { tilt, tiltEnabled } from '#lib/tilt.ts';
	let { class: cls = '', children }: { class?: string; children: Snippet } = $props();
	const tilting = tiltEnabled();
</script>

<div use:tilt class="relative overflow-hidden {tilting ? '' : '[&_img]:transition [&_img]:duration-300 group-hover:[&_img]:scale-105'} {cls}">
	{@render children()}
	{#if tilting}<div data-glare class="pointer-events-none absolute inset-0 opacity-0 mix-blend-soft-light"></div>{/if}
</div>
