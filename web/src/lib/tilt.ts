// Poster hover (chosen on the mockup test page): the poster tilts up to 16° toward the mouse and lifts with a deep
// shadow, and a soft glare follows the mouse.
// Mouse only; off with "reduce motion". Phones and tablets are untouched.
const DEG = 16;
const LIFT = 1.07;
const EASE = 0.18; // tilt follows the mouse
const ZOOM_EASE = 0.07; // lift in/out is slower and softer

export const tiltEnabled = () =>
	typeof matchMedia !== 'undefined' && matchMedia('(hover: hover) and (pointer: fine)').matches && !matchMedia('(prefers-reduced-motion: reduce)').matches;

/** Svelte action on the poster frame. Its parent (never transformed) is what the mouse is measured against. */
export function tilt(frame: HTMLElement) {
	const card = frame.parentElement;
	if (!card || !tiltEnabled()) return;
	const glare = frame.querySelector<HTMLElement>('[data-glare]');
	// target (mouse) and current (eased toward it each frame), both -0.5 … 0.5
	let tx = 0, ty = 0, cx = 0, cy = 0, cz = 1, on = false, raf = 0;

	const draw = () => {
		cx += (tx - cx) * EASE;
		cy += (ty - cy) * EASE;
		cz += ((on ? LIFT : 1) - cz) * ZOOM_EASE;
		// 0 → 1 as the poster lifts: the shadow and glare grow and fade with it instead of snapping.
		const k = Math.max(0, Math.min(1, (cz - 1) / (LIFT - 1)));
		// perspective() must be on the frame's own transform; on a parent it only reaches direct children.
		frame.style.transform = `perspective(600px) rotateX(${(-cy * DEG).toFixed(2)}deg) rotateY(${(cx * DEG).toFixed(2)}deg) scale(${cz.toFixed(4)})`;
		// Shadow cast away from the pressed corner, plus a tight one under the poster.
		// Kept close to the poster (small drop, negative spread) so it never spills down onto the title/date/rating text.
		frame.style.boxShadow = `${(-cx * 18 * k).toFixed(1)}px ${((12 - cy * 10) * k).toFixed(1)}px ${(32 * k).toFixed(1)}px -10px rgb(0 0 0 / ${(0.9 * k).toFixed(3)}), 0 ${(6 * k).toFixed(1)}px ${(14 * k).toFixed(1)}px -4px rgb(0 0 0 / ${(0.55 * k).toFixed(3)})`;
		const at = `${((cx + 0.5) * 100).toFixed(1)}% ${((cy + 0.5) * 100).toFixed(1)}%`;
		if (glare) {
			glare.style.opacity = k.toFixed(3);
			glare.style.background = `radial-gradient(circle at ${at}, rgb(255 255 255 / 0.55), transparent 60%)`;
		}
		if (on || Math.abs(tx - cx) > 0.001 || Math.abs(ty - cy) > 0.001 || Math.abs(cz - 1) > 0.001) raf = requestAnimationFrame(draw);
		else {
			raf = 0; // settled: snap exactly back to rest
			cx = cy = 0;
			cz = 1;
			frame.style.transform = frame.style.boxShadow = '';
		}
	};
	const kick = () => raf || (raf = requestAnimationFrame(draw));

	const move = (e: PointerEvent) => {
		if (e.pointerType !== 'mouse') return;
		const r = card.getBoundingClientRect();
		tx = Math.max(-0.5, Math.min(0.5, (e.clientX - r.left) / frame.offsetWidth - 0.5));
		ty = Math.max(-0.5, Math.min(0.5, (e.clientY - r.top) / frame.offsetHeight - 0.5));
		on = true;
		kick();
	};
	const leave = () => {
		on = false;
		tx = ty = 0;
		kick();
	};
	card.addEventListener('pointermove', move);
	card.addEventListener('pointerleave', leave);
	return {
		destroy() {
			cancelAnimationFrame(raf);
			card.removeEventListener('pointermove', move);
			card.removeEventListener('pointerleave', leave);
		}
	};
}
