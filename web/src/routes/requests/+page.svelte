<script lang="ts">
	// Requests: your own, or everyone's for people who can see them (Mine / Everyone), grouped by where they
	// are. Tap a card for its details window (timeline, live progress, and the admin buttons for people who
	// manage requests). Cards update on their own while the page is open.
	import Select from '#lib/components/Select.svelte';
	import { api, fmtDate } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { session, can } from '#lib/session.svelte.ts';
	import type { Request } from '#lib/types.ts';
	import Img from '#lib/components/Img.svelte';
	import PlexMark from '#lib/components/PlexMark.svelte';
	import RequestDetail from '#lib/components/RequestDetail.svelte';
	import { statusOf } from '#lib/availability.svelte.ts';
	import { simple, text, TONE, NEUTRAL, SIMPLE_STEPS } from '#lib/requeststatus.ts';
	import { on } from '#lib/live.svelte.ts';

	let list = $state<Request[] | null>(null);
	let approval = $state(false);
	const admin = $derived(can('manageRequests'));
	const everyone = $derived(can('viewRequests') || admin);
	let scope = $state<'mine' | 'all'>('all');
	let group = $state<'all' | 'onway' | 'attention' | 'ready' | 'ended'>('all');
	let query = $state('');
	let sort = $state<'new' | 'old' | 'title' | 'status'>('new');
	let busy = $state(0);
	let detail = $state(0);
	let detailOpen = $state(false);

	async function load() {
		try {
			const d = await api<{ requests: Request[]; approvalRequired: boolean }>('/requests');
			list = d.requests;
			approval = d.approvalRequired;
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not load requests.', 'orange');
		}
	}
	$effect(() => {
		document.title = 'Flixarr · Requests';
		load();
		const offs = [
			on('request', (d: { request: Request }) => {
				const r = d.request;
				if (!list) return;
				if (r.status === ('deleted' as Request['status'])) list = list.filter((x) => x.id !== r.id);
				else if (list.some((x) => x.id === r.id)) list = list.map((x) => (x.id === r.id ? { ...x, ...r, userName: x.userName || r.userName } : x));
				else load();
			}),
			on('resync', load)
		];
		return () => offs.forEach((off) => off());
	});

	// Where a request is. Waiting for approval needs attention from people who approve; for everyone else
	// it's on its way.
	const groupOf = (r: Request) =>
		r.status === 'available' ? 'ready'
		: r.status === 'declined' || r.status === 'expired' ? 'ended'
		: r.status === 'failed' || (r.status === 'pending' && admin) ? 'attention'
		: 'onway';
	const ORDER = ['pending', 'failed', 'importing', 'downloading', 'grabbed', 'sent', 'approved', 'available', 'declined', 'expired'];
	const scoped = $derived((list ?? []).filter((r) => scope === 'all' || r.userId === session.me?.id));
	const shown = $derived.by(() => {
		const q = query.trim().toLowerCase();
		const out = scoped.filter((r) => (group === 'all' || groupOf(r) === group) && (!q || r.title.toLowerCase().includes(q) || r.userName.toLowerCase().includes(q)));
		const by: Record<typeof sort, (a: Request, b: Request) => number> = {
			new: (a, b) => b.id - a.id,
			old: (a, b) => a.id - b.id,
			title: (a, b) => a.title.localeCompare(b.title),
			status: (a, b) => ORDER.indexOf(a.status) - ORDER.indexOf(b.status) || b.id - a.id
		};
		return out.sort(by[sort]);
	});
	const count = (g: string) => scoped.filter((r) => groupOf(r) === g).length;
	const GROUPS: [typeof group, string][] = [['all', 'All'], ['onway', 'On its way'], ['attention', 'Needs attention'], ['ready', 'Ready'], ['ended', 'Ended']];

	const what = (r: Request) =>
		r.type !== 'series' ? '' : r.mode === 'all' ? 'Whole show' : r.mode === 'episodes' ? `${r.episodes.length} episodes` : r.seasons.length === 1 ? `Season ${r.seasons[0]}` : `Seasons ${r.seasons.join(', ')}`;
	// People who manage requests see the exact status; everyone else the short one.
	const status = (r: Request) => (admin ? { label: text(r.status, r.progress), tone: TONE[r.status] ?? NEUTRAL } : simple(r.status, r.progress));

	async function approve(r: Request) {
		busy = r.id;
		try {
			const next = await api<Request>(`/requests/${r.id}/approve`, { method: 'POST', body: '{}' });
			list = list!.map((x) => (x.id === r.id ? next : x));
			notify(next.status === 'failed' ? `Couldn't send it: ${next.error}` : 'Approved and sent.', next.status === 'failed' ? 'orange' : 'green');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not approve it.', 'orange');
		} finally {
			busy = 0;
		}
	}
	function openDetail(r: Request) {
		detail = r.id;
		detailOpen = true;
	}
</script>

<main class="mx-auto max-w-[1200px] px-4 pb-28 pt-20 laptop:px-10 laptop:pt-24">
	<div class="flex flex-wrap items-end justify-between gap-3">
		<div>
			<h1 class="text-2xl font-bold laptop:text-3xl">Requests</h1>
			<p class="text-sm text-gray-400">{approval ? 'New requests wait for an admin to approve them.' : 'Requests are sent straight to Radarr and Sonarr.'} Tap one for its details.</p>
		</div>
		{#if everyone}
			<div class="inline-flex gap-1 rounded-lg border border-gray-700/60 bg-gray-800/70 p-0.5 text-sm" role="radiogroup" aria-label="Whose requests">
				{#each [['mine', 'Mine'], ['all', 'Everyone']] as [k, l] (k)}
					<button class="rounded-md px-3 py-1 {scope === k ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'}" role="radio" aria-checked={scope === k} onclick={() => (scope = k as typeof scope)}>{l}</button>
				{/each}
			</div>
		{/if}
	</div>

	<div class="mt-5 flex flex-wrap items-center gap-2">
		<input class="w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60 tablet:w-64" placeholder="Search titles or people" bind:value={query} />
		<Select bind:value={sort} class="w-44" options={[{ value: 'new', label: 'Newest first' }, { value: 'old', label: 'Oldest first' }, { value: 'title', label: 'Title A to Z' }, { value: 'status', label: 'By status' }]} />
	</div>
	<div class="mt-3 flex gap-2 overflow-x-auto pb-1 text-sm">
		{#each GROUPS as [k, l] (k)}
			{@const n = k === 'all' ? scoped.length : count(k)}
			<button class="shrink-0 rounded-full border px-3 py-1 {group === k ? 'border-primary bg-primary/15 text-white' : 'border-gray-700 text-gray-300 hover:text-white'} {k === 'attention' && n ? 'ring-1 ring-red-500/40' : ''}" onclick={() => (group = k)}>{l} <span class="text-gray-500">{n}</span></button>
		{/each}
	</div>

	{#if !list}
		<p class="mt-8 text-sm text-gray-400">Loading…</p>
	{:else if !shown.length}
		<div class="card mt-6 p-8 text-center text-sm text-gray-400">{scoped.length ? (query ? 'Nothing matches that search.' : 'Nothing here.') : 'No requests yet. Find a movie or series and hit Request.'}</div>
	{:else}
		<div class="mt-5 grid gap-3 laptop:grid-cols-2">
			{#each shown as r (r.id)}
				{@const st = status(r)}
				{@const s = simple(r.status, r.progress)}
				<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
				<div class="card relative cursor-pointer overflow-hidden transition hover:ring-1 hover:ring-primary/50" onclick={() => openDetail(r)}>
					{#if r.backdrop}<div class="pointer-events-none absolute inset-0 opacity-15"><Img path={r.backdrop} kind="backdrop" sizes="(min-width: 1024px) 600px, 100vw" class="h-full w-full" /></div>{/if}
					<div class="relative flex gap-4 p-4">
						<div class="w-20 shrink-0 overflow-hidden rounded-lg ring-1 ring-gray-700/50 tablet:w-24">{#if r.poster}<Img path={r.poster} kind="poster" sizes="96px" class="aspect-[2/3] w-full" />{:else}<div class="aspect-[2/3] bg-gray-800"></div>{/if}</div>
						<div class="flex min-w-0 flex-1 flex-col">
							<div class="flex flex-wrap items-baseline gap-x-2">
								<span class="font-semibold">{r.title}</span><span class="text-sm text-gray-400">{r.year}</span>
								<span class="rounded bg-gray-800/80 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-gray-400">{r.type === 'movie' ? 'Movie' : 'Series'}</span>
							</div>
							<div class="mt-0.5 text-xs text-gray-400">
								{r.source === 'redownload' ? 'Redownload by ' : ''}{r.userName || 'Someone'} · {fmtDate(r.createdAt, { month: 'short', day: 'numeric' })}{what(r) ? ` · ${what(r)}` : ''}
							</div>
							<div class="mt-3 flex flex-wrap items-center gap-2">
								{#if st}<span class="inline-flex rounded-lg px-2.5 py-1 text-xs font-medium ring-1 {st.tone}">{st.label}</span>{/if}
								{#if r.status === 'available' && statusOf(`${r.type}:${r.tmdbId}`)?.plex}
									<a href={statusOf(`${r.type}:${r.tmdbId}`)?.plex} target="_blank" rel="noopener" class="btn-plex px-3 py-1 text-xs" onclick={(e) => e.stopPropagation()}><PlexMark class="size-3.5" color="#111827" />Play</a>
								{/if}
								{#if admin && r.status === 'pending'}
									<button class="btn px-3 py-1 text-xs" disabled={busy === r.id} onclick={(e) => (e.stopPropagation(), approve(r))}>{busy === r.id ? 'Approving…' : 'Approve'}</button>
								{/if}
							</div>
							{#if s && s.step >= 0}
								<div class="mt-auto grid max-w-sm grid-cols-3 gap-1 pt-3">
									{#each SIMPLE_STEPS as label, i (label)}
										<div class="h-1 overflow-hidden rounded-full bg-gray-700/60"><div class="h-full rounded-full {r.status === 'available' ? 'bg-green-500' : 'bg-primary'}" style="width:{i < s.step ? 100 : i === s.step ? (r.status === 'downloading' ? r.progress : r.status === 'importing' || r.status === 'available' ? 100 : 15) : 0}%"></div></div>
									{/each}
								</div>
							{:else if r.error}
								<p class="mt-2 line-clamp-2 text-xs {r.status === 'failed' ? 'text-red-300' : 'text-gray-400'}">{r.status === 'declined' ? 'Reason: ' : ''}{r.error}</p>
							{/if}
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</main>

{#if detail}<RequestDetail bind:open={detailOpen} id={detail} onchanged={(r) => { if (!r) list = list!.filter((x) => x.id !== detail); else list = list!.map((x) => (x.id === r.id ? { ...x, ...r } : x)); }} />{/if}
