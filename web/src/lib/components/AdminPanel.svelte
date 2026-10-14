<script lang="ts">
	// A title's admin panel (people who manage requests): its requests, what's downloading, the media in
	// Radarr / Sonarr and Plex, open issues, and the actions. Opens from the side; full screen on phones.
	import { api, fmtDate, fmtBytes, ago } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { can } from '#lib/session.svelte.ts';
	import { on } from '#lib/live.svelte.ts';
	import { remember } from '#lib/availability.svelte.ts';
	import type { Movie, Series, Request, User } from '#lib/types.ts';
	import { text, TONE, NEUTRAL } from '#lib/requeststatus.ts';
	import Modal from './Modal.svelte';
	import Icon from './Icon.svelte';
	import Avatar from './Avatar.svelte';
	import PlexMark from './PlexMark.svelte';
	import RequestDetail from './RequestDetail.svelte';
	import RedownloadModal from './RedownloadModal.svelte';
	import LeavingActions from './LeavingActions.svelte';

	type Download = { season?: number; episode?: number; percent: number; timeLeft?: string; state: string; size: number; release?: string; problem?: string; detail?: string };
	type Live = { inArr: boolean; monitored: boolean; hasFile: boolean; file?: { quality?: string; size: number; release?: string; path?: string }; queue: Download[]; seasons?: { number: number; have: number; total: number; inPlex: number }[]; link?: string; server: string; error?: string };
	type Comment = { userName: string; userAvatar: string; body: string; createdAt: string };
	type Issue = { id: number; kind: string; sub: string; note: string; season: number; episode: number; parts?: [number, number][]; userName: string; userAvatar: string; createdAt: string; comments: number; last?: Comment };
	type Info = { live: Live; requests: Request[]; blocked: boolean; plex: string; issues: Issue[]; kept: boolean };

	let { open = $bindable(false), item }: { open?: boolean; item: Movie | Series } = $props();
	let info = $state<Info | null>(null);
	let busy = $state('');
	let detail = $state(0);
	let detailOpen = $state(false);
	let redownloadOpen = $state(false);
	let confirm = $state<'' | 'remove' | 'clear' | 'delete' | 'block'>('');
	let replyTo = $state(0);
	let reply = $state('');
	const key = $derived(`${item.type}:${item.id}`);
	const series = $derived(item.type === 'series');

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
	const openReq = $derived(info?.requests.find((r) => !['failed', 'declined', 'expired'].includes(r.status)));
	const current = $derived.by(() => {
		if (!info) return null;
		if (openReq) return { label: text(openReq.status, openReq.progress), tone: TONE[openReq.status] ?? NEUTRAL };
		if (info.plex) return { label: 'Available', tone: TONE.available! };
		if (info.live.hasFile) return { label: `In ${info.live.server}, not in Plex yet`, tone: NEUTRAL };
		if (info.live.inArr) return { label: info.live.monitored ? `In ${info.live.server}, no file yet` : `In ${info.live.server}, not monitored`, tone: NEUTRAL };
		return { label: 'Not requested', tone: NEUTRAL };
	});
	const left = (t?: string) => {
		if (!t) return '';
		const [h, m] = t.split(':').map(Number);
		return h ? `${h} h ${m} min left` : m ? `${m} min left` : 'under a minute left';
	};
	const what = (r: Request) => (r.type !== 'series' ? '' : r.mode === 'all' ? 'Whole show' : r.mode === 'episodes' ? `${r.episodes.length} episodes` : r.seasons.length === 1 ? `Season ${r.seasons[0]}` : `Seasons ${r.seasons.join(', ')}`);
	const person = (name: string, avatar = '') => ({ username: name || '?', avatar }) as User;
	const where = (i: Issue) => {
		const p = i.parts?.length ? i.parts : i.season ? [[i.season, i.episode] as [number, number]] : [];
		return p.map(([s, e]) => (e ? `S${s}E${e}` : `Season ${s}`)).join(', ');
	};
	const ISSUE_TONE: Record<string, string> = { Video: 'bg-blue-500/15 text-blue-300 ring-blue-500/40', Audio: 'bg-primary/15 text-primary-light ring-primary/40', Subtitles: 'bg-yellow-500/15 text-yellow-300 ring-yellow-500/40', Other: 'bg-gray-800 text-gray-300 ring-gray-700/60' };
	// The folder, split like the Services cards: its own name big, the path above it small.
	const folder = $derived.by(() => {
		const p = info?.live.file?.path ?? '';
		const cut = p.replace(/\/$/, '').lastIndexOf('/');
		return { name: cut >= 0 ? p.slice(cut + 1) : p, parent: cut >= 0 ? p.slice(0, cut + 1) : '' };
	});

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
	// One issue card's Resolve and reply, right in the panel.
	async function resolve(i: Issue) {
		busy = `resolve${i.id}`;
		try {
			await api(`/issues/${i.id}/resolve`, { method: 'POST' });
			notify('Resolved. The person who reported it is told.');
			load();
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not resolve it.', 'orange');
		} finally {
			busy = '';
		}
	}
	async function send(i: Issue) {
		if (!reply.trim()) return;
		busy = `reply${i.id}`;
		try {
			await api(`/issues/${i.id}/comments`, { method: 'POST', body: JSON.stringify({ body: reply }) });
			reply = '';
			replyTo = 0;
			load();
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not send it.', 'orange');
		} finally {
			busy = '';
		}
	}

	// Remove from Radarr / Sonarr, Clear data and Delete everywhere: each asks first (the confirm says
	// exactly what goes), then the panel shows what's left.
	async function run(action: 'remove' | 'clear' | 'delete') {
		confirm = '';
		busy = action;
		try {
			const d = await api<{ done: string }>(`/titles/${item.type}/${item.id}/${action}`, { method: 'POST' });
			if (action !== 'remove') remember(key, null);
			notify(d.done);
			load();
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not do it.', 'orange');
			load();
		} finally {
			busy = '';
		}
	}
	const count = (n: number, one: string) => `${n} ${one}${n === 1 ? '' : 's'}`;
	// What each one takes, from what the panel shows.
	const goes = $derived.by(() => {
		if (!info) return { remove: [], clear: [], delete: [] } as Record<string, string[]>;
		const files = info.live.hasFile ? `${series ? 'Its files' : 'The file'}${info.live.file?.size ? ` (${fmtBytes(info.live.file.size)})` : ''} are deleted.` : '';
		const data = [info.requests.length ? count(info.requests.length, 'request') : '', info.issues.length ? count(info.issues.length, 'open issue') : ''].filter(Boolean);
		const open = info.requests.filter((r) => ['sent', 'grabbed', 'downloading', 'importing', 'available'].includes(r.status)).length;
		return {
			remove: [`${item.title} is removed from ${info.live.server}.`, files, open ? `${count(open, 'request')} for it end as failed, so it can be requested again.` : '', info.plex ? 'Plex drops it after its next scan.' : ''].filter(Boolean),
			clear: [`Everything Flixarr has for it goes${data.length ? `: ${data.join(' and ')}` : ''}, with their history and comments.`, `Nothing changes in ${info.live.server || 'Radarr or Sonarr'} or Plex. If it's in Plex, it shows up again after the next scan.`],
			delete: [
				info.live.inArr ? `Removed from ${info.live.server}${info.live.hasFile ? ' with its files' : ''}${info.live.file?.size ? ` (${fmtBytes(info.live.file.size)})` : ''}.` : '',
				info.plex ? 'Deleted from Plex.' : '',
				info.kept ? 'Maintainerr stops keeping it.' : '',
				`Flixarr's data for it is cleared${data.length ? ` (${data.join(', ')})` : ''}.`
			].filter(Boolean)
		};
	});

	async function block(on: boolean) {
		confirm = '';
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

{#snippet head(t: string, n?: number)}<h3 class="mb-2.5 text-base font-bold">{t}{#if n}<span class="ml-1.5 text-sm font-normal text-gray-500">{n}</span>{/if}</h3>{/snippet}

<Modal bind:open title={series ? 'Manage series' : 'Manage movie'} sub={item.title} side>
	{#if !info}
		<p class="p-5 text-sm text-gray-400">Loading…</p>
	{:else}
		<div class="space-y-7 p-5 text-sm">
			{#if current}
				<div class="flex flex-wrap items-center gap-2">
					<span class="inline-flex rounded-lg px-3 py-1 text-sm font-medium ring-1 {current.tone}">{current.label}</span>
					{#if info.blocked}<span class="inline-flex rounded-lg bg-red-500/15 px-3 py-1 text-sm font-medium text-red-300 ring-1 ring-red-500/40">Blocked</span>{/if}
				</div>
			{/if}
			{#if info.live.error}<p class="-mt-4 text-xs text-red-300">{info.live.error}</p>{/if}

			<!-- Requests -->
			<section>
				{@render head('Requests', info.requests.length)}
				{#if !info.requests.length}
					<p class="rounded-xl border border-dashed border-gray-700/60 px-4 py-3 text-xs text-gray-500">Nobody has requested it through Flixarr.</p>
				{:else}
					<div class="space-y-2">
						{#each info.requests as r (r.id)}
							<button class="card flex w-full items-center gap-3 p-3 text-left transition hover:ring-1 hover:ring-primary/50" onclick={() => ((detail = r.id), (detailOpen = true))}>
								<Avatar user={person(r.userName, r.userAvatar)} class="size-8 text-xs" />
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium text-gray-100">{r.source === 'redownload' ? 'Redownload by ' : ''}{r.userName || 'Someone'}</span>
									<span class="block truncate text-[11px] text-gray-500">{[what(r), r.profileName, fmtDate(r.createdAt, { month: 'short', day: 'numeric', year: 'numeric' })].filter(Boolean).join(' · ')}</span>
								</span>
								<span class="shrink-0 rounded-md px-2 py-0.5 text-[11px] font-medium ring-1 {TONE[r.status] ?? NEUTRAL}">{text(r.status, r.progress)}</span>
								<Icon name="chevron" class="size-3.5 shrink-0 text-gray-500" />
							</button>
						{/each}
					</div>
				{/if}
			</section>

			<!-- Downloading -->
			{#if info.live.queue.length}
				<section>
					{@render head('Downloading', info.live.queue.length)}
					<div class="card divide-y divide-gray-700/40">
						{#each info.live.queue as q, i (i)}
							<div class="p-3">
								<div class="flex items-baseline justify-between gap-3 text-xs">
									<span class="font-medium text-gray-200">{q.season ? `S${q.season}${q.episode ? 'E' + q.episode : ''}` : item.title} <span class="font-normal text-gray-500">· {q.state}</span></span>
									<span class="shrink-0 tabular-nums text-gray-400">{q.percent}%{q.timeLeft && q.state === 'downloading' ? ` · ${left(q.timeLeft)}` : ''}</span>
								</div>
								<div class="mt-2 h-1.5 overflow-hidden rounded-full bg-gray-700/60"><div class="h-full rounded-full {q.state === 'warning' || q.state === 'failed' ? 'bg-yellow-500' : 'bg-blue-500'}" style="width:{q.percent}%"></div></div>
								{#if q.detail || q.problem}<p class="mt-1.5 text-xs text-yellow-200">{q.detail || q.problem}</p>{/if}
								{#if q.release}<p class="mt-1 truncate font-mono text-[10px] text-gray-500">{q.release}</p>{/if}
							</div>
						{/each}
					</div>
				</section>
			{/if}

			<!-- Media -->
			<section>
				{@render head('Media')}
				<div class="card divide-y divide-gray-700/40">
					{#if info.live.file}
						<div class="p-3">
							<div class="flex items-end justify-between gap-3">
								<span class="flex min-w-0 items-center gap-2">
									<span class="grid size-7 shrink-0 place-items-center rounded-md bg-gray-900/70 text-gray-400 ring-1 ring-gray-700/50"><Icon name="folder" class="size-3.5" /></span>
									<span class="min-w-0 leading-tight"><span class="block truncate text-xs font-medium text-gray-200">{folder.name || 'Files'}</span>{#if folder.parent}<span class="block truncate font-mono text-[10px] text-gray-500">{folder.parent}</span>{/if}</span>
								</span>
								<span class="shrink-0 text-right leading-tight"><span class="block text-xs font-semibold tabular-nums text-gray-100">{fmtBytes(info.live.file.size)}</span>{#if info.live.file.quality}<span class="block text-[10px] text-gray-500">{info.live.file.quality}</span>{/if}</span>
							</div>
							{#if info.live.file.release}<p class="mt-2 break-all font-mono text-[10px] text-gray-500">{info.live.file.release}</p>{/if}
						</div>
					{:else}
						<p class="p-3 text-xs text-gray-500">{info.live.inArr ? `No file in ${info.live.server} yet.` : `Not in ${info.live.server || 'Radarr or Sonarr'}.`}</p>
					{/if}
					{#if info.live.seasons?.length}
						<div class="space-y-2.5 p-3">
							{#each info.live.seasons as s (s.number)}
								{@const inPlex = s.total > 0 && s.inPlex >= s.total}
								<div>
									<div class="flex items-baseline justify-between gap-3 text-xs">
										<span class="font-medium text-gray-200">Season {s.number}</span>
										<span class="flex items-center gap-2 tabular-nums text-gray-400">{s.have} of {s.total}{#if inPlex}<span class="inline-flex items-center gap-1 rounded bg-[#e5a00d]/15 px-1.5 py-0.5 text-[10px] font-semibold text-[#e5a00d]"><PlexMark class="size-2.5" color="#e5a00d" />In Plex</span>{:else if s.inPlex}<span class="text-[10px] text-gray-500">{s.inPlex} in Plex</span>{/if}</span>
									</div>
									<div class="mt-1.5 h-1.5 overflow-hidden rounded-full bg-gray-700/60"><div class="h-full rounded-full {inPlex ? 'bg-green-500' : 'bg-primary'}" style="width:{s.total ? Math.min(100, (s.have / s.total) * 100) : 0}%"></div></div>
								</div>
							{/each}
						</div>
					{/if}
					{#if info.live.link || info.plex}
						<div class="grid grid-cols-2 gap-2 p-3">
							{#if info.live.link}<a href={info.live.link} target="_blank" rel="noopener" class="btn-ghost justify-center px-3 py-2 text-xs {info.plex ? '' : 'col-span-2'}"><Icon name="external" class="size-3.5" />Open in {info.live.server}</a>{/if}
							{#if info.plex}<a href={info.plex} target="_blank" rel="noopener" class="btn-ghost justify-center px-3 py-2 text-xs {info.live.link ? '' : 'col-span-2'}"><PlexMark class="size-3.5" color="#e5a00d" />Open in Plex</a>{/if}
						</div>
					{/if}
				</div>
			</section>

			<!-- Issues: open ones stand out, and can be answered or resolved right here -->
			<section>
				<h3 class="mb-2.5 flex items-center gap-2 text-base font-bold">Issues{#if info.issues.length}<span class="rounded-full bg-yellow-500/20 px-2 py-0.5 text-xs font-semibold text-yellow-300 ring-1 ring-yellow-500/40">{info.issues.length} open</span>{/if}</h3>
				{#if !info.issues.length}
					<p class="rounded-xl border border-dashed border-gray-700/60 px-4 py-3 text-xs text-gray-500">No open issues.</p>
				{:else}
					<div class="space-y-2">
						{#each info.issues as i (i.id)}
							<div class="rounded-xl bg-yellow-500/[0.06] p-3 ring-1 ring-yellow-500/30">
								<div class="flex flex-wrap items-center gap-1.5">
									<span class="rounded-md px-2 py-0.5 text-[11px] font-semibold ring-1 {ISSUE_TONE[i.kind] ?? ISSUE_TONE.Other}">{i.kind}</span>
									{#if i.sub}<span class="text-xs text-gray-200">{i.sub}</span>{/if}
									{#if where(i)}<span class="text-xs text-gray-400">· {where(i)}</span>{/if}
								</div>
								{#if i.note}<p class="mt-2 line-clamp-3 text-xs text-gray-300">{i.note}</p>{/if}
								<div class="mt-2 flex items-center gap-2 text-[11px] text-gray-500">
									<Avatar user={person(i.userName, i.userAvatar)} class="size-5 text-[10px]" />
									<span class="truncate">{i.userName} · {ago(i.createdAt)}</span>
								</div>
								{#if i.last}
									<div class="mt-2.5 rounded-lg bg-gray-900/60 p-2.5 text-xs">
										<p class="mb-1 text-[11px] text-gray-500"><span class="font-medium text-gray-300">{i.last.userName || 'Someone'}</span> · {ago(i.last.createdAt)}{i.comments > 1 ? ` · ${count(i.comments, 'comment')}` : ''}</p>
										<p class="line-clamp-3 text-gray-300">{i.last.body}</p>
									</div>
								{/if}
								{#if replyTo === i.id}
									<div class="mt-2.5 space-y-2">
										<textarea rows="2" maxlength="2000" class="w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-xs outline-none focus:ring-2 focus:ring-primary/60" placeholder="Write a reply…" bind:value={reply}></textarea>
										<div class="flex justify-end gap-2"><button class="btn-ghost px-3 py-1 text-xs" onclick={() => ((replyTo = 0), (reply = ''))}>Cancel</button><button class="btn px-3 py-1 text-xs" disabled={!reply.trim() || !!busy} onclick={() => send(i)}>{busy === `reply${i.id}` ? 'Sending…' : 'Send'}</button></div>
									</div>
								{:else}
									<div class="mt-2.5 flex flex-wrap gap-2">
										{#if can('manageIssues')}<button class="btn-ghost px-3 py-1.5 text-xs" disabled={!!busy} onclick={() => resolve(i)}><Icon name="check" class="size-3.5" />{busy === `resolve${i.id}` ? 'Resolving…' : 'Resolve'}</button>{/if}
										<button class="btn-ghost px-3 py-1.5 text-xs" onclick={() => ((replyTo = i.id), (reply = ''))}>Reply</button>
										<a href="/issues?open={i.id}" class="ml-auto inline-flex items-center gap-1 px-1 py-1.5 text-xs text-gray-400 hover:text-gray-200">Open<Icon name="chevron" class="size-3" /></a>
									</div>
								{/if}
							</div>
						{/each}
					</div>
				{/if}
			</section>

			<div class="flex flex-wrap empty:hidden"><LeavingActions type={item.type as 'movie' | 'series'} id={item.id} title={item.title} /></div>

			<!-- Actions: repairs first, then what removes things (each asks first) -->
			<section>
				{@render head('Actions')}
				<div class="space-y-3">
					{#if info.live.inArr}
						<div>
							<button class="btn-ghost w-full justify-center py-2.5" disabled={!!busy} onclick={fix}><Icon name="check" class="size-4" />{busy === 'fix' ? 'Fixing…' : 'Fix'}</button>
							<p class="mt-1.5 text-[11px] text-gray-500">{openReq ? 'Adds it back if it went missing, turns monitoring back on for what was asked, and starts a search.' : `Makes sure ${info.live.server} is watching for it and starts a search.`}</p>
						</div>
					{/if}
					{#if info.live.hasFile && can('redownload')}
						<div>
							<button class="btn-ghost w-full justify-center py-2.5" onclick={() => (redownloadOpen = true)}><Icon name="refresh" class="size-4" />Redownload</button>
							<p class="mt-1.5 text-[11px] text-gray-500">Deletes the current {series ? 'files of the seasons you pick' : 'file'} and downloads a new copy. The release it came from isn't grabbed again.</p>
						</div>
					{/if}
				</div>
			</section>

			<section>
				<h3 class="mb-2.5 text-base font-bold text-red-300">Remove</h3>
				<div class="divide-y divide-red-500/20 rounded-xl ring-1 ring-red-500/30">
					{#snippet danger(action: 'remove' | 'clear' | 'delete' | 'block', label: string, help: string)}
						<div class="p-3">
							{#if confirm === action}
								<div class="space-y-2 text-xs">
									<p class="font-semibold text-gray-100">{label}?</p>
									{#if action === 'block'}<p class="text-gray-300">Nobody can request it until it's unblocked (Settings, Blocklist).</p>
									{:else}<ul class="list-disc space-y-1 pl-4 text-gray-300">{#each goes[action] as g}<li>{g}</li>{/each}</ul>
										<p class="text-gray-500">This can't be undone.</p>{/if}
									<div class="flex justify-end gap-2"><button class="btn-ghost px-3 py-1 text-xs" onclick={() => (confirm = '')}>Cancel</button><button class="inline-flex items-center rounded-lg bg-red-600/90 px-3 py-1 text-xs font-medium text-white hover:bg-red-600" onclick={() => (action === 'block' ? block(true) : run(action))}>{label}</button></div>
								</div>
							{:else}
								<div class="flex items-center justify-between gap-3">
									<span class="min-w-0"><span class="block text-sm font-medium text-gray-100">{label}</span><span class="block text-[11px] text-gray-500">{help}</span></span>
									<button class="shrink-0 rounded-lg border border-red-500/40 px-3 py-1.5 text-xs font-medium text-red-300 transition hover:bg-red-500/10" disabled={!!busy} onclick={() => (confirm = action)}>{busy === action ? 'Working…' : label.split(' ')[0]}</button>
								</div>
							{/if}
						</div>
					{/snippet}
					{#if info.blocked}
						<div class="flex items-center justify-between gap-3 p-3">
							<span class="min-w-0"><span class="block text-sm font-medium text-gray-100">Blocked</span><span class="block text-[11px] text-gray-500">Nobody can request it.</span></span>
							<button class="btn-ghost shrink-0 px-3 py-1.5 text-xs" disabled={!!busy} onclick={() => block(false)}>Unblock</button>
						</div>
					{:else if !info.plex && !openReq}
						{@render danger('block', 'Block', 'Stops anyone from requesting it.')}
					{/if}
					{#if info.live.inArr && can('redownload')}{@render danger('remove', `Remove from ${info.live.server}`, `Takes it out of ${info.live.server} with its files.`)}{/if}
					{@render danger('clear', 'Clear data', 'Forgets its requests and issues in Flixarr only.')}
					{#if can('redownload')}{@render danger('delete', 'Delete everywhere', `${info.live.server || 'Radarr or Sonarr'}, Plex, Maintainerr and Flixarr.`)}{/if}
				</div>
			</section>
		</div>
	{/if}
</Modal>
{#if detail}<RequestDetail bind:open={detailOpen} id={detail} onchanged={() => load()} />{/if}
<RedownloadModal bind:open={redownloadOpen} {item} />
