<script lang="ts">
	// Settings → Plex: which Plex server Flixarr reads, and which of its libraries. Titles in those
	// libraries show as available, whoever added them.
	import { api, fmtDate } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import Toggle from '#lib/components/Toggle.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import PlexMark from '#lib/components/PlexMark.svelte';

	type Section = { key: string; type: 'movie' | 'show'; title: string; enabled: boolean; items: number };
	type Status = { connected: boolean; id?: string; name?: string; url?: string; sections: Section[]; lastFull?: string; error?: string };
	type Server = { id: string; name: string; owned: boolean; connections: { uri: string; local: boolean; relay: boolean }[] };

	let st = $state<Status | null>(null);
	let servers = $state<Server[] | null>(null);
	let picking = $state(false);
	let busy = $state('');
	let error = $state('');
	let forgetOpen = $state(false);

	async function load() {
		try {
			st = await api<Status>('/plex');
			if (!st.connected) findServers();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load Plex settings.';
		}
	}
	$effect(() => {
		load();
	});

	async function findServers() {
		picking = true;
		servers = null;
		try {
			servers = await api<Server[]>('/plex/servers');
		} catch (e) {
			servers = [];
			notify(e instanceof Error ? e.message : 'Could not list your Plex servers.', 'orange');
		}
	}

	async function connect(s: Server) {
		busy = s.id;
		try {
			st = await api<Status>('/plex/connect', { method: 'POST', body: JSON.stringify({ id: s.id }) });
			picking = false;
			notify(`Connected to ${s.name}. Reading its libraries now.`, 'green');
			setTimeout(load, 5000);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not connect.', 'orange');
		} finally {
			busy = '';
		}
	}

	async function toggle(sec: Section, on: boolean) {
		const keys = st!.sections.filter((s) => (s.key === sec.key ? on : s.enabled)).map((s) => s.key);
		try {
			st = await api<Status>('/plex/libraries', { method: 'PUT', body: JSON.stringify({ keys }) });
			setTimeout(load, 4000);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}

	async function scan() {
		busy = 'scan';
		try {
			await api('/plex/scan', { method: 'POST' });
			await new Promise((r) => setTimeout(r, 4000));
			await load();
			notify('Plex libraries scanned.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not scan.', 'orange');
		} finally {
			busy = '';
		}
	}

	async function forget() {
		forgetOpen = false;
		try {
			st = await api<Status>('/plex', { method: 'DELETE' });
			notify('Plex server disconnected.');
			findServers();
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not disconnect.', 'orange');
		}
	}

	const where = (c: Server['connections'][number]) => (c.relay ? 'Plex relay' : c.local ? 'Local network' : 'Remote');
	const total = $derived(st?.sections.filter((s) => s.enabled).reduce((n, s) => n + s.items, 0) ?? 0);
</script>

{#snippet heading(t: string)}<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">{t}</div>{/snippet}

{#if error}
	<div class="card p-4 text-sm text-red-300">{error}</div>
{:else if st}
	{#if st.connected && !picking}
		<div class="card flex flex-wrap items-center gap-4 p-4">
			<span class="grid size-10 shrink-0 place-items-center rounded-xl bg-[#1f2326] ring-1 ring-white/10"><PlexMark class="h-3/5 w-3/5 translate-x-[6%]" /></span>
			<div class="min-w-0 flex-1 text-sm">
				<div class="font-medium">{st.name}</div>
				<div class="truncate text-xs text-gray-400">{st.url}</div>
				<div class="text-xs text-gray-500">{total.toLocaleString()} titles{st.lastFull ? ` · last full scan ${fmtDate(st.lastFull, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })}` : ''}</div>
			</div>
			<div class="flex shrink-0 gap-2">
				<button class="btn-ghost px-3 py-1.5 text-xs" disabled={busy === 'scan'} onclick={scan}>{busy === 'scan' ? 'Scanning…' : 'Scan now'}</button>
				<button class="btn-ghost px-3 py-1.5 text-xs" onclick={findServers}>Change</button>
			</div>
		</div>
		{#if st.error}<p class="rounded-lg bg-yellow-500/10 px-3 py-2 text-sm text-yellow-300 ring-1 ring-yellow-500/30">{st.error}</p>{/if}

		{#if st.sections.length}
			<div>
				{@render heading('Libraries')}
				<div class="card divide-y divide-gray-700/40 px-4">
					{#each st.sections as sec (sec.key)}
						<div class="py-3">
							<Toggle checked={sec.enabled} label={sec.title} about="{sec.type === 'movie' ? 'Movies' : 'TV'}{sec.enabled ? ` · ${sec.items.toLocaleString()} titles` : ''}" onchange={(on) => toggle(sec, on)} />
						</div>
					{/each}
				</div>
				<p class="mt-2 text-xs text-gray-500">Titles in these libraries show as available, and can't be requested again. New additions show up within 5 minutes; removals with the nightly full scan.</p>
			</div>
		{/if}
		<button class="text-xs text-gray-500 hover:text-red-300" onclick={() => (forgetOpen = true)}>Disconnect this server</button>
	{:else}
		<div>
			{@render heading('Choose your Plex server')}
			{#if !servers}
				<div class="card p-4 text-sm text-gray-400">Looking for servers on your Plex account…</div>
			{:else if !servers.length}
				<div class="card p-4 text-sm text-gray-400">No Plex servers found on your account. <button class="text-primary-light hover:underline" onclick={findServers}>Look again</button></div>
			{:else}
				<div class="card divide-y divide-gray-700/40 px-4">
					{#each servers as s (s.id)}
						<div class="flex items-center gap-4 py-3">
							<div class="min-w-0 flex-1 text-sm">
								<div class="font-medium">{s.name}{#if s.id === st.id}<span class="ml-2 text-xs font-normal text-green-400">Connected</span>{/if}</div>
								<div class="text-xs text-gray-500">{s.owned ? 'Your server' : 'Shared with you'} · {[...new Set(s.connections.map(where))].join(', ') || 'no addresses'}</div>
							</div>
							<button class="btn shrink-0 px-4 py-1.5 text-xs" disabled={!!busy} onclick={() => connect(s)}>{busy === s.id ? 'Connecting…' : 'Use this server'}</button>
						</div>
					{/each}
				</div>
				<p class="mt-2 text-xs text-gray-500">Flixarr tries the server's local address first, then its remote ones.</p>
			{/if}
			{#if st.connected}<button class="mt-3 text-xs text-gray-400 hover:text-white" onclick={() => (picking = false)}>Keep {st.name}</button>{/if}
		</div>
	{/if}
{/if}

<Modal bind:open={forgetOpen} title="Disconnect {st?.name ?? 'Plex'}?">
	<p class="p-5 text-sm text-gray-300">Flixarr stops reading this server and forgets what's in its libraries. Nothing changes on Plex. Requests stay.</p>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (forgetOpen = false)}>Keep it</button>
		<button class="btn px-6" onclick={forget}>Disconnect</button>
	{/snippet}
</Modal>
