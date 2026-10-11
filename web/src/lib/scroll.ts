// Put the page back at a saved scroll position after Back. The restored content can still be settling for a few
// frames (and the page is too short until it does), so keep re-applying it for up to a second. Stops early if the
// user scrolls on their own.
export function restoreScroll(y: number) {
	let stable = 0;
	let frames = 0;
	let stopped = false;
	const stop = () => (stopped = true);
	const opts = { once: true, passive: true } as const;
	addEventListener('wheel', stop, opts);
	addEventListener('touchstart', stop, opts);
	addEventListener('keydown', stop, opts);
	const step = () => {
		if (stopped) return;
		if (Math.abs(scrollY - y) > 1) {
			scrollTo(0, y);
			stable = 0;
		} else stable++;
		if (stable < 5 && ++frames < 60) requestAnimationFrame(step);
	};
	step();
}
