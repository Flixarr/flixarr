<script lang="ts">
	// Bottom of an infinite list: a spinner while loading, a quiet end note when there's nothing left.
	let { busy, done }: { busy: boolean; done: boolean } = $props();
</script>

<div class="flex h-16 items-center justify-center text-sm text-gray-500">
	{#if busy}<span class="size-6 animate-spin rounded-full border-2 border-gray-600 border-t-primary"></span>
	{:else if done}That's everything.{/if}
</div>
