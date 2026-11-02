<script lang="ts">
	// A person's profile, the same page for everyone (#149, #193): who they are, their numbers, their latest
	// requests and what they've watched lately. Settings open in a window: your own on your profile
	// (MySettings), someone else's for people who manage users (UserSettings, #160).
	import { goto } from '$app/navigation';
	import { api, ago, fmtDate } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { session, signOut } from '#lib/session.svelte.ts';
	import type { Card, UserProfile } from '#lib/types.ts';
	import { ROLE_TONE, roleLabel } from '#lib/roles.ts';
	import Avatar from './Avatar.svelte';
	import Icon from './Icon.svelte';
	import Modal from './Modal.svelte';
	import Row from './Row.svelte';
	import Poster from './Poster.svelte';
	import MySettings from './MySettings.svelte';
	import UserSettings from './UserSettings.svelte';

	let { id }: { id: number } = $props();
	let info = $state<UserProfile | null>(null);
	let watched = $state<Card[] | null>(null); // Recently watched in Plex; null while it loads
	let settingsOpen = $state(false);
	const me = $derived(id === session.me?.id);

	async function load(id: number) {
		watched = null;
		api<{ results: Card[] }>(`/users/${id}/watched`).then((d) => (watched = d.results)).catch(() => (watched = []));
		try {
			info = await api<UserProfile>(`/users/${id}`);
			document.title = me ? 'Flixarr · Profile' : `Flixarr · ${info.user.username}`;
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not load them.', 'orange');
			goto(me ? '/' : '/users', { replaceState: true });
		}
	}
	$effect(() => {
		if (id) load(id);
	});

	const u = $derived(info?.user);
	async function out() {
		await signOut();
		goto('/login');
	}
</script>

<main class="mx-auto max-w-[1200px] space-y-6 px-4 pb-32 pt-20 laptop:px-10 laptop:pt-24">
	{#if !me}<a href="/users" class="inline-flex items-center gap-1 text-sm text-gray-400 hover:text-white">← Users</a>{/if}
	{#if !info || !u}
		<p class="text-sm text-gray-400">Loading…</p>
	{:else}
		<section class="flex flex-wrap items-center gap-4">
			<Avatar user={u} class="size-16 text-2xl" />
			<div class="min-w-0 flex-1">
				<h1 class="text-2xl font-bold">{u.username}{#if me}<span class="ml-2 text-sm font-normal text-gray-500">You</span>{/if}</h1>
				<div class="flex flex-wrap items-center gap-2 text-sm text-gray-400">
					<span class="rounded-md px-1.5 py-0.5 text-xs ring-1 {ROLE_TONE[roleLabel(u)] ?? ROLE_TONE.User}">{roleLabel(u)}</span>
					{#if u.perms.includes('watchlist') && !u.perms.includes('admin')}<span class="rounded-md bg-gray-800 px-1.5 py-0.5 text-xs text-gray-400 ring-1 ring-gray-700/60">Watchlist</span>{/if}
					<span>{[u.email, u.local ? 'Local account' : 'Plex account', u.owner ? 'Owner' : ''].filter(Boolean).join(' · ')}</span>
				</div>
				<div class="mt-0.5 text-xs text-gray-500">{['Joined ' + fmtDate(u.createdAt), me ? '' : u.lastSeen ? `last seen ${ago(u.lastSeen)}` : 'not seen yet'].filter(Boolean).join(' · ')}</div>
			</div>
			<div class="flex gap-2">
				<button class="btn-ghost text-sm" onclick={() => (settingsOpen = true)}><Icon name="settings" class="size-4" />{me || info.canChange ? 'Settings' : 'Permissions'}</button>
				{#if me}<button class="btn-ghost text-sm" onclick={out}><Icon name="out" class="size-4" />Sign out</button>{/if}
			</div>
		</section>

		<section class="grid grid-cols-2 gap-2 tablet:grid-cols-4">
			{#each [['Requests', info.stats.requests, '/requests'], ['Available', info.stats.available, '/requests'], ['In progress', info.stats.inProgress + info.stats.waiting, '/requests'], ['Open issues', info.stats.openIssues, '/issues']] as [l, n, h] (l)}
				{#if me}
					<a href={h as string} class="card px-4 py-3 transition hover:ring-1 hover:ring-primary/50"><div class="text-2xl font-bold">{n}</div><div class="text-xs text-gray-400">{l}</div></a>
				{:else}
					<div class="card px-4 py-3"><div class="text-2xl font-bold">{n}</div><div class="text-xs text-gray-400">{l}</div></div>
				{/if}
			{/each}
		</section>
		{#if info.requests.length}
			<div class="-mx-4 laptop:mx-0"><Row title={me ? 'My requests' : `${u.username}'s requests`} items={info.requests} more={me ? '/requests' : undefined}>{#snippet item(c)}<Poster item={c} />{/snippet}</Row></div>
		{/if}
		{#if watched === null || watched.length}
			<div class="-mx-4 laptop:mx-0"><Row title="Recently watched" items={watched ?? []} loading={watched === null}>{#snippet item(c)}<Poster item={c} />{/snippet}</Row></div>
		{/if}
	{/if}
</main>

{#if me}
	<Modal bind:open={settingsOpen} wide title="Your settings">
		<MySettings />
		{#snippet footer()}<button class="btn-ghost" onclick={() => (settingsOpen = false)}>Done</button>{/snippet}
	</Modal>
{:else if info}
	<UserSettings bind:info bind:open={settingsOpen} />
{/if}
