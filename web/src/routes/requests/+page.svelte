<script lang="ts">
	// Requests: newest first. Everyone sees their own; View all requests shows everyone's, and Manage
	// requests can approve, decline, retry and remove. Statuses update on their own while the page is open.
	import { api, fmtDate } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { session, can } from '#lib/session.svelte.ts';
	import type { Request } from '#lib/types.ts';
	import Img from '#lib/components/Img.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import PlexMark from '#lib/components/PlexMark.svelte';
	import { statusOf } from '#lib/availability.svelte.ts';
	import { STEPS, step, text } from '#lib/requeststatus.ts';
	import { on } from '#lib/live.svelte.ts';

	let list = $state<Request[] | null>(null);
	let approval = $state(false);
	let filter = $state<'all' | 'waiting' | 'active' | 'available' | 'problems'>('all');
	let busy = $state(0);
	let removing = $state<Request | null>(null);
	let removeOpen = $state(false);
	const admin = $derived(can('manageRequests'));
	const everyone = $derived(can('viewRequests') || admin);

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
		// Live: requests change in place; a new one (or a gap in the connection) reloads the list.
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

	const GROUPS: Record<string, Request['status'][]> = { waiting: ['pending'], active: ['approved', 'sent', 'grabbed', 'downloading', 'importing'], available: ['available'], problems: ['failed', 'declined'] };
	const shown = $derived(list ? (filter === 'all' ? list : list.filter((r) => GROUPS[filter].includes(r.status))) : []);
	const count = (f: keyof typeof GROUPS) => list?.filter((r) => GROUPS[f].includes(r.status)).length ?? 0;

	const TEXT_TONE: Record<string, string> = { pending: 'text-yellow-300', grabbed: 'text-blue-300', downloading: 'text-blue-300', importing: 'text-purple-300', available: 'text-green-400', failed: 'text-red-400', declined: 'text-gray-500' };

	let declining = $state<Request | null>(null);
	let declineOpen = $state(false);
	let reason = $state('');

	async function act(r: Request, what: 'approve' | 'decline' | 'retry', why = '') {
		busy = r.id;
		try {
			const next = await api<Request>(`/requests/${r.id}/${what}`, { method: 'POST', body: JSON.stringify({ reason: why }) });
			list = list!.map((x) => (x.id === r.id ? next : x));
			notify(next.status === 'failed' ? `Couldn't send it: ${next.error}` : what === 'decline' ? 'Declined.' : 'Sent.', next.status === 'failed' ? 'orange' : 'green');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not do that.', 'orange');
		} finally {
			busy = 0;
		}
	}

	// Fix: for a request stuck at Requested (never added to Radarr/Sonarr, not monitored, or never searched).
	async function fix(r: Request) {
		busy = r.id;
		try {
			const d = await api<{ request: Request; done: string }>(`/requests/${r.id}/fix`, { method: 'POST' });
			list = list!.map((x) => (x.id === r.id ? d.request : x));
			notify(d.done || 'Checked it and started a search.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not fix it.', 'orange');
		} finally {
			busy = 0;
		}
	}

	async function remove() {
		const r = removing!;
		removeOpen = false;
		try {
			await api(`/requests/${r.id}`, { method: 'DELETE' });
			list = list!.filter((x) => x.id !== r.id);
			notify('Request removed.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not remove it.', 'orange');
		}
	}

	const seasonsText = (r: Request) =>
		r.type !== 'series' ? ''
		: r.mode === 'all' ? 'Whole show'
		: r.mode === 'episodes' ? r.episodes.map(([se, e]) => `S${se}E${e}`).slice(0, 4).join(', ') + (r.episodes.length > 4 ? ` +${r.episodes.length - 4} more` : '')
		: r.seasons.length === 1 ? `Season ${r.seasons[0]}` : `Seasons ${r.seasons.join(', ')}`;
</script>

<main class="mx-auto max-w-[1200px] px-4 pb-28 pt-20 laptop:px-10 laptop:pt-24">
	<h1 class="text-2xl font-bold laptop:text-3xl">Requests</h1>
	<p class="text-sm text-gray-400">{everyone ? "Everyone's requests." : 'Your requests.'} {approval ? 'New requests wait for an admin to approve them.' : 'Requests are sent straight to Radarr and Sonarr.'}</p>

	<div class="mt-5 flex gap-2 overflow-x-auto pb-1 text-sm">
		{#each [['all', 'All', list?.length ?? 0], ['waiting', 'Waiting', count('waiting')], ['active', 'In progress', count('active')], ['available', 'Available', count('available')], ['problems', 'Failed or declined', count('problems')]] as [k, l, n] (k)}
			<button class="shrink-0 rounded-full border px-3 py-1 {filter === k ? 'border-primary bg-primary/15 text-white' : 'border-gray-700 text-gray-300 hover:text-white'}" onclick={() => (filter = k as typeof filter)}>{l} <span class="text-gray-500">{n}</span></button>
		{/each}
	</div>

	{#if !list}
		<p class="mt-8 text-sm text-gray-400">Loading…</p>
	{:else if !shown.length}
		<div class="card mt-6 p-8 text-center text-sm text-gray-400">{list.length ? 'Nothing here.' : 'No requests yet. Find a movie or series and hit Request.'}</div>
	{:else}
		<div class="mt-5 space-y-3">
			{#each shown as r (r.id)}
				<div class="card relative overflow-hidden">
					{#if r.backdrop}<Img path={r.backdrop} kind="backdrop" sizes="(min-width: 1024px) 1000px, 100vw" class="absolute inset-0 h-full w-full opacity-15" />{/if}
					<div class="relative flex gap-4 p-4">
						<a href="/{r.type}/{r.tmdbId}" class="w-16 shrink-0 overflow-hidden rounded-lg ring-1 ring-gray-700/50 tablet:w-20">{#if r.poster}<Img path={r.poster} kind="poster" sizes="80px" class="aspect-[2/3] w-full" />{/if}</a>
						<div class="min-w-0 flex-1">
							<div class="flex flex-wrap items-baseline gap-x-2">
								<a href="/{r.type}/{r.tmdbId}" class="font-semibold hover:underline">{r.title}</a><span class="text-sm text-gray-400">{r.year}</span>
								<span class="rounded bg-gray-800/80 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-gray-400">{r.type === 'movie' ? 'Movie' : 'Series'}</span>
							</div>
							<div class="mt-0.5 text-xs text-gray-400">
								{r.userName || 'Someone'} · {fmtDate(r.createdAt, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })}{seasonsText(r) ? ` · ${seasonsText(r)}` : ''}{r.profileName ? ` · ${r.profileName}` : ''}
							</div>
							<div class="mt-2 flex items-center gap-2 text-xs {TEXT_TONE[r.status] ?? 'text-gray-300'}">{text(r.status, r.progress)}</div>
							{#if step(r.status) >= 0}
								<div class="mt-1.5 grid max-w-md grid-cols-5 gap-1">
									{#each STEPS as s, i (s)}
										<div class="h-1 overflow-hidden rounded-full bg-gray-700/60"><div class="h-full rounded-full {r.status === 'available' ? 'bg-green-500' : 'bg-primary'}" style="width:{i < step(r.status) ? 100 : i === step(r.status) ? (r.status === 'downloading' ? r.progress : 100) : 0}%"></div></div>
									{/each}
								</div>
							{/if}
							{#if r.error}<p class="mt-2 text-xs {r.status === 'declined' ? 'text-gray-400' : 'text-red-300'}">{r.status === 'declined' ? 'Reason: ' : ''}{r.error}</p>{/if}
						</div>
						<div class="flex shrink-0 flex-col items-end gap-2">
							{#if r.status === 'available' && statusOf(`${r.type}:${r.tmdbId}`)?.plex}
								<a href={statusOf(`${r.type}:${r.tmdbId}`)?.plex} target="_blank" rel="noopener" class="btn-plex px-3 py-1 text-xs"><PlexMark class="size-3.5" color="#111827" />Play</a>
							{/if}
							{#if admin && r.status === 'pending'}
								<button class="btn px-3 py-1 text-xs" disabled={busy === r.id} onclick={() => act(r, 'approve')}>Approve</button>
								<button class="btn-ghost px-3 py-1 text-xs" disabled={busy === r.id} onclick={() => { declining = r; reason = ''; declineOpen = true; }}>Decline</button>
							{:else if admin && r.status === 'failed'}
								<button class="btn-ghost px-3 py-1 text-xs" disabled={busy === r.id} onclick={() => act(r, 'retry')}>{busy === r.id ? 'Trying…' : 'Try again'}</button>
							{:else if admin && (r.status === 'approved' || r.status === 'sent')}
								<button class="btn-ghost px-3 py-1 text-xs" disabled={busy === r.id} onclick={() => fix(r)}>{busy === r.id ? 'Fixing…' : 'Fix'}</button>
							{/if}
							{#if admin || (r.status === 'pending' && r.userId === session.me?.id)}
								<button class="px-2 text-xs text-gray-500 hover:text-red-300" onclick={() => { removing = r; removeOpen = true; }}>{r.status === 'pending' && !admin ? 'Cancel' : 'Remove'}</button>
							{/if}
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</main>

<Modal bind:open={removeOpen} title="Remove this request?" sub={removing?.title}>
	<p class="p-5 text-sm text-gray-300">The request is removed from Flixarr. Anything already added to Radarr or Sonarr stays there.</p>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (removeOpen = false)}>Keep it</button>
		<button class="btn px-6" onclick={remove}>Remove</button>
	{/snippet}
</Modal>

<Modal bind:open={declineOpen} title="Decline this request?" sub={declining?.title}>
	<div class="p-5">
		<label for="decline-why" class="block text-sm">Reason <span class="text-gray-500">(optional)</span></label>
		<textarea id="decline-why" rows="2" maxlength="500" bind:value={reason} class="mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60" placeholder="e.g. it's not out in good quality yet"></textarea>
		<p class="mt-1 text-xs text-gray-500">{declining?.userName || 'They'} see{declining?.userName ? 's' : ''} it on the request and in their notification.</p>
	</div>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (declineOpen = false)}>Cancel</button>
		<button class="btn px-6" onclick={() => { declineOpen = false; act(declining!, 'decline', reason.trim()); }}>Decline</button>
	{/snippet}
</Modal>
