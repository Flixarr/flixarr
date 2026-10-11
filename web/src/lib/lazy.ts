// One shared IntersectionObserver for images. Unlike loading="lazy" (which starts 1–2.5k px early on slow
// connections), this waits until an image is within MARGIN of being visible, including inside sideways rows.
const MARGIN = '300px';
const waiting = new Map<Element, () => void>();
let io: IntersectionObserver | undefined;

/** Svelte action: calls show() once the element is near the screen. */
export function nearScreen(node: Element, show: () => void) {
	io ??= new IntersectionObserver(
		(entries) => {
			for (const e of entries) {
				if (!e.isIntersecting) continue;
				waiting.get(e.target)?.();
				waiting.delete(e.target);
				io!.unobserve(e.target);
			}
		},
		{ rootMargin: MARGIN }
	);
	waiting.set(node, show);
	io.observe(node);
	return {
		destroy() {
			waiting.delete(node);
			io?.unobserve(node);
		}
	};
}
