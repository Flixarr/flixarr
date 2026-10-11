<script lang="ts">
	import { api, fmtBytes } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { ArrInfo, QueueRow, ServiceKind, ServiceView } from '#lib/types.ts';
	import Toggle from '#lib/components/Toggle.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import Select from '#lib/components/Select.svelte';

	// Each kind is a list of connection cards. Connections are stored as a list; for now each kind
	// takes one, so "Add" shows only while a list is empty.
	const KINDS: { kind: ServiceKind; name: string; what: string; port: number; about: string; color: string }[] = [
		{ kind: 'radarr', name: 'Radarr', what: 'movies', port: 7878, about: 'Movie requests are sent here.', color: '#ffc230' },
		{ kind: 'sonarr', name: 'Sonarr', what: 'series', port: 8989, about: 'Series requests are sent here.', color: '#35c5f4' }
	];
	const nameOf = (kind: ServiceKind) => KINDS.find((k) => k.kind === kind)!.name;

	type Form = { name: string; url: string; apiKey: string; rootFolder: string; qualityProfileId: number; seasonFolders: boolean; searchOnAdd: boolean; enabled: boolean };
	let saved = $state<Record<ServiceKind, ServiceView | null> | null>(null);
	let now = $state(Date.now());

	// The open modal.
	let open = $state(false);
	let kind = $state<ServiceKind>('radarr');
	let form = $state<Form>(blank('radarr'));
	let info = $state<ArrInfo | null>(null);
	let error = $state('');
	let testing = $state(false);
	let saving = $state(false);
	let showKey = $state(false);
	let finding = $state(false);
	let found = $state<string[] | null>(null);
	let confirmRemove = $state(false);

	function blank(k: ServiceKind): Form {
		return { name: nameOf(k), url: '', apiKey: '', rootFolder: '', qualityProfileId: 0, seasonFolders: true, searchOnAdd: true, enabled: true };
	}

	// The cards show what the "Radarr / Sonarr: refresh status" job last read; reload while the page is open.
	$effect(() => {
		const load = () => api<Record<ServiceKind, ServiceView | null>>('/services').then((d) => (saved = d));
		load();
		const t = setInterval(load, 20000);
		const tick = setInterval(() => (now = Date.now()), 10000);
		return () => {
			clearInterval(t);
			clearInterval(tick);
		};
	});

	function ago(iso: string | null | undefined) {
		if (!iso) return 'never';
		const s = Math.max(0, Math.round((now - Date.parse(iso)) / 1000));
		if (s < 60) return `${s} sec ago`;
		if (s < 3600) return `${Math.round(s / 60)} min ago`;
		if (s < 86400) return `${Math.round(s / 3600)} hr ago`;
		return `${Math.round(s / 86400)} days ago`;
	}
	const short = (v?: string) => (v ? 'v' + v.split('.').slice(0, 2).join('.') : '');
	const STATE_TEXT: Record<string, string> = { queued: 'Waiting', paused: 'Paused', importing: 'Importing', warning: 'Needs attention', error: 'Failed' };
	const STATE_TONE: Record<string, string> = { warning: 'text-yellow-400', error: 'text-red-400', paused: 'text-gray-400', queued: 'text-gray-500', importing: 'text-green-400' };
	function queueLabel(q: QueueRow[]) {
		if (q.length && q.every((r) => r.state === 'paused')) return `${q.length} paused`;
		const n = (f: (r: QueueRow) => boolean) => q.filter(f).length;
		return ([[n((r) => r.state === 'downloading'), 'downloading'], [n((r) => r.state === 'importing'), 'importing'], [n((r) => r.state === 'queued'), 'waiting'],
			[n((r) => r.state === 'warning' || r.state === 'error'), 'need attention'], [n((r) => r.state === 'paused'), 'paused']] as [number, string][])
			.filter(([c]) => c).map(([c, w]) => `${c} ${w}`).join(' · ');
	}

	function edit(k: ServiceKind) {
		const v = saved![k];
		kind = k;
		form = v ? { name: v.name, url: v.url, apiKey: '', rootFolder: v.rootFolder, qualityProfileId: v.qualityProfileId, seasonFolders: v.seasonFolders, searchOnAdd: v.searchOnAdd, enabled: v.enabled } : blank(k);
		info = null;
		error = '';
		showKey = false;
		found = null;
		open = true;
		if (v) test(); // fills the folder and quality lists
	}

	async function test() {
		testing = true;
		error = '';
		try {
			info = await api<ArrInfo>(`/services/${kind}/test`, { method: 'POST', body: JSON.stringify(form) });
			if (!info.folders.some((f) => f.path === form.rootFolder)) form.rootFolder = info.folders[0]?.path ?? '';
			if (!info.profiles.some((p) => p.id === form.qualityProfileId)) form.qualityProfileId = info.profiles[0]?.id ?? 0;
		} catch (e) {
			info = null;
			error = e instanceof Error ? e.message : 'Test failed.';
		} finally {
			testing = false;
		}
	}

	async function find() {
		finding = true;
		found = null;
		try {
			found = (await api<{ found: { url: string }[] }>(`/services/${kind}/discover`, { method: 'POST' })).found.map((f) => f.url);
			if (found.length === 1) pick(found[0]);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Search failed.', 'orange');
		} finally {
			finding = false;
		}
	}

	function pick(url: string) {
		form.url = url;
		addressChanged();
	}

	// A new address may be a different server, so its folders and qualities have to be loaded again.
	function addressChanged() {
		info = null;
		error = '';
	}

	async function save() {
		saving = true;
		try {
			const v = await api<ServiceView>(`/services/${kind}`, { method: 'PUT', body: JSON.stringify(form) });
			saved![kind] = v;
			open = false;
			notify(`${v.name} saved.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		} finally {
			saving = false;
		}
	}

	async function remove() {
		try {
			await api(`/services/${kind}`, { method: 'DELETE' });
			const n = saved![kind]?.name ?? nameOf(kind);
			saved![kind] = null;
			confirmRemove = false;
			open = false;
			notify(`${n} removed.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not remove.', 'orange');
		}
	}

	const input = 'mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60';
	const k = $derived(KINDS.find((x) => x.kind === kind)!);
</script>

{#snippet chip(text: string, bad: boolean)}
	<span class="inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[11px] ring-1 {bad ? 'bg-yellow-500/10 text-yellow-300 ring-yellow-500/30' : 'bg-gray-900/60 text-gray-400 ring-gray-700/50'}"><span class="size-1.5 rounded-full {bad ? 'bg-yellow-400' : 'bg-green-500'}"></span>{text}</span>
{/snippet}

{#snippet section(title: string)}<div class="text-xs font-semibold uppercase tracking-wide text-gray-500">{title}</div>{/snippet}

{#if saved}
	{#each KINDS as s (s.kind)}
		{@const v = saved[s.kind]}
		<div>
			<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">{s.name}</div>
			<p class="-mt-1 mb-3 text-xs text-gray-500">{s.about}</p>
			<div class="grid gap-3 tablet:grid-cols-2">
				{#if v}
					{@const st = v.status}
					{@const issues = st?.health.length ?? 0}
					<button class="card block w-full p-4 text-left transition hover:border-gray-600" onclick={() => edit(s.kind)}>
						<span class="flex items-center gap-3">
							<img src="/logos/{s.kind}.svg" alt="" class="size-8 shrink-0" />
							<span class="min-w-0 flex-1"><span class="block font-semibold">{v.name}</span><span class="block truncate text-[11px] text-gray-500">{v.url}{st?.version ? ` · ${short(st.version)}` : ''}</span></span>
							{#if !st}<span class="text-xs text-gray-500">Checking…</span>
							{:else if !st.connected}<span class="flex items-center gap-1.5 text-xs text-red-400"><span class="size-2 rounded-full bg-red-500"></span>Not connected</span>
							{:else if issues}<span class="flex items-center gap-1.5 text-xs text-yellow-400"><span class="size-2 rounded-full bg-yellow-400"></span>{issues} issue{issues > 1 ? 's' : ''}</span>
							{:else}<span class="flex items-center gap-1.5 text-xs text-green-400"><span class="size-2 rounded-full bg-green-500"></span>Healthy</span>{/if}
						</span>

						{#if st && !st.connected}
							<span class="mt-4 flex items-center gap-2 rounded-lg bg-red-500/10 px-2.5 py-1.5 text-[11px] text-red-300 ring-1 ring-red-500/30">✕ {st.error}</span>
							<span class="mt-3 flex justify-between text-[11px] text-gray-500"><span>Last connected {ago(st.lastConnected)}</span><span>Updated {ago(st.updatedAt)}</span></span>
						{:else if st}
							{#if !st.queue.length}
								<span class="mt-4 flex items-center justify-between rounded-lg bg-gray-900/50 px-3 py-2 text-xs ring-1 ring-gray-700/40"><span class="text-[11px] font-semibold uppercase tracking-wide text-gray-500">Queue</span><span class="text-gray-500">Nothing downloading</span></span>
							{:else}
								<span class="mt-4 block rounded-lg bg-gray-900/50 px-3 pb-1 pt-2 ring-1 ring-gray-700/40">
									<span class="flex items-center justify-between text-[11px]"><span class="font-semibold uppercase tracking-wide text-gray-500">Queue</span><span class="text-gray-400">{queueLabel(st.queue)}</span></span>
									<span class="block divide-y divide-gray-700/30">
										{#each st.queue.slice(0, 3) as q, i (i)}
											<span class="block py-2">
												<span class="flex items-baseline justify-between gap-3 text-xs"><span class="min-w-0 truncate text-gray-200">{q.title}</span>
													<span class="shrink-0 tabular-nums {STATE_TONE[q.state] ?? 'text-gray-400'}">{q.state === 'downloading' ? `${q.percent}%${q.timeLeft ? ` · ${q.timeLeft}` : ''}` : STATE_TEXT[q.state]}</span></span>
												<span class="mt-1 block h-1 overflow-hidden rounded-full bg-gray-700/60"><span class="block h-full rounded-full {q.state === 'importing' ? 'arr-stripes' : ''}"
													style="width:{q.state === 'queued' ? 0 : Math.max(q.percent, 2)}%; background:{q.state === 'error' ? '#ef4444' : q.state === 'warning' ? '#facc15' : q.state === 'paused' ? '#6b7280' : s.color}"></span></span>
												<span class="mt-1 block text-[11px] text-gray-500">{q.quality}{q.size ? ` · ${fmtBytes(q.size)}` : ''}</span>
												{#if q.message}<span class="mt-1 block text-[11px] {STATE_TONE[q.state]}">⚠ {q.message}</span>{/if}
											</span>
										{/each}
									</span>
									{#if st.queue.length > 3}<span class="block border-t border-gray-700/30 py-2 text-[11px] text-gray-500">+ {st.queue.length - 3} more in the queue</span>{/if}
								</span>
							{/if}

							<span class="mt-4 block">
								<span class="flex justify-between gap-3 text-[11px]"><span class="truncate text-gray-400">{st.folder}</span>
									<span class="shrink-0 tabular-nums text-gray-300">{st.totalSpace ? `${fmtBytes(st.totalSpace - st.freeSpace)} · ` : ''}{fmtBytes(st.freeSpace)} free</span></span>
								{#if st.totalSpace}<span class="mt-1 block h-1.5 overflow-hidden rounded-full bg-gray-700/60"><span class="block h-full rounded-full" style="width:{Math.round((1 - st.freeSpace / st.totalSpace) * 100)}%; background:{s.color}"></span></span>{/if}
							</span>
							<span class="mt-3 flex flex-wrap items-center gap-1.5">
								{#each st.clients as c (c)}{@render chip(c, st.clientsDown)}{/each}
								{@render chip(`${st.indexers} indexer${st.indexers === 1 ? '' : 's'}${st.indexersDown ? ` · ${st.indexersDown} down` : ''}`, st.indexersDown > 0)}
								<span class="ml-auto text-[11px] text-gray-500">Updated {ago(st.updatedAt)}</span>
							</span>
							{#if issues}
								<span class="mt-3 block rounded-lg bg-yellow-500/10 px-2.5 py-1.5 text-[11px] text-yellow-300 ring-1 ring-yellow-500/30">
									<span class="block">⚠ {st.health[0]}</span>
									{#if issues > 1}<span class="mt-0.5 block pl-4 text-yellow-300/70">…and {issues - 1} more issue{issues > 2 ? 's' : ''}</span>{/if}
								</span>
							{/if}
						{/if}
					</button>
				{:else}
					<button class="flex items-center justify-center gap-2 rounded-xl border border-dashed border-gray-700 p-6 text-sm text-gray-400 transition hover:border-gray-500 hover:text-white" onclick={() => edit(s.kind)}>
						<Icon name="plus" class="size-4" />Add {s.name} server
					</button>
				{/if}
			</div>
		</div>
	{/each}

	<Modal bind:open title={saved[kind] ? `Edit ${saved[kind]!.name}` : `Add ${k.name} server`} sub={k.about}>
		<div class="space-y-6 p-5">
			<div class="space-y-4">
				{@render section('Connection')}
				<div>
					<label for="svc-url" class="block text-sm">Address</label>
					<div class="relative">
						<input id="svc-url" bind:value={form.url} oninput={addressChanged} placeholder="http://192.168.1.10:{k.port}" autocomplete="off" spellcheck="false" class="{input} pr-20" />
						<button class="absolute right-2 top-1/2 mt-0.5 -translate-y-1/2 px-2 text-xs text-gray-400 hover:text-white disabled:opacity-50" disabled={finding} onclick={find}>{finding ? 'Looking…' : 'Find it'}</button>
					</div>
					{#if found && found.length > 1}
						<div class="mt-2 flex flex-wrap gap-2">
							{#each found as f (f)}<button class="rounded-full border px-3 py-1 text-xs {form.url === f ? 'border-primary bg-primary/15 text-white' : 'border-gray-700 text-gray-300 hover:text-white'}" onclick={() => pick(f)}>{f}</button>{/each}
						</div>
					{:else if found && found.length === 0}
						<p class="mt-1 text-xs text-yellow-400">No {k.name} found nearby. Enter its address.</p>
					{/if}
				</div>
				<div>
					<label for="svc-key" class="block text-sm">API key</label>
					<div class="relative">
						<input id="svc-key" bind:value={form.apiKey} oninput={() => (error = '')} type={showKey ? 'text' : 'password'} autocomplete="off" spellcheck="false"
							placeholder={saved[kind]?.apiKeySet ? `Saved, ends in ${saved[kind]!.apiKeyEnd}` : ''} class="{input} pr-16 font-mono" />
						<button class="absolute right-2 top-1/2 mt-0.5 -translate-y-1/2 px-2 text-xs text-gray-400 hover:text-white" onclick={() => (showKey = !showKey)}>{showKey ? 'Hide' : 'Show'}</button>
					</div>
					<p class="mt-1 text-xs text-gray-500">In {k.name}: Settings → General → Security.</p>
				</div>
				<button class="btn-ghost w-full py-2 text-sm" disabled={testing || !form.url} onclick={test}>{testing ? 'Testing…' : info ? 'Test again' : 'Test connection'}</button>
				{#if error || info}
					<p class="-mt-2 flex items-center gap-2 text-xs {error ? 'text-red-400' : 'text-green-400'}">
						<span class="size-2 shrink-0 rounded-full {error ? 'bg-red-500' : 'bg-green-500'}"></span>
						{error || `Connected to ${k.name} ${info!.version}`}
					</p>
				{/if}
			</div>

			{#if info}
				<div class="space-y-4">
					{@render section('Defaults')}
					<div>
						<label for="svc-root" class="block text-sm">Root folder</label>
						<Select id="svc-root" bind:value={form.rootFolder} placeholder="Pick a folder" search={info.folders.length > 8}
							options={info.folders.map((f) => ({ value: f.path, label: f.path, hint: f.accessible ? `${fmtBytes(f.freeSpace)} free` : 'not reachable' }))} />
						{#if !info.folders.length}<p class="mt-1 text-xs text-yellow-400">Add a root folder in {k.name} first (Settings → Media Management).</p>{/if}
					</div>
					<div>
						<label for="svc-quality" class="block text-sm">Default quality</label>
						<Select id="svc-quality" bind:value={form.qualityProfileId} placeholder="Pick a quality" search={info.profiles.length > 8}
							options={info.profiles.map((p) => ({ value: p.id, label: p.name }))} />
					</div>
					<Toggle checked={form.searchOnAdd} label="Search right away" about="Start looking for a download as soon as something is requested." onchange={(on) => (form.searchOnAdd = on)} />
					{#if kind === 'sonarr'}
						<Toggle checked={form.seasonFolders} label="Season folders" about="Each season gets its own folder." onchange={(on) => (form.seasonFolders = on)} />
					{/if}
				</div>

				<div class="space-y-4">
					{@render section('Display')}
					<div>
						<label for="svc-name" class="block text-sm">Name</label>
						<input id="svc-name" bind:value={form.name} maxlength="60" placeholder={k.name} class={input} />
					</div>
				</div>
			{/if}
		</div>
		{#snippet footer()}
			{#if saved?.[kind]}<button class="btn-ghost mr-auto text-red-300" onclick={() => { open = false; confirmRemove = true; }}>Remove</button>{/if}
			<button class="btn-ghost" onclick={() => (open = false)}>Cancel</button>
			<button class="btn px-6" disabled={!info || saving || !form.rootFolder || !form.qualityProfileId} onclick={save}>{saving ? 'Saving…' : 'Save'}</button>
		{/snippet}
	</Modal>

	<Modal bind:open={confirmRemove} title="Remove {saved[kind]?.name ?? k.name}?" sub="The connection and its settings are deleted from Flixarr.">
		<p class="p-5 text-sm text-gray-300">Nothing changes inside {k.name}. New requests can't be sent there until it's connected again.</p>
		{#snippet footer()}
			<button class="btn-ghost" onclick={() => { confirmRemove = false; edit(kind); }}>Cancel</button>
			<button class="btn px-6" onclick={remove}>Remove</button>
		{/snippet}
	</Modal>
{/if}
