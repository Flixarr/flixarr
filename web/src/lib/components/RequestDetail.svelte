<script lang="ts">
	// A request's details window: what's going on in plain words, live progress (per season for series),
	// the timeline of each step, and who asked for what. People who manage requests also see the release
	// and get the buttons (approve, decline, try again, fix, open in Radarr / Sonarr, remove).
	import { api, fmtDate } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { can, session } from '#lib/session.svelte.ts';
	import { on } from '#lib/live.svelte.ts';
	import type { Request } from '#lib/types.ts';
	import { simple, text, TONE, NEUTRAL, SIMPLE_STEPS, EVENT_TEXT } from '#lib/requeststatus.ts';
	import Modal from './Modal.svelte';
	import Img from './Img.svelte';
	import Icon from './Icon.svelte';

	type Download = { season?: number; episode?: number; percent: number; timeLeft?: string; state: string; size: number; release?: string; problem?: string };
	type Live = { inArr: boolean; monitored: boolean; hasFile: boolean; file?: { quality?: string; size: number; release?: string; path?: string }; queue: Download[]; seasons?: { number: number; have: number; total: number; inPlex: number }[]; link?: string; server: string; error?: string };
	type Detail = { request: Request; events: { status: string; note: string; at: string }[]; live: Live; hint?: string };

	let { open = $bindable(false), id, onchanged }: { open?: boolean; id: number; onchanged?: (r: Request | null) => void } = $props();
	let d = $state<Detail | null>(null);
	let busy = $state('');
	let declining = $state(false);
	let reason = $state('');
	let removing = $state(false);
	const admin = $derived(can('manageRequests'));

	async function load() {
		try {
			d = await api<Detail>(`/requests/${id}/detail`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not load it.', 'orange');
			open = false;
		}
	}
	$effect(() => {
		if (!open) return;
		d = null;
		declining = removing = false;
		load();
		// Live: this request moving along reloads the window (at most every couple of seconds).
		let timer: ReturnType<typeof setTimeout> | null = null;
		const off = on('request', (e: { request: { id: number } }) => {
			if (e.request.id !== id || timer) return;
			timer = setTimeout(() => ((timer = null), load()), 1500);
		});
		return () => (off(), timer && clearTimeout(timer));
	});

	const r = $derived(d?.request);
	const st = $derived(r ? (admin ? { label: text(r.status, r.progress), tone: TONE[r.status] ?? NEUTRAL, step: -1 } : simple(r.status, r.progress)) : null);
	const ustep = $derived(r ? simple(r.status, r.progress)?.step ?? -1 : -1);
	const size = (b: number) => (b >= 1e12 ? `${(b / 1e12).toFixed(1)} TB` : b >= 1e9 ? `${(b / 1e9).toFixed(1)} GB` : `${Math.round(b / 1e6)} MB`);
	const left = (t?: string) => {
		if (!t) return '';
		const [h, m] = t.split(':').map(Number);
		return h ? `${h} h ${m} min left` : m ? `${m} min left` : 'under a minute left';
	};
	const what = (r: Request) =>
		r.type !== 'series' ? 'Movie' : r.mode === 'all' ? 'Whole show' : r.mode === 'episodes' ? `${r.episodes.length} episodes` : r.seasons.length === 1 ? `Season ${r.seasons[0]}` : `Seasons ${r.seasons.join(', ')}`;

	async function act(what: 'approve' | 'decline' | 'retry' | 'fix', body: object = {}) {
		busy = what;
		try {
			if (what === 'fix') {
				const x = await api<{ request: Request; done: string }>(`/requests/${id}/fix`, { method: 'POST' });
				notify(x.done || 'Checked it and started a search.');
				onchanged?.(x.request);
			} else {
				const x = await api<Request>(`/requests/${id}/${what}`, { method: 'POST', body: JSON.stringify(body) });
				notify(x.status === 'failed' ? `Couldn't send it: ${x.error}` : what === 'decline' ? 'Declined.' : 'Sent.', x.status === 'failed' ? 'orange' : 'green');
				onchanged?.(x);
			}
			declining = false;
			await load();
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not do that.', 'orange');
		} finally {
			busy = '';
		}
	}
	async function remove() {
		busy = 'remove';
		try {
			await api(`/requests/${id}`, { method: 'DELETE' });
			notify('Request removed.');
			onchanged?.(null);
			open = false;
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not remove it.', 'orange');
		} finally {
			busy = '';
		}
	}
</script>

<Modal bind:open title={r?.title ?? 'Request'} wide>
	{#snippet top()}
		<div class="relative overflow-hidden">
			{#if r?.backdrop}<div class="pointer-events-none absolute inset-0 opacity-25"><Img path={r.backdrop} kind="backdrop" sizes="768px" class="h-full w-full" /></div>{/if}
			<div class="relative flex gap-4 bg-linear-to-b from-gray-900/40 to-gray-900 p-5 pr-12">
				<div class="w-20 shrink-0 overflow-hidden rounded-lg ring-1 ring-gray-700/50">{#if r?.poster}<Img path={r.poster} kind="poster" sizes="80px" class="aspect-[2/3] w-full" />{:else}<div class="aspect-[2/3] bg-gray-800"></div>{/if}</div>
				<div class="min-w-0 flex-1 space-y-2">
					{#if r}
						<a href="/{r.type}/{r.tmdbId}" class="text-lg font-bold hover:underline">{r.title}</a> <span class="text-sm text-gray-400">{r.year}</span>
						<div class="text-xs text-gray-400">{what(r)} · {r.source === 'redownload' ? 'Redownload by' : 'Asked by'} {r.userName || 'someone'} · {fmtDate(r.createdAt, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })}{r.profileName ? ` · ${r.profileName}` : ''}</div>
						{#if st}<span class="inline-flex rounded-lg px-3 py-1 text-sm font-medium ring-1 {st.tone}">{st.label}</span>{/if}
					{:else}
						<div class="h-5 w-40 animate-pulse rounded bg-gray-800"></div>
					{/if}
				</div>
			</div>
		</div>
	{/snippet}
	{#if d && r}
		<div class="space-y-5 p-5 pt-0 text-sm">
			{#if d.hint}<p class="rounded-lg px-3 py-2 ring-1 {r.status === 'failed' ? 'bg-red-500/10 text-red-200 ring-red-500/30' : 'bg-gray-800/60 text-gray-300 ring-gray-700/60'}">{d.hint}</p>{/if}

			{#if ustep >= 0}
				<div class="grid grid-cols-3 gap-1.5">
					{#each SIMPLE_STEPS as s, i (s)}
						<div>
							<div class="h-1.5 overflow-hidden rounded-full bg-gray-700/60"><div class="h-full rounded-full {r.status === 'available' ? 'bg-green-500' : 'bg-primary'}" style="width:{i < ustep ? 100 : i === ustep ? (r.status === 'downloading' ? r.progress : r.status === 'importing' || r.status === 'available' ? 100 : 15) : 0}%"></div></div>
							<div class="mt-1 text-[11px] {i <= ustep ? 'text-gray-300' : 'text-gray-500'}">{s}</div>
						</div>
					{/each}
				</div>
			{/if}

			{#if d.live.queue.length}
				<div class="space-y-2">
					<div class="text-xs font-semibold uppercase tracking-wide text-gray-500">Downloading</div>
					{#each d.live.queue as q, i (i)}
						<div class="rounded-lg bg-gray-800/50 px-3 py-2">
							<div class="flex items-baseline justify-between gap-3 text-xs">
								<span class="text-gray-200">{q.season ? `S${q.season}${q.episode ? 'E' + q.episode : ''}` : r.title}{q.state === 'importing' ? ' · adding to Plex' : q.state === 'queued' ? ' · waiting its turn' : ''}</span>
								<span class="shrink-0 text-gray-400">{q.percent}%{q.timeLeft && q.state === 'downloading' ? ` · ${left(q.timeLeft)}` : ''}{q.size ? ` · ${size(q.size)}` : ''}</span>
							</div>
							<div class="mt-1.5 h-1 overflow-hidden rounded-full bg-gray-700/60"><div class="h-full rounded-full {q.state === 'warning' || q.state === 'failed' ? 'bg-yellow-500' : 'bg-blue-500'}" style="width:{q.percent}%"></div></div>
							{#if q.problem}<p class="mt-1.5 text-xs text-yellow-200">{q.problem}</p>{/if}
							{#if admin && q.release}<p class="mt-1 truncate text-[11px] text-gray-500">{q.release}</p>{/if}
						</div>
					{/each}
				</div>
			{/if}

			{#if d.live.seasons?.length}
				<div class="space-y-1.5">
					<div class="text-xs font-semibold uppercase tracking-wide text-gray-500">Seasons</div>
					{#each d.live.seasons as s (s.number)}
						{@const done = s.total > 0 && s.inPlex >= s.total}
						<div class="flex items-center gap-3 text-xs">
							<span class="w-16 shrink-0 text-gray-300">Season {s.number}</span>
							<div class="h-1.5 flex-1 overflow-hidden rounded-full bg-gray-700/60"><div class="h-full rounded-full {done ? 'bg-green-500' : 'bg-primary'}" style="width:{s.total ? Math.min(100, (Math.max(s.have, s.inPlex) / s.total) * 100) : 0}%"></div></div>
							<span class="w-28 shrink-0 text-right text-gray-400">{done ? 'In Plex' : `${Math.max(s.have, s.inPlex)} of ${s.total} episodes`}</span>
						</div>
					{/each}
				</div>
			{/if}

			{#if d.live.file && (admin || r.status === 'available')}
				<div class="space-y-1">
					<div class="text-xs font-semibold uppercase tracking-wide text-gray-500">File</div>
					<p class="text-xs text-gray-300">{d.live.file.quality ? `${d.live.file.quality} · ` : ''}{size(d.live.file.size)}</p>
					{#if admin && d.live.file.release}<p class="truncate text-[11px] text-gray-500">{d.live.file.release}</p>{/if}
					{#if admin && d.live.file.path}<p class="truncate text-[11px] text-gray-500">{d.live.file.path}</p>{/if}
				</div>
			{/if}

			<div class="space-y-2">
				<div class="text-xs font-semibold uppercase tracking-wide text-gray-500">Timeline</div>
				<ol class="relative ml-1.5 space-y-3 border-l border-gray-700/60 pl-4">
					{#each d.events as e, i (i)}
						<li class="relative">
							<span class="absolute -left-[21px] top-1 size-2.5 rounded-full ring-2 ring-gray-900 {e.status === 'failed' ? 'bg-red-500' : e.status === 'available' ? 'bg-green-500' : e.status === 'declined' || e.status === 'expired' ? 'bg-gray-500' : 'bg-primary'}"></span>
							<div class="text-xs text-gray-200">{EVENT_TEXT[e.status] ?? e.status}{e.note && (e.status === 'approved' || e.status === 'declined') && admin ? ` by ${e.note}` : ''}</div>
							<div class="text-[11px] text-gray-500">{fmtDate(e.at, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })}</div>
							{#if e.note && (e.status === 'failed' || e.status === 'expired')}<div class="text-[11px] text-gray-400">{e.note}</div>{/if}
						</li>
					{/each}
				</ol>
			</div>

			{#if declining}
				<div class="space-y-2 rounded-lg bg-gray-800/50 p-3">
					<label for="rd-why" class="block text-sm">Reason <span class="text-gray-500">(optional)</span></label>
					<textarea id="rd-why" rows="2" maxlength="500" bind:value={reason} class="w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60" placeholder="e.g. it's not out in good quality yet"></textarea>
					<div class="flex justify-end gap-2"><button class="btn-ghost px-3 py-1 text-xs" onclick={() => (declining = false)}>Cancel</button><button class="btn px-3 py-1 text-xs" disabled={!!busy} onclick={() => act('decline', { reason: reason.trim() })}>Decline</button></div>
				</div>
			{/if}
			{#if removing}
				<div class="space-y-2 rounded-lg bg-gray-800/50 p-3 text-xs text-gray-300">
					<p>The request is removed from Flixarr. Anything already in {d.live.server || 'Radarr or Sonarr'} stays there.</p>
					<div class="flex justify-end gap-2"><button class="btn-ghost px-3 py-1 text-xs" onclick={() => (removing = false)}>Keep it</button><button class="btn px-3 py-1 text-xs" disabled={!!busy} onclick={remove}>Remove</button></div>
				</div>
			{/if}
		</div>
	{:else}
		<p class="p-5 text-sm text-gray-400">Loading…</p>
	{/if}
	{#snippet footer()}
		{#if r && admin}
			<button class="mr-auto px-2 text-xs text-gray-500 hover:text-red-300" onclick={() => (removing = true)}>Remove</button>
			{#if d?.live.link}<a href={d.live.link} target="_blank" rel="noopener" class="btn-ghost px-3 py-1.5 text-xs">Open in {d.live.server} <Icon name="external" class="size-3" /></a>{/if}
			{#if r.status === 'pending'}
				<button class="btn-ghost px-3 py-1.5 text-xs" disabled={!!busy} onclick={() => ((declining = true), (reason = ''))}>Decline</button>
				<button class="btn px-4 py-1.5 text-xs" disabled={!!busy} onclick={() => act('approve')}>{busy === 'approve' ? 'Approving…' : 'Approve'}</button>
			{:else if r.status === 'failed'}
				<button class="btn px-4 py-1.5 text-xs" disabled={!!busy} onclick={() => act('retry')}>{busy === 'retry' ? 'Trying…' : 'Try again'}</button>
			{:else if r.status !== 'available' && r.status !== 'declined' && r.status !== 'expired'}
				<button class="btn-ghost px-3 py-1.5 text-xs" disabled={!!busy} onclick={() => act('fix')}>{busy === 'fix' ? 'Fixing…' : 'Fix'}</button>
			{/if}
		{:else if r && r.status === 'pending' && r.userId === session.me?.id}
			<button class="mr-auto px-2 text-xs text-gray-500 hover:text-red-300" onclick={() => (removing = true)}>Cancel request</button>
			<button class="btn-ghost" onclick={() => (open = false)}>Close</button>
		{:else}
			<button class="btn-ghost" onclick={() => (open = false)}>Close</button>
		{/if}
	{/snippet}
</Modal>
