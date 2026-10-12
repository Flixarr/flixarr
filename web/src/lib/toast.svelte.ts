// Toasts stack at the bottom of the screen: a new one joins the stack instead of replacing the
// last. Each fades out on its own after a few seconds; at most 4 show at once (oldest go first).
export type Toast = { id: number; text: string; tone: 'green' | 'orange'; leaving: boolean };
export const toasts = $state<Toast[]>([]);
let next = 1;
const MAX = 4;

export function notify(text: string, tone: 'green' | 'orange' = 'green') {
	const t: Toast = { id: next++, text, tone, leaving: false };
	toasts.push(t);
	while (toasts.length > MAX) toasts.shift();
	setTimeout(() => dismiss(t.id), 2800);
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
