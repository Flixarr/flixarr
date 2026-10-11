<script lang="ts">
	import Modal from './Modal.svelte';
	let { key, open = $bindable(false) }: { key: string; open?: boolean } = $props();
</script>

<Modal bind:open title="Trailer" wide>
	<div class="aspect-video bg-black">
		{#if open}<iframe class="h-full w-full" title="Trailer" src="https://www.youtube-nocookie.com/embed/{key}?autoplay=1" allow="autoplay; encrypted-media; picture-in-picture" allowfullscreen></iframe>{/if}
	</div>
</Modal>
