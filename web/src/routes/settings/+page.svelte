<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, fmtDate } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import type { UpdateStatus } from '#lib/types.ts';
	import Icon from '#lib/components/Icon.svelte';
	import { CREDITS } from '#lib/credits.ts';
	import CacheSettings from '#lib/components/settings/CacheSettings.svelte';
	import JobsSettings from '#lib/components/settings/JobsSettings.svelte';
	import ServicesSettings from '#lib/components/settings/ServicesSettings.svelte';
	import ProfilesSettings from '#lib/components/settings/ProfilesSettings.svelte';
	import GeneralSettings from '#lib/components/settings/GeneralSettings.svelte';
	import { markAlertsSeen, servicesUnread } from '#lib/alerts.svelte.ts';

	// Section key → label, icon, one-line description (side menu / phone list).
	const SECTIONS: [string, string, string, string][] = [
		['general', 'General', 'settings', 'Requests, approvals, region'],
		['plex', 'Plex', 'play', 'Server, libraries, scans'],
		['services', 'Services', 'plus', 'Radarr, Sonarr'],
		['profiles', 'Quality profiles', 'flag', "Flixarr's profiles for Radarr and Sonarr"],
		['notifications', 'Notifications', 'bell', 'Email, Discord, webhooks, push'],
		['discover', 'Discover', 'discover', 'Discover page rows'],
		['cache', 'Cache', 'refresh', 'Saved results, images'],
		['jobs', 'Jobs', 'clock', 'Scheduled tasks'],
		['about', 'About', 'info', 'Version, updates, credits']
	];

	const isAdmin = $derived(session.me?.role === 'Admin');
	const v = $derived(session.status?.version);
	const hash = $derived(page.url.hash.slice(1));
	const tab = $derived(SECTIONS.some(([k]) => k === hash) ? hash : 'about');
	const label = $derived(SECTIONS.find(([k]) => k === tab)?.[1]);

	let upd = $state<UpdateStatus | null>(null);
	let payload = $state<Record<string, string> | null>(null);
	let showSent = $state(false);
	let busy = $state(false);

	$effect(() => {
		if (session.me && !isAdmin) goto('/', { replaceState: true });
	});
	// Reaching Services is the end of the red-dot trail.
	$effect(() => {
		if (tab === 'services' && hash === 'services' && servicesUnread()) markAlertsSeen();
	});
	$effect(() => {
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
</script>

{#snippet nav(drill: boolean)}
	{#each SECTIONS as [k, l, i, d] (k)}
		{@const on = k === tab && !drill}
		<a href="#{k}" class="flex items-center gap-3 rounded-xl text-sm transition {drill ? 'card px-4 py-4' : 'px-3 py-2.5'} {on ? 'bg-primary/15 text-white ring-1 ring-primary/50' : 'text-gray-300 hover:bg-white/5 hover:text-white'}">
			<span class={on ? 'text-primary-light' : 'text-gray-500'}><Icon name={i} class="size-4" /></span>
			<span class="flex-1"><span class="block">{l}</span>{#if drill}<span class="block text-xs text-gray-500">{d}</span>{/if}</span>
			{#if k === 'services' && servicesUnread()}<span class="alert-dot"></span>{/if}
			{#if drill}<Icon name="chevron" class="size-4 text-gray-500" />{/if}
		</a>
	{/each}
{/snippet}

{#if isAdmin}
	<main class="mx-auto max-w-[1200px] px-4 pb-28 pt-20 laptop:px-10 laptop:pt-24">
		<h1 class="text-2xl font-bold laptop:text-3xl">Settings</h1>
		<p class="text-sm text-gray-400">Server-wide settings. Only admins see this.</p>

		<div class="mt-6 tablet:grid tablet:grid-cols-[13rem_1fr] tablet:gap-8">
			<!-- Phone: section list until one is picked (design rule 9). -->
			<nav class="space-y-3 tablet:hidden {hash ? 'hidden' : ''}">{@render nav(true)}</nav>
			<nav class="hidden space-y-1 self-start tablet:sticky tablet:top-24 tablet:block">{@render nav(false)}</nav>

			<section class="space-y-4 {hash ? '' : 'hidden tablet:block'}">
				<a href="/settings" class="inline-flex items-center gap-1 text-sm text-gray-400 hover:text-white tablet:hidden"><span class="rotate-180"><Icon name="chevron" class="size-4" /></span>Settings</a>
				<h2 class="text-lg font-semibold">{label}</h2>

				{#if tab === 'general'}
					<GeneralSettings />
				{:else if tab === 'services'}
					<ServicesSettings />
				{:else if tab === 'profiles'}
					<ProfilesSettings />
				{:else if tab === 'cache'}
					<CacheSettings />
				{:else if tab === 'jobs'}
					<JobsSettings />
				{:else if tab === 'about'}
					<div>
						<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Flixarr</div>
						<div class="card divide-y divide-gray-700/40 px-4">
							<div class="flex justify-between py-3 text-sm"><span class="text-gray-400">Version</span><span class="font-mono">{v?.full} <span class="text-xs text-gray-500">({v?.commit})</span></span></div>
							<div class="flex justify-between py-3 text-sm"><span class="text-gray-400">Channel</span><span class="capitalize">{v?.channel}</span></div>
							<div class="flex items-center justify-between gap-4 py-3 text-sm">
								<span class="text-gray-400">Updates</span>
								<span class="text-right">
									{#if !upd?.enabled}<span class="text-gray-400">Not checking</span>
									{:else if upd.available && upd.latest}<span class="font-medium text-primary-light">Update available: {upd.latest.version}.{upd.latest.build}</span>
									{:else if upd.checkedAt}<span class="text-green-400">Up to date</span>
									{:else}<span class="text-gray-400">First check in a few minutes</span>{/if}
									{#if upd?.checkedAt}<span class="block text-xs text-gray-500">Checked {fmtDate(upd.checkedAt, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })}</span>{/if}
								</span>
							</div>
							{#if upd}
								<div class="space-y-3 py-3">
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
						</div>
					</div>
					<div>
						<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Credits</div>
						<div class="card divide-y divide-gray-700/40 px-4">
							<a href="https://github.com/marchershey" target="_blank" rel="noopener" class="group flex items-center gap-4 py-4">
								<img src="/credits/marc-hershey.png" alt="Marc Hershey" class="size-14 shrink-0 rounded-full ring-2 ring-primary/60" />
								<span class="flex-1"><span class="block text-xs text-gray-500">Made by</span><span class="block font-semibold group-hover:text-primary-light">Marc Hershey</span><span class="block text-xs text-gray-400">github.com/marchershey</span></span>
							</a>
							<div class="py-4">
								<div class="mb-3 text-xs text-gray-500">Built with</div>
								<div class="grid gap-2 phone:grid-cols-2 laptop:grid-cols-3">
									{#each CREDITS as c (c.id)}
										<a href={c.url} target="_blank" rel="noopener" class="flex items-center gap-3 rounded-lg border border-gray-700/40 bg-gray-900/40 px-3 py-2.5 transition hover:border-gray-600">
											<svg viewBox="0 0 24 24" class="size-7 shrink-0" fill={c.color} aria-hidden="true"><path d={c.path} /></svg>
											<span class="min-w-0"><span class="block text-sm">{c.name}</span><span class="block text-xs text-gray-500">{c.role}</span></span>
										</a>
									{/each}
								</div>
							</div>
							<div class="flex items-center gap-4 py-4">
								<span class="rounded-md bg-linear-to-r from-[#90cea1] to-tmdb px-2 py-1 text-xs font-black tracking-wide text-gray-950">TMDB</span>
								<p class="text-xs text-gray-400">This product uses the TMDB API but is not endorsed or certified by TMDB.</p>
							</div>
						</div>
					</div>
				{:else}
					<div class="card flex items-center gap-4 p-6">
						<span class="grid size-12 shrink-0 place-items-center rounded-full bg-primary/15 text-primary-light"><Icon name="clock" class="size-6" /></span>
						<div><div class="font-medium">Coming in the next update</div><p class="text-sm text-gray-400">{label} settings arrive with Radarr, Sonarr and Plex library support.</p></div>
					</div>
				{/if}
			</section>
		</div>
	</main>
{/if}
