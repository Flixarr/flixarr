<script lang="ts">
	import { goto } from '$app/navigation';
	import { session, signOut } from '#lib/session.svelte.ts';
	import Avatar from '#lib/components/Avatar.svelte';
	import Icon from '#lib/components/Icon.svelte';

	const v = $derived(session.status?.version);
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
			<div class="flex items-center gap-4 px-4 py-4">
				<span class="rounded-md bg-linear-to-r from-[#90cea1] to-tmdb px-2 py-1 text-xs font-black tracking-wide text-gray-950">TMDB</span>
				<p class="text-xs text-gray-400">This product uses the TMDB API but is not endorsed or certified by TMDB.</p>
			</div>
		</section>
	{/if}
</main>
