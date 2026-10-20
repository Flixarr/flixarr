<script lang="ts">
	// Top bar: transparent over banners, easing to frosted glass on scroll; account + notification menus.
	import { afterNavigate, goto } from '$app/navigation';
	import { page } from '$app/state';
	import Icon from './Icon.svelte';
	import Avatar from './Avatar.svelte';
	import { session, signOut, can } from '#lib/session.svelte.ts';
	import { alerts, anyNew, openedBell, readAllAlerts, servicesUnread, show as showCount, tapAlert, untapped, watchAlerts } from '#lib/alerts.svelte.ts';
	import { ago, api, img } from '#lib/api.ts';
	import { attention, watchAttention } from '#lib/attention.svelte.ts';
	import { roleLabel } from '#lib/roles.ts';

	let { transparent = false }: { transparent?: boolean } = $props();
	let scrolled = $state(false);
	let menu = $state<'' | 'user' | 'notif' | 'test'>('');
	// Dev install: the DEV badge, and View as for admins (the server applies it).
	const testInstall = $derived(!!session.status?.testInstall);
	const viewing = $derived(session.me?.viewAs ?? '');
	const canViewAs = $derived(testInstall && (can('admin') || !!viewing));
	async function viewAs(role: string) {
		menu = '';
		await api('/test/view-as', { method: 'POST', body: JSON.stringify({ role }) });
		location.reload();
	}
	// The bar's height, for windows that open below it on phones (--top-bar).
	let barHeight = $state(0);
	$effect(() => {
		document.documentElement.style.setProperty('--top-bar', `${barHeight}px`);
		return () => document.documentElement.style.removeProperty('--top-bar');
	});

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

	afterNavigate(() => {
		menu = '';
		setTimeout(showCount); // the new page has set its title: put the count back in front
	});

	$effect(() => {
		if (session.me) {
			const a = watchAlerts(), b = watchAttention();
			return () => (a?.(), b());
		}
	});
	// Opening the bell marks what's in it as seen.
	$effect(() => {
		if (menu === 'notif') openedBell();
	});

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

{#snippet alertsPanel()}
	<div class="flex flex-col overflow-hidden rounded-xl border border-gray-600/70 bg-gray-900 shadow-[0_24px_64px_-8px_rgba(0,0,0,0.95),0_8px_24px_rgba(0,0,0,0.7)]">
		<div class="flex items-center justify-between gap-3 border-b border-gray-700/40 px-4 py-3">
			<span class="font-semibold">Alerts</span>
			<span class="flex items-center gap-3">
				{#if untapped()}<button class="text-xs text-gray-400 hover:text-white" onclick={readAllAlerts}>Mark all as read</button>{/if}
			</span>
		</div>
		{#if alerts.list.length}
			<div class="max-h-[70vh] divide-y divide-gray-700/40 overflow-y-auto tablet:max-h-[60vh]">
				{#each alerts.list as a (a.id)}
					<button class="flex w-full items-start gap-3 px-4 py-3 text-left hover:bg-white/5 {a.kind === 'todo' ? 'border-l-2 border-l-primary' : ''}" onclick={() => { menu = ''; if (!a.tapped) tapAlert(a.id); goto(a.link); }}>
						{#if a.poster}<img src={img(a.poster, 'w92')} alt="" class="h-12 w-8 shrink-0 rounded object-cover {a.tapped ? 'opacity-50' : ''}" />
						{:else if a.key.startsWith('arr:')}<img src="/logos/{a.key.split(':')[1]}.svg" alt="" class="mt-0.5 size-8 shrink-0 {a.tapped ? 'opacity-50' : ''}" />
						{:else}<span class="grid h-12 w-8 shrink-0 place-items-center rounded bg-gray-800 text-gray-400"><Icon name="bell" class="size-4" /></span>{/if}
						<span class="min-w-0 flex-1">
							<span class="block truncate text-sm {a.tapped ? 'text-gray-400' : 'font-semibold text-white'}">{a.title}</span>
							<span class="block truncate text-xs {a.tapped ? 'text-gray-500' : 'text-gray-300'}">{a.detail}</span>
							<span class="mt-0.5 block text-[11px] text-gray-500">{ago(a.at)}</span>
						</span>
						{#if a.new}<span class="mt-1.5 size-2 shrink-0 rounded-full bg-red-500"></span>{/if}
					</button>
				{/each}
			</div>
		{:else}
			<div class="px-4 py-10 text-center text-sm text-gray-400">You're all caught up.</div>
		{/if}
	</div>
{/snippet}

<header bind:offsetHeight={barHeight} class="fixed inset-x-0 top-0 z-30 border-b pt-[max(0px,calc(env(safe-area-inset-top)-0.75rem))] transition-[background-color,border-color,backdrop-filter] duration-500 ease-out {solid ? 'border-gray-700/40 bg-gray-950/85 backdrop-blur-xl' : 'border-transparent bg-transparent backdrop-blur-[0px]'}">
	<div class="mx-auto flex max-w-[1600px] items-center gap-6 px-4 py-3 laptop:px-10">
		<div class="flex items-center gap-2">
			<a href="/" class="text-logo text-3xl laptop:text-4xl">Flixarr</a>
			{#if testInstall}
				<div class="relative" data-menu>
					<button class="rounded-md bg-yellow-400/15 px-1.5 py-0.5 text-[10px] font-bold tracking-wider text-yellow-300 ring-1 ring-yellow-400/40 {canViewAs ? 'hover:bg-yellow-400/25' : 'cursor-default'}" aria-label="Dev install{viewing ? `, viewing as ${viewing}` : ''}" onclick={() => canViewAs && (menu = menu === 'test' ? '' : 'test')}>DEV{#if viewing}<span class="ml-1 font-semibold">· {viewing}</span>{/if}</button>
					{#if menu === 'test'}
						<div class="absolute left-0 top-8 z-50 w-56 overflow-hidden rounded-xl border border-gray-700/60 bg-gray-900 py-1.5 shadow-[0_24px_64px_-8px_rgba(0,0,0,0.95),0_8px_24px_rgba(0,0,0,0.7)]">
							<div class="px-4 pb-1.5 pt-1 text-[11px] text-gray-500">Dev install. View the app as:</div>
							{#each [['', 'Yourself'], ['Admin', 'An admin'], ['Manager', 'A manager'], ['Trusted', 'A trusted user'], ['User', 'A user']] as [role, label] (role)}
								<button class="flex w-full items-center justify-between px-4 py-2 text-left text-sm hover:bg-white/5 {viewing === role ? 'text-white' : 'text-gray-300'}" onclick={() => viewAs(role)}>{label}{#if viewing === role}<Icon name="check" class="size-4 text-primary-light" />{/if}</button>
							{/each}
						</div>
					{/if}
				</div>
			{/if}
		</div>
		<nav class="hidden items-center gap-1 text-[15px] tablet:flex">
			{#each NAV as [label, h] (h)}
				<a href={h} class="relative rounded-lg px-3 py-1.5 {active(h) ? 'bg-white/10 font-semibold text-white' : 'text-gray-300 hover:text-white'}">{label}{#if h === '/requests' && attention()}<span class="absolute -right-1 -top-1 grid min-w-4 place-items-center rounded-full bg-red-500 px-1 text-[10px] font-bold leading-4 text-white">{attention() > 99 ? '99+' : attention()}</span>{/if}</a>
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
				<button class="relative rounded-full p-2 hover:bg-white/10" aria-label="Alerts{untapped() ? ` (${untapped()} not read)` : ''}" onclick={() => (menu = menu === 'notif' ? '' : 'notif')}>
					<Icon name="bell" />
					{#if untapped()}
						<span class="absolute -right-0.5 -top-0.5 grid min-w-4 place-items-center rounded-full px-1 text-[10px] font-bold leading-4 text-white {anyNew() ? 'alert-pulse bg-red-500' : 'bg-gray-600'}">{untapped() > 99 ? '99+' : untapped()}</span>
					{/if}
				</button>
				{#if menu === 'notif'}
					<div class="absolute right-0 top-11 hidden w-96 tablet:block">
						{@render alertsPanel()}
					</div>
				{/if}
			</div>
			{#if session.me}
				<div class="relative" data-menu>
					<button class="relative rounded-full p-1 hover:bg-white/10" aria-label="Account menu" onclick={() => (menu = menu === 'user' ? '' : 'user')}><Avatar user={session.me} />{#if admin && servicesUnread()}<span class="alert-dot absolute right-0.5 top-0.5"></span>{/if}</button>
					{#if menu === 'user'}
						<div class="fixed right-3 top-[calc(4rem+max(0px,calc(env(safe-area-inset-top)-0.75rem)))] w-60 tablet:absolute tablet:right-0 tablet:top-11">
							<div class="overflow-hidden rounded-xl border border-gray-700/60 bg-gray-900 py-1.5 shadow-2xl">
								<div class="flex items-center gap-3 px-4 py-2"><Avatar user={session.me} class="size-9 text-sm" /><div><div class="text-sm font-medium">{session.me.username}</div><div class="text-xs text-gray-400">{roleLabel(session.me)}</div></div></div>
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

<!-- Phones: the alerts dropdown spans the screen's width under the top bar. It sits outside the header
     because the header's backdrop blur makes it the box fixed children are placed in (#101). -->
{#if menu === 'notif'}
	<div class="fixed inset-x-3 top-[calc(4rem+max(0px,calc(env(safe-area-inset-top)-0.75rem)))] z-50 tablet:hidden" data-menu>
		{@render alertsPanel()}
	</div>
{/if}

