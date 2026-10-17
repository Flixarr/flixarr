<script lang="ts">
	// Bottom sheet on phone, centered dialog on tablet+. Freezes the page behind it (design rule 2):
	// iOS ignores overflow:hidden on body, so the body is pinned at its scroll offset and restored on close.
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';
	import { lock } from '#lib/scrolllock.ts';

	// `top` replaces the standard title bar (the close button stays, top right). `center` keeps it a
	// centered dialog on phones too (the trailer: a video doesn't belong in a bottom sheet). Panels that slide in
	// from the side are SidePanel.svelte.
	let { open = $bindable(false), title, sub = '', wide = false, center = false, children, footer, top }: {
		open?: boolean; title: string; sub?: string; wide?: boolean; center?: boolean; children: Snippet; footer?: Snippet; top?: Snippet;
	} = $props();

	$effect(() => {
		if (!open) return;
		const unlock = lock();
		const onKey = (e: KeyboardEvent) => e.key === 'Escape' && (open = false);
		addEventListener('keydown', onKey);
		return () => {
			removeEventListener('keydown', onKey);
			unlock();
		};
	});
</script>

{#if open}
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<div class="fixed inset-0 z-50 flex overscroll-none justify-center bg-gray-950/80 backdrop-blur-sm tablet:items-center tablet:p-4 {center ? 'items-center p-3' : 'items-end'}"
		onclick={(e) => e.target === e.currentTarget && (open = false)}>
		<div class="card w-full overflow-y-auto overscroll-contain bg-gray-900 max-h-[90dvh] tablet:rounded-b-xl {center ? '' : 'rounded-b-none'} {wide ? 'tablet:max-w-3xl' : 'tablet:max-w-lg'}" role="dialog" aria-modal="true" aria-label={title}>
			{#if top}
				<div class="relative">
					{@render top()}
					<button class="absolute right-3 top-3 rounded-lg p-1 text-gray-300 hover:bg-white/10 hover:text-white" onclick={() => (open = false)} aria-label="Close"><Icon name="close" /></button>
				</div>
			{:else}
				<div class="sticky top-0 z-10 flex items-start justify-between gap-4 border-b border-gray-700/40 bg-gray-900/95 px-5 py-4 backdrop-blur">
					<div><div class="font-semibold">{title}</div>{#if sub}<div class="text-xs text-gray-400">{sub}</div>{/if}</div>
					<button class="rounded-lg p-1 text-gray-400 hover:bg-white/10 hover:text-white" onclick={() => (open = false)} aria-label="Close"><Icon name="close" /></button>
				</div>
			{/if}
			{@render children()}
			{#if footer}
				<div class="sticky bottom-0 flex justify-end gap-2 border-t border-gray-700/40 bg-gray-900/95 px-5 pt-4 pb-[calc(1rem+env(safe-area-inset-bottom))] backdrop-blur tablet:pb-4">{@render footer()}</div>
			{:else if !center}
				<div class="h-[calc(1.5rem+env(safe-area-inset-bottom))] tablet:hidden"></div>
			{/if}
		</div>
	</div>
{/if}
