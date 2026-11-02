<script lang="ts">
	// Requests: yours or everyone's (anyone can see all requests), one row per request (#189). Tap a row for
	// its window (timeline, live progress, the title's issues, and the buttons for people who manage requests).
	// People who manage requests can also tick rows and act on them together (#190). Rows update on their
	// own while the page is open.
	import Select from '#lib/components/Select.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import Check from '#lib/components/Check.svelte';
	import RequestRow, { rowCols } from '#lib/components/RequestRow.svelte';
	import RequestDetail from '#lib/components/RequestDetail.svelte';
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { session, can } from '#lib/session.svelte.ts';
	import type { IssueBrief, Request } from '#lib/types.ts';
	import { on } from '#lib/live.svelte.ts';
	import { page } from '$app/state';
	import { infinite } from '#lib/infinite.ts';
	import { SvelteSet } from 'svelte/reactivity';

	let list = $state<Request[] | null>(null);
	let issues = $state<Record<string, IssueBrief[]>>({});
	let approval = $state(false);
	const admin = $derived(can('manageRequests'));
	// Your requests or all of them; people who manage requests start on all. Remembered on the device.
	let scope = $state<'mine' | 'all'>((localStorage.getItem('flixarr:requests-scope') as 'mine' | 'all') ?? (can('manageRequests') ? 'all' : 'mine'));
	function pick(k: 'mine' | 'all') {
		scope = k;
		localStorage.setItem('flixarr:requests-scope', k);
	}
	const mine = $derived((list ?? []).filter((r) => r.userId === session.me?.id).length);
	let group = $state<'all' | 'onway' | 'attention' | 'ready' | 'ended' | 'issues'>('all');
	let query = $state('');
	let sort = $state<'new' | 'old' | 'title' | 'status'>('new');
	let busy = $state(0);
	let detail = $state(0);
	let detailOpen = $state(false);

	async function load() {
		try {
			const d = await api<{ requests: Request[]; approvalRequired: boolean; issues: Record<string, IssueBrief[]> }>('/requests');
			list = d.requests;
			issues = d.issues ?? {};
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

	const issuesOf = (r: Request) => issues[`${r.type}:${r.tmdbId}`] ?? [];
	// Where a request is. Waiting for approval needs attention from people who approve; for everyone else
	// it's on its way.
	const groupOf = (r: Request) =>
		r.status === 'available' ? 'ready'
		: r.status === 'declined' || r.status === 'expired' || r.status === 'removed' ? 'ended'
		: r.status === 'failed' || r.trouble || (r.status === 'pending' && admin) ? 'attention'
		: 'onway';
	const inGroup = (r: Request, g: typeof group) => g === 'all' || (g === 'issues' ? issuesOf(r).length > 0 : groupOf(r) === g);
	const ORDER = ['pending', 'failed', 'importing', 'downloading', 'grabbed', 'sent', 'approved', 'available', 'declined', 'removed', 'expired'];
	const scoped = $derived((list ?? []).filter((r) => scope === 'all' || r.userId === session.me?.id));
	const shown = $derived.by(() => {
		const q = query.trim().toLowerCase();
		const out = scoped.filter((r) => inGroup(r, group) && (!q || r.title.toLowerCase().includes(q) || r.userName.toLowerCase().includes(q)));
		const by: Record<typeof sort, (a: Request, b: Request) => number> = {
			new: (a, b) => b.id - a.id,
			old: (a, b) => a.id - b.id,
			title: (a, b) => a.title.localeCompare(b.title),
			status: (a, b) => ORDER.indexOf(a.status) - ORDER.indexOf(b.status) || b.id - a.id
		};
		return out.sort(by[sort]);
	});
	const count = (g: typeof group) => scoped.filter((r) => inGroup(r, g)).length;
	// Rows are drawn 60 at a time, more as you scroll (#127); a new filter, search or sort starts over.
	const STEP = 60;
	let drawn = $state(STEP);
	$effect(() => {
		void [scope, group, query, sort];
		drawn = STEP;
	});
	const visible = $derived(shown.slice(0, drawn));
	const more = async () => {
		if (drawn >= shown.length) return false;
		drawn += STEP;
		return true;
	};
	const GROUPS: [typeof group, string][] = [['all', 'All'], ['onway', 'On its way'], ['attention', 'Needs attention'], ['ready', 'Ready'], ['ended', 'Ended'], ['issues', 'Issues']];

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

	// Picking several (#190). Each button acts only on the picked requests it applies to and says how many.
	const picked = new SvelteSet<number>();
	const sel = $derived((list ?? []).filter((r) => picked.has(r.id)));
	const allOn = $derived(shown.length > 0 && shown.every((r) => picked.has(r.id)));
	const someOn = $derived(shown.some((r) => picked.has(r.id)));
	function pickAll() {
		if (allOn) shown.forEach((r) => picked.delete(r.id));
		else shown.forEach((r) => picked.add(r.id));
	}
	// Leaving the page's own list (another tab or person's requests) keeps nothing hidden ticked.
	$effect(() => {
		void scope;
		picked.clear();
	});
	type Act = 'approve' | 'decline' | 'retry' | 'fix' | 'delete';
	const FOR: Record<Act, (r: Request) => boolean> = {
		approve: (r) => r.status === 'pending',
		decline: (r) => r.status === 'pending',
		retry: (r) => r.status === 'failed',
		fix: (r) => ['approved', 'sent', 'grabbed', 'downloading', 'importing'].includes(r.status),
		delete: () => true
	};
	const ACTS: [Act, string, string][] = [['approve', 'Approve', 'btn'], ['decline', 'Decline', 'btn-ghost'], ['retry', 'Try again', 'btn-ghost'], ['fix', 'Fix', 'btn-ghost'], ['delete', 'Delete', 'btn-ghost text-red-300']];
	const DONE: Record<Act, string> = { approve: 'Approved', decline: 'Declined', retry: 'Tried again for', fix: 'Fixed', delete: 'Deleted' };
	let running = $state<Act | ''>('');
	let declineIds = $state<number[]>([]);
	let declineOpen = $state(false);
	let reason = $state('');
	let deleteOpen = $state(false);

	function ask(a: Act, ids: number[]) {
		if (a === 'decline') return ((declineIds = ids), (reason = ''), (declineOpen = true));
		if (a === 'delete') return (deleteOpen = true);
		run(a, ids);
	}
	async function run(a: Act, ids: number[], body: object = {}) {
		running = a;
		let done = 0;
		let failed = '';
		for (const id of ids) {
			try {
				if (a === 'delete') {
					await api(`/requests/${id}`, { method: 'DELETE' });
					list = list!.filter((x) => x.id !== id);
				} else {
					const x = await api<Request | { request: Request }>(`/requests/${id}/${a}`, { method: 'POST', body: JSON.stringify(body) });
					const next = 'request' in x ? x.request : x;
					list = list!.map((r) => (r.id === id ? { ...r, ...next } : r));
				}
				picked.delete(id);
				done++;
			} catch (e) {
				failed ||= e instanceof Error ? e.message : 'Something went wrong.';
			}
		}
		running = '';
		const of = ids.length > 1 ? ` of ${ids.length}` : '';
		notify(done ? `${DONE[a]} ${done}${of}.${failed ? ` ${failed}` : ''}` : failed, failed ? 'orange' : 'green');
	}
</script>

<main class="mx-auto max-w-[1600px] px-4 pt-20 laptop:px-10 laptop:pt-24 {admin && sel.length ? 'pb-44 tablet:pb-28' : 'pb-28'}">
	<div>
		<h1 class="text-2xl font-bold laptop:text-3xl">Requests</h1>
		<p class="text-sm text-gray-400">{approval ? 'New requests wait for someone who manages requests to approve them.' : 'Requests start right away.'} Tap one for its details.</p>
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
			{@const n = count(k)}
			{#if k !== 'issues' || n}
				<button class="shrink-0 rounded-full border px-3 py-1 {group === k ? 'border-primary bg-primary/15 text-white' : 'border-gray-700 text-gray-300 hover:text-white'} {k === 'attention' && n ? 'ring-1 ring-red-500/40' : ''} {k === 'issues' && group !== k ? 'text-amber-200' : ''}" onclick={() => (group = k)}>{l} <span class="text-gray-500">{n}</span></button>
			{/if}
		{/each}
	</div>

	{#if !list}
		<p class="mt-8 text-sm text-gray-400">Loading…</p>
	{:else if !shown.length}
		<div class="card mt-6 p-8 text-center text-sm text-gray-400">{scoped.length ? (query ? 'Nothing matches that search.' : 'Nothing here.') : scope === 'mine' ? "You haven't requested anything yet. Find a movie or series and hit Request." : 'No requests yet.'}</div>
	{:else}
		<div class="mt-5 overflow-hidden rounded-xl border border-gray-700/50 bg-gray-900/40">
			<div class="hidden items-center gap-4 border-b border-gray-700/50 px-4 py-2 text-[11px] font-semibold uppercase tracking-wide text-gray-500 laptop:grid {rowCols(admin)}">
				{#if admin}<Check on={allOn} some={someOn} label={allOn ? 'Unpick all' : 'Pick all shown'} onchange={pickAll} />{/if}
				<span></span><span>Title</span><span>Requested by</span>{#if admin}<span>Quality</span>{/if}<span>Status</span>
			</div>
			{#each visible as r (r.id)}
				<RequestRow {r} issues={issuesOf(r)} manager={admin} picked={picked.has(r.id)} busy={busy === r.id}
					onpick={() => (picked.has(r.id) ? picked.delete(r.id) : picked.add(r.id))} onopen={() => openDetail(r)} onapprove={() => approve(r)} ondecline={() => ask('decline', [r.id])} />
			{/each}
		</div>
		{#if drawn < shown.length}<div use:infinite={more} class="py-6 text-center text-xs text-gray-500">Loading more…</div>{/if}
	{/if}
</main>

{#if admin && sel.length}
	<div class="fixed inset-x-4 bottom-20 z-20 mx-auto max-w-2xl rounded-xl border border-gray-700 bg-gray-900/95 px-4 py-3 shadow-2xl backdrop-blur tablet:bottom-6">
		<div class="flex flex-wrap items-center gap-2">
			<span class="mr-auto text-sm font-medium">{sel.length} picked</span>
			{#each ACTS as [a, label, cls] (a)}
				{@const ids = sel.filter(FOR[a]).map((r) => r.id)}
				{#if ids.length}
					<button class="{cls} px-3 py-1 text-xs" disabled={!!running} onclick={() => ask(a, ids)}>{running === a ? 'Working…' : label}{#if ids.length < sel.length}<span class="ml-1 opacity-70">{ids.length}</span>{/if}</button>
				{/if}
			{/each}
			<button class="px-1 text-gray-400 hover:text-white" aria-label="Unpick all" onclick={() => picked.clear()}>✕</button>
		</div>
	</div>
{/if}

<Modal bind:open={declineOpen} title={declineIds.length === 1 ? 'Decline this request' : `Decline ${declineIds.length} requests`}>
	<div class="space-y-2 p-5 text-sm">
		<label for="rq-why" class="block">Reason <span class="text-gray-500">(optional)</span></label>
		<textarea id="rq-why" rows="2" maxlength="500" bind:value={reason} class="w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60" placeholder="Shown to the person who asked"></textarea>
	</div>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (declineOpen = false)}>Cancel</button>
		<button class="btn" onclick={() => ((declineOpen = false), run('decline', declineIds, { reason: reason.trim() }))}>Decline</button>
	{/snippet}
</Modal>

<Modal bind:open={deleteOpen} title={sel.length === 1 ? 'Delete this request' : `Delete ${sel.length} requests`}>
	<p class="p-5 text-sm text-gray-300">The {sel.length === 1 ? 'request is' : 'requests are'} removed. Anything already downloaded stays.</p>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (deleteOpen = false)}>Keep {sel.length === 1 ? 'it' : 'them'}</button>
		<button class="btn" onclick={() => ((deleteOpen = false), run('delete', sel.map((r) => r.id)))}>Delete</button>
	{/snippet}
</Modal>

{#if detail}<RequestDetail bind:open={detailOpen} id={detail} onchanged={(r) => { if (!r) list = list!.filter((x) => x.id !== detail); else list = list!.map((x) => (x.id === r.id ? { ...x, ...r } : x)); }} />{/if}
