<script lang="ts">
	// A panel that slides in from the right edge: the full width on phones, 28rem wide on bigger screens.
	// Used for the admin panel. Tapping the dimmed page, the close button or Escape closes it; the page
	// behind stays put while it's open. The title row leaves room for an iPhone's clock and Dynamic Island.
	import type { Snippet } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import Icon from './Icon.svelte';
	import { lock } from '#lib/scrolllock.ts';

	let { open = $bindable(false), title, sub = '', children }: { open?: boolean; title: string; sub?: string; children: Snippet } = $props();

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
	<div class="fixed inset-0 z-50 bg-gray-950/70" transition:fade={{ duration: 150 }} onclick={() => (open = false)}></div>
	<div class="fixed inset-y-0 right-0 z-50 flex w-full flex-col bg-gray-900 shadow-2xl tablet:max-w-md tablet:border-l tablet:border-gray-700/40" role="dialog" aria-modal="true" aria-label={title} transition:fly={{ x: 480, duration: 220, opacity: 1 }}>
		<div class="flex shrink-0 items-start justify-between gap-4 border-b border-gray-700/40 px-5 pb-4 pt-[max(1rem,env(safe-area-inset-top))]">
			<div class="min-w-0"><div class="font-semibold">{title}</div>{#if sub}<div class="truncate text-xs text-gray-400">{sub}</div>{/if}</div>
			<button class="rounded-lg p-1 text-gray-400 hover:bg-white/10 hover:text-white" onclick={() => (open = false)} aria-label="Close"><Icon name="close" /></button>
		</div>
		<div class="min-h-0 flex-1 overflow-y-auto overscroll-contain">{@render children()}</div>
	</div>
{/if}
