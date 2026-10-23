// Toasts stack at the bottom of the screen: a new one joins the stack instead of replacing the
// last. Each fades out on its own after a few seconds; at most 4 show at once (oldest go first).
export type Toast = { id: number; text: string; tone: 'green' | 'orange' | 'alert'; leaving: boolean; sub?: string; href?: string; open?: () => void };
export const toasts = $state<Toast[]>([]);
let next = 1;
const MAX = 4;

export function notify(text: string, tone: 'green' | 'orange' = 'green') {
	add({ id: next++, text, tone, leaving: false }, 2800);
}

// A new alert in the bell, shown as a toast too (#121): its title and detail; tapping it opens the alert.
export function alertToast(text: string, sub: string, href: string, open: () => void) {
	add({ id: next++, text, tone: 'alert', leaving: false, sub, href, open }, 6000);
}

function add(t: Toast, ms: number) {
	toasts.push(t);
	while (toasts.length > MAX) toasts.shift();
	setTimeout(() => dismiss(t.id), ms);
}

export function dismiss(id: number) {
	const t = toasts.find((x) => x.id === id);
	if (!t || t.leaving) return;
	t.leaving = true; // fade out, then remove
	setTimeout(() => {
		const i = toasts.findIndex((x) => x.id === id);
		if (i >= 0) toasts.splice(i, 1);
	}, 300);
}
