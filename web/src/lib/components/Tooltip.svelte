<script lang="ts">
	// A styled tooltip (never the browser's own title popup). Shows on mouse hover and keyboard
	// focus, and on tap on phones. `align` picks which edge of the trigger the bubble lines up with.
	import type { Snippet } from 'svelte';
	let { text, align = 'center', children }: { text: string; align?: 'left' | 'center' | 'right'; children: Snippet } = $props();
	let open = $state(false);
	let root: HTMLSpanElement;

	$effect(() => {
		if (!open) return;
		const away = (e: PointerEvent) => !root.contains(e.target as Node) && (open = false);
		document.addEventListener('pointerdown', away);
		return () => document.removeEventListener('pointerdown', away);
	});
	const pos = { left: 'left-0', center: 'left-1/2 -translate-x-1/2', right: 'right-0' };
</script>

<span bind:this={root} class="relative inline-flex" role="group"
	onpointerenter={(e) => e.pointerType === 'mouse' && (open = true)}
	onpointerleave={(e) => e.pointerType === 'mouse' && (open = false)}
	onpointerup={(e) => e.pointerType !== 'mouse' && (open = !open)}
	onfocusin={() => (open = true)} onfocusout={() => (open = false)}>
	{@render children()}
	{#if open && text}
		<span role="tooltip" class="pointer-events-none absolute bottom-full z-30 mb-2 whitespace-nowrap rounded-md border border-gray-700/60 bg-gray-900 px-2 py-1 text-[11px] font-normal text-gray-200 shadow-[0_8px_24px_-4px_rgb(0_0_0/0.7)] {pos[align]}">{text}</span>
	{/if}
</span>
