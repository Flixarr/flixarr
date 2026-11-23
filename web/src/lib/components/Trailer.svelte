<script lang="ts">
	import Modal from './Modal.svelte';
	import Spinner from './Spinner.svelte';
	let { key, open = $bindable(false) }: { key: string; open?: boolean } = $props();

	// The player loads once the window has finished opening (so the animation never stutters) and fades in when
	// it's ready. Until then the box shows a spinner.
	let ready = $state(false);
	let loaded = $state(false);
	$effect(() => {
		if (!open) ready = loaded = false;
	});

	// YouTube's own controls, with suggestions from other channels (rel), pop ups (iv_load_policy) and its keyboard
	// shortcuts (disablekb) off. playsinline=0: phones play it in their own full screen player.
	const params = 'autoplay=1&rel=0&iv_load_policy=3&disablekb=1&playsinline=0';
</script>

<Modal bind:open title="Trailer" wide center cinema onopened={() => (ready = true)}>
	<div class="relative aspect-video">
		{#if !loaded}<div class="absolute inset-0 grid place-items-center"><Spinner /></div>{/if}
		<!-- YouTube refuses embeds that don't say which site they're on ("video player configuration error", 153).
		     The app's Referrer-Policy is same-origin, so this player alone sends just the origin. -->
		{#if ready && open}<iframe class="absolute inset-0 h-full w-full transition-opacity duration-300 {loaded ? '' : 'opacity-0'}" title="Trailer" src="https://www.youtube-nocookie.com/embed/{key}?{params}" onload={() => (loaded = true)} allow="autoplay; encrypted-media; picture-in-picture; fullscreen" allowfullscreen referrerpolicy="strict-origin-when-cross-origin"></iframe>{/if}
	</div>
</Modal>
