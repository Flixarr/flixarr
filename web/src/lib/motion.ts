// Opening animations for windows (#268). Each returns its animations, held at the end while the window is open;
// closing plays the same ones backwards (Modal.svelte).

const reduced = () => matchMedia('(prefers-reduced-motion: reduce)').matches;
const fade = (el: HTMLElement, duration: number) => el.animate([{ opacity: 0 }, { opacity: 1 }], { duration, fill: 'both' });

// Lights down + focus pull (the trailer): the page dims to almost black and sinks back a little, like a theater
// before the movie, while the window starts big and blurry and sharpens into focus. `page` is the page behind
// (none when the window opens on top of another one). Reduced motion gets a short fade.
export function cinema(backdrop: HTMLElement, win: HTMLElement, page: HTMLElement | null): Animation[] {
	if (reduced()) return [fade(backdrop, 150), fade(win, 150)];
	const dim = { duration: 650, easing: 'ease-in-out', fill: 'both' } as const;
	const anims = [
		backdrop.animate([{ opacity: 0 }, { opacity: 1 }], dim),
		win.animate(
			[{ opacity: 0, transform: 'scale(1.12)', filter: 'blur(18px)' }, { opacity: 1, offset: 0.4 }, { opacity: 1, transform: 'none', filter: 'blur(0)' }],
			{ duration: 520, easing: 'cubic-bezier(.2,.7,.2,1)', fill: 'both' }
		)
	];
	if (page) {
		// Sink toward the middle of the screen, wherever the page is scrolled to.
		const r = page.getBoundingClientRect();
		page.style.transformOrigin = `${innerWidth / 2 - r.left}px ${innerHeight / 2 - r.top}px`;
		anims.push(page.animate([{ transform: 'none', filter: 'none' }, { transform: 'scale(.97)', filter: 'blur(3px)' }], dim));
	}
	return anims;
}
