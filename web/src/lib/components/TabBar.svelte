<script lang="ts">
	// Phone-only bottom tabs: Discover · Browse · Search · Requests. Profile lives in the avatar menu.
	// Browse opens Movies or Series, whichever was used last (Browse.svelte remembers it).
	import { page } from '$app/state';
	import { lastBrowse } from './Browse.svelte';
	import Icon from './Icon.svelte';
	const browse = $derived((void page.url.pathname, lastBrowse()));
	const TABS = $derived([['Discover', '/', 'discover'], ['Browse', browse, 'browse'], ['Search', '/search', 'search'], ['Requests', '/requests', 'requests']] as const);
	const on = (h: string) => {
		const p = page.url.pathname;
		if (h === '/') return p === '/';
		if (h === browse) return ['/movies', '/series', '/discover/'].some((x) => p.startsWith(x));
		return p.startsWith(h);
	};
</script>

<!-- Bottom: the home indicator area, less 16px; iOS keeps that space even while the indicator is hidden. -->
<nav class="fixed inset-x-0 bottom-0 z-30 border-t border-gray-700/40 bg-gray-950/85 pb-[max(4px,calc(env(safe-area-inset-bottom)-16px))] backdrop-blur-xl tablet:hidden">
	<div class="grid grid-cols-4">
		{#each TABS as [label, h, icon] (label)}
			<a href={h} class="flex flex-col items-center gap-1 py-2.5 text-[11px] {on(h) ? 'text-primary-light' : 'text-gray-400'}"><Icon name={icon} class="size-6" />{label}</a>
		{/each}
	</div>
</nav>
