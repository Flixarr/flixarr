<script lang="ts">
	// Pull down at the top of a page to reload it. Only in the home-screen app: there iOS has no pull to
	// refresh of its own (browsers do, so it stays off there).
	import Icon from './Icon.svelte';

	let pull = $state(0); // px the indicator has come down
	const READY = 70;

	$effect(() => {
		const standalone = matchMedia('(display-mode: standalone)').matches || (navigator as Navigator & { standalone?: boolean }).standalone;
		if (!standalone) return;
		let x0 = 0, y0 = 0, on = false;
		const start = (e: TouchEvent) => {
			// Not while a window is open (the page is pinned then) or the page is scrolled.
			on = scrollY <= 0 && document.body.style.position !== 'fixed' && e.touches.length === 1;
			x0 = e.touches[0].clientX;
			y0 = e.touches[0].clientY;
		};
		const move = (e: TouchEvent) => {
			if (!on) return;
			const dx = e.touches[0].clientX - x0, dy = e.touches[0].clientY - y0;
			if (dy <= 0 || Math.abs(dx) > dy) { pull = 0; if (Math.abs(dx) > 10) on = false; return; }
			pull = Math.min(dy * 0.5, READY + 20);
		};
		const end = () => {
			if (on && pull >= READY) location.reload();
			on = false;
			pull = 0;
		};
		addEventListener('touchstart', start, { passive: true });
		addEventListener('touchmove', move, { passive: true });
		addEventListener('touchend', end);
		addEventListener('touchcancel', end);
		return () => {
			removeEventListener('touchstart', start);
			removeEventListener('touchmove', move);
			removeEventListener('touchend', end);
			removeEventListener('touchcancel', end);
		};
	});
</script>

{#if pull > 4}
	<div class="pointer-events-none fixed inset-x-0 z-40 flex justify-center" style="top:calc(env(safe-area-inset-top) + {pull - 30}px)">
		<span class="grid size-9 place-items-center rounded-full bg-gray-900 shadow-xl ring-1 ring-gray-700/60 {pull >= READY ? 'text-primary-light' : 'text-gray-400'}" style="transform:rotate({pull * 3}deg)"><Icon name="refresh" class="size-4" /></span>
	</div>
{/if}
