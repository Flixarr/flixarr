<script lang="ts">
	// A small "?" button that shows a short note: on hover with a mouse, on tap on phones.
	import type { Snippet } from 'svelte';
	let { label = 'More info', children }: { label?: string; children: Snippet } = $props();
	let open = $state(false);
	let root: HTMLSpanElement;

	$effect(() => {
		if (!open) return;
		const away = (e: PointerEvent) => !root.contains(e.target as Node) && (open = false);
		const esc = (e: KeyboardEvent) => e.key === 'Escape' && (open = false);
		document.addEventListener('pointerdown', away);
		document.addEventListener('keydown', esc);
		return () => {
			document.removeEventListener('pointerdown', away);
			document.removeEventListener('keydown', esc);
		};
	});
</script>

<span class="relative inline-flex align-middle" role="group" bind:this={root}
	onpointerenter={(e) => e.pointerType === 'mouse' && (open = true)} onpointerleave={(e) => e.pointerType === 'mouse' && (open = false)}>
	<button type="button" class="inline-flex size-4 items-center justify-center rounded-full border border-gray-500 text-[10px] font-semibold leading-none text-gray-400 hover:border-gray-300 hover:text-white"
		aria-label={label} aria-expanded={open} onclick={(e) => { e.preventDefault(); open = !open; }}>?</button>
	{#if open}
		<span role="tooltip" class="absolute left-0 top-6 z-30 w-64 rounded-lg border border-gray-700/60 bg-gray-900 px-3 py-2 text-xs font-normal text-gray-300 shadow-xl">
			{@render children()}
		</span>
	{/if}
</span>
