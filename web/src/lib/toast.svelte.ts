// One toast at a time, bottom of the screen.
export const toast = $state<{ text: string; tone: 'green' | 'orange'; show: boolean }>({ text: '', tone: 'green', show: false });
let timer: ReturnType<typeof setTimeout> | undefined;

export function notify(text: string, tone: 'green' | 'orange' = 'green') {
	toast.text = text;
	toast.tone = tone;
	toast.show = true;
	clearTimeout(timer);
	timer = setTimeout(() => (toast.show = false), 2800);
}
