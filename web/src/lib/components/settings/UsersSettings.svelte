<script lang="ts">
	// Settings → Users: who can sign in and what new people start as. Each person's own switches are on
	// the Users page.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import Toggle from '#lib/components/Toggle.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import Modal from '#lib/components/Modal.svelte';

	type Info = { plexSignIn: boolean; defaultRole: 'User' | 'Trusted'; signIn: 'both' | 'plex' | 'local' };
	const WAYS = [['both', 'Both'], ['plex', 'Plex'], ['local', 'Username']] as const;
	const SAID = { both: 'People can sign in with Plex or a username.', plex: 'People sign in with Plex only.', local: 'People sign in with a username only. Admins can still use Plex.' };
	let info = $state<Info | null>(null);
	$effect(() => {
		api<Info>('/settings/users').then((d) => (info = d));
	});
	// Everyone's own settings (#110): an admin turns some off for everyone, or sets the notifications people
	// start with and whether they can change them.
	type Everyone = { watchlistOff: boolean; localeLocked: boolean; defaultOff: Record<string, string[]>; offLocked: boolean; channels: string[]; events: string[] };
	let all = $state<Everyone | null>(null);
	let resetOpen = $state(false);
	$effect(() => {
		api<Everyone>('/users/everyone').then((d) => (all = d));
	});
	async function saveAll(change: Partial<Everyone>, msg: string) {
		try {
			all = await api<Everyone>('/users/everyone', { method: 'PUT', body: JSON.stringify({ ...all, ...change }) });
			notify(msg);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}
	const CH_LABEL: Record<string, string> = { email: 'Email', push: 'Push', discord: 'Discord', telegram: 'Telegram', pushover: 'Pushover', ntfy: 'ntfy' };
	const EV_LABEL: Record<string, string> = {
		request_submitted: 'Someone requests something', request_approved: 'Their request is approved', request_declined: 'Their request is declined',
		request_available: 'Their request is ready to watch', request_failed: "Their request couldn't be added", issue_reported: 'Someone reports an issue',
		issue_resolved: 'Their issue is fixed', server_problem: 'Radarr or Sonarr has a problem', request_leaving: 'Their request leaves Plex soon'
	};
	function setDefault(ch: string, ev: string, on: boolean) {
		const now = all!.defaultOff[ch] ?? [];
		saveAll({ defaultOff: { ...all!.defaultOff, [ch]: on ? now.filter((e) => e !== ev) : [...now, ev] } }, 'Saved.');
	}
	async function resetEveryone() {
		resetOpen = false;
		try {
			await api('/users/everyone/apply', { method: 'POST' });
			notify("Everyone's notification choices are back to these.");
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not reset.', 'orange');
		}
	}
	async function save(change: Partial<Info>, msg: string) {
		try {
			info = await api<Info>('/settings/users', { method: 'PUT', body: JSON.stringify({ ...info, ...change }) });
			notify(msg);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}
</script>

{#if info}
	<div class="card divide-y divide-gray-700/40 px-4">
		<div class="flex flex-wrap items-center justify-between gap-3 py-3">
			<span><span class="block text-sm">Sign in with</span><span class="block text-xs text-gray-500">Plex, a username and password (local accounts, added on the Users page), or both. With Username only, admins can still sign in with Plex.</span></span>
			<div class="inline-flex gap-1 rounded-lg border border-gray-700/60 bg-gray-800/70 p-0.5 text-sm" role="radiogroup" aria-label="Sign in with">
				{#each WAYS as [v, label] (v)}
					<button class="rounded-md px-3 py-1 {info.signIn === v ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'}" role="radio" aria-checked={info.signIn === v} onclick={() => save({ signIn: v }, SAID[v])}>{label}</button>
				{/each}
			</div>
		</div>
		<div class="py-3 {info.signIn === 'local' ? 'opacity-50' : ''}">
			<Toggle checked={info.plexSignIn} label="Let new Plex users sign in" about="On: anyone your Plex server is shared with can sign in. Off: only people already on the Users page can." onchange={(on) => save({ plexSignIn: on }, on ? 'Anyone your Plex server is shared with can sign in.' : 'Only people on the Users page can sign in.')} />
		</div>
		<div class="flex flex-wrap items-center justify-between gap-3 py-3">
			<span><span class="block text-sm">New people start as</span><span class="block text-xs text-gray-500">User: requests wait for approval when it's on. Trusted: requests go straight through, and they can pick the quality.</span></span>
			<div class="inline-flex gap-1 rounded-lg border border-gray-700/60 bg-gray-800/70 p-0.5 text-sm" role="radiogroup" aria-label="New people start as">
				{#each ['User', 'Trusted'] as r (r)}
					<button class="rounded-md px-3 py-1 {info.defaultRole === r ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'}" role="radio" aria-checked={info.defaultRole === r} onclick={() => save({ defaultRole: r as Info['defaultRole'] }, `New people start as ${r}.`)}>{r}</button>
				{/each}
			</div>
		</div>
	</div>
	<a href="/users" class="card flex items-center gap-3 px-4 py-3 text-sm transition hover:ring-1 hover:ring-primary/50"><Icon name="users" class="size-4 text-gray-400" /><span class="flex-1">People, roles and permissions, request limits, Import Plex users</span><Icon name="chevron" class="size-4 text-gray-500" /></a>
{/if}

{#if all}
	<div>
		<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Everyone's own settings</div>
		<div class="card divide-y divide-gray-700/40 px-4">
			<p class="py-3 text-xs text-gray-400">Settings people choose for themselves on their Profile. Turn them off for everyone, or set what everyone starts with.</p>
			<div class="py-3"><Toggle checked={all.watchlistOff} label="Plex Watchlist requests off for everyone" about="Nobody's Plex Watchlist is requested from, even people with the permission, and the switch is gone from their Profile." onchange={(on) => saveAll({ watchlistOff: on }, on ? 'Plex Watchlist requests are off for everyone.' : 'People with the permission can turn Plex Watchlist requests on again.')} /></div>
			<div class="py-3"><Toggle checked={all.localeLocked} label="Everyone uses the server's region and language" about="What Discover and title pages show follows Settings, General for everyone; people can't pick their own." onchange={(on) => saveAll({ localeLocked: on }, on ? "Everyone uses the server's region and language." : 'People can pick their own region and language again.')} /></div>
			<div class="py-3">
				<div class="text-sm">Notifications people start with</div>
				<p class="text-xs text-gray-500">{all.channels.length ? "For people who haven't changed theirs. Each person only gets the ones about them (their own requests and issues, or the ones they handle)." : 'Set up email, Web Push or another service in Notifications first.'}</p>
				{#if all.channels.length}
					<div class="mt-2 overflow-x-auto">
						<div class="mb-1 flex min-w-max items-center text-xs text-gray-500"><span class="w-60 flex-1">Tell them when</span>{#each all.channels as ch (ch)}<span class="w-16 text-center">{CH_LABEL[ch] ?? ch}</span>{/each}</div>
						{#each all.events as ev (ev)}
							<div class="flex min-w-max items-center py-1.5 text-sm">
								<span class="w-60 flex-1 text-gray-300">{EV_LABEL[ev] ?? ev}</span>
								{#each all.channels as ch (ch)}
									<span class="w-16 text-center"><input type="checkbox" class="accent-[var(--color-primary)]" aria-label="{EV_LABEL[ev]} by {CH_LABEL[ch]}" checked={!(all.defaultOff[ch] ?? []).includes(ev)} onchange={(e) => setDefault(ch, ev, (e.currentTarget as HTMLInputElement).checked)} /></span>
								{/each}
							</div>
						{/each}
					</div>
					<div class="mt-3"><Toggle checked={all.offLocked} label="People can't change theirs" about="Everyone gets exactly these, and the choices on their Profile are greyed out." onchange={(on) => saveAll({ offLocked: on }, on ? 'Everyone gets these notifications now.' : 'People can change their notifications again.')} /></div>
					<div class="mt-3 flex flex-wrap items-center justify-between gap-3">
						<span class="text-xs text-gray-500">People who already changed theirs keep their choices until you reset them.</span>
						<button class="btn-ghost shrink-0 px-3 py-1.5 text-xs" onclick={() => (resetOpen = true)}>Reset everyone to these</button>
					</div>
				{/if}
			</div>
		</div>
	</div>
{/if}

<Modal bind:open={resetOpen} title="Reset everyone's notifications?">
	<p class="p-5 text-sm text-gray-300">Everyone's own choices are dropped and they get the notifications above. People can change theirs again afterwards, unless that's locked.</p>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (resetOpen = false)}>Cancel</button>
		<button class="btn px-6" onclick={resetEveryone}>Reset everyone</button>
	{/snippet}
</Modal>
