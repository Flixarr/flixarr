<script lang="ts">
	// Bottom sheet on phone, centered dialog on tablet+. Keeps the page behind it still (design rule 2,
	// lib/scrolllock.ts). Drawn at the app's root (use:portal), so it's never boxed in by a window it opens from.
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';
	import { lock, locked } from '#lib/scrolllock.ts';
	import { portal } from '#lib/portal.ts';
	import { cinema as cinemaMotion } from '#lib/motion.ts';
	import { untrack } from 'svelte';

	// `top` replaces the standard title bar (the close button stays, top right). `center` keeps it a
	// centered dialog on phones too (the trailer: a video doesn't belong in a bottom sheet). Panels that slide in
	// from the side are SidePanel.svelte. `cinema` opens it with the lights down + focus pull animation (lib/motion.ts)
	// and plays it backwards on close; `onopened` runs once it has finished opening.
	let { open = $bindable(false), title, sub = '', wide = false, center = false, cinema = false, onopened, children, footer, top }: {
		open?: boolean; title: string; sub?: string; wide?: boolean; center?: boolean; cinema?: boolean; onopened?: () => void; children: Snippet; footer?: Snippet; top?: Snippet;
	} = $props();

	// Drawn while open, and while the closing animation plays.
	let shown = $state(false);
	let backdrop = $state<HTMLElement>();
	let win = $state<HTMLElement>();
	let anims: Animation[] = [];
	let page: HTMLElement | null = null;
	$effect(() => {
		if (open) shown = true;
	});

	// Opened on top of another window or the side panel: a darker backdrop and a lighter edge, so the two
	// are easy to tell apart.
	let stacked = $state(false);
	$effect(() => {
		if (!shown) return;
		stacked = locked();
		const unlock = lock();
		const onKey = (e: KeyboardEvent) => e.key === 'Escape' && (open = false);
		addEventListener('keydown', onKey);
		return () => {
			removeEventListener('keydown', onKey);
			unlock();
		};
	});

	$effect(() => {
		const want = open;
		if (!backdrop || !win) return;
		untrack(() => play(want));
	});

	function play(want: boolean) {
		if (!cinema) {
			if (want) onopened?.();
			else shown = false;
			return;
		}
		// A fresh animation each way, picking up where the last one was (the end of opening, or halfway if it turns
		// around). Playing the finished opening backwards made some browsers (Safari) flash the window away first.
		const old = anims;
		if (!old.length) page = stacked ? null : document.querySelector<HTMLElement>('[data-page]');
		const mine = (anims = cinemaMotion(backdrop!, win!, page, !want));
		old.forEach((a, i) => {
			const end = Number(a.effect?.getComputedTiming().endTime ?? 0);
			if (mine[i]) mine[i].currentTime = end - Number(a.currentTime ?? end);
			a.cancel();
		});
		Promise.all(mine.map((a) => a.finished)).then(
			() => {
				if (anims !== mine) return; // turned around halfway
				if (want) return onopened?.();
				for (const a of mine) a.cancel();
				anims = [];
				if (page) page.style.transformOrigin = '';
				shown = false;
			},
			() => {}
		);
	}
</script>

{#if shown}
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<div use:portal bind:this={backdrop} class="fixed inset-0 z-50 flex overscroll-none justify-center {cinema ? 'bg-black/[.93]' : stacked ? 'bg-black/75' : 'bg-gray-950/80'} backdrop-blur-sm tablet:items-center tablet:p-4 {center ? 'items-center p-3' : 'items-end'}"
		onclick={(e) => e.target === e.currentTarget && (open = false)}>
		<div bind:this={win} class="card w-full overflow-y-auto overscroll-contain bg-gray-900 {stacked ? 'shadow-[0_24px_80px_rgba(0,0,0,0.9)] ring-1 ring-gray-500/60' : ''} max-h-[min(90dvh,calc(100dvh-env(safe-area-inset-top)-var(--ios-blur,0px)))] tablet:max-h-[90dvh] tablet:rounded-b-xl {center ? '' : 'rounded-b-none'} {wide ? 'tablet:max-w-3xl' : 'tablet:max-w-lg'}" role="dialog" aria-modal="true" aria-label={title}>
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
