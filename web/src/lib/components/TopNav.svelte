<script lang="ts">
	// Top bar: transparent over banners, easing to frosted glass on scroll; account + notification menus.
	import { afterNavigate, goto } from '$app/navigation';
	import { page } from '$app/state';
	import Icon from './Icon.svelte';
	import Avatar from './Avatar.svelte';
	import { session, signOut, can } from '#lib/session.svelte.ts';
	import { alerts, markAlertsSeen, servicesUnread, unreadCount, watchAlerts } from '#lib/alerts.svelte.ts';

	let { transparent = false }: { transparent?: boolean } = $props();
	let scrolled = $state(false);
	let menu = $state<'' | 'user' | 'notif'>('');

	const NAV = [['Discover', '/'], ['Movies', '/movies'], ['Series', '/series'], ['Requests', '/requests']] as const;
	const active = (h: string) => (h === '/' ? page.url.pathname === '/' : page.url.pathname.startsWith(h));
	const solid = $derived(!transparent || scrolled);
	const admin = $derived(can('admin'));

	$effect(() => {
		const check = () => (scrolled = scrollY > 40);
		check();
		addEventListener('scroll', check, { passive: true });
		return () => removeEventListener('scroll', check);
	});
	$effect(() => {
		if (!menu) return;
		// Close on a click outside the menu, or on a link inside it.
		const close = (e: MouseEvent) => {
			const t = e.target as Element;
			if (!t.closest('[data-menu]') || t.closest('[data-menu] a')) menu = '';
		};
		addEventListener('click', close);
		return () => removeEventListener('click', close);
	});

	afterNavigate(() => (menu = ''));

	$effect(() => {
		if (session.me) return watchAlerts();
	});
	// Opening the bell marks what's in it as seen.
	$effect(() => {
		if (menu === 'notif') markAlertsSeen();
	});
	function since(iso: string) {
		const m = Math.round((Date.now() - Date.parse(iso)) / 60000);
		return m < 1 ? 'just now' : m < 60 ? `${m} min ago` : m < 1440 ? `${Math.round(m / 60)} hr ago` : `${Math.round(m / 1440)} days ago`;
	}

	// Already on the search page: just put the cursor back in its box.
	function focusIfHere(e: MouseEvent) {
		if (page.url.pathname !== '/search') return;
		e.preventDefault();
		dispatchEvent(new CustomEvent('flixarr:focus-search'));
	}

	async function out() {
		menu = '';
		await signOut();
		goto('/login');
	}
</script>

<header class="fixed inset-x-0 top-0 z-30 border-b pt-[max(0px,calc(env(safe-area-inset-top)-0.75rem))] transition-[background-color,border-color,backdrop-filter] duration-500 ease-out {solid ? 'border-gray-700/40 bg-gray-950/85 backdrop-blur-xl' : 'border-transparent bg-transparent backdrop-blur-[0px]'}">
	<div class="mx-auto flex max-w-[1600px] items-center gap-6 px-4 py-3 laptop:px-10">
		<a href="/" class="text-logo text-3xl laptop:text-4xl">Flixarr</a>
		<nav class="hidden items-center gap-1 text-[15px] tablet:flex">
			{#each NAV as [label, h] (h)}
				<a href={h} class="rounded-lg px-3 py-1.5 {active(h) ? 'bg-white/10 font-semibold text-white' : 'text-gray-300 hover:text-white'}">{label}</a>
			{/each}
		</nav>
		<!-- Search: a link to the search page (which focuses its box). Laptop+: looks like a search box. Tablet: an icon.
		     Phone: nothing here, Search is in the bottom tabs. -->
		<a href="/search" onclick={focusIfHere} aria-label="Search" class="ml-auto hidden w-72 items-center gap-2 rounded-xl border border-gray-700/50 bg-gray-900/60 px-3 py-2 text-sm text-gray-500 backdrop-blur transition hover:border-gray-600 hover:text-gray-400 laptop:flex">
			<Icon name="search" class="size-4 text-gray-400" />Search movies, series, people
		</a>
		<div class="ml-auto flex items-center gap-1.5 laptop:ml-0">
			<a href="/search" onclick={focusIfHere} class="hidden rounded-full p-2 hover:bg-white/10 tablet:inline-flex laptop:hidden" aria-label="Search"><Icon name="search" /></a>
			<div class="relative" data-menu>
				<button class="relative rounded-full p-2 hover:bg-white/10" aria-label="Alerts{alerts.list.length ? ` (${alerts.list.length})` : ''}" onclick={() => (menu = menu === 'notif' ? '' : 'notif')}>
					<Icon name="bell" />
					{#if alerts.list.length}
						<span class="absolute -right-0.5 -top-0.5 grid min-w-4 place-items-center rounded-full px-1 text-[10px] font-bold leading-4 text-white {unreadCount() ? 'alert-pulse bg-red-500' : 'bg-gray-600'}">{alerts.list.length}</span>
					{/if}
				</button>
				{#if menu === 'notif'}
					<div class="fixed inset-x-3 top-[calc(4rem+max(0px,calc(env(safe-area-inset-top)-0.75rem)))] tablet:absolute tablet:inset-x-auto tablet:right-0 tablet:top-11 tablet:w-96">
						<div class="overflow-hidden rounded-xl border border-gray-700/60 bg-gray-900 shadow-2xl">
							<div class="border-b border-gray-700/40 px-4 py-3 font-semibold">Alerts</div>
							{#if alerts.list.length}
								<div class="max-h-[60vh] divide-y divide-gray-700/40 overflow-y-auto">
									{#each alerts.list as a (a.id)}
										<a href={a.link} class="flex items-start gap-3 px-4 py-3 hover:bg-white/5">
											{#if a.key.startsWith('arr:')}<img src="/logos/{a.key.split(':')[1]}.svg" alt="" class="mt-0.5 size-6 shrink-0" />{/if}
											<span class="min-w-0 flex-1">
												<span class="flex items-center gap-2 text-sm font-medium">{a.title}<span class="size-1.5 rounded-full {a.severity === 'error' ? 'bg-red-500' : a.severity === 'info' ? 'bg-green-400' : 'bg-yellow-400'}"></span></span>
												<span class="block text-xs {a.severity === 'error' ? 'text-red-300' : a.severity === 'info' ? 'text-gray-300' : 'text-yellow-300'}">{a.detail}</span>
												<span class="mt-0.5 block text-[11px] text-gray-500">Since {since(a.startedAt)}</span>
											</span>
										</a>
									{/each}
								</div>
							{:else}
								<div class="px-4 py-8 text-center text-sm text-gray-400">Nothing needs your attention.</div>
							{/if}
						</div>
					</div>
				{/if}
			</div>
			{#if session.me}
				<div class="relative" data-menu>
					<button class="relative rounded-full p-1 hover:bg-white/10" aria-label="Account menu" onclick={() => (menu = menu === 'user' ? '' : 'user')}><Avatar user={session.me} />{#if admin && servicesUnread()}<span class="alert-dot absolute right-0.5 top-0.5"></span>{/if}</button>
					{#if menu === 'user'}
						<div class="fixed right-3 top-[calc(4rem+max(0px,calc(env(safe-area-inset-top)-0.75rem)))] w-60 tablet:absolute tablet:right-0 tablet:top-11">
							<div class="overflow-hidden rounded-xl border border-gray-700/60 bg-gray-900 py-1.5 shadow-2xl">
								<div class="flex items-center gap-3 px-4 py-2"><Avatar user={session.me} class="size-9 text-sm" /><div><div class="text-sm font-medium">{session.me.username}</div><div class="text-xs text-gray-400">{session.me.role}</div></div></div>
								<div class="my-1 border-t border-gray-700/40"></div>
								<a href="/profile" class="flex items-center gap-3 px-4 py-2.5 text-sm text-gray-300 hover:bg-white/5 hover:text-white"><Icon name="profile" class="size-4" />Profile</a>
								{#if can('manageIssues') || can('reportIssues')}
									<a href="/issues" class="flex items-center gap-3 px-4 py-2.5 text-sm text-gray-300 hover:bg-white/5 hover:text-white"><Icon name="issues" class="size-4" />{can('manageIssues') ? 'Issues' : 'My issues'}</a>
								{/if}
								{#if admin}
									<a href="/users" class="flex items-center gap-3 px-4 py-2.5 text-sm text-gray-300 hover:bg-white/5 hover:text-white"><Icon name="users" class="size-4" />Users</a>
									<a href="/settings" class="flex items-center gap-3 px-4 py-2.5 text-sm text-gray-300 hover:bg-white/5 hover:text-white"><Icon name="settings" class="size-4" />Settings{#if servicesUnread()}<span class="alert-dot ml-auto"></span>{/if}</a>
								{/if}
								<div class="my-1 border-t border-gray-700/40"></div>
								<button onclick={out} class="flex w-full items-center gap-3 px-4 py-2.5 text-sm text-gray-300 hover:bg-white/5 hover:text-white"><Icon name="out" class="size-4" />Sign out</button>
							</div>
						</div>
					{/if}
				</div>
			{/if}
		</div>
	</div>
</header>
