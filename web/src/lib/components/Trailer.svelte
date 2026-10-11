<script lang="ts">
	import Modal from './Modal.svelte';
	let { key, open = $bindable(false) }: { key: string; open?: boolean } = $props();
</script>

<Modal bind:open title="Trailer" wide>
	<div class="aspect-video bg-black">
		<!-- YouTube refuses embeds that don't say which site they're on ("video player configuration error", 153).
		     The app's Referrer-Policy is same-origin, so this player alone sends just the origin. -->
		{#if open}<iframe class="h-full w-full" title="Trailer" src="https://www.youtube-nocookie.com/embed/{key}?autoplay=1" allow="autoplay; encrypted-media; picture-in-picture" allowfullscreen referrerpolicy="strict-origin-when-cross-origin"></iframe>{/if}
	</div>
</Modal>
