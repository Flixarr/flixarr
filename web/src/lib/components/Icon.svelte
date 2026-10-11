<script lang="ts">
	// Line icons used across Flixarr (24px grid, stroke-based).
	const PATHS: Record<string, string> = {
		discover: '<path d="m3 11 9-8 9 8v9a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z"/>',
		search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
		requests: '<path d="M9 5H7a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2h-2"/><rect x="9" y="3" width="6" height="4" rx="1"/><path d="m9 14 2 2 4-4"/>',
		profile: '<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>',
		bell: '<path d="M6 8a6 6 0 1 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/>',
		settings: '<circle cx="12" cy="12" r="3"/><path d="M12 2v3m0 14v3M4.2 4.2l2.1 2.1m11.4 11.4 2.1 2.1M2 12h3m14 0h3M4.2 19.8l2.1-2.1M17.7 6.3l2.1-2.1"/>',
		users: '<circle cx="9" cy="8" r="4"/><path d="M1 21a8 8 0 0 1 16 0"/><path d="M17 4a4 4 0 0 1 0 8m6 9a8 8 0 0 0-5-7.4"/>',
		issues: '<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/><path d="M12 9v4m0 4h.01"/>',
		out: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="m16 17 5-5-5-5m5 5H9"/>',
		close: '<path d="M18 6 6 18M6 6l12 12"/>',
		check: '<path d="m5 12 5 5L20 7"/>',
		play: '<path d="M7 4v16l13-8z" fill="currentColor"/>',
		flag: '<path d="M4 22V4a1 1 0 0 1 1-1h12l-2 5 2 5H5"/>',
		chevron: '<path d="m9 18 6-6-6-6"/>',
		plus: '<path d="M12 5v14M5 12h14"/>',
		clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
		refresh: '<path d="M21 12a9 9 0 0 1-15.4 6.4L3 16m0 5v-5h5M3 12a9 9 0 0 1 15.4-6.4L21 8m0-5v5h-5"/>',
		info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5m0-8h.01"/>'
	};
	let { name, class: cls = 'size-5' }: { name: string; class?: string } = $props();
</script>

<svg class={cls} fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" viewBox="0 0 24 24" aria-hidden="true">
	{@html PATHS[name] ?? ''}
</svg>
