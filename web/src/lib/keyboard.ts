// iPhone and iPad only open the keyboard when a text box is focused during the tap itself, not after the
// page changes (#162). Tapping Search focuses an invisible stand-in box right away, which opens the
// keyboard; the Search page then moves the focus to its own box, and the keyboard stays up.
let standIn: HTMLInputElement | null = null;

const ios = () => /iPhone|iPad|iPod/.test(navigator.userAgent) || (navigator.maxTouchPoints > 1 && /Macintosh/.test(navigator.userAgent));

export function primeKeyboard() {
	if (typeof window === 'undefined' || !ios()) return;
	releaseKeyboard();
	const el = document.createElement('input');
	el.setAttribute('aria-hidden', 'true');
	el.tabIndex = -1;
	// 16px: smaller text makes iOS zoom the page when the box gets focus.
	Object.assign(el.style, { position: 'fixed', top: '0', left: '0', width: '1px', height: '1px', opacity: '0', fontSize: '16px', border: '0', padding: '0' });
	document.body.appendChild(el);
	el.focus({ preventScroll: true });
	standIn = el;
	setTimeout(() => standIn === el && releaseKeyboard(), 3000); // nothing took over: let it go
}

export function releaseKeyboard() {
	standIn?.remove();
	standIn = null;
}
