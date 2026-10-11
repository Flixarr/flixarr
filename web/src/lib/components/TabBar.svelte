<script lang="ts">
	// Phone-only bottom tabs: Discover · Search · Requests · Profile (confirmed by Marc).
	import { page } from '$app/state';
	import Icon from './Icon.svelte';
	const TABS = [['Discover', '/', 'discover'], ['Search', '/search', 'search'], ['Requests', '/requests', 'requests'], ['Profile', '/profile', 'profile']] as const;
	const on = (h: string) => {
		const p = page.url.pathname;
		if (h === '/') return p === '/';
		if (h === '/profile') return ['/profile', '/settings', '/users', '/issues'].some((x) => p.startsWith(x));
		return p.startsWith(h);
	};
</script>

<nav class="fixed inset-x-0 bottom-0 z-30 border-t border-gray-700/40 bg-gray-950/85 pb-[env(safe-area-inset-bottom)] backdrop-blur-xl tablet:hidden">
	<div class="grid grid-cols-4">
		{#each TABS as [label, h, icon] (h)}
			<a href={h} class="flex flex-col items-center gap-1 py-2.5 text-[11px] {on(h) ? 'text-primary-light' : 'text-gray-400'}"><Icon name={icon} class="size-6" />{label}</a>
		{/each}
	</div>
</nav>
