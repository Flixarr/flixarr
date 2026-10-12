<script lang="ts">
	import { api, fmtBytes } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { ArrInfo, FlixarrProfile, QueueRow, ServiceKind, ServiceView } from '#lib/types.ts';
	import Toggle from '#lib/components/Toggle.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import Select from '#lib/components/Select.svelte';
	import Tooltip from '#lib/components/Tooltip.svelte';

	// Each kind is a list of connection cards. Connections are stored as a list; for now each kind
	// takes one, so "Add" shows only while a list is empty.
	const KINDS: { kind: ServiceKind; name: string; what: string; port: number; about: string; color: string; rgb: string }[] = [
		{ kind: 'radarr', name: 'Radarr', what: 'movies', port: 7878, about: 'Movie requests are sent here.', color: '#ffc230', rgb: '255 194 48' },
		{ kind: 'sonarr', name: 'Sonarr', what: 'series', port: 8989, about: 'Series requests are sent here.', color: '#35c5f4', rgb: '53 197 244' }
	];
	const nameOf = (kind: ServiceKind) => KINDS.find((k) => k.kind === kind)!.name;

	type Form = { name: string; url: string; webUrl: string; apiKey: string; rootFolder: string; qualityProfileId: number; seasonFolders: boolean; searchOnAdd: boolean; enabled: boolean; addProfiles: string[]; defaultProfile: string };
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
	let flixarrProfiles = $state<FlixarrProfile[]>([]);
	let profileBusy = $state('');
	let webFromArr = $state(false); // the web address was filled in from the Application URL
	let changing = $state(false); // editing: the connection form is open ("Change")
	let initial = $state(''); // the form as it opened, so Save only lights up after a change
	const dirty = $derived(JSON.stringify(form) !== initial);
	let more = $state<ServiceKind | null>(null); // which card's ⋯ menu is open
	let refreshing = $state<ServiceKind | null>(null);
	// Queue boxes start small (the top item); Show expands them. Remembered in this browser.
	let expanded = $state<Record<string, boolean>>(JSON.parse(localStorage.getItem('flixarr.queueOpen') || '{}'));
	function toggleQueue(k: ServiceKind) {
		expanded[k] = !expanded[k];
		localStorage.setItem('flixarr.queueOpen', JSON.stringify(expanded));
	}

	$effect(() => {
		if (!more) return;
		const away = (e: PointerEvent) => !(e.target as Element).closest('[data-svc-menu]') && (more = null);
		document.addEventListener('pointerdown', away);
		return () => document.removeEventListener('pointerdown', away);
	});

	async function refresh(k: ServiceKind) {
		refreshing = k;
		try {
			saved![k] = await api<ServiceView>(`/services/${k}/refresh`, { method: 'POST' });
			notify(`${saved![k]!.name} refreshed.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not refresh.', 'orange');
		} finally {
			refreshing = null;
			more = null;
		}
	}

	async function copy(text: string) {
		more = null;
		try {
			await navigator.clipboard.writeText(text);
			notify('Address copied.');
		} catch {
			notify(text, 'orange');
		}
	}

	function blank(k: ServiceKind): Form {
		return { addProfiles: [], defaultProfile: '', name: nameOf(k), url: '', webUrl: '', apiKey: '', rootFolder: '', qualityProfileId: 0, seasonFolders: true, searchOnAdd: true, enabled: true };
	}

	// The cards show what the "Radarr / Sonarr: refresh status" job last read. They reload right after
	// each run of the job (its "done" event), with a slow backup check in case the event stream drops.
	let load = () => {};
	$effect(() => {
		// Backup check: only fires when no fresh data has come in for 90 seconds (the events stream
		// broke). Every load restarts it, so while events work there's one call per job run.
		let backup: ReturnType<typeof setTimeout>;
		load = () => api<Record<ServiceKind, ServiceView | null>>('/services').then((d) => {
			clearTimeout(backup);
			backup = setTimeout(load, 90000);
			saved = d;
		}).catch(() => {
			// Couldn't reach Flixarr: try again on the backup schedule.
			clearTimeout(backup);
			backup = setTimeout(load, 90000);
		});
		load();
		const tick = setInterval(() => (now = Date.now()), 5000);
		return () => {
			clearTimeout(backup);
			clearInterval(tick);
		};
	});

	// Reload the cards right after each run of the refresh-status job (pushed over /api/events).
	$effect(() => {
		const es = new EventSource('/api/events');
		es.addEventListener('job', (m) => {
			const e = JSON.parse((m as MessageEvent).data) as { job: string; phase: string; result?: string; error?: string; seconds?: number };
			if (e.job !== 'arr-status') return;
			if (e.phase === 'done') load();
		});
		return () => es.close();
	});

	function ago(iso: string | null | undefined) {
		if (!iso) return 'never';
		const s = Math.max(0, Math.round((now - Date.parse(iso)) / 1000));
		if (s < 60) return `${s} sec ago`;
		if (s < 3600) return `${Math.round(s / 60)} min ago`;
		if (s < 86400) return `${Math.round(s / 3600)} hr ago`;
		return `${Math.round(s / 86400)} days ago`;
	}
	// "Next update in 42 sec" normally; "Updates paused" when the job is off; "Updated X ago" in
	// yellow when the last read is overdue (more than 3 minutes old). The other wording is the hover text.
	function updateText(v: ServiceView): { text: string; title: string; warn: boolean } {
		const st = v.status!;
		const age = `Updated ${ago(st.updatedAt)}`;
		if (v.updatesPaused) return { text: 'Updates paused', title: age, warn: false };
		if (now - Date.parse(st.updatedAt) > 180000) return { text: age, title: 'The refresh-status job is overdue', warn: true };
		const secs = v.nextUpdate ? Math.round((Date.parse(v.nextUpdate) - now) / 1000) : -1;
		if (secs <= 0) return { text: 'Updating…', title: age, warn: false };
		const left = secs < 60 ? `${secs} sec` : secs < 3600 ? `${Math.round(secs / 60)} min` : `${Math.round(secs / 3600)} hr`;
		return { text: `Next update in ${left}`, title: age, warn: false };
	}
	const short = (v?: string) => (v ? 'v' + v.split('.').slice(0, 2).join('.') : '');
	const STATE_TEXT: Record<string, string> = { queued: 'Waiting', paused: 'Paused', importing: 'Importing', warning: 'Needs attention', error: 'Failed' };
	const STATE_TONE: Record<string, string> = { warning: 'text-yellow-400', error: 'text-red-400', paused: 'text-gray-400', queued: 'text-gray-500', importing: 'text-green-400' };
	// Queue summary parts, e.g. 2 downloading · 1 waiting · 3 need attention (that part in yellow, red when something failed).
	function queueLabel(q: QueueRow[]): { text: string; tone: string }[] {
		if (q.length && q.every((r) => r.state === 'paused')) return [{ text: `${q.length} paused`, tone: '' }];
		const n = (f: (r: QueueRow) => boolean) => q.filter(f).length;
		const failed = n((r) => r.state === 'error');
		return ([[n((r) => r.state === 'downloading'), 'downloading', ''], [n((r) => r.state === 'importing'), 'importing', ''], [n((r) => r.state === 'queued'), 'waiting', ''],
			[n((r) => r.state === 'warning' || r.state === 'error'), 'need attention', failed ? 'text-red-400' : 'text-yellow-400'], [n((r) => r.state === 'paused'), 'paused', '']] as [number, string, string][])
			.filter(([c]) => c).map(([c, w, tone]) => ({ text: `${c} ${w === 'need attention' && c === 1 ? 'needs attention' : w}`, tone }));
	}


	function edit(k: ServiceKind) {
		const v = saved![k];
		kind = k;
		form = v ? { addProfiles: [], defaultProfile: '', name: v.name, url: v.url, webUrl: v.webUrl, apiKey: '', rootFolder: v.rootFolder, qualityProfileId: v.qualityProfileId, seasonFolders: v.seasonFolders, searchOnAdd: v.searchOnAdd, enabled: v.enabled } : blank(k);
		info = null;
		error = '';
		showKey = false;
		found = null;
		changing = false;
		initial = JSON.stringify(form);
		open = true;
		webFromArr = false;
		// Flixarr's own profiles: offered (all ticked) when adding a server, and the default quality.
		if (!v) api<{ profiles: FlixarrProfile[] }>('/profiles').then((d) => {
			flixarrProfiles = d.profiles.filter((p) => p.apps.includes(k));
			form.addProfiles = flixarrProfiles.map((p) => p.id);
			form.defaultProfile = form.addProfiles.includes('standard') ? 'standard' : (form.addProfiles[0] ?? '');
			initial = JSON.stringify(form);
		}).catch(() => (flixarrProfiles = []));
		if (v) test(true); // fills the folder and quality lists
	}

	// auto = the quiet check when an existing connection is opened; it doesn't fill in anything.
	async function test(auto = false) {
		testing = true;
		error = '';
		try {
			info = await api<ArrInfo>(`/services/${kind}/test`, { method: 'POST', body: JSON.stringify(form) });
			if (!info.folders.some((f) => f.path === form.rootFolder)) form.rootFolder = info.folders[0]?.path ?? '';
			if (!info.profiles.some((p) => p.id === form.qualityProfileId)) form.qualityProfileId = info.profiles[0]?.id ?? 0;
			changing = false;
			// Use Radarr/Sonarr's own Application URL for Open, unless something is typed in already.
			if (!auto && !form.webUrl.trim() && info.applicationUrl) {
				form.webUrl = info.applicationUrl;
				webFromArr = true;
			}
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

	// Existing server: add one Flixarr profile, or take Flixarr's version of one edited in Radarr/Sonarr.
	async function profileAction(key: string, action: 'add' | 'use-flixarr') {
		profileBusy = key;
		try {
			const v = await api<ServiceView>(action === 'add' ? `/services/${kind}/profiles` : `/services/${kind}/profiles/${key}/use-flixarr`, { method: 'POST', body: JSON.stringify({ keys: [key] }) });
			saved![kind] = v;
			await test(true); // the new profile shows up in the default quality list
			notify(action === 'add' ? 'Profile added.' : "Flixarr's version is back in place.");
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not do that.', 'orange');
		} finally {
			profileBusy = '';
		}
	}

	// The default quality picker: Flixarr profiles first (ones about to be added count, marked so), then the server's own.
	const qualityOptions = $derived.by(() => {
		if (!info) return [];
		const opts: { value: string; label: string; hint?: string }[] = [];
		for (const p of flixarrProfiles) if (form.addProfiles.includes(p.id)) opts.push({ value: `f:${p.id}`, label: p.arrName, hint: 'added on save' });
		const own = [...info.profiles].sort((a, b) => Number(b.name.startsWith('Flixarr · ')) - Number(a.name.startsWith('Flixarr · ')));
		for (const p of own) if (!opts.some((o) => o.label === p.name)) opts.push({ value: `p:${p.id}`, label: p.name });
		return opts;
	});
	const qualityValue = $derived(form.defaultProfile ? `f:${form.defaultProfile}` : `p:${form.qualityProfileId}`);
	function pickQuality(v: string) {
		if (v.startsWith('f:')) { form.defaultProfile = v.slice(2); form.qualityProfileId = 0; }
		else { form.defaultProfile = ''; form.qualityProfileId = +v.slice(2); }
	}
	function toggleAdd(key: string, on: boolean) {
		form.addProfiles = on ? [...form.addProfiles, key] : form.addProfiles.filter((k) => k !== key);
		if (!on && form.defaultProfile === key) { form.defaultProfile = ''; form.qualityProfileId = info?.profiles[0]?.id ?? 0; }
	}

	async function save() {
		saving = true;
		const wasNew = !saved![kind];
		try {
			const v = await api<ServiceView>(`/services/${kind}`, { method: 'PUT', body: JSON.stringify({ ...form, addProfiles: wasNew ? form.addProfiles : [] }) });
			saved![kind] = v;
			open = false;
			if (v.warning) notify(v.warning, 'orange');
			else notify(wasNew && form.addProfiles.length ? `${v.name} added, with Flixarr's quality profiles.` : `${v.name} saved.`);
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

{#snippet queueRow(q: QueueRow, color: string)}
	<span class="block py-2">
		<span class="flex items-baseline justify-between gap-3 text-xs"><span class="min-w-0 truncate text-gray-200">{q.title}</span>
			<span class="shrink-0 tabular-nums {STATE_TONE[q.state] ?? 'text-gray-400'}">{q.state === 'downloading' ? `${q.percent}%${q.timeLeft ? ` · ${q.timeLeft}` : ''}` : STATE_TEXT[q.state]}</span></span>
		<span class="mt-1 block h-1 overflow-hidden rounded-full bg-gray-700/60"><span class="block h-full rounded-full {q.state === 'importing' ? 'arr-stripes' : ''}"
			style="width:{q.state === 'queued' ? 0 : Math.max(q.percent, 2)}%; background:{q.state === 'error' ? '#ef4444' : q.state === 'warning' ? '#facc15' : q.state === 'paused' ? '#6b7280' : color}"></span></span>
		<span class="mt-1 block text-[11px] text-gray-500">{q.quality}{q.size ? ` · ${fmtBytes(q.size)}` : ''}</span>
		{#if q.message}<span class="mt-1 block text-[11px] {STATE_TONE[q.state]}">⚠ {q.message}</span>{/if}
	</span>
{/snippet}

{#snippet flixarrBox()}
	{@const isNewServer = !saved?.[kind]}
	{@const list = saved?.[kind]?.profiles ?? []}
	{#if isNewServer ? flixarrProfiles.length : list.length}
		<div class="rounded-xl border border-primary/40 bg-primary/10 p-4">
			<div class="flex items-center gap-2 text-sm font-medium"><span class="text-primary-light">◆</span>Flixarr quality profiles</div>
			<p class="mt-0.5 text-xs text-gray-300">Tuned profiles Flixarr adds to {KINDS.find((x) => x.kind === kind)?.name} and keeps up to date. Your own profiles aren't changed.</p>
			<div class="mt-3 space-y-2">
				{#if isNewServer}
					{#each flixarrProfiles as p (p.id)}
						<label class="flex cursor-pointer items-start gap-3 rounded-lg bg-gray-900/50 px-3 py-2 ring-1 ring-gray-700/50">
							<input type="checkbox" class="mt-0.5 accent-[var(--color-primary)]" checked={form.addProfiles.includes(p.id)} onchange={(e) => toggleAdd(p.id, (e.currentTarget as HTMLInputElement).checked)} />
							<span class="min-w-0"><span class="block text-sm">{p.label}</span><span class="block text-xs text-gray-400">{p.description}</span></span>
						</label>
					{/each}
				{:else}
					{#each list as p (p.key)}
						<div class="flex items-center gap-3 rounded-lg bg-gray-900/50 px-3 py-2 ring-1 ring-gray-700/50">
							<span class="min-w-0 flex-1"><span class="block text-sm">{p.label}</span>
								<span class="block text-xs {p.state === 'ok' ? 'text-green-400' : p.state === 'customized' ? 'text-yellow-300' : 'text-gray-400'}">
									{#if p.state === 'ok'}Added · up to date{:else if p.state === 'update'}Added · updating on the next sync{:else if p.state === 'customized'}Edited in {KINDS.find((x) => x.kind === kind)?.name}{p.updateAvailable ? ' · a newer Flixarr version is available' : ''}{:else if p.state === 'missing'}Deleted in {KINDS.find((x) => x.kind === kind)?.name}{:else}Not added{/if}
								</span></span>
							{#if !p.installed}
								<button class="btn-ghost shrink-0 px-3 py-1 text-xs" disabled={profileBusy !== ''} onclick={() => profileAction(p.key, 'add')}>{profileBusy === p.key ? 'Adding…' : p.state === 'missing' ? 'Add again' : 'Add'}</button>
							{:else if p.state === 'customized' && p.updateAvailable}
								<button class="btn-ghost shrink-0 px-3 py-1 text-xs" disabled={profileBusy !== ''} onclick={() => profileAction(p.key, 'use-flixarr')}>{profileBusy === p.key ? 'Updating…' : "Use Flixarr's version"}</button>
							{/if}
						</div>
					{/each}
				{/if}
			</div>
		</div>
	{/if}
{/snippet}

{#snippet chip(text: string, bad: boolean)}
	<span class="inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[11px] ring-1 {bad ? 'bg-yellow-500/10 text-yellow-300 ring-yellow-500/30' : 'bg-gray-900/60 text-gray-400 ring-gray-700/50'}"><span class="size-1.5 rounded-full {bad ? 'bg-yellow-400' : 'bg-green-500'}"></span>{text}</span>
{/snippet}


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
					{@const web = v.webUrl || v.url}
					<div class="card block w-full overflow-hidden text-left"><div class="p-4">
						<span class="flex items-center gap-3">
							<Tooltip text="Open {v.name}" align="left"><a href={web} target="_blank" rel="noopener" aria-label="Open {v.name}" class="shrink-0 rounded-md transition hover:opacity-80"><img src="/logos/{s.kind}.svg" alt="" class="size-8" /></a></Tooltip>
							<span class="min-w-0 flex-1">
								<a href={web} target="_blank" rel="noopener" class="group/n inline-flex items-center gap-1 font-semibold hover:underline hover:underline-offset-4">{v.name}<span class="text-gray-500 opacity-0 transition group-hover/n:opacity-100"><Icon name="external" class="size-3" /></span></a>
								<span class="block truncate text-[11px] text-gray-500">{v.url}{st?.version ? ` · ${short(st.version)}` : ''}</span>
							</span>
							{#if !st}<span class="text-xs text-gray-500">Checking…</span>
							{:else if !st.connected}<span class="flex items-center gap-1.5 text-xs text-red-400"><span class="size-2 rounded-full bg-red-500"></span>Not connected</span>
							{:else if issues}<span class="flex items-center gap-1.5 text-xs text-yellow-400"><span class="size-2 rounded-full bg-yellow-400"></span>{issues} issue{issues > 1 ? 's' : ''}</span>
							{:else}<span class="flex items-center gap-1.5 text-xs text-green-400"><span class="size-2 rounded-full bg-green-500"></span>Healthy</span>{/if}
							<span class="relative ml-1" data-svc-menu>
								<button class="grid size-8 place-items-center rounded-lg border border-gray-700/60 bg-gray-800/60 text-gray-300 hover:border-gray-500 hover:text-white" aria-label="More for {v.name}" aria-expanded={more === s.kind} onclick={() => (more = more === s.kind ? null : s.kind)}><Icon name="dots" class="size-4" /></button>
								{#if more === s.kind}
									<span class="absolute right-0 top-full z-20 mt-1 block w-44 overflow-hidden rounded-lg border border-gray-700/60 bg-gray-900 py-1 shadow-[0_16px_40px_-4px_rgb(0_0_0/0.75),0_4px_12px_rgb(0_0_0/0.5)]">
										<button class="flex w-full items-center gap-2 px-3 py-2 text-left text-gray-300 hover:bg-white/5 hover:text-white disabled:opacity-50" disabled={refreshing === s.kind} onclick={() => refresh(s.kind)}><Icon name="refresh" class="size-3.5" />{refreshing === s.kind ? 'Refreshing…' : 'Refresh now'}</button>
										<button class="flex w-full items-center gap-2 px-3 py-2 text-left text-gray-300 hover:bg-white/5 hover:text-white" onclick={() => copy(web)}><Icon name="copy" class="size-3.5" />Copy address</button>
										<button class="flex w-full items-center gap-2 px-3 py-2 text-left text-red-300 hover:bg-white/5" onclick={() => { more = null; kind = s.kind; confirmRemove = true; }}><Icon name="trash" class="size-3.5" />Remove</button>
									</span>
								{/if}
							</span>
						</span>

						{#if st && !st.connected}
							<span class="mt-4 flex items-center gap-2 rounded-lg bg-red-500/10 px-2.5 py-1.5 text-[11px] text-red-300 ring-1 ring-red-500/30">✕ {st.error}</span>
							<span class="mt-3 flex justify-between text-[11px] text-gray-500"><span>Last connected {ago(st.lastConnected)}</span><span>Updated {ago(st.updatedAt)}</span></span>
						{:else if st}
							{#if !st.queue.length}
								<span class="mt-4 flex items-center justify-between rounded-lg bg-gray-900/50 px-3 py-2 text-xs ring-1 ring-gray-700/40"><span class="text-[11px] font-semibold uppercase tracking-wide text-gray-500">Queue</span><span class="text-gray-500">Nothing downloading</span></span>
							{:else}
								{@const big = expanded[s.kind]}
								<span class="mt-4 block rounded-lg bg-gray-900/50 px-3 pb-1 pt-2 ring-1 ring-gray-700/40">
									<span class="flex items-center justify-between gap-3 border-b border-gray-700/30 pb-2 text-[11px]"><span class="font-semibold uppercase tracking-wide text-gray-500">Queue</span>
										<span class="flex min-w-0 items-center gap-2"><span class="truncate text-gray-400">{#each queueLabel(st.queue) as part, i (part.text)}{i ? ' · ' : ''}<span class={part.tone}>{part.text}</span>{/each}</span></span></span>
									<span class="block divide-y divide-gray-700/30">
										{#each st.queue.slice(0, big ? 3 : 1) as q, i (i)}{@render queueRow(q, s.color)}{/each}
									</span>
									{#if st.queue.length > 1}
										<span class="flex items-center justify-between gap-3 pb-1.5 text-[11px] text-gray-500 {big ? 'border-t border-gray-700/30 pt-1.5' : ''}">
											<span>{#if !big}+ {st.queue.length - 1} more{:else if st.queue.length > 3}+ {st.queue.length - 3} more in the queue{/if}</span>
											<button class="flex shrink-0 items-center gap-1 rounded-md px-1.5 py-0.5 text-gray-400 hover:bg-white/5 hover:text-white" aria-expanded={big} onclick={() => toggleQueue(s.kind)}>{big ? 'Hide' : 'Show'}<span class="transition {big ? '-rotate-90' : 'rotate-90'}"><Icon name="chevron" class="size-3.5" /></span></button>
										</span>
									{/if}
								</span>
							{/if}

							{@const fp = v.profiles.filter((p) => p.installed)}
							{@const edited = fp.filter((p) => p.state === 'customized').length}
							{#if v.profiles.length}
								<span class="mt-3 flex items-center gap-2 text-[11px]">
									<span class="text-primary-light">◆</span>
									{#if !fp.length}
										<span class="text-gray-400">Flixarr quality profiles not added</span>
										<button class="text-primary-light hover:underline" onclick={() => edit(s.kind)}>Add them</button>
									{:else if edited}
										<span class="text-gray-300">{fp.length} Flixarr profile{fp.length === 1 ? '' : 's'}</span><span class="text-yellow-300">· {edited} edited in {s.name}</span>
									{:else}
										<span class="text-gray-300">{fp.length} Flixarr profile{fp.length === 1 ? '' : 's'}</span><span class="text-gray-500">· up to date</span>
									{/if}
								</span>
							{/if}
							{@const cut = st.folder.lastIndexOf('/')}
							{@const used = st.totalSpace ? Math.round((1 - st.freeSpace / st.totalSpace) * 100) : 0}
							<span class="mt-4 block">
								<span class="flex items-end justify-between gap-3">
									<span class="flex min-w-0 items-center gap-2">
										<span class="grid size-7 shrink-0 place-items-center rounded-md bg-gray-900/70 text-gray-400 ring-1 ring-gray-700/50"><Icon name="folder" class="size-3.5" /></span>
										<span class="min-w-0 leading-tight"><span class="block truncate text-xs font-medium text-gray-200">{st.folder.slice(cut + 1) || st.folder}</span><span class="block truncate font-mono text-[10px] text-gray-500">{st.folder.slice(0, cut + 1)}</span></span>
									</span>
									<span class="shrink-0 text-right leading-tight"><span class="block text-xs font-semibold tabular-nums text-gray-100">{fmtBytes(st.freeSpace)} free</span>
										{#if st.totalSpace}<span class="block text-[10px] tabular-nums text-gray-500">{used}% of {fmtBytes(st.totalSpace)} used</span>{/if}</span>
								</span>
								{#if st.totalSpace}<span class="mt-2 block h-1.5 overflow-hidden rounded-full bg-gray-700/60"><span class="block h-full rounded-full" style="width:{used}%; background:{used > 90 ? '#ef4444' : s.color}"></span></span>{/if}
							</span>
							<span class="mt-3 flex flex-wrap items-center gap-1.5">
								{#each st.clients as c (c)}{@render chip(c, st.clientsDown)}{/each}
								{@render chip(`${st.indexers} indexer${st.indexers === 1 ? '' : 's'}${st.indexersDown ? ` · ${st.indexersDown} down` : ''}`, st.indexersDown > 0)}
								<span class="ml-auto"><Tooltip text={updateText(v).title} align="right"><span class="cursor-default text-[11px] tabular-nums outline-none {updateText(v).warn ? 'text-yellow-400' : 'text-gray-500'}">{updateText(v).text}</span></Tooltip></span>
							</span>
							{#if issues}
								<span class="mt-3 block rounded-lg bg-yellow-500/10 px-2.5 py-1.5 text-[11px] text-yellow-300 ring-1 ring-yellow-500/30">
									<span class="block">⚠ {st.health[0]}</span>
									{#if issues > 1}<span class="mt-0.5 block pl-4 text-yellow-300/70">…and {issues - 1} more issue{issues > 2 ? 's' : ''}</span>{/if}
								</span>
							{/if}
						{/if}
					</div>
						<div class="grid grid-cols-2 divide-x divide-gray-700/40 border-t border-gray-700/40 text-xs">
							<button class="flex items-center justify-center gap-1.5 py-2.5 text-gray-300 hover:bg-white/5 hover:text-white" onclick={() => edit(s.kind)}><Icon name="settings" class="size-3.5" />Edit connection</button>
							<a href={web} target="_blank" rel="noopener" class="flex items-center justify-center gap-1.5 py-2.5 text-gray-300 hover:bg-white/5 hover:text-white">Open {v.name}<Icon name="external" class="size-3.5" /></a>
						</div>
					</div>
				{:else}
					<button class="flex items-center justify-center gap-2 rounded-xl border border-dashed border-gray-700 p-6 text-sm text-gray-400 transition hover:border-gray-500 hover:text-white" onclick={() => edit(s.kind)}>
						<Icon name="plus" class="size-4" />Add {s.name} server
					</button>
				{/if}
			</div>
		</div>
	{/each}

	{@const isNew = !saved[kind]}
	{@const connected = !!info && !changing}
	<Modal bind:open title={isNew ? `Add ${k.name} server` : `Edit ${saved[kind]!.name}`}>
		{#snippet top()}
			<!-- The window's top is a band in the app's color, holding the connection (design E4.2). -->
			<div class="px-5 pb-5 pt-5" style="background:linear-gradient(160deg, rgb({k.rgb} / 0.3), rgb({k.rgb} / 0.04))">
				<div class="mb-4 flex items-center gap-3 pr-8">
					<img src="/logos/{kind}.svg" alt="" class="size-10 shrink-0" />
					<div><div class="text-lg font-semibold">{isNew ? `Add ${k.name} server` : `Edit ${saved![kind]!.name}`}</div><div class="text-xs text-gray-300">{k.about}</div></div>
				</div>
				{#if connected}
					<div class="swap-in flex items-center gap-3 rounded-xl border border-green-500/40 bg-gray-900/90 p-4 shadow-xl">
						<span class="size-2 shrink-0 rounded-full bg-green-500"></span>
						<div class="min-w-0 flex-1"><div class="text-sm font-medium text-green-400">Connected · v{info!.version.split('.').slice(0, 2).join('.')}</div><div class="truncate text-xs text-gray-500">{form.url}</div></div>
						<button class="text-xs text-primary-light hover:underline" onclick={() => (changing = true)}>Change</button>
					</div>
				{:else}
					<div class="space-y-3 rounded-xl border border-gray-700/60 bg-gray-900/90 p-4 shadow-xl">
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
									placeholder={saved?.[kind]?.apiKeySet ? `Saved, ends in ${saved![kind]!.apiKeyEnd}` : `${k.name} → Settings → General → Security`} class="{input} pr-16 font-mono" />
								<button class="absolute right-2 top-1/2 mt-0.5 -translate-y-1/2 px-2 text-xs text-gray-400 hover:text-white" onclick={() => (showKey = !showKey)}>{showKey ? 'Hide' : 'Show'}</button>
							</div>
						</div>
						<div class="flex gap-2">
							<button class="btn-ghost flex-1 py-2 text-sm" disabled={testing || !form.url} onclick={() => test()}>{testing ? 'Testing…' : 'Test'}</button>
							{#if changing && info}<button class="btn-ghost px-4 py-2 text-sm" onclick={() => (changing = false)}>Cancel</button>{/if}
						</div>
						{#if error}<p class="flex items-center gap-2 text-xs text-red-400"><span class="size-2 shrink-0 rounded-full bg-red-500"></span>{error}</p>{/if}
					</div>
				{/if}
			</div>
		{/snippet}

		<!-- The settings stay hidden until the connection works. -->
		<div class="reveal px-5 {info ? 'open' : ''}" inert={!info}>
			<div>
				{#if info}
					<div class="space-y-4 py-5">
						<div>
							<label for="svc-root" class="block text-sm">Root folder</label>
							<Select id="svc-root" bind:value={form.rootFolder} placeholder="Pick a folder" search={info.folders.length > 8}
								options={info.folders.map((f) => ({ value: f.path, label: f.path, hint: f.accessible ? `${fmtBytes(f.freeSpace)} free` : 'not reachable' }))} />
							{#if !info.folders.length}<p class="mt-1 text-xs text-yellow-400">Add a root folder in {k.name} first (Settings → Media Management).</p>{/if}
						</div>
						{@render flixarrBox()}
						<div>
							<label for="svc-quality" class="block text-sm">Default quality</label>
							<Select id="svc-quality" value={qualityValue} placeholder="Pick a quality" search={qualityOptions.length > 8} options={qualityOptions} onchange={pickQuality} />
						</div>
						<Toggle checked={form.searchOnAdd} label="Search right away" about="Start looking for a download as soon as something is requested." onchange={(on) => (form.searchOnAdd = on)} />
						{#if kind === 'sonarr'}
							<Toggle checked={form.seasonFolders} label="Season folders" about="Each season gets its own folder." onchange={(on) => (form.seasonFolders = on)} />
						{/if}
						<div>
							<label for="svc-name" class="block text-sm">Name</label>
							<input id="svc-name" bind:value={form.name} maxlength="60" placeholder={k.name} class={input} />
						</div>
						<div>
							<label for="svc-web" class="block text-sm">Web address <span class="text-gray-500">(optional)</span></label>
							<input id="svc-web" bind:value={form.webUrl} oninput={() => (webFromArr = false)} placeholder={form.url || `https://${k.kind}.example.com`} autocomplete="off" spellcheck="false" class={input} />
							<p class="mt-1 text-xs text-gray-500">{webFromArr ? `From ${k.name}'s Application URL (Settings → General → Host). Change it if Flixarr should link somewhere else.` : `Used by Open ${k.name} when the address above only works from the server.`}</p>
						</div>
					</div>
				{/if}
			</div>
		</div>
		{#snippet footer()}
			{#if !isNew}<button class="btn-ghost mr-auto text-red-300" onclick={() => { open = false; confirmRemove = true; }}>Remove</button>{/if}
			<button class="btn-ghost" onclick={() => (open = false)}>Cancel</button>
			<button class="btn px-6" disabled={!info || changing || saving || !form.rootFolder || (!form.qualityProfileId && !form.defaultProfile) || (!isNew && !dirty)} onclick={save}>{saving ? 'Saving…' : isNew ? 'Add server' : 'Save'}</button>
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
