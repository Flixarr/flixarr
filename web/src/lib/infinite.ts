// Infinite scroll: put this action on an element after the grid. While it's within MARGIN of the screen it keeps
// calling more() (one page at a time) until more() returns false or the element scrolls out of range.
const MARGIN = '1200px';

export function infinite(node: Element, more: () => Promise<boolean>) {
	let near = false;
	let running = false;
	let current = more;
	const pump = async () => {
		if (running || !near) return;
		running = true;
		try {
			while (near && (await current())) await new Promise((r) => requestAnimationFrame(r));
		} finally {
			running = false;
		}
	};
	const io = new IntersectionObserver(([e]) => {
		near = e.isIntersecting;
		pump();
	}, { rootMargin: MARGIN });
	io.observe(node);
	return {
		update(next: () => Promise<boolean>) {
			current = next;
		},
		destroy() {
			io.disconnect();
		}
	};
}
