// Poster hover (chosen on the mockup test page): the poster tilts up to 16° toward the mouse, lifts with a deep
// shadow, a glare and an orange edge glow follow the mouse, and one streak of light sweeps across on enter.
// Mouse only; off with "reduce motion". Phones and tablets are untouched.
const DEG = 16;
const LIFT = 1.07;
const EASE = 0.18;
const SHINE_MS = 1800;

export const tiltEnabled = () =>
	typeof matchMedia !== 'undefined' && matchMedia('(hover: hover) and (pointer: fine)').matches && !matchMedia('(prefers-reduced-motion: reduce)').matches;

/** Svelte action on the poster frame. Its parent (the card, never transformed) is what the mouse is measured against. */
export function tilt(frame: HTMLElement) {
	const card = frame.parentElement;
	if (!card || !tiltEnabled()) return;
	const glare = frame.querySelector<HTMLElement>('[data-glare]');
	const edge = frame.querySelector<HTMLElement>('[data-edge]');
	const shine = frame.querySelector<HTMLElement>('[data-shine]');
	// target (mouse) and current (eased toward it each frame), both -0.5 … 0.5
	let tx = 0, ty = 0, cx = 0, cy = 0, cz = 1, on = false, raf = 0;

	const draw = () => {
		cx += (tx - cx) * EASE;
		cy += (ty - cy) * EASE;
		cz += ((on ? LIFT : 1) - cz) * EASE;
		// perspective() must be on the frame's own transform; on a parent it only reaches direct children.
		frame.style.transform = `perspective(600px) rotateX(${(-cy * DEG).toFixed(2)}deg) rotateY(${(cx * DEG).toFixed(2)}deg) scale(${cz.toFixed(4)})`;
		// Deep shadow cast away from the pressed corner, plus a tight one under the poster.
		frame.style.boxShadow = on ? `${(-cx * 44).toFixed(0)}px ${(40 - cy * 30).toFixed(0)}px 50px -6px rgb(0 0 0 / 0.9), 0 12px 24px -4px rgb(0 0 0 / 0.6)` : '';
		const at = `${((cx + 0.5) * 100).toFixed(1)}% ${((cy + 0.5) * 100).toFixed(1)}%`;
		if (glare) {
			glare.style.opacity = on ? '1' : '0';
			glare.style.background = `radial-gradient(circle at ${at}, rgb(255 255 255 / 0.55), transparent 60%)`;
		}
		if (edge) {
			edge.style.opacity = on ? '1' : '0';
			edge.style.background = `radial-gradient(circle at ${at}, var(--color-orange-400) 0%, rgb(249 115 22 / 0.85) 30%, rgb(234 88 12 / 0.35) 55%, transparent 80%)`;
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
	const enter = (e: PointerEvent) => {
		if (e.pointerType !== 'mouse') return;
		shine?.animate([{ backgroundPosition: '150% 0' }, { backgroundPosition: '-50% 0' }], { duration: SHINE_MS, easing: 'cubic-bezier(.3,.6,.4,1)' });
	};
	const leave = () => {
		on = false;
		tx = ty = 0;
		kick();
	};
	card.addEventListener('pointerenter', enter);
	card.addEventListener('pointermove', move);
	card.addEventListener('pointerleave', leave);
	return {
		destroy() {
			cancelAnimationFrame(raf);
			card.removeEventListener('pointerenter', enter);
			card.removeEventListener('pointermove', move);
			card.removeEventListener('pointerleave', leave);
		}
	};
}
