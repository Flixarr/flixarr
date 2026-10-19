<script lang="ts">
	// Bottom sheet on phone, centered dialog on tablet+. Keeps the page behind it still (design rule 2,
	// lib/scrolllock.ts). Drawn at the app's root (use:portal), so it's never boxed in by a window it opens from.
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';
	import { lock, locked } from '#lib/scrolllock.ts';
	import { portal } from '#lib/portal.ts';

	// `top` replaces the standard title bar (the close button stays, top right). `center` keeps it a
	// centered dialog on phones too (the trailer: a video doesn't belong in a bottom sheet). Panels that slide in
	// from the side are SidePanel.svelte.
	let { open = $bindable(false), title, sub = '', wide = false, center = false, children, footer, top }: {
		open?: boolean; title: string; sub?: string; wide?: boolean; center?: boolean; children: Snippet; footer?: Snippet; top?: Snippet;
	} = $props();

	// Opened on top of another window or the side panel: a darker backdrop and a lighter edge, so the two
	// are easy to tell apart.
	let stacked = $state(false);
	$effect(() => {
		if (!open) return;
		stacked = locked();
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
	<div use:portal class="fixed inset-x-0 bottom-0 top-(--top-bar,0px) z-50 flex overscroll-none justify-center {stacked ? 'bg-black/75' : 'bg-gray-950/80'} backdrop-blur-sm tablet:top-0 tablet:items-center tablet:p-4 {center ? 'items-center p-3' : 'items-end'}"
		onclick={(e) => e.target === e.currentTarget && (open = false)}>
		<div class="card w-full overflow-y-auto overscroll-contain bg-gray-900 {stacked ? 'shadow-[0_24px_80px_rgba(0,0,0,0.9)] ring-1 ring-gray-500/60' : ''} max-h-[calc(100dvh-var(--top-bar,0px)-1.5rem)] tablet:max-h-[90dvh] tablet:rounded-b-xl {center ? '' : 'rounded-b-none'} {wide ? 'tablet:max-w-3xl' : 'tablet:max-w-lg'}" role="dialog" aria-modal="true" aria-label={title}>
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
