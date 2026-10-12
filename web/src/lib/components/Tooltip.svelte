<script lang="ts">
	// A styled tooltip (never the browser's own title popup). Shows on mouse hover and keyboard
	// focus, and on tap on phones. It's positioned on the screen (fixed), so cards and scrolling
	// boxes can't cut it off; it flips below when there's no room above. `align` picks which edge of
	// the trigger the bubble lines up with.
	import type { Snippet } from 'svelte';
	let { text, align = 'center', children }: { text: string; align?: 'left' | 'center' | 'right'; children: Snippet } = $props();
	let open = $state(false);
	let root: HTMLSpanElement;
	let style = $state('');

	function place() {
		const r = root.getBoundingClientRect();
		const below = r.top < 40;
		const y = below ? `top:${r.bottom + 8}px` : `top:${r.top - 8}px; transform:translateY(-100%)`;
		const x = align === 'left' ? `left:${r.left}px` : align === 'right' ? `right:${innerWidth - r.right}px` : `left:${r.left + r.width / 2}px; translate:-50% 0`;
		style = `${x}; ${y}`;
	}
	function show() {
		place();
		open = true;
	}

	$effect(() => {
		if (!open) return;
		const away = (e: PointerEvent) => !root.contains(e.target as Node) && (open = false);
		const hide = () => (open = false); // scrolling moves the trigger, so the tip goes away
		document.addEventListener('pointerdown', away);
		addEventListener('scroll', hide, true);
		return () => {
			document.removeEventListener('pointerdown', away);
			removeEventListener('scroll', hide, true);
		};
	});
</script>

<span bind:this={root} class="relative inline-flex" role="group"
	onpointerenter={(e) => e.pointerType === 'mouse' && show()}
	onpointerleave={(e) => e.pointerType === 'mouse' && (open = false)}
	onpointerup={(e) => e.pointerType !== 'mouse' && (open ? (open = false) : show())}
	onfocusin={show} onfocusout={() => (open = false)}>
	{@render children()}
	{#if open && text}
		<span role="tooltip" {style} class="pointer-events-none fixed z-[70] whitespace-nowrap rounded-md border border-gray-700/60 bg-gray-900 px-2 py-1 text-[11px] font-normal text-gray-200 shadow-[0_8px_24px_-4px_rgb(0_0_0/0.7)]">{text}</span>
	{/if}
</span>
