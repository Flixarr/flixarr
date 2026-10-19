<script lang="ts">
	// Requests: yours or everyone's (anyone can see all requests), grouped by where they are. Tap a card for its details window (timeline, live progress, and the admin buttons for people who
	// manage requests). Cards update on their own while the page is open.
	import Select from '#lib/components/Select.svelte';
	import { api, ago } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { session, can } from '#lib/session.svelte.ts';
	import type { Request, User } from '#lib/types.ts';
	import Avatar from '#lib/components/Avatar.svelte';
	import StepBar from '#lib/components/StepBar.svelte';
	import Img from '#lib/components/Img.svelte';
	import PlexMark from '#lib/components/PlexMark.svelte';
	import RequestDetail from '#lib/components/RequestDetail.svelte';
	import { statusOf } from '#lib/availability.svelte.ts';
	import { simple, askedFor } from '#lib/requeststatus.ts';
	import { on } from '#lib/live.svelte.ts';
	import { page } from '$app/state';

	let list = $state<Request[] | null>(null);
	let approval = $state(false);
	const admin = $derived(can('manageRequests'));
	// Your requests or all of them; people who manage requests start on all. Remembered on the device.
	let scope = $state<'mine' | 'all'>((localStorage.getItem('flixarr:requests-scope') as 'mine' | 'all') ?? (can('manageRequests') ? 'all' : 'mine'));
	function pick(k: 'mine' | 'all') {
		scope = k;
		localStorage.setItem('flixarr:requests-scope', k);
	}
	const mine = $derived((list ?? []).filter((r) => r.userId === session.me?.id).length);
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
		: r.status === 'declined' || r.status === 'expired' || r.status === 'removed' ? 'ended'
		: r.status === 'failed' || r.trouble || (r.status === 'pending' && admin) ? 'attention'
		: 'onway';
	const ORDER = ['pending', 'failed', 'importing', 'downloading', 'grabbed', 'sent', 'approved', 'available', 'declined', 'removed', 'expired'];
	const scoped = $derived((list ?? []).filter((r) => scope === 'all' || r.userId === session.me?.id));
	const person = (r: Request) => ({ username: r.userName || '?', avatar: r.userAvatar }) as User;
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

	const status = (r: Request) => simple(r.status, r.progress);

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
	// ?open=12 (from an alert): open that request's window.
	$effect(() => {
		const id = Number(page.url.searchParams.get('open'));
		if (id) (detail = id), (detailOpen = true);
	});
	function openDetail(r: Request) {
		detail = r.id;
		detailOpen = true;
	}
</script>

<main class="mx-auto max-w-[1200px] px-4 pb-28 pt-20 laptop:px-10 laptop:pt-24">
	<div>
		<h1 class="text-2xl font-bold laptop:text-3xl">Requests</h1>
		<p class="text-sm text-gray-400">{approval ? 'New requests wait for an admin to approve them.' : 'Requests start right away.'} Tap one for its details.</p>
	</div>

	<div class="mt-5 flex gap-6 border-b border-gray-700/60 text-sm" role="tablist">
		{#each [['mine', 'Your requests', mine], ['all', 'All requests', list?.length ?? 0]] as [k, l, n] (k)}
			<button class="-mb-px border-b-2 pb-2.5 font-medium transition {scope === k ? 'border-primary text-white' : 'border-transparent text-gray-400 hover:text-white'}" role="tab" aria-selected={scope === k} onclick={() => pick(k as 'mine' | 'all')}>{l} <span class="ml-0.5 text-xs text-gray-500">{n}</span></button>
		{/each}
	</div>

	<div class="mt-4 flex flex-wrap items-center gap-2">
		<input class="w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60 tablet:w-64" placeholder="Search titles or people" bind:value={query} />
		<Select bind:value={sort} class="w-44" options={[{ value: 'new', label: 'Newest first' }, { value: 'old', label: 'Oldest first' }, { value: 'title', label: 'Title A to Z' }, { value: 'status', label: 'By status' }]} />
	</div>
	<div class="no-scrollbar mt-3 flex gap-2 overflow-x-auto pb-1 text-sm">
		{#each GROUPS as [k, l] (k)}
			{@const n = k === 'all' ? scoped.length : count(k)}
			<button class="shrink-0 rounded-full border px-3 py-1 {group === k ? 'border-primary bg-primary/15 text-white' : 'border-gray-700 text-gray-300 hover:text-white'} {k === 'attention' && n ? 'ring-1 ring-red-500/40' : ''}" onclick={() => (group = k)}>{l} <span class="text-gray-500">{n}</span></button>
		{/each}
	</div>

	{#if !list}
		<p class="mt-8 text-sm text-gray-400">Loading…</p>
	{:else if !shown.length}
		<div class="card mt-6 p-8 text-center text-sm text-gray-400">{scoped.length ? (query ? 'Nothing matches that search.' : 'Nothing here.') : scope === 'mine' ? "You haven't requested anything yet. Find a movie or series and hit Request." : 'No requests yet.'}</div>
	{:else}
		<div class="mt-5 grid gap-3 laptop:grid-cols-2">
			{#each shown as r (r.id)}
				{@const st = status(r)}
				<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
				<div class="relative cursor-pointer overflow-hidden rounded-xl border border-gray-700/40 shadow-xl transition hover:border-primary/50" onclick={() => openDetail(r)}>
					{#if r.backdrop}<div class="pointer-events-none absolute inset-0 opacity-30"><Img path={r.backdrop} kind="backdrop" sizes="(min-width: 1024px) 600px, 100vw" class="h-full w-full" /></div>{/if}
					<div class="absolute inset-0 bg-linear-to-r from-gray-950/95 via-gray-950/85 to-gray-950/55"></div>
					<div class="relative flex gap-3 p-3">
						<div class="w-16 shrink-0 overflow-hidden rounded-lg shadow-lg ring-1 ring-gray-700/60 tablet:w-20">{#if r.poster}<Img path={r.poster} kind="poster" sizes="80px" class="aspect-[2/3] w-full" />{:else}<div class="aspect-[2/3] bg-gray-800"></div>{/if}</div>
						<div class="flex min-w-0 flex-1 flex-col gap-1">
							<div class="flex items-start justify-between gap-2">
								<div class="min-w-0">
									<div class="text-[11px] text-gray-400">{[r.year, r.type === 'movie' ? 'Movie' : 'Series', askedFor(r)].filter(Boolean).join(' · ')}</div>
									<div class="truncate font-semibold">{r.title}</div>
								</div>
								{#if st}<span class="shrink-0 rounded-lg px-2 py-0.5 text-[11px] font-medium ring-1 {st.tone}">{st.label}</span>{/if}
							</div>
							<div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-400">
								<span class="flex items-center gap-1.5 text-gray-300"><Avatar user={person(r)} class="size-4 text-[9px]" />{r.source === 'redownload' ? 'Redownload by ' : ''}{r.userName || 'Someone'}</span>
								{#if admin && r.profileName}<span>{r.profileName}</span>{/if}
								<span>{ago(r.createdAt)}</span>
							</div>
							{#if st && st.step >= 0 && r.status !== 'available'}<StepBar status={r.status} progress={r.progress} class="mt-1.5 max-w-xs" />{/if}
							{#if r.trouble && r.status !== 'failed'}<p class="mt-1 text-xs text-yellow-200">{admin ? 'The download is stuck. Open it to fix it.' : 'The download hit a problem. Someone who manages requests can sort it out.'}</p>{/if}
							{#if r.error && (r.status === 'failed' || r.status === 'declined' || r.status === 'removed')}<p class="mt-1 line-clamp-2 text-xs {r.status === 'failed' ? 'text-red-300' : 'text-gray-400'}">{r.status === 'declined' ? 'Reason: ' : ''}{r.error}</p>{/if}
							{#if (r.status === 'available' && statusOf(`${r.type}:${r.tmdbId}`)?.plex) || (admin && r.status === 'pending')}
								<div class="mt-auto flex justify-end gap-2 pt-2">
									{#if r.status === 'available' && statusOf(`${r.type}:${r.tmdbId}`)?.plex}
										<a href={statusOf(`${r.type}:${r.tmdbId}`)?.plex} target="_blank" rel="noopener" class="btn-plex px-3 py-1 text-xs" onclick={(e) => e.stopPropagation()}><PlexMark class="size-3.5" color="#111827" />Play</a>
									{/if}
									{#if admin && r.status === 'pending'}
										<button class="btn px-3 py-1 text-xs" disabled={busy === r.id} onclick={(e) => (e.stopPropagation(), approve(r))}>{busy === r.id ? 'Approving…' : 'Approve'}</button>
									{/if}
								</div>
							{/if}
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</main>

{#if detail}<RequestDetail bind:open={detailOpen} id={detail} onchanged={(r) => { if (!r) list = list!.filter((x) => x.id !== detail); else list = list!.map((x) => (x.id === r.id ? { ...x, ...r } : x)); }} />{/if}
