<script lang="ts">
	// Phone-only bottom tabs: Discover · Browse · Search · Requests · Issues (#192, for anyone who can report or manage
	// issues). Profile lives in the avatar menu.
	// Browse opens Movies or Series, whichever was used last (Browse.svelte remembers it).
	import { page } from '$app/state';
	import { lastBrowse } from './Browse.svelte';
	import Icon from './Icon.svelte';
	import { attention, openIssues } from '#lib/attention.svelte.ts';
	import { can } from '#lib/session.svelte.ts';
	import CountBadge from './CountBadge.svelte';
	import { primeKeyboard } from '#lib/keyboard.ts';
	const browse = $derived((void page.url.pathname, lastBrowse()));
	const TABS = $derived([['Discover', '/', 'discover'], ['Browse', browse, 'browse'], ['Search', '/search', 'search'], ['Requests', '/requests', 'requests'], ...(can('manageIssues') || can('reportIssues') ? [['Issues', '/issues', 'issues']] : [])] as [string, string, string][]);
	const badge = (h: string) => (h === '/requests' ? attention() : h === '/issues' ? openIssues() : 0);
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
	<div class="grid {TABS.length === 5 ? 'grid-cols-5' : 'grid-cols-4'}">
		{#each TABS as [label, h, icon] (label)}
			<a href={h} onclick={(e) => h === '/search' && searchTap(e)} class="flex flex-col items-center gap-1 py-2.5 text-[11px] {on(h) ? 'text-primary-light' : 'text-gray-400'}" aria-label={badge(h) ? `${label}, ${badge(h)} ${h === '/issues' ? 'open' : 'need attention'}` : undefined}><span class="relative"><Icon name={icon} class="size-6" /><CountBadge n={badge(h)} class="-right-2.5 -top-1.5" /></span>{label}</a>
		{/each}
	</div>
</nav>
