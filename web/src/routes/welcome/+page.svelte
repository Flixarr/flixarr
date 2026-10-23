<script lang="ts">
	// The rest of first-run setup, right after the admin's first sign-in: pick the Plex server (one you own)
	// and its libraries, bring things over from Seerr if switching, connect Radarr and Sonarr, done. Every step can be skipped and changed later in
	// Settings.
	import { goto } from '$app/navigation';
	import { api } from '#lib/api.ts';
	import { session, loadSession } from '#lib/session.svelte.ts';
	import PlexSettings from '#lib/components/settings/PlexSettings.svelte';
	import ServicesSettings from '#lib/components/settings/ServicesSettings.svelte';
	import ImportSettings from '#lib/components/settings/ImportSettings.svelte';

	const STEPS = [
		{ key: 'plex', title: 'Your Plex server', about: "Pick the Plex server Flixarr works with and the libraries it reads. Titles in them show as available, and Flixarr hears from Plex the moment something new is added." },
		{ key: 'seerr', title: 'Switching from Seerr?', about: 'Bring over your users, requests, issues and Radarr and Sonarr from Seerr, Overseerr or Jellyseerr. Starting fresh? Skip this.' },
		{ key: 'services', title: 'Radarr and Sonarr', about: 'Requests are sent to Radarr (movies) and Sonarr (series). Connect them now, or later in Settings → Services.' },
		{ key: 'done', title: "You're all set", about: '' }
	];
	let i = $state(0);
	const step = $derived(STEPS[i]);

	async function finish() {
		await api('/setup/done', { method: 'POST' });
		await loadSession();
		goto('/', { replaceState: true });
	}
</script>

<div class="mx-auto min-h-dvh max-w-[900px] px-4 pb-[calc(4rem+env(safe-area-inset-bottom))] pt-[max(2rem,env(safe-area-inset-top))]">
	<div class="flex items-center justify-between gap-4 py-4">
		<span class="text-logo text-3xl">Flixarr</span>
		<div class="flex items-center gap-2" aria-label="Step {i + 1} of {STEPS.length}">
			{#each STEPS as s, n (s.key)}<span class="h-1.5 rounded-full transition-all {n === i ? 'w-6 bg-primary' : n < i ? 'w-3 bg-primary/60' : 'w-3 bg-gray-700'}"></span>{/each}
		</div>
	</div>

	<div class="mt-4 space-y-1">
		<p class="text-xs uppercase tracking-wide text-gray-500">Step {i + 1} of {STEPS.length}</p>
		<h1 class="text-2xl font-bold">{step.title}</h1>
		{#if step.about}<p class="text-sm text-gray-400">{step.about}</p>{/if}
	</div>

	<div class="mt-6 space-y-4">
		{#if step.key === 'plex'}
			<PlexSettings />
		{:else if step.key === 'seerr'}
			<ImportSettings setup />
		{:else if step.key === 'services'}
			<ServicesSettings />
		{:else}
			<div class="card space-y-3 p-5 text-sm text-gray-300">
				<p>Welcome aboard{session.me ? `, ${session.me.username}` : ''}. A few things you may want next, all in Settings:</p>
				<ul class="list-disc space-y-1 pl-5 text-gray-400">
					<li><b class="text-gray-200">Users</b>: import the people your Plex server is shared with, or let them sign in on their own.</li>
					<li><b class="text-gray-200">Requests</b>: whether requests wait for your approval, and how series can be requested.</li>
					<li><b class="text-gray-200">Notifications</b>: Discord, email, webhooks or phone notifications.</li>
				</ul>
			</div>
		{/if}
	</div>

	<div class="mt-8 flex items-center justify-between gap-3">
		{#if i > 0}<button class="btn-ghost" onclick={() => i--}>Back</button>{:else}<span></span>{/if}
		<div class="flex gap-2">
			{#if step.key !== 'done'}
				<button class="btn-ghost" onclick={() => i++}>Skip for now</button>
				<button class="btn px-6" onclick={() => i++}>Next</button>
			{:else}
				<button class="btn px-6" onclick={finish}>Go to Flixarr</button>
			{/if}
		</div>
	</div>
</div>
