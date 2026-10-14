<script lang="ts">
	// A title's admin panel (people who manage requests): what Radarr / Sonarr say about it right now, every
	// request for it, the file, and the admin buttons. Opens from the side; full screen on phones.
	import { api, fmtDate } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { can } from '#lib/session.svelte.ts';
	import { on } from '#lib/live.svelte.ts';
	import { remember } from '#lib/availability.svelte.ts';
	import type { Movie, Series, Request } from '#lib/types.ts';
	import { text, TONE, NEUTRAL } from '#lib/requeststatus.ts';
	import Modal from './Modal.svelte';
	import Icon from './Icon.svelte';
	import RequestDetail from './RequestDetail.svelte';
	import RedownloadModal from './RedownloadModal.svelte';
	import LeavingActions from './LeavingActions.svelte';

	type Download = { season?: number; episode?: number; percent: number; timeLeft?: string; state: string; size: number; release?: string; problem?: string };
	type Live = { inArr: boolean; monitored: boolean; hasFile: boolean; file?: { quality?: string; size: number; release?: string; path?: string }; queue: Download[]; seasons?: { number: number; have: number; total: number; inPlex: number }[]; link?: string; server: string; error?: string };
	type Info = { live: Live; requests: Request[]; blocked: boolean; plex: string; issues: number };

	let { open = $bindable(false), item }: { open?: boolean; item: Movie | Series } = $props();
	let info = $state<Info | null>(null);
	let busy = $state('');
	let detail = $state(0);
	let detailOpen = $state(false);
	let redownloadOpen = $state(false);
	let blockOpen = $state(false);
	const key = $derived(`${item.type}:${item.id}`);

	async function load() {
		try {
			info = await api<Info>(`/titles/${item.type}/${item.id}/admin`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not load it.', 'orange');
		}
	}
	$effect(() => {
		if (!open) return;
		info = null;
		load();
		let timer: ReturnType<typeof setTimeout> | null = null;
		const soon = () => timer || (timer = setTimeout(() => ((timer = null), load()), 1500));
		const offs = [on('request', (e: { key: string }) => e.key === key && soon()), on('plex', (keys: string[]) => keys.includes(key) && soon())];
		return () => (offs.forEach((off) => off()), timer && clearTimeout(timer));
	});

	// The state in one line: the newest open request's exact status, or what Radarr / Sonarr have.
	const open_ = $derived(info?.requests.find((r) => !['failed', 'declined', 'expired'].includes(r.status)));
	const current = $derived.by(() => {
		if (!info) return null;
		if (open_) return { label: text(open_.status, open_.progress), tone: TONE[open_.status] ?? NEUTRAL };
		if (info.plex) return { label: 'In Plex (not requested through Flixarr)', tone: TONE.available! };
		if (info.live.hasFile) return { label: `In ${info.live.server}, not in Plex yet`, tone: NEUTRAL };
		if (info.live.inArr) return { label: `In ${info.live.server}, no file${info.live.monitored ? '' : ', not monitored'}`, tone: NEUTRAL };
		return { label: 'Not requested', tone: NEUTRAL };
	});
	const size = (b: number) => (b >= 1e12 ? `${(b / 1e12).toFixed(1)} TB` : b >= 1e9 ? `${(b / 1e9).toFixed(1)} GB` : `${Math.round(b / 1e6)} MB`);
	const left = (t?: string) => {
		if (!t) return '';
		const [h, m] = t.split(':').map(Number);
		return h ? `${h} h ${m} min left` : m ? `${m} min left` : 'under a minute left';
	};
	const what = (r: Request) => (r.type !== 'series' ? '' : r.mode === 'all' ? 'Whole show' : r.mode === 'episodes' ? `${r.episodes.length} episodes` : r.seasons.length === 1 ? `Season ${r.seasons[0]}` : `Seasons ${r.seasons.join(', ')}`);

	async function fix() {
		busy = 'fix';
		try {
			const d = await api<{ done: string }>(`/titles/${item.type}/${item.id}/fix`, { method: 'POST' });
			notify(d.done || 'Checked it and started a search.');
			load();
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not fix it.', 'orange');
		} finally {
			busy = '';
		}
	}
	async function block(on: boolean) {
		blockOpen = false;
		busy = 'block';
		try {
			if (on) await api('/blocklist', { method: 'POST', body: JSON.stringify({ type: item.type, tmdbId: item.id, title: item.title, year: item.year, poster: item.poster ?? '' }) });
			else await api(`/blocklist/${item.type}/${item.id}`, { method: 'DELETE' });
			remember(key, on ? { id: 0, status: 'blocked', progress: 0, blocked: true } : null);
			notify(on ? `${item.title} is blocked.` : `${item.title} can be requested again.`);
			load();
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not change it.', 'orange');
		} finally {
			busy = '';
		}
	}
</script>

{#snippet head(t: string)}<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">{t}</div>{/snippet}

<Modal bind:open title="Admin" sub={item.title} side>
	{#if !info}
		<p class="p-5 text-sm text-gray-400">Loading…</p>
	{:else}
		<div class="space-y-5 p-5 text-sm">
			<div class="card space-y-3 p-4">
				{#if current}<span class="inline-flex rounded-lg px-3 py-1 text-sm font-medium ring-1 {current.tone}">{current.label}</span>{/if}
				{#if info.live.error}<p class="text-xs text-red-300">{info.live.error}</p>{/if}
				<div class="flex flex-wrap gap-2">
					{#if info.live.link}<a href={info.live.link} target="_blank" rel="noopener" class="btn-ghost px-3 py-1.5 text-xs">Open in {info.live.server} <Icon name="external" class="size-3" /></a>{/if}
					{#if info.plex}<a href={info.plex} target="_blank" rel="noopener" class="btn-ghost px-3 py-1.5 text-xs">Open in Plex <Icon name="external" class="size-3" /></a>{/if}
					{#if info.live.inArr}<button class="btn-ghost px-3 py-1.5 text-xs" disabled={!!busy} onclick={fix}>{busy === 'fix' ? 'Fixing…' : 'Fix'}</button>{/if}
					{#if info.live.hasFile && can('redownload')}<button class="btn-ghost px-3 py-1.5 text-xs" onclick={() => (redownloadOpen = true)}><Icon name="refresh" class="size-3.5" />Redownload</button>{/if}
				</div>
				{#if info.live.inArr}<p class="text-[11px] text-gray-500">{open_ ? 'Fix adds it back if it went missing, turns monitoring back on for what was asked, and starts a search.' : `Fix makes sure ${info.live.server} is watching for it and starts a search.`}</p>{/if}
			</div>

			{#if info.live.queue.length}
				<div>
					{@render head('Downloading')}
					<div class="space-y-2">
						{#each info.live.queue as q, i (i)}
							<div class="rounded-lg bg-gray-800/50 px-3 py-2">
								<div class="flex items-baseline justify-between gap-3 text-xs">
									<span class="text-gray-200">{q.season ? `S${q.season}${q.episode ? 'E' + q.episode : ''}` : item.title} · {q.state}</span>
									<span class="shrink-0 text-gray-400">{q.percent}%{q.timeLeft && q.state === 'downloading' ? ` · ${left(q.timeLeft)}` : ''}</span>
								</div>
								<div class="mt-1.5 h-1 overflow-hidden rounded-full bg-gray-700/60"><div class="h-full rounded-full {q.state === 'warning' || q.state === 'failed' ? 'bg-yellow-500' : 'bg-blue-500'}" style="width:{q.percent}%"></div></div>
								{#if q.problem}<p class="mt-1.5 text-xs text-yellow-200">{q.problem}</p>{/if}
								{#if q.release}<p class="mt-1 truncate text-[11px] text-gray-500">{q.release}</p>{/if}
							</div>
						{/each}
					</div>
				</div>
			{/if}

			{#if info.live.seasons?.length}
				<div>
					{@render head('Seasons')}
					<div class="space-y-1.5">
						{#each info.live.seasons as s (s.number)}
							<div class="flex items-center justify-between gap-3 text-xs">
								<span class="text-gray-300">Season {s.number}</span>
								<span class="text-gray-400">{s.have} of {s.total} in {info.live.server} · {s.inPlex} in Plex</span>
							</div>
						{/each}
					</div>
				</div>
			{/if}

			{#if info.live.file}
				<div>
					{@render head('File')}
					<p class="text-xs text-gray-300">{info.live.file.quality ? `${info.live.file.quality} · ` : ''}{size(info.live.file.size)}</p>
					{#if info.live.file.release}<p class="mt-1 break-all text-[11px] text-gray-500">{info.live.file.release}</p>{/if}
					{#if info.live.file.path}<p class="mt-1 break-all text-[11px] text-gray-500">{info.live.file.path}</p>{/if}
				</div>
			{/if}

			<div>
				{@render head(`Requests (${info.requests.length})`)}
				{#if !info.requests.length}
					<p class="text-xs text-gray-500">Nobody has requested it through Flixarr.</p>
				{:else}
					<div class="divide-y divide-gray-700/40 overflow-hidden rounded-lg border border-gray-700/50">
						{#each info.requests as r (r.id)}
							<button class="flex w-full items-center gap-3 px-3 py-2 text-left hover:bg-white/5" onclick={() => ((detail = r.id), (detailOpen = true))}>
								<span class="min-w-0 flex-1">
									<span class="block truncate text-xs text-gray-200">{r.source === 'redownload' ? 'Redownload by' : ''} {r.userName || 'Someone'}{what(r) ? ` · ${what(r)}` : ''}</span>
									<span class="block text-[11px] text-gray-500">{fmtDate(r.createdAt, { month: 'short', day: 'numeric', year: 'numeric' })}{r.profileName ? ` · ${r.profileName}` : ''}</span>
								</span>
								<span class="shrink-0 rounded-md px-2 py-0.5 text-[11px] ring-1 {TONE[r.status] ?? NEUTRAL}">{text(r.status, r.progress)}</span>
								<Icon name="chevron" class="size-3.5 shrink-0 text-gray-500" />
							</button>
						{/each}
					</div>
				{/if}
			</div>

			<div class="flex flex-wrap"><LeavingActions type={item.type as 'movie' | 'series'} id={item.id} title={item.title} /></div>

			<div class="flex flex-wrap items-center justify-between gap-2 border-t border-gray-700/40 pt-4">
				{#if info.issues}<a href="/issues" class="text-xs text-primary-light hover:underline">{info.issues} open issue{info.issues === 1 ? '' : 's'} on this title</a>{:else}<span class="text-xs text-gray-500">No open issues</span>{/if}
				{#if info.blocked}<button class="btn-ghost px-3 py-1.5 text-xs" disabled={!!busy} onclick={() => block(false)}>Unblock</button>
				{:else if !info.plex && !open_}<button class="btn-ghost px-3 py-1.5 text-xs" disabled={!!busy} onclick={() => (blockOpen = true)}>Block</button>{/if}
			</div>
			{#if blockOpen}
				<div class="space-y-2 rounded-lg bg-gray-800/50 p-3 text-xs text-gray-300">
					<p>Nobody can request it until it's unblocked (Settings → Blocklist).</p>
					<div class="flex justify-end gap-2"><button class="btn-ghost px-3 py-1 text-xs" onclick={() => (blockOpen = false)}>Cancel</button><button class="btn px-3 py-1 text-xs" onclick={() => block(true)}>Block</button></div>
				</div>
			{/if}
		</div>
	{/if}
</Modal>
{#if detail}<RequestDetail bind:open={detailOpen} id={detail} onchanged={() => load()} />{/if}
<RedownloadModal bind:open={redownloadOpen} {item} />
