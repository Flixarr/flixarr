<script lang="ts">
	import { goto } from '$app/navigation';
	import { session, signOut, can } from '#lib/session.svelte.ts';
	import Avatar from '#lib/components/Avatar.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import Row from '#lib/components/Row.svelte';
	import Poster from '#lib/components/Poster.svelte';
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import Toggle from '#lib/components/Toggle.svelte';
	import { pushSupported, needsHomeScreen, subscribed, subscribe, unsubscribe } from '#lib/push.ts';
	import type { Card, Page } from '#lib/types.ts';

	const v = $derived(session.status?.version);
	type Stats = { requests: number; available: number; waiting: number; inProgress: number; openIssues: number };
	let stats = $state<Stats | null>(null);
	let mine = $state<Card[]>([]);
	$effect(() => {
		document.title = 'Flixarr · Profile';
		api<Stats>('/me/stats').then((d) => (stats = d)).catch(() => {});
		api<Page<Card>>('/discover/recent-requests?mine=1').then((p) => (mine = p.results)).catch(() => {});
		api<Watch>('/me/watchlist').then((d) => (watch = d)).catch(() => {});
		api<{ on: boolean }>('/push').then((d) => (pushOn = d.on)).catch(() => {});
		api<Prefs>('/me/notifications').then((d) => (prefs = d)).catch(() => {});
		subscribed().then((v) => (device = v));
	});

	// Web Push on this device.
	let pushOn = $state(false);
	let device = $state(false);
	let pushBusy = $state(false);
	async function setDevice(on: boolean) {
		pushBusy = true;
		try {
			if (on) await subscribe();
			else await unsubscribe();
			device = await subscribed();
			notify(device ? 'Notifications are on for this device.' : 'Notifications are off for this device.');
		} catch (e) {
			device = await subscribed();
			notify(e instanceof Error ? e.message : 'Could not change it.', 'orange');
		} finally {
			pushBusy = false;
		}
	}

	// Which events reach me, per personal channel (everything on until switched off).
	type Prefs = { channels: Record<'email' | 'push', boolean>; events: string[]; sent: Record<'email' | 'push', string[]>; off: Record<'email' | 'push', string[]> };
	let prefs = $state<Prefs | null>(null);
	const EVENT_TEXT: Record<string, string> = {
		request_submitted: 'Someone requests something', request_approved: 'My request is approved', request_declined: 'My request is declined',
		request_available: 'My request is ready to watch', request_failed: "My request couldn't be added", issue_reported: 'Someone reports an issue',
		issue_resolved: 'My issue is fixed', server_problem: 'Radarr or Sonarr has a problem'
	};
	const CH: ['email' | 'push', string][] = [['email', 'Email'], ['push', 'Push']];
	const shownCh = $derived(prefs ? CH.filter(([c]) => prefs!.channels[c]) : []);
	async function setPref(ch: 'email' | 'push', ev: string, on: boolean) {
		const off = { ...prefs!.off, [ch]: on ? prefs!.off[ch].filter((e) => e !== ev) : [...prefs!.off[ch], ev] };
		try {
			prefs = await api<Prefs>('/me/notifications', { method: 'PUT', body: JSON.stringify({ off }) });
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}

	type Watch = { on: boolean; allowed: boolean; signedIn: boolean };
	let watch = $state<Watch | null>(null);
	async function setWatch(on: boolean) {
		try {
			watch = await api<Watch>('/me/watchlist', { method: 'PUT', body: JSON.stringify({ on }) });
			notify(on ? 'Your Plex Watchlist is being requested now, and new additions within 15 minutes.' : 'Plex Watchlist requests are off.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}

	async function out() {
		await signOut();
		goto('/login');
	}
</script>

<main class="mx-auto max-w-[1200px] space-y-6 px-4 pb-28 pt-20 laptop:px-10 laptop:pt-24">
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

		{#if stats}
			<section class="grid grid-cols-2 gap-2 tablet:grid-cols-4">
				{#each [['Requests', stats.requests, '/requests'], ['Available', stats.available, '/requests'], ['In progress', stats.inProgress + stats.waiting, '/requests'], ['Open issues', stats.openIssues, '/issues']] as [l, n, h] (l)}
					<a href={h as string} class="card px-4 py-3 transition hover:ring-1 hover:ring-primary/50"><div class="text-2xl font-bold">{n}</div><div class="text-xs text-gray-400">{l}</div></a>
				{/each}
			</section>
		{/if}
		{#if mine.length}
			<div class="-mx-4 laptop:mx-0"><Row title="My requests" items={mine} more="/requests">{#snippet item(c)}<Poster item={c} />{/snippet}</Row></div>
		{/if}

		{#if pushOn || shownCh.length}
			<section>
				<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Notifications</div>
				<div class="card divide-y divide-gray-700/40">
					{#if pushOn}
						<div class="px-4 py-3">
							{#if !pushSupported() || needsHomeScreen()}
								<p class="text-sm text-gray-300">Notifications on this device</p>
								<p class="text-xs text-gray-500">{needsHomeScreen() ? 'On iPhone and iPad, add Flixarr to your home screen first (Share, Add to Home Screen), open it from there, then turn this on.' : "This browser can't receive notifications."}</p>
							{:else}
								<Toggle checked={device} disabled={pushBusy} label="Notifications on this device" about="Push notifications reach every device you turn this on for." onchange={setDevice} />
							{/if}
						</div>
					{/if}
					{#if prefs && shownCh.length}
						<div class="px-4 py-3">
							<div class="mb-1 flex items-center text-xs text-gray-500"><span class="flex-1">Tell me when</span>{#each shownCh as [, l] (l)}<span class="w-14 text-center">{l}</span>{/each}</div>
							{#each prefs.events as ev (ev)}
								<div class="flex items-center py-1.5 text-sm">
									<span class="flex-1 text-gray-300">{EVENT_TEXT[ev] ?? ev}</span>
									{#each shownCh as [ch] (ch)}
										{@const sent = prefs.sent[ch].includes(ev)}
										<span class="w-14 text-center"><input type="checkbox" class="accent-[var(--color-primary)]" aria-label="{EVENT_TEXT[ev]} by {ch}" disabled={!sent} checked={sent && !prefs.off[ch].includes(ev)} onchange={(e) => setPref(ch, ev, (e.currentTarget as HTMLInputElement).checked)} /></span>
									{/each}
								</div>
							{/each}
							{#if prefs.events.some((ev) => shownCh.some(([ch]) => !prefs!.sent[ch].includes(ev)))}<p class="mt-2 text-xs text-gray-500">Greyed out: not sent by this server at all (an admin sets that in Settings, Notifications).</p>{/if}
						</div>
					{/if}
				</div>
			</section>
		{/if}

		{#if watch?.allowed}
			<section>
				<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Plex Watchlist</div>
				<div class="card px-4 py-3">
					<Toggle checked={watch.on} disabled={!watch.signedIn && !watch.on} label="Request from my Plex Watchlist" about={watch.signedIn ? 'Everything on your Plex Watchlist gets requested (titles already in the library are skipped), then new additions within 15 minutes. Request limits and approval still apply.' : 'Sign out and back in with Plex once so Flixarr can read your Watchlist.'} onchange={setWatch} />
				</div>
			</section>
		{/if}

		{#if can('admin')}
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
			<div class="flex justify-between px-4 py-3 text-sm"><span class="text-gray-400">Flixarr version</span><span class="font-mono">{v?.full} <span class="text-xs text-gray-500">({v?.commit})</span></span></div>
			<div class="flex justify-between px-4 py-3 text-sm"><span class="text-gray-400">Channel</span><span class="capitalize">{v?.channel}</span></div>
			<div class="flex items-center gap-4 px-4 py-4">
				<span class="rounded-md bg-linear-to-r from-[#90cea1] to-tmdb px-2 py-1 text-xs font-black tracking-wide text-gray-950">TMDB</span>
				<p class="text-xs text-gray-400">This product uses the TMDB API but is not endorsed or certified by TMDB.</p>
			</div>
		</section>
	{/if}
</main>
