<script lang="ts">
	import { goto } from '$app/navigation';
	import { api, fmtDate } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { session, signOut } from '#lib/session.svelte.ts';
	import type { UpdateStatus } from '#lib/types.ts';
	import Avatar from '#lib/components/Avatar.svelte';
	import Icon from '#lib/components/Icon.svelte';

	const v = $derived(session.status?.version);
	const isAdmin = $derived(session.me?.role === 'Admin');
	let upd = $state<UpdateStatus | null>(null);
	let payload = $state<Record<string, string> | null>(null);
	let showSent = $state(false);
	let busy = $state(false);

	$effect(() => {
		upd = session.status?.update ?? null;
		if (isAdmin) api<{ status: UpdateStatus; payload: Record<string, string> }>('/update').then((d) => { upd = d.status; payload = d.payload; });
	});

	async function setEnabled(on: boolean) {
		try {
			upd = await api<UpdateStatus>('/update/settings', { method: 'PUT', body: JSON.stringify({ enabled: on }) });
			notify(on ? 'Daily update checks are on.' : 'Update checks are off. Nothing is sent.', on ? 'green' : 'orange');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}
	async function checkNow() {
		busy = true;
		try {
			upd = await api<UpdateStatus>('/update/check', { method: 'POST' });
			notify(upd.available ? 'An update is available.' : "You're up to date.");
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Check failed.', 'orange');
		} finally {
			busy = false;
		}
	}
	async function out() {
		await signOut();
		goto('/login');
	}
</script>

<main class="mx-auto max-w-[1200px] space-y-6 px-4 pt-20 laptop:px-10 laptop:pt-24">
	{#if session.me}
		<section class="flex flex-wrap items-center gap-4">
			<Avatar user={session.me} class="size-16 text-2xl" />
			<div class="min-w-0 flex-1">
				<h1 class="text-2xl font-bold">{session.me.username}</h1>
				<div class="flex flex-wrap items-center gap-2 text-sm text-gray-400">
					<span class="rounded-md bg-primary/15 px-1.5 py-0.5 text-xs text-primary-light ring-1 ring-primary/40">{session.me.role}</span>{session.me.email}<span>· Plex account</span>
				</div>
			</div>
			<button class="btn-ghost text-sm" onclick={out}><Icon name="out" class="size-4" />Sign out</button>
		</section>

		{#if session.me.role === 'Admin'}
			<section>
				<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Admin</div>
				<div class="grid gap-2 tablet:grid-cols-3">
					{#each [['Issues', '/issues', 'issues', 'Reported problems'], ['Users', '/users', 'users', 'People and permissions'], ['Settings', '/settings', 'settings', 'Server settings']] as [l, h, i, d] (h)}
						<a href={h} class="card flex items-center gap-3 px-4 py-3 transition hover:ring-1 hover:ring-primary/50">
							<span class="grid size-9 place-items-center rounded-lg bg-gray-800 text-gray-300"><Icon name={i} class="size-4" /></span>
							<span class="flex-1"><span class="block text-sm font-medium">{l}</span><span class="block text-xs text-gray-500">{d}</span></span>
							<Icon name="chevron" class="size-4 text-gray-500" />
						</a>
					{/each}
				</div>
			</section>
		{/if}

		<section class="card divide-y divide-gray-700/40">
			<div class="flex justify-between px-4 py-3 text-sm"><span class="text-gray-400">Version</span><span class="font-mono">{v?.full} <span class="text-xs text-gray-500">({v?.commit})</span></span></div>
			<div class="flex justify-between px-4 py-3 text-sm"><span class="text-gray-400">Channel</span><span class="capitalize">{v?.channel}</span></div>
			<div class="flex items-center justify-between gap-4 px-4 py-3 text-sm">
				<span class="text-gray-400">Updates</span>
				<span class="text-right">
					{#if !upd?.enabled}<span class="text-gray-400">Not checking</span>
					{:else if upd.available && upd.latest}<span class="font-medium text-primary-light">Update available: {upd.latest.version}.{upd.latest.build}</span>
					{:else if upd.checkedAt}<span class="text-green-400">Up to date</span>
					{:else}<span class="text-gray-400">First check in a few minutes</span>{/if}
					{#if upd?.checkedAt}<span class="block text-xs text-gray-500">Checked {fmtDate(upd.checkedAt, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })}</span>{/if}
				</span>
			</div>
			{#if isAdmin && upd}
				<div class="space-y-3 px-4 py-3">
					<label class="flex cursor-pointer items-center justify-between gap-4">
						<span><span class="block text-sm">Check for updates daily</span><span class="block text-xs text-gray-500">Asks api.flixarr.app once a day if a newer build exists. That check is also how Flixarr counts how many installs are out there. Turn it off and nothing is sent.</span></span>
						<span class="relative inline-flex shrink-0">
							<input type="checkbox" class="peer sr-only" checked={upd.enabled} disabled={upd.locked} onchange={(e) => setEnabled((e.currentTarget as HTMLInputElement).checked)} />
							<span class="h-6 w-11 rounded-full bg-gray-700 transition peer-checked:bg-primary peer-disabled:opacity-50"></span>
							<span class="absolute left-0.5 top-0.5 size-5 rounded-full bg-white shadow transition peer-checked:translate-x-5"></span>
						</span>
					</label>
					{#if upd.locked}<p class="text-xs text-yellow-400">Turned off on the server with FLIXARR_UPDATE_CHECK=false.</p>{/if}
					<div class="flex flex-wrap gap-2">
						<button class="btn-ghost px-3 py-1.5 text-xs" disabled={!upd.enabled || busy} onclick={checkNow}>{busy ? 'Checking…' : 'Check now'}</button>
						<button class="btn-ghost px-3 py-1.5 text-xs" onclick={() => (showSent = !showSent)}>{showSent ? 'Hide' : 'Show'} exactly what's sent</button>
					</div>
					{#if showSent && payload}
						<pre class="overflow-x-auto rounded-lg bg-gray-950/70 p-3 text-xs text-gray-300">{JSON.stringify(payload, null, 2)}</pre>
						<p class="text-xs text-gray-500">The install ID is random, made once for this server, and used for nothing else. Never sent: your users, requests, Plex details or library.</p>
					{/if}
				</div>
			{/if}
			<div class="flex items-center gap-4 px-4 py-4">
				<span class="rounded-md bg-linear-to-r from-[#90cea1] to-tmdb px-2 py-1 text-xs font-black tracking-wide text-gray-950">TMDB</span>
				<p class="text-xs text-gray-400">This product uses the TMDB API but is not endorsed or certified by TMDB.</p>
			</div>
		</section>
	{/if}
</main>
