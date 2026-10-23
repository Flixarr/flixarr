<script lang="ts">
	// Phone-only bottom tabs: Discover · Browse · Search · Requests. Profile lives in the avatar menu.
	// Browse opens Movies or Series, whichever was used last (Browse.svelte remembers it).
	import { page } from '$app/state';
	import { lastBrowse } from './Browse.svelte';
	import Icon from './Icon.svelte';
	import { attention } from '#lib/attention.svelte.ts';
	import { primeKeyboard } from '#lib/keyboard.ts';
	const browse = $derived((void page.url.pathname, lastBrowse()));
	const TABS = $derived([['Discover', '/', 'discover'], ['Browse', browse, 'browse'], ['Search', '/search', 'search'], ['Requests', '/requests', 'requests']] as const);
	// Search: open the iPhone keyboard during the tap (#162); already on Search, just focus its box.
	function searchTap(e: MouseEvent) {
		if (page.url.pathname === '/search') {
			e.preventDefault();
			dispatchEvent(new CustomEvent('flixarr:focus-search'));
		} else primeKeyboard();
	}
	const on = (h: string) => {
		const p = page.url.pathname;
		if (h === '/') return p === '/';
		if (h === browse) return ['/movies', '/series', '/movie/', '/collection/', '/discover/'].some((x) => p.startsWith(x)); // title pages count as Browse (#105)
		return p.startsWith(h);
	};
</script>

<!-- Bottom: the home indicator area, less 16px; iOS keeps that space even while the indicator is hidden. -->
<nav class="fixed inset-x-0 bottom-0 z-30 border-t border-gray-700/40 bg-gray-950/85 pb-[max(4px,calc(env(safe-area-inset-bottom)-16px))] backdrop-blur-xl tablet:hidden">
	<div class="grid grid-cols-4">
		{#each TABS as [label, h, icon] (label)}
			<a href={h} onclick={(e) => h === '/search' && searchTap(e)} class="flex flex-col items-center gap-1 py-2.5 text-[11px] {on(h) ? 'text-primary-light' : 'text-gray-400'}" aria-label={h === '/requests' && attention() ? `${label}, ${attention()} need attention` : undefined}><span class="relative"><Icon name={icon} class="size-6" />{#if h === '/requests'}{#if attention()}<span class="absolute -right-2.5 -top-1.5 grid min-w-4 place-items-center rounded-full bg-red-500 px-1 text-[10px] font-bold leading-4 text-white">{attention() > 99 ? '99+' : attention()}</span>{/if}{/if}</span>{label}</a>
		{/each}
	</div>
</nav>
