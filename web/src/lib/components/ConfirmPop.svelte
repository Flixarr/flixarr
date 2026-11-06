<script lang="ts">
	// A confirmation that opens right on top of the button that asked for it, so it's always in view
	// (a box further down the page can be out of sight). The button is the children snippet.
	import type { Snippet } from 'svelte';

	let { open = $bindable(false), text, confirm, cancel = 'Cancel', busy = false, align = 'left', class: cls = '', onconfirm, children }: {
		open?: boolean; text: string; confirm: string; cancel?: string; busy?: boolean; align?: 'left' | 'right'; class?: string; onconfirm: () => void; children: Snippet;
	} = $props();
	let box = $state<HTMLElement>();
</script>

<svelte:window
	onkeydown={(e) => open && e.key === 'Escape' && (open = false)}
	onpointerdown={(e) => open && box && !box.contains(e.target as Node) && (open = false)}
/>

<span bind:this={box} class="relative inline-flex {cls}">
	{@render children()}
	{#if open}
		<div class="absolute bottom-full z-30 mb-2 w-64 max-w-[calc(100vw-2.5rem)] space-y-2 rounded-lg bg-gray-800 p-3 text-xs text-gray-300 shadow-[0_12px_40px_rgba(0,0,0,0.7)] ring-1 ring-gray-700/60 {align === 'left' ? 'left-0' : 'right-0'}" role="alertdialog" aria-label={confirm}>
			<p>{text}</p>
			<div class="flex justify-end gap-2">
				<button class="btn-ghost px-3 py-1 text-xs" onclick={() => (open = false)}>{cancel}</button>
				<button class="btn px-3 py-1 text-xs" disabled={busy} onclick={onconfirm}>{confirm}</button>
			</div>
		</div>
	{/if}
</span>
