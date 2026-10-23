// use:portal moves an element to the end of the app's root element, so windows opened from inside another
// window or panel (Fix in the request window, say) are drawn on their own: a blurred or scrolling parent
// can't box in their backdrop. The app root (not <body>) keeps Svelte's event handling working.
export function portal(node: HTMLElement) {
	const root = node.closest('body > *') ?? document.body;
	root.appendChild(node);
	return {
		destroy() {
			node.remove();
		}
	};
}
