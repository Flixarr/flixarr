<script lang="ts">
	// Your own settings, in a window on your profile (#193): notifications (this device, your devices, which events
	// reach you where), your accounts on the server's notification services, region and language, Plex Watchlist
	// requests and, for local accounts, your password. Each change saves on its own.
	import { session } from '#lib/session.svelte.ts';
	import { api, forgetMeta, fmtDate } from '#lib/api.ts';
	import Select from './Select.svelte';
	import Toggle from './Toggle.svelte';
	import { REGIONS, LANGUAGES } from '#lib/locales.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { pushSupported, needsHomeScreen, subscribed, subscribe, unsubscribe } from '#lib/push.ts';

	$effect(() => {
		api<Watch>('/me/watchlist').then((d) => (watch = d)).catch(() => {});
		api<{ on: boolean }>('/push').then((d) => (pushOn = d.on)).catch(() => {});
		api<Prefs>('/me/notifications').then((d) => (prefs = d)).catch(() => {});
		loadDevices();
		api<Locale>('/me/locale').then((d) => (locale = d)).catch(() => {});
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
			loadDevices();
			notify(device ? 'Notifications are on for this device.' : 'Notifications are off for this device.');
		} catch (e) {
			device = await subscribed();
			notify(e instanceof Error ? e.message : 'Could not change it.', 'orange');
		} finally {
			pushBusy = false;
		}
	}

	// Which events reach me, per personal channel (everything on until switched off).
	// Own accounts (#124): Discord, Telegram, Pushover and ntfy show once an admin set them up; news about
	// your own requests then also reaches your account there.
	type Ch = 'email' | 'push' | 'discord' | 'telegram' | 'pushover' | 'ntfy';
	type Acct = 'discord' | 'telegram' | 'pushover' | 'ntfy';
	type Prefs = { channels: Record<Ch, boolean>; services: Record<Acct, boolean>; accounts: Record<Acct, string>; events: string[]; sent: Record<Ch, string[]>; off: Record<Ch, string[]>; locked?: boolean };
	let prefs = $state<Prefs | null>(null);
	const EVENT_TEXT: Record<string, string> = {
		request_submitted: 'Someone requests something', request_approved: 'My request is approved', request_declined: 'My request is declined',
		request_available: 'My request is ready to watch', request_failed: "My request couldn't be added", issue_reported: 'Someone reports an issue',
		issue_resolved: 'My issue is fixed', server_problem: 'Radarr or Sonarr has a problem', request_leaving: 'My request leaves Plex soon'
	};
	const CH: [Ch, string][] = [['email', 'Email'], ['push', 'Push'], ['discord', 'Discord'], ['telegram', 'Telegram'], ['pushover', 'Pushover'], ['ntfy', 'ntfy']];
	const shownCh = $derived(prefs ? CH.filter(([c]) => prefs!.channels[c]) : []);
	const ACCOUNTS: [Acct, string, string, string][] = [
		['discord', 'Discord user ID', '123456789012345678', 'Discord, Settings, Advanced: turn on Developer Mode, then right-click your name and Copy User ID. Messages in the server\'s Discord channel mention you.'],
		['telegram', 'Telegram chat ID', '123456789', 'Start a chat with the server\'s Flixarr bot first, then ask @userinfobot for your ID.'],
		['pushover', 'Pushover user key', '30 letters and numbers', 'On your Pushover dashboard, at the top.'],
		['ntfy', 'ntfy topic', 'my-flixarr', 'Subscribe to this topic in the ntfy app. Pick something hard to guess.']
	];
	const shownAcct = $derived(prefs ? ACCOUNTS.filter(([a]) => prefs!.services?.[a]) : []);
	let acct = $state<Record<string, string>>({});
	$effect(() => {
		if (prefs) acct = { ...prefs.accounts };
	});
	async function saveAccounts() {
		try {
			prefs = await api<Prefs>('/me/notifications', { method: 'PUT', body: JSON.stringify({ accounts: acct }) });
			notify('Saved.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}
	const acctDirty = $derived(!!prefs && shownAcct.some(([a]) => (acct[a] ?? '') !== (prefs!.accounts[a] ?? '')));

	// Devices getting push notifications (#124): the browser each was added from, and Remove.
	type Device = { endpoint: string; device: string; createdAt: string };
	let devices = $state<Device[]>([]);
	let thisDevice = $state('');
	async function loadDevices() {
		devices = await api<Device[]>('/me/devices').catch(() => []);
		thisDevice = (await navigator.serviceWorker?.getRegistration('/').then((r) => r?.pushManager.getSubscription()).catch(() => null))?.endpoint ?? '';
	}
	async function removeDevice(d: Device) {
		if (d.endpoint === thisDevice) return setDevice(false);
		await api('/push', { method: 'DELETE', body: JSON.stringify({ endpoint: d.endpoint }) }).catch(() => {});
		loadDevices();
	}
	// "Chrome on Windows" from a user agent.
	function deviceName(ua: string) {
		// The home-screen app on iPhone and iPad reports WebKit without naming Safari.
		const b = /Edg\//.test(ua) ? 'Edge' : /Firefox\/|FxiOS/.test(ua) ? 'Firefox' : /Chrome\/|CriOS/.test(ua) ? 'Chrome' : /Safari\//.test(ua) ? 'Safari' : /AppleWebKit/.test(ua) && /iPhone|iPad/.test(ua) ? 'Home screen app' : 'A browser';
		const o = /iPhone/.test(ua) ? 'iPhone' : /iPad/.test(ua) ? 'iPad' : /Android/.test(ua) ? 'Android' : /Windows/.test(ua) ? 'Windows' : /Mac OS X/.test(ua) ? 'Mac' : /Linux/.test(ua) ? 'Linux' : '';
		return o ? `${b} on ${o}` : b;
	}
	async function setPref(ch: Ch, ev: string, on: boolean) {
		const off = { ...prefs!.off, [ch]: on ? prefs!.off[ch].filter((e) => e !== ev) : [...prefs!.off[ch], ev] };
		try {
			prefs = await api<Prefs>('/me/notifications', { method: 'PUT', body: JSON.stringify({ off }) });
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}

	// Local accounts change their own password.
	let pw = $state({ current: '', next: '' });
	let pwBusy = $state(false);
	async function changePassword() {
		pwBusy = true;
		try {
			await api('/me/password', { method: 'PUT', body: JSON.stringify({ current: pw.current, new: pw.next }) });
			pw = { current: '', next: '' };
			notify('Password changed. Other devices are signed out.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not change it.', 'orange');
		} finally {
			pwBusy = false;
		}
	}

	// Your own region and language ("" = the server's).
	type Locale = { region: string; language: string; serverRegion: string; serverLanguage: string; locked?: boolean };
	let locale = $state<Locale | null>(null);
	const nameOf = (list: [string, string][], code: string) => list.find(([c]) => c === code)?.[1] ?? code;
	async function setLocale(change: Partial<Locale>) {
		try {
			locale = await api<Locale>('/me/locale', { method: 'PUT', body: JSON.stringify({ region: locale!.region, language: locale!.language, ...change }) });
			forgetMeta();
			notify('Saved. Pages you open from now on use it.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}

	type Watch = { on: boolean; allowed: boolean; signedIn: boolean; locked?: boolean };
	let watch = $state<Watch | null>(null);
	async function setWatch(on: boolean) {
		try {
			watch = await api<Watch>('/me/watchlist', { method: 'PUT', body: JSON.stringify({ on }) });
			notify(on ? 'Your Plex Watchlist is being requested now, and new additions within 15 minutes.' : 'Plex Watchlist requests are off.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}
</script>

{#if session.me}
	<div class="space-y-6 p-5">
		{#if pushOn || shownCh.length || shownAcct.length}
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
					{#if devices.length}
						<div class="px-4 py-3">
							<div class="mb-1 text-xs text-gray-500">Devices getting push notifications</div>
							{#each devices as d (d.endpoint)}
								<div class="flex items-center gap-3 py-1.5 text-sm">
									<span class="min-w-0 flex-1 truncate text-gray-300">{deviceName(d.device)}{#if d.endpoint === thisDevice}<span class="ml-2 text-xs text-gray-500">This device</span>{/if}</span>
									<span class="hidden text-xs text-gray-500 tablet:inline">since {fmtDate(d.createdAt)}</span>
									<button class="text-xs text-gray-400 hover:text-red-300" onclick={() => removeDevice(d)}>Remove</button>
								</div>
							{/each}
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
										<span class="w-14 text-center"><input type="checkbox" class="accent-[var(--color-primary)]" aria-label="{EVENT_TEXT[ev]} by {ch}" disabled={!sent || prefs.locked} checked={sent && !prefs.off[ch].includes(ev)} onchange={(e) => setPref(ch, ev, (e.currentTarget as HTMLInputElement).checked)} /></span>
									{/each}
								</div>
							{/each}
							{#if prefs.locked}<p class="mt-2 text-xs text-gray-500">An admin set which notifications everyone gets.</p>
							{:else if prefs.events.some((ev) => shownCh.some(([ch]) => !prefs!.sent[ch].includes(ev)))}<p class="mt-2 text-xs text-gray-500">Greyed out: not sent by this server at all (an admin sets that in Settings, Notifications).</p>{/if}
						</div>
					{/if}
				</div>
			</section>
		{/if}

		{#if shownAcct.length}
			<section>
				<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Your accounts</div>
				<div class="card divide-y divide-gray-700/40 px-4">
					<p class="py-3 text-xs text-gray-400">Add yours and news about your own requests and issues also reaches you there. Choose which in the table above.</p>
					{#each shownAcct as [a, label, ph, how] (a)}
						<div class="py-3">
							<label for="acct-{a}" class="block text-sm">{label}</label>
							<input id="acct-{a}" class="mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60" autocomplete="off" spellcheck="false" placeholder={ph} bind:value={acct[a]} />
							<p class="mt-1 text-xs text-gray-500">{how}</p>
						</div>
					{/each}
					<div class="flex justify-end py-3"><button class="btn px-5 py-1.5 text-sm" disabled={!acctDirty} onclick={saveAccounts}>Save</button></div>
				</div>
			</section>
		{/if}

		{#if locale?.locked}
			<section>
				<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Region and language</div>
				<div class="card px-4 py-3 text-sm text-gray-400">{nameOf(REGIONS, locale.serverRegion)}, {nameOf(LANGUAGES, locale.serverLanguage)}. An admin set these for everyone.</div>
			</section>
		{:else if locale}
			<section>
				<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Region and language</div>
				<div class="card grid gap-3 px-4 py-3 tablet:grid-cols-2">
					<div>
						<label for="my-region" class="block text-xs text-gray-400">Region <span class="text-gray-500">(release dates, age ratings, what's upcoming)</span></label>
						<Select id="my-region" search value={locale.region} options={[{ value: '', label: `Same as the server (${nameOf(REGIONS, locale.serverRegion)})` }, ...REGIONS.map(([v, l]) => ({ value: v, label: l }))]} onchange={(v) => setLocale({ region: v })} />
					</div>
					<div>
						<label for="my-language" class="block text-xs text-gray-400">Language <span class="text-gray-500">(titles and descriptions)</span></label>
						<Select id="my-language" search value={locale.language} options={[{ value: '', label: `Same as the server (${nameOf(LANGUAGES, locale.serverLanguage)})` }, ...LANGUAGES.map(([v, l]) => ({ value: v, label: l }))]} onchange={(v) => setLocale({ language: v })} />
					</div>
				</div>
			</section>
		{/if}

		{#if watch?.allowed && !watch.locked && !session.me.local}
			<section>
				<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Plex Watchlist</div>
				<div class="card px-4 py-3">
					<Toggle checked={watch.on} disabled={!watch.signedIn && !watch.on} label="Request from my Plex Watchlist" about={watch.signedIn ? 'Everything on your Plex Watchlist gets requested (titles already in the library are skipped), then new additions within 15 minutes. Request limits and approval still apply.' : 'Sign out and back in with Plex once so Flixarr can read your Watchlist.'} onchange={setWatch} />
				</div>
			</section>
		{/if}

		{#if session.me.local}
			<section>
				<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Password</div>
				<form class="card grid gap-3 px-4 py-3 tablet:grid-cols-[1fr_1fr_auto] tablet:items-end" onsubmit={(e) => { e.preventDefault(); changePassword(); }}>
					<div><label for="pw-cur" class="block text-xs text-gray-400">Current password</label><input id="pw-cur" type="password" autocomplete="current-password" class="mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60" bind:value={pw.current} /></div>
					<div><label for="pw-new" class="block text-xs text-gray-400">New password (8 or more characters)</label><input id="pw-new" type="password" autocomplete="new-password" class="mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60" bind:value={pw.next} /></div>
					<button class="btn px-5" disabled={pwBusy || !pw.current || pw.next.length < 8}>{pwBusy ? 'Saving…' : 'Change'}</button>
				</form>
			</section>
		{/if}
	</div>
{/if}
