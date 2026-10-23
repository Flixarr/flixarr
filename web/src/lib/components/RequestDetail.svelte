<script lang="ts">
	// A request's window: the title with where it's at, who asked for what (and who approved), live
	// downloads, seasons for series, the files, and the timeline. People who manage requests also see every
	// step, the release, the folder and the buttons (approve, decline, try again, search again, open in
	// Radarr / Sonarr, remove).
	import { api, fmtDate, ago, meta, timeLeft, fmtBytes } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { can, session } from '#lib/session.svelte.ts';
	import { on } from '#lib/live.svelte.ts';
	import type { Movie, Request, Series, User } from '#lib/types.ts';
	import { simple, text, EVENT_TEXT, askedFor, eventDot } from '#lib/requeststatus.ts';
	import Modal from './Modal.svelte';
	import Img from './Img.svelte';
	import Avatar from './Avatar.svelte';
	import StepBar from './StepBar.svelte';
	import Timeline from './Timeline.svelte';
	import FolderCard from './FolderCard.svelte';
	import FixMenu from './FixMenu.svelte';
	import RedownloadModal from './RedownloadModal.svelte';
	import FixEpisodesModal from './FixEpisodesModal.svelte';

	type Download = { queueId?: number; season?: number; episode?: number; percent: number; timeLeft?: string; state: string; size: number; release?: string; problem?: string; detail?: string };
	type Live = { inArr: boolean; monitored: boolean; hasFile: boolean; file?: { quality?: string; size: number; release?: string; path?: string }; queue: Download[]; seasons?: { number: number; have: number; total: number; inPlex: number }[]; link?: string; server: string; error?: string };
	type Detail = { request: Request; events: { status: string; note: string; at: string }[]; live: Live; hint?: string };
	type Title = { genres?: string[]; runtime?: number; cert?: string };

	let { open = $bindable(false), id, onchanged }: { open?: boolean; id: number; onchanged?: (r: Request | null) => void } = $props();
	let d = $state<Detail | null>(null);
	let title = $state<Title | null>(null);
	let busy = $state('');
	let declining = $state(false);
	let reason = $state('');
	let removing = $state(false);
	const admin = $derived(can('manageRequests'));

	async function load() {
		try {
			d = await api<Detail>(`/requests/${id}/detail`);
			if (!title) meta<Title>(`${d.request.type}/${d.request.tmdbId}`).then((t) => (title = t)).catch(() => {});
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not load it.', 'orange');
			open = false;
		}
	}
	$effect(() => {
		if (!open) return;
		d = title = null;
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
	const st = $derived(r ? simple(r.status, r.progress) : null);
	const series = $derived(r?.type === 'series');
	const ended = $derived(!!r && ['available', 'declined', 'expired', 'removed'].includes(r.status));
	// Fix (#139): only while the request is in play or available (a bad file can be replaced).
	const fixable = $derived(!!r && !['declined', 'expired', 'removed', 'pending', 'failed'].includes(r.status));
	let redoOpen = $state(false);
	const asItem = $derived(r ? ({ id: r.tmdbId, type: r.type, title: r.title, year: r.year, poster: r.poster } as unknown as Movie & Series) : null);
	// Who decided, and how long it took to arrive.
	const decided = $derived(d?.events.findLast((e) => (e.status === 'approved' || e.status === 'declined') && e.note));
	const took = $derived.by(() => {
		const from = d?.events[0]?.at, to = d?.events.findLast((e) => e.status === 'available')?.at;
		if (!from || !to) return '';
		const m = Math.round((Date.parse(to) - Date.parse(from)) / 60000);
		return m < 60 ? `${Math.max(1, m)} min` : m < 2880 ? `${Math.round(m / 60)} hr` : `${Math.round(m / 1440)} days`;
	});
	const episodes = $derived(d?.live.seasons?.reduce((n, s) => n + Math.max(s.have, s.inPlex), 0) ?? 0);
	const steps = $derived(
		(d?.events ?? []).map((e) => ({
			// A Fix step's note is the whole line: "Got a different release by Marc".
			text: e.status === 'fix' ? e.note : (EVENT_TEXT[e.status] ?? e.status) + (e.note && (e.status === 'approved' || e.status === 'declined') ? ` by ${e.note}` : ''),
			at: e.at,
			dot: eventDot(e.status),
			note: e.note && ['failed', 'expired', 'removed'].includes(e.status) ? e.note : undefined
		}))
	);
	const person = (name: string, avatar = '') => ({ username: name || '?', avatar }) as User;
	const facts = $derived(r ? [r.year, series ? 'Series' : 'Movie', ...(title?.genres?.slice(0, 2) ?? []), !series && title?.runtime ? `${Math.floor(title.runtime / 60) ? Math.floor(title.runtime / 60) + 'h ' : ''}${title.runtime % 60}m` : ''].filter(Boolean) : []);

	async function act(what: 'approve' | 'decline' | 'retry' | 'fix', body: object = {}) {
		busy = what;
		try {
			if (what === 'fix') {
				const x = await api<{ request: Request; done: string }>(`/requests/${id}/fix`, { method: 'POST' });
				notify(x.done || 'Asked for a search.');
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

{#snippet head(t: string, n?: string)}<h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">{t}{#if n}<span class="ml-1.5 font-normal normal-case tracking-normal text-gray-500">{n}</span>{/if}</h3>{/snippet}

<Modal bind:open title={r?.title ?? 'Request'} wide>
	{#snippet top()}
		<div class="relative overflow-hidden">
			{#if r?.backdrop}<div class="pointer-events-none absolute inset-0 opacity-40"><Img path={r.backdrop} kind="backdrop" sizes="768px" class="h-full w-full" /></div>{/if}
			<div class="absolute inset-0 bg-linear-to-r from-gray-950/95 via-gray-950/80 to-gray-950/40"></div>
			<div class="relative flex gap-4 p-5 pr-12">
				<a href={r ? `/${r.type}/${r.tmdbId}` : undefined} class="w-24 shrink-0 overflow-hidden rounded-lg shadow-xl ring-1 ring-gray-700/50">{#if r?.poster}<Img path={r.poster} kind="poster" sizes="96px" class="aspect-[2/3] w-full" />{:else}<div class="aspect-[2/3] bg-gray-800"></div>{/if}</a>
				<div class="flex min-w-0 flex-1 flex-col">
					{#if r}
						<div class="text-[11px] text-gray-400">{facts.join(' · ')}</div>
						<a href="/{r.type}/{r.tmdbId}" class="text-xl font-bold leading-tight hover:underline">{r.title}</a>
						<div class="mt-2 flex flex-wrap items-center gap-2">
							{#if admin}<span class="inline-flex rounded-lg px-2.5 py-1 text-xs font-medium ring-1 {st?.tone ?? 'bg-gray-800/70 text-gray-200 ring-gray-700/60'}">{text(r.status, r.progress)}</span>
							{:else if st}<span class="inline-flex rounded-lg px-2.5 py-1 text-xs font-medium ring-1 {st.tone}">{st.label}</span>{/if}
							{#if took}<span class="text-xs text-gray-400">Arrived in {took}</span>{/if}
						</div>
						<StepBar status={r.status} progress={r.progress} full={admin} labels class="mt-auto max-w-md pt-3" />
					{:else}
						<div class="h-5 w-40 animate-pulse rounded bg-gray-800"></div>
					{/if}
				</div>
			</div>
		</div>
	{/snippet}
	{#if d && r}
		<div class="space-y-5 p-5 text-sm">
			{#if d.hint}<p class="rounded-lg px-3 py-2 ring-1 {r.status === 'failed' ? 'bg-red-500/10 text-red-200 ring-red-500/30' : 'bg-gray-800/60 text-gray-300 ring-gray-700/60'}">{d.hint}</p>{/if}
			{#if admin && r.detail}<p class="-mt-3 text-[11px] text-gray-500">{d.live.server || 'Radarr / Sonarr'} said: {r.detail}</p>{/if}

			<!-- Who asked for what -->
			<section>
				{@render head(r.source === 'redownload' ? 'Redownload by' : 'Requested by')}
				<div class="card flex items-center gap-3 p-3">
					<Avatar user={person(r.userName, r.userAvatar)} class="size-10 text-sm" />
					<div class="min-w-0 flex-1">
						<div class="font-medium text-gray-100">{r.userName || 'Someone'}{#if r.userId === session.me?.id}<span class="ml-1.5 text-xs font-normal text-gray-500">(you)</span>{/if}</div>
						<div class="text-xs text-gray-400">{fmtDate(r.createdAt, { month: 'short', day: 'numeric', year: 'numeric', hour: 'numeric', minute: '2-digit' })} · {ago(r.createdAt)}</div>
					</div>
				</div>
				<div class="mt-2 flex flex-wrap gap-1.5 text-[11px]">
					{#if askedFor(r)}<span class="rounded-md bg-gray-800/80 px-2 py-1 text-gray-300 ring-1 ring-gray-700/60">{askedFor(r)}</span>{/if}
					{#if r.profileName}<span class="rounded-md bg-gray-800/80 px-2 py-1 text-gray-300 ring-1 ring-gray-700/60">{r.profileName}</span>{/if}
					{#if decided}<span class="rounded-md bg-gray-800/80 px-2 py-1 text-gray-300 ring-1 ring-gray-700/60">{decided.status === 'approved' ? 'Approved' : 'Declined'} by {decided.note}</span>{/if}
					{#if r.status === 'declined' && r.error}<span class="rounded-md bg-gray-800/80 px-2 py-1 text-gray-300 ring-1 ring-gray-700/60">Reason: {r.error}</span>{/if}
				</div>
			</section>

			{#if d.live.queue.length}
				<section>
					{@render head('Downloading', `${d.live.queue.length}`)}
					<div class="card divide-y divide-gray-700/40">
						{#each d.live.queue as q, i (i)}
							<div class="p-3">
								<div class="flex items-baseline justify-between gap-3 text-xs">
									<span class="font-medium text-gray-200">{q.season ? `S${q.season}${q.episode ? 'E' + q.episode : ''}` : r.title}<span class="font-normal text-gray-500">{q.state === 'importing' ? ' · adding to Plex' : q.state === 'queued' ? ' · waiting its turn' : ''}</span></span>
									<span class="shrink-0 tabular-nums text-gray-400">{q.percent}%{q.timeLeft && q.state === 'downloading' ? ` · ${timeLeft(q.timeLeft)}` : ''}{q.size ? ` · ${fmtBytes(q.size)}` : ''}</span>
								</div>
								<div class="mt-2 h-1.5 overflow-hidden rounded-full bg-gray-700/60"><div class="h-full rounded-full {q.state === 'warning' || q.state === 'failed' ? 'bg-yellow-500' : 'bg-blue-500'}" style="width:{q.percent}%"></div></div>
								{#if q.problem}<p class="mt-1.5 text-xs text-yellow-200">{q.problem}</p>{/if}
								{#if q.detail}<p class="mt-1 text-[11px] text-gray-500">{q.detail}</p>{/if}
								{#if admin && q.release}<p class="mt-1 truncate font-mono text-[10px] text-gray-500">{q.release}</p>{/if}
							</div>
						{/each}
					</div>
				</section>
			{/if}

			{#if d.live.seasons?.length}
				<section>
					{@render head('Seasons')}
					<div class="card space-y-2.5 p-3">
						{#each d.live.seasons as s (s.number)}
							{@const n = Math.max(s.have, s.inPlex)}
							{@const done = s.total > 0 && s.inPlex >= s.total}
							<div>
								<div class="flex items-baseline justify-between gap-3 text-xs">
									<span class="font-medium text-gray-200">Season {s.number}</span>
									<span class="tabular-nums {done ? 'text-green-400' : 'text-gray-400'}">{done ? 'In Plex' : `${n} of ${s.total} episodes`}</span>
								</div>
								<div class="mt-1.5 h-1.5 overflow-hidden rounded-full bg-gray-700/60"><div class="h-full rounded-full {done ? 'bg-green-500' : 'bg-primary'}" style="width:{s.total ? Math.min(100, (n / s.total) * 100) : 0}%"></div></div>
							</div>
						{/each}
					</div>
				</section>
			{/if}

			{#if d.live.file}
				<section>
					{@render head(series ? 'Files' : 'File')}
					<div class="card">
						<FolderCard path={d.live.file.path} size={d.live.file.size} quality={series ? `${episodes} episode${episodes === 1 ? '' : 's'}` : d.live.file.quality} release={d.live.file.release} />
					</div>
				</section>
			{/if}

			<section>
				{@render head('Timeline')}
				<Timeline steps={steps} />
			</section>

			{#if declining}
				<div class="space-y-2 rounded-lg bg-gray-800/50 p-3">
					<label for="rd-why" class="block text-sm">Reason <span class="text-gray-500">(optional)</span></label>
					<textarea id="rd-why" rows="2" maxlength="500" bind:value={reason} class="w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60" placeholder="e.g. it's not out in good quality yet"></textarea>
					<div class="flex justify-end gap-2"><button class="btn-ghost px-3 py-1 text-xs" onclick={() => (declining = false)}>Cancel</button><button class="btn px-3 py-1 text-xs" disabled={!!busy} onclick={() => act('decline', { reason: reason.trim() })}>Decline</button></div>
				</div>
			{/if}
			{#if removing}
				<div class="space-y-2 rounded-lg bg-gray-800/50 p-3 text-xs text-gray-300">
					<p>The request is removed. Anything already downloaded stays.</p>
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
			{#if d?.live.link}<a href={d.live.link} target="_blank" rel="noopener" class="btn-ghost px-3 py-1.5 text-xs"><img src="/logos/{series ? 'sonarr' : 'radarr'}.svg" alt="" class="size-3.5" />Open in {d.live.server}</a>{/if}
			{#if r.status === 'pending'}
				<button class="btn-ghost px-3 py-1.5 text-xs" disabled={!!busy} onclick={() => ((declining = true), (reason = ''))}>Decline</button>
				<button class="btn px-4 py-1.5 text-xs" disabled={!!busy} onclick={() => act('approve')}>{busy === 'approve' ? 'Approving…' : 'Approve'}</button>
			{:else if r.status === 'failed'}
				<button class="btn px-4 py-1.5 text-xs" disabled={!!busy} onclick={() => act('retry')}>{busy === 'retry' ? 'Trying…' : 'Try again'}</button>
			{:else if fixable && d}
				<FixMenu title={r.title} {series} live={d.live} search={() => act('fix')} redownload={() => (redoOpen = true)} ondone={load} />
			{/if}
		{:else if r && r.status === 'pending' && r.userId === session.me?.id}
			<button class="mr-auto px-2 text-xs text-gray-500 hover:text-red-300" onclick={() => (removing = true)}>Cancel request</button>
			<button class="btn-ghost" onclick={() => (open = false)}>Close</button>
		{:else}
			{#if r && d && fixable && can('redownload')}<span class="mr-auto"><FixMenu title={r.title} {series} live={d.live} redownload={() => (redoOpen = true)} ondone={load} /></span>{/if}
			<button class="btn-ghost" onclick={() => (open = false)}>Close</button>
		{/if}
	{/snippet}
</Modal>

{#if asItem && can('redownload')}
	{#if series}<FixEpisodesModal bind:open={redoOpen} item={asItem} ondone={load} />{:else}<RedownloadModal bind:open={redoOpen} item={asItem} />{/if}
{/if}
