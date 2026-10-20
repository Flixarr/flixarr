<script lang="ts">
	// Pull down at the top of a page to reload it. Only in the home-screen app: there iOS has no pull to
	// refresh of its own (browsers do, so it stays off there). Feels like it's on a spring (#106): the pull
	// gets heavier the further it goes, the icon pops when letting go would refresh, springs back with a
	// little overshoot when let go early, and spins smoothly while the page reloads.
	import Icon from './Icon.svelte';
	import { locked } from '#lib/scrolllock.ts';

	let pull = $state(0); // px the indicator has come down
	let dragging = $state(false);
	let refreshing = $state(false);
	const READY = 70;
	const MAX = 120;
	// Rubber band: follows the finger at first, then slows down toward MAX.
	const band = (dy: number) => MAX * (1 - Math.exp(-dy / (MAX * 1.6)));
	const ready = $derived(pull >= READY);
	const progress = $derived(Math.min(pull / READY, 1));

	$effect(() => {
		const standalone = matchMedia('(display-mode: standalone)').matches || (navigator as Navigator & { standalone?: boolean }).standalone;
		if (!standalone) return;
		let x0 = 0, y0 = 0, on = false;
		const start = (e: TouchEvent) => {
			// Not while a window is open (the page is pinned then), the page is scrolled, or it's reloading.
			on = scrollY <= 0 && !locked() && !refreshing && e.touches.length === 1;
			x0 = e.touches[0].clientX;
			y0 = e.touches[0].clientY;
		};
		const move = (e: TouchEvent) => {
			if (!on) return;
			const dx = e.touches[0].clientX - x0, dy = e.touches[0].clientY - y0;
			if (dy <= 0 || Math.abs(dx) > dy) { pull = 0; dragging = false; if (Math.abs(dx) > 10) on = false; return; }
			dragging = true;
			pull = band(dy);
		};
		const end = () => {
			dragging = false;
			if (on && pull >= READY) {
				refreshing = true;
				pull = READY - 10; // settle into place, spin, then reload
				setTimeout(() => location.reload(), 450);
			} else pull = 0;
			on = false;
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

<div class="pointer-events-none fixed inset-x-0 z-40 flex justify-center"
	style="top:calc(env(safe-area-inset-top) - 30px); transform:translateY({pull}px); opacity:{pull > 4 ? Math.min(1, 0.3 + progress) : 0}; transition:{dragging ? 'none' : 'transform 420ms cubic-bezier(0.34, 1.56, 0.64, 1), opacity 300ms ease-out'}">
	<span class="grid size-9 place-items-center rounded-full bg-gray-900 shadow-xl ring-1 ring-gray-700/60 transition-[color,scale] duration-300 ease-[cubic-bezier(0.34,1.56,0.64,1)] {ready || refreshing ? 'text-primary-light' : 'text-gray-400'}"
		style="scale:{refreshing ? 1 : ready ? 1.15 : 0.6 + 0.4 * progress}">
		<span class="grid place-items-center {refreshing ? 'animate-spin' : ''}" style={refreshing ? '' : `transform:rotate(${pull * 3}deg)`}><Icon name="refresh" class="size-4" /></span>
	</span>
</div>
