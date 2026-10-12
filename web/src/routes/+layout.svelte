<script lang="ts">
	// App root: loads the session, sends people to setup / sign-in when needed, and draws the shell.
	import '../app.css';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import TopNav from '#lib/components/TopNav.svelte';
	import TabBar from '#lib/components/TabBar.svelte';
	import Toast from '#lib/components/Toast.svelte';
	import PullRefresh from '#lib/components/PullRefresh.svelte';
	import { session, loadSession } from '#lib/session.svelte.ts';

	let { children } = $props();
	let failed = $state('');
	const bare = $derived(page.url.pathname === '/setup' || page.url.pathname === '/login');
	// Pages with a full-bleed banner start with a transparent top bar.
	const transparent = $derived(page.url.pathname === '/' || /^\/(movie|series|collection)\/\d+/.test(page.url.pathname));

	// iOS home-screen app: after the keyboard closes, iOS sometimes leaves the page laid out as if it were
	// still open, so the tab bar floats above the bottom. Nudging the scroll position makes it lay out again.
	$effect(() => {
		const fix = () =>
			setTimeout(() => {
				const y = scrollY;
				scrollTo(scrollX, y + 1);
				requestAnimationFrame(() => scrollTo(scrollX, y));
			}, 80);
		addEventListener('focusout', fix);
		visualViewport?.addEventListener('resize', fix);
		return () => {
			removeEventListener('focusout', fix);
			visualViewport?.removeEventListener('resize', fix);
		};
	});

	$effect(() => {
		loadSession().catch((e) => (failed = e instanceof Error ? e.message : 'Flixarr is unreachable.'));
	});

	$effect(() => {
		if (!session.ready) return;
		const p = page.url.pathname;
		if (session.status?.setupRequired && p !== '/setup') goto('/setup', { replaceState: true });
		else if (!session.status?.setupRequired && !session.me && p !== '/login') goto('/login', { replaceState: true });
		else if (session.me && bare) goto('/', { replaceState: true });
	});
</script>

{#if failed}
	<div class="grid min-h-dvh place-items-center p-6 text-center"><div><span class="text-logo text-5xl">Flixarr</span><p class="mt-6 text-gray-400">{failed}</p></div></div>
{:else if session.ready}
	{#if bare}
		{@render children()}
	{:else if session.me}
		<TopNav {transparent} />
		<div class="pb-24 tablet:pb-12">{@render children()}</div>
		<TabBar />
		<PullRefresh />
	{/if}
{/if}
<Toast />
