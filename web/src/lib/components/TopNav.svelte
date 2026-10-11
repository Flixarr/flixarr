<script lang="ts">
	// Top bar: transparent over banners, easing to frosted glass on scroll; account + notification menus.
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import Icon from './Icon.svelte';
	import Avatar from './Avatar.svelte';
	import { session, signOut } from '#lib/session.svelte.ts';

	let { transparent = false }: { transparent?: boolean } = $props();
	let scrolled = $state(false);
	let menu = $state<'' | 'user' | 'notif'>('');
	let q = $state('');

	const NAV = [['Discover', '/'], ['Movies', '/movies'], ['Series', '/series'], ['Requests', '/requests']] as const;
	const active = (h: string) => (h === '/' ? page.url.pathname === '/' : page.url.pathname.startsWith(h));
	const solid = $derived(!transparent || scrolled);
	const admin = $derived(session.me?.role === 'Admin');

	$effect(() => {
		const check = () => (scrolled = scrollY > 40);
		check();
		addEventListener('scroll', check, { passive: true });
		return () => removeEventListener('scroll', check);
	});
	$effect(() => {
		if (!menu) return;
		const close = (e: MouseEvent) => !(e.target as Element).closest('[data-menu]') && (menu = '');
		addEventListener('click', close);
		return () => removeEventListener('click', close);
	});

	async function out() {
		await signOut();
		goto('/login');
	}
</script>

<header class="fixed inset-x-0 top-0 z-30 border-b transition-[background-color,border-color,backdrop-filter] duration-500 ease-out {solid ? 'border-gray-700/40 bg-gray-950/85 backdrop-blur-xl' : 'border-transparent bg-transparent backdrop-blur-[0px]'}">
	<div class="mx-auto flex max-w-[1600px] items-center gap-6 px-4 py-3 laptop:px-10">
		<a href="/" class="text-logo text-3xl laptop:text-4xl">Flixarr</a>
		<nav class="hidden items-center gap-1 text-[15px] tablet:flex">
			{#each NAV as [label, h] (h)}
				<a href={h} class="rounded-lg px-3 py-1.5 {active(h) ? 'bg-white/10 font-semibold text-white' : 'text-gray-300 hover:text-white'}">{label}</a>
			{/each}
		</nav>
		<form class="ml-auto hidden w-72 items-center gap-2 rounded-xl border border-gray-700/50 bg-gray-900/60 px-3 py-2 text-gray-400 backdrop-blur laptop:flex"
			onsubmit={(e) => { e.preventDefault(); if (q.trim()) goto(`/search?q=${encodeURIComponent(q.trim())}`); }}>
			<Icon name="search" class="size-4" />
			<input bind:value={q} class="w-full bg-transparent text-sm text-white placeholder-gray-500 outline-none" placeholder="Search movies, series, people" />
		</form>
		<div class="ml-auto flex items-center gap-1.5 laptop:ml-0">
			<a href="/search" class="rounded-full p-2 hover:bg-white/10 laptop:hidden" aria-label="Search"><Icon name="search" /></a>
			<div class="relative" data-menu>
				<button class="rounded-full p-2 hover:bg-white/10" aria-label="Notifications" onclick={() => (menu = menu === 'notif' ? '' : 'notif')}><Icon name="bell" /></button>
				{#if menu === 'notif'}
					<div class="fixed inset-x-3 top-16 tablet:absolute tablet:inset-x-auto tablet:right-0 tablet:top-11 tablet:w-96">
						<div class="overflow-hidden rounded-xl border border-gray-700/60 bg-gray-900 shadow-2xl">
							<div class="border-b border-gray-700/40 px-4 py-3 font-semibold">Notifications</div>
							<div class="px-4 py-8 text-center text-sm text-gray-400">Nothing yet. You'll see request updates here.</div>
						</div>
					</div>
				{/if}
			</div>
			{#if session.me}
				<div class="relative" data-menu>
					<button class="rounded-full p-1 hover:bg-white/10" aria-label="Account menu" onclick={() => (menu = menu === 'user' ? '' : 'user')}><Avatar user={session.me} /></button>
					{#if menu === 'user'}
						<div class="fixed inset-x-3 top-16 tablet:absolute tablet:inset-x-auto tablet:right-0 tablet:top-11 tablet:w-60">
							<div class="overflow-hidden rounded-xl border border-gray-700/60 bg-gray-900 py-1.5 shadow-2xl">
								<div class="flex items-center gap-3 px-4 py-2"><Avatar user={session.me} class="size-9 text-sm" /><div><div class="text-sm font-medium">{session.me.username}</div><div class="text-xs text-gray-400">{session.me.role}</div></div></div>
								<div class="my-1 border-t border-gray-700/40"></div>
								<a href="/profile" class="flex items-center gap-3 px-4 py-2.5 text-sm text-gray-300 hover:bg-white/5 hover:text-white"><Icon name="profile" class="size-4" />Profile</a>
								{#if admin}
									<a href="/issues" class="flex items-center gap-3 px-4 py-2.5 text-sm text-gray-300 hover:bg-white/5 hover:text-white"><Icon name="issues" class="size-4" />Issues</a>
									<a href="/users" class="flex items-center gap-3 px-4 py-2.5 text-sm text-gray-300 hover:bg-white/5 hover:text-white"><Icon name="users" class="size-4" />Users</a>
									<a href="/settings" class="flex items-center gap-3 px-4 py-2.5 text-sm text-gray-300 hover:bg-white/5 hover:text-white"><Icon name="settings" class="size-4" />Settings</a>
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
