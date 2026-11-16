// Put the page back at a saved scroll position after Back. The restored content can still be loading (and the page
// too short until it does), so keep re-applying it for up to three seconds. Stops early if the
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
		if (stable < 5 && ++frames < 180) requestAnimationFrame(step);
	};
	step();
}

// Sideways rows (cast, recommendations, Discover rows) by their data-row name, so Back puts each one where it was.
export const rowSpots = () => Object.fromEntries([...document.querySelectorAll<HTMLElement>('[data-row]')].map((r) => [r.dataset.row, r.scrollLeft]));
export function restoreRows(spots: Record<string, number>) {
	let frames = 0;
	const step = () => {
		let left = 0;
		for (const [name, x] of Object.entries(spots)) {
			const r = document.querySelector<HTMLElement>(`[data-row="${CSS.escape(name)}"]`);
			if (!r) left++;
			else if (Math.abs(r.scrollLeft - x) > 1) {
				r.scrollLeft = x;
				if (Math.abs(r.scrollLeft - x) > 1) left++; // row not filled in yet
			}
		}
		if (left && ++frames < 180) requestAnimationFrame(step);
	};
	step();
}
