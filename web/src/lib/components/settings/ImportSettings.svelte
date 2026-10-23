<script lang="ts">
	// Settings → Import, also the "Switching from Seerr?" step in first-time setup: paste Seerr's (or
	// Overseerr's / Jellyseerr's) address and API key, see what would come over, then import. The import
	// runs on the server; its progress arrives over the live connection. Seerr already gone? Its database
	// file (and settings.json for Radarr/Sonarr) can be uploaded instead.
	import { api, fmtNum, fmtDate } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { on } from '#lib/live.svelte.ts';
	import Icon from '#lib/components/Icon.svelte';

	let { setup = false }: { setup?: boolean } = $props();

	type Preview = {
		version: string;
		users: { total: number; new: number; existing: number; noPassword: number; imported: number };
		requests: { total: number; movies: number; series: number; skipped: number; imported: number; byStatus: Record<string, number> };
		issues: { total: number; open: number; resolved: number; imported: number };
		servers: { kind: string; name: string; address: string; action: string; why: string }[];
		notes: string[];
	};
	type Result = { version: string; users: number; requests: number; issues: number; servers: string[]; at: string; plan?: Preview };
	type State = { running: boolean; stage: string; done: number; total: number; error?: string; result?: Result; finished?: string };

	const STATUSES: [string, string, string, string][] = [
		['available', 'Available', 'in Plex', 'bg-green-500'],
		['importing', 'Almost ready', 'files in Radarr or Sonarr, not in Plex yet', 'bg-blue-500'],
		['pending', 'Waiting for approval', '', 'bg-yellow-500'],
		['declined', 'Declined', '', 'bg-gray-500'],
		['expired', 'Expired', 'never arrived or gone since; can be requested again', 'bg-gray-600']
	];
	const input = 'mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60';

	let url = $state('');
	let key = $state('');
	let preview = $state<Preview | null>(null);
	let st = $state<State | null>(null);
	let busy = $state('');
	let mode = $state<'api' | 'file'>('api');
	let dbFile = $state<File | null>(null);
	let settingsFile = $state<File | null>(null);
	// The summary (#126): open right after an import finishes; the last import's can be opened again.
	let summary = $state(false);

	$effect(() => {
		api<State>('/import/seerr').then((s) => (st = s));
		return on('import', (s: State) => {
			const was = st?.running;
			st = s;
			if (was && !s.running) {
				preview = null;
				if (s.error) notify(s.error, 'orange');
				else summary = !!s.result?.plan;
			}
		});
	});

	const body = () => JSON.stringify({ url: url.trim(), key: key.trim() });
	async function look() {
		busy = 'preview';
		try {
			if (mode === 'file') {
				const fd = new FormData();
				fd.append('db', dbFile!);
				if (settingsFile) fd.append('settings', settingsFile);
				preview = await api<Preview>('/import/seerr/file/preview', { method: 'POST', body: fd });
			} else preview = await api<Preview>('/import/seerr/preview', { method: 'POST', body: body() });
		} catch (e) {
			notify(e instanceof Error ? e.message : "Couldn't read Seerr.", 'orange');
		} finally {
			busy = '';
		}
	}
	async function start() {
		busy = 'import';
		try {
			st = mode === 'file' ? await api<State>('/import/seerr/file', { method: 'POST' }) : await api<State>('/import/seerr', { method: 'POST', body: body() });
		} catch (e) {
			notify(e instanceof Error ? e.message : "Couldn't start the import.", 'orange');
		} finally {
			busy = '';
		}
	}
	const pct = $derived(st?.running && st.total ? Math.round((st.done / st.total) * 100) : 0);
</script>

{#snippet heading(t: string)}<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">{t}</div>{/snippet}
{#snippet stat(label: string, n: number, sub: string)}
	<div class="card p-4"><div class="text-xs uppercase tracking-wide text-gray-500">{label}</div><div class="mt-1 text-2xl font-bold tabular-nums">{fmtNum(n)}</div><div class="text-xs text-gray-500">{sub}</div></div>
{/snippet}

{#snippet report(r: Result, p: Preview)}
	{@const by = p.requests.byStatus}
	{@const next = [
		p.users.noPassword && [`Set a password for ${fmtNum(p.users.noPassword)} local account${p.users.noPassword === 1 ? '' : 's'}, then give it to them. They came over without one.`, '/users', 'Users'],
		[`Set up notifications so people hear when their requests are ready.`, '/settings#notifications', 'Notifications'],
		by.importing && [`${fmtNum(by.importing)} request${by.importing === 1 ? ' is' : 's are'} almost ready. Flixarr asks Plex to scan for them, and they turn Available on their own.`, '/requests', 'Requests'],
		by.pending && [`${fmtNum(by.pending)} request${by.pending === 1 ? '' : 's'} wait for approval.`, '/requests', 'Requests'],
		r.servers.length && [`Check ${r.servers.join(' and ')}: the quality and folder new requests use.`, '/settings#services', 'Services']
	].filter(Boolean) as [string, string, string][]}
	<div class="space-y-4">
		<div class="flex items-center gap-2 text-sm font-medium"><span class="grid size-5 place-items-center rounded-full bg-green-500/20 text-green-400"><Icon name="check" class="size-3" /></span>Imported from Seerr {r.version} · {fmtDate(r.at, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })}</div>
		<div class="grid gap-3 tablet:grid-cols-3">
			{@render stat('People', p.users.total, [p.users.new && `${fmtNum(p.users.new)} added`, p.users.existing && `${fmtNum(p.users.existing)} matched to people already here`, p.users.imported && `${fmtNum(p.users.imported)} brought over before`].filter(Boolean).join(' · ') || 'None')}
			{@render stat('Requests', r.requests, `${fmtNum(p.requests.movies)} movies · ${fmtNum(p.requests.series)} series`)}
			{@render stat('Issues', r.issues, `${fmtNum(p.issues.open)} open · ${fmtNum(p.issues.resolved)} resolved`)}
		</div>
		<div>
			{@render heading('How the requests came in')}
			<div class="card divide-y divide-gray-700/40 px-4 text-sm">
				{#each STATUSES as [k, l, about, dot] (k)}
					<div class="flex items-center gap-3 py-2.5"><span class="size-2 shrink-0 rounded-full {dot}"></span><span class="min-w-0 flex-1">{l} {#if about}<span class="text-xs text-gray-500">{about}</span>{/if}</span><span class="tabular-nums">{fmtNum(by[k] ?? 0)}</span></div>
				{/each}
			</div>
		</div>
		{#if p.servers.length}
			<div>
				{@render heading('Radarr and Sonarr')}
				<div class="card divide-y divide-gray-700/40 px-4 text-sm">
					{#each p.servers as s (s.kind + s.name + s.address)}
						<div class="flex items-center gap-3 py-2.5"><span class="min-w-0 flex-1 truncate">{s.name} <span class="text-xs text-gray-500">{s.address}</span></span><span class="shrink-0 text-xs {s.action === 'add' ? 'text-green-400' : 'text-gray-400'}">{s.action === 'add' ? 'Added' : s.why}</span></div>
					{/each}
				</div>
			</div>
		{/if}
		{#if p.notes.length || p.requests.skipped || p.requests.imported}
			<div>
				{@render heading('Left out')}
				<div class="card space-y-1 px-4 py-3 text-xs text-gray-400">
					{#if p.requests.imported}<p>{fmtNum(p.requests.imported)} request{p.requests.imported === 1 ? ' was' : 's were'} brought over before, so {p.requests.imported === 1 ? 'it was' : 'they were'} skipped.</p>{/if}
					{#each p.notes as n (n)}<p>{n}</p>{/each}
				</div>
			</div>
		{/if}
		<div>
			{@render heading("What's next")}
			<div class="card divide-y divide-gray-700/40 px-4 text-sm">
				{#each next as [text, href, label] (href + text)}
					<a {href} class="flex items-center gap-3 py-3 hover:text-white"><span class="min-w-0 flex-1 text-gray-300">{text}</span><span class="shrink-0 text-xs text-primary-light">{label}</span><Icon name="chevron" class="size-4 shrink-0 text-gray-500" /></a>
				{/each}
			</div>
		</div>
	</div>
{/snippet}

{#if summary && st?.result?.plan && !st.running}
	<div class="space-y-4">
		{@render report(st.result, st.result.plan)}
		{#if !setup}<div class="flex justify-end"><button class="btn px-6" onclick={() => (summary = false)}>Done</button></div>{/if}
	</div>
{:else if st?.running}
	<div class="card space-y-3 p-4 text-sm">
		<div class="flex justify-between gap-3"><span>{st.stage}</span>{#if st.total}<span class="tabular-nums text-gray-400">{fmtNum(st.done)} of {fmtNum(st.total)}</span>{/if}</div>
		<div class="h-1.5 overflow-hidden rounded-full bg-gray-700/60"><div class="h-full rounded-full bg-primary transition-all" style="width:{pct}%"></div></div>
		<p class="text-xs text-gray-500">Runs on the server, so leaving this page is fine. Titles are looked up one by one for their names and posters, which takes a few minutes for a big Seerr.</p>
	</div>
{:else if preview}
	<div class="space-y-4">
		<div class="grid gap-3 tablet:grid-cols-3">
			{@render stat('Users', preview.users.total, [preview.users.new && `${fmtNum(preview.users.new)} new`, preview.users.existing && `${fmtNum(preview.users.existing)} already here`, preview.users.imported && `${fmtNum(preview.users.imported)} brought over before`].filter(Boolean).join(' · ') || 'None')}
			{@render stat('Requests', preview.requests.total, `${fmtNum(preview.requests.movies)} movies · ${fmtNum(preview.requests.series)} series`)}
			{@render stat('Issues', preview.issues.total, `${fmtNum(preview.issues.open)} open · ${fmtNum(preview.issues.resolved)} resolved`)}
		</div>
		<div>
			{@render heading('How the requests come in')}
			<div class="card divide-y divide-gray-700/40 px-4 text-sm">
				{#each STATUSES as [k, l, about, dot] (k)}
					<div class="flex items-center gap-3 py-2.5"><span class="size-2 shrink-0 rounded-full {dot}"></span><span class="min-w-0 flex-1">{l} {#if about}<span class="text-xs text-gray-500">{about}</span>{/if}</span><span class="tabular-nums">{fmtNum(preview.requests.byStatus[k] ?? 0)}</span></div>
				{/each}
				{#if preview.requests.imported}<div class="flex items-center gap-3 py-2.5 text-gray-400"><span class="flex-1">Brought over before, skipped</span><span class="tabular-nums">{fmtNum(preview.requests.imported)}</span></div>{/if}
			</div>
			<p class="mt-2 text-xs text-gray-500">Worked out from Plex, Radarr and Sonarr as they are now, not copied from Seerr.</p>
		</div>
		{#if preview.servers.length}
			<div>
				{@render heading('Radarr and Sonarr')}
				<div class="card divide-y divide-gray-700/40 px-4 text-sm">
					{#each preview.servers as s (s.kind + s.name + s.address)}
						<div class="flex items-center gap-3 py-2.5"><span class="min-w-0 flex-1 truncate">{s.name} <span class="text-xs text-gray-500">{s.address}</span></span><span class="shrink-0 text-xs {s.action === 'add' ? 'text-green-400' : 'text-gray-400'}">{s.why}</span></div>
					{/each}
				</div>
			</div>
		{/if}
		{#if preview.notes.length}
			<div class="rounded-xl bg-yellow-500/10 p-3 text-xs text-yellow-200 ring-1 ring-yellow-500/30">
				<div class="font-medium">Worth knowing</div>
				<ul class="mt-1.5 list-disc space-y-1 pl-4 text-yellow-200/80">{#each preview.notes as n (n)}<li>{n}</li>{/each}</ul>
			</div>
		{/if}
		<div class="flex flex-wrap items-center justify-end gap-2">
			<p class="mr-auto text-xs text-gray-500">Importing again later only adds what's new. Nothing already in Flixarr is changed or removed.</p>
			<button class="btn-ghost" onclick={() => (preview = null)}>Back</button>
			<button class="btn px-6" disabled={!!busy} onclick={start}>{busy === 'import' ? 'Starting…' : 'Import'}</button>
		</div>
	</div>
{:else}
	<div class="space-y-4">
		{#if st?.result && !setup}
			<div class="card space-y-1 p-4 text-sm">
				<div class="flex items-center gap-2 font-medium"><span class="grid size-5 place-items-center rounded-full bg-green-500/20 text-green-400"><Icon name="check" class="size-3" /></span>Imported from Seerr {st.result.version}</div>
				<p class="text-xs text-gray-400">{fmtNum(st.result.users)} new {st.result.users === 1 ? 'person' : 'people'}, {fmtNum(st.result.requests)} request{st.result.requests === 1 ? '' : 's'}, {fmtNum(st.result.issues)} issue{st.result.issues === 1 ? '' : 's'}{st.result.servers.length ? ` and ${st.result.servers.join(', ')}` : ''}. {fmtDate(st.result.at, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })}.</p>
				{#if st.result.plan}<button class="text-xs text-primary-light hover:underline" onclick={() => (summary = true)}>See the summary</button>{/if}
			</div>
		{/if}
		{#if st?.error}<p class="rounded-xl bg-red-500/10 p-3 text-xs text-red-300 ring-1 ring-red-500/30">The last import stopped: {st.error}</p>{/if}
		<div>
			{#if !setup}{@render heading('Seerr, Overseerr or Jellyseerr')}{/if}
			<div class="card divide-y divide-gray-700/40 px-4">
				{#if !setup}<p class="py-3 text-xs text-gray-500">Brings over users, requests, issues and the Radarr and Sonarr connections. Seerr is only read, never changed.</p>{/if}
				{#if mode === 'api'}
					<div class="py-3"><label for="seerr-url" class="block text-sm">Address</label><input id="seerr-url" class={input} bind:value={url} placeholder="http://192.168.1.10:5055" autocomplete="off" spellcheck="false" /></div>
					<div class="py-3">
						<label for="seerr-key" class="block text-sm">API key</label>
						<input id="seerr-key" class={input} type="password" bind:value={key} autocomplete="off" spellcheck="false" />
						<p class="mt-1 text-xs text-gray-500">In Seerr: Settings, General, API Key.</p>
					</div>
				{:else}
					<div class="py-3">
						<label for="seerr-db" class="block text-sm">Seerr's database</label>
						<input id="seerr-db" class="{input} file:mr-3 file:rounded-md file:border-0 file:bg-gray-700 file:px-3 file:py-1 file:text-xs file:text-gray-200" type="file" accept=".sqlite3,.sqlite,.db" onchange={(e) => (dbFile = (e.currentTarget as HTMLInputElement).files?.[0] ?? null)} />
						<p class="mt-1 text-xs text-gray-500">db.sqlite3, in the db folder of Seerr's config folder. Seerr on PostgreSQL has no such file: start Seerr once and use its address instead.</p>
					</div>
					<div class="py-3">
						<label for="seerr-settings" class="block text-sm">Seerr's settings.json <span class="text-gray-500">(optional)</span></label>
						<input id="seerr-settings" class="{input} file:mr-3 file:rounded-md file:border-0 file:bg-gray-700 file:px-3 file:py-1 file:text-xs file:text-gray-200" type="file" accept=".json" onchange={(e) => (settingsFile = (e.currentTarget as HTMLInputElement).files?.[0] ?? null)} />
						<p class="mt-1 text-xs text-gray-500">Next to the db folder. It holds the Radarr and Sonarr connections; without it, add them in Services.</p>
					</div>
				{/if}
				<div class="flex justify-end py-3"><button class="btn px-6" disabled={!!busy || (mode === 'api' ? !url.trim() || !key.trim() : !dbFile)} onclick={look}>{busy === 'preview' ? (mode === 'file' ? 'Uploading…' : 'Reading Seerr…') : 'Preview'}</button></div>
			</div>
			<p class="mt-2 text-xs text-gray-500">
				{#if mode === 'api'}Seerr already gone? <button class="text-primary-light hover:underline" onclick={() => (mode = 'file')}>Import from a backup file</button>{:else}Seerr still running? <button class="text-primary-light hover:underline" onclick={() => (mode = 'api')}>Use its address instead</button> (the better way: works with any database and version).{/if}
			</p>
		</div>
	</div>
{/if}
