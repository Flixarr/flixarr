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
	// While the phone keyboard is open the tabs step aside (#295): iOS otherwise lets them ride up over the keyboard
	// when the page scrolls. The keyboard is open when a text box has focus and the visible part of the page is
	// clearly shorter than the page (iOS shrinks only the visible part). A box focused without a keyboard (iOS
	// doesn't open one on its own) leaves the tabs alone.
	let typing = $state(false);
	const isText = (el: Element | null) => el instanceof HTMLElement && (el.matches('textarea, [contenteditable=""], [contenteditable="true"]') || (el.matches('input') && !['checkbox', 'radio', 'range', 'button', 'submit', 'file', 'color'].includes((el as HTMLInputElement).type)));
	$effect(() => {
		const vv = visualViewport;
		if (!vv) return;
		const check = () => (typing = isText(document.activeElement) && vv.height < innerHeight - 120);
		const later = () => setTimeout(check, 60);
		check();
		vv.addEventListener('resize', check);
		addEventListener('focusin', later);
		addEventListener('focusout', later);
		return () => { vv.removeEventListener('resize', check); removeEventListener('focusin', later); removeEventListener('focusout', later); };
	});
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
<nav aria-hidden={typing} class="{typing ? 'invisible translate-y-full opacity-0' : ''} fixed inset-x-0 bottom-0 z-30 border-t border-gray-700/40 bg-gray-950/85 pb-[max(4px,calc(env(safe-area-inset-bottom)-16px))] backdrop-blur-xl tablet:hidden">
	<div class="grid {TABS.length === 5 ? 'grid-cols-5' : 'grid-cols-4'}">
		{#each TABS as [label, h, icon] (label)}
			<a href={h} onclick={(e) => h === '/search' && searchTap(e)} class="flex flex-col items-center gap-1 py-2.5 text-[11px] {on(h) ? 'text-primary-light' : 'text-gray-400'}" aria-label={badge(h) ? `${label}, ${badge(h)} ${h === '/issues' ? 'open' : 'need attention'}` : undefined}><span class="relative"><Icon name={icon} class="size-6" /><CountBadge n={badge(h)} class="-right-2.5 -top-1.5" /></span>{label}</a>
		{/each}
	</div>
</nav>
