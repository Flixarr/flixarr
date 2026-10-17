// Keeps the page still while a window or side panel is open (design rule 2).
//
// The page isn't pinned (position: fixed) or set to overflow: hidden: in an iPhone home-screen app either one
// makes iOS treat the screen as shorter by the status bar's height, which left a dark strip under full-height
// panels and pushed the bottom tabs up after a window closed (#102, #103). Instead, scrolls and swipes that
// would move the page behind are stopped; the window's own scrolling still works. Windows can stack (a
// window opened from the admin panel), so the lock counts them.

let count = 0;

export const locked = () => count > 0;

// Can this element (or one of its parents inside the window) still scroll in this direction?
function canScroll(from: Element | null, dy: number): boolean {
	for (let el = from; el && el !== document.body; el = el.parentElement) {
		if (el.scrollHeight <= el.clientHeight + 1) continue;
		const y = getComputedStyle(el).overflowY;
		if (y !== 'auto' && y !== 'scroll') continue;
		if (dy < 0 ? el.scrollTop > 0 : el.scrollTop + el.clientHeight < el.scrollHeight - 1) return true;
		if (el.matches('[role=dialog], [role=dialog] *')) return false; // at the window's edge: don't hand it to the page
	}
	return false;
}

function inWindow(t: EventTarget | null) {
	return t instanceof Element && !!t.closest('[role=dialog]');
}

function onWheel(e: WheelEvent) {
	if (!inWindow(e.target) || !canScroll(e.target as Element, e.deltaY)) e.preventDefault();
}

let startY = 0;
function onTouchStart(e: TouchEvent) {
	startY = e.touches[0]?.clientY ?? 0;
}
function onTouchMove(e: TouchEvent) {
	const dy = startY - (e.touches[0]?.clientY ?? startY); // finger up = scroll down
	if (e.touches.length > 1) return; // pinch
	if (!inWindow(e.target) || !canScroll(e.target as Element, dy)) e.preventDefault();
}

const KEYS = new Set([' ', 'PageUp', 'PageDown', 'Home', 'End', 'ArrowUp', 'ArrowDown']);
function onKey(e: KeyboardEvent) {
	const t = e.target as Element | null;
	if (!KEYS.has(e.key) || inWindow(t) || t?.closest('input, textarea, select, [contenteditable]')) return;
	e.preventDefault();
}

/** lock() keeps the page still until the returned function is called. */
export function lock(): () => void {
	if (count++ === 0) {
		document.documentElement.classList.add('modal-open');
		addEventListener('wheel', onWheel, { passive: false });
		addEventListener('touchstart', onTouchStart, { passive: true });
		addEventListener('touchmove', onTouchMove, { passive: false });
		addEventListener('keydown', onKey);
	}
	let done = false;
	return () => {
		if (done) return;
		done = true;
		if (--count === 0) {
			document.documentElement.classList.remove('modal-open');
			removeEventListener('wheel', onWheel);
			removeEventListener('touchstart', onTouchStart);
			removeEventListener('touchmove', onTouchMove);
			removeEventListener('keydown', onKey);
		}
	};
}
