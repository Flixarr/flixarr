<script lang="ts">
	// Issues: problems reported on titles in the library. People who manage issues see everyone's and can
	// resolve, reopen and remove them; everyone else sees their own ("My issues") and can comment.
	import { api, fmtDate } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { session, can } from '#lib/session.svelte.ts';
	import Img from '#lib/components/Img.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import Avatar from '#lib/components/Avatar.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import type { User } from '#lib/types.ts';

	type Issue = {
		id: number; type: 'movie' | 'series'; tmdbId: number; title: string; year: string; poster: string; kind: string; sub: string;
		season: number; episode: number; parts: [number, number][]; note: string; userId: number; userName: string; userAvatar: string; status: 'open' | 'resolved';
		comments: number; createdAt: string; updatedAt: string;
	};
	type Comment = { id: number; userId: number; userName: string; userAvatar: string; body: string; createdAt: string };

	let list = $state<Issue[] | null>(null);
	let filter = $state<'open' | 'resolved' | 'all'>('open');
	let current = $state<Issue | null>(null);
	let comments = $state<Comment[]>([]);
	let detailOpen = $state(false);
	let reply = $state('');
	let busy = $state(false);
	let removeOpen = $state(false);
	const manage = $derived(can('manageIssues'));

	async function load() {
		try {
			list = (await api<{ issues: Issue[] }>('/issues')).issues;
			// ?open=<id> (from a title's admin panel) opens that issue.
			const want = Number(new URLSearchParams(location.search).get('open'));
			if (want) {
				const hit = list.find((i) => i.id === want);
				if (hit) openIssue(hit);
			}
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not load issues.', 'orange');
		}
	}
	$effect(() => {
		document.title = 'Flixarr · Issues';
		load();
	});

	const shown = $derived(list ? (filter === 'all' ? list : list.filter((i) => i.status === filter)) : []);
	const count = (f: 'open' | 'resolved') => list?.filter((i) => i.status === f).length ?? 0;
	const TONE: Record<string, string> = { Video: 'text-blue-400', Audio: 'text-primary-light', Subtitles: 'text-yellow-400', Other: 'text-gray-300' };
	const where = (i: Issue) => {
		if (i.type !== 'series') return '';
		const p = i.parts?.length ? i.parts : i.season ? [[i.season, i.episode] as [number, number]] : [];
		const t = p.map(([se, e]) => (e ? `S${se}E${e}` : `Season ${se}`));
		return t.length > 4 ? `${t.slice(0, 4).join(', ')} +${t.length - 4} more` : t.join(', ');
	};
	const when = (t: string) => fmtDate(t, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' });
	// Avatar wants a User; comments and issues carry just the name and picture.
	const person = (name: string, avatar: string) => ({ username: name, avatar }) as User;

	async function openIssue(i: Issue) {
		current = i;
		comments = [];
		reply = '';
		detailOpen = true;
		try {
			const d = await api<{ issue: Issue; comments: Comment[] }>(`/issues/${i.id}`);
			current = d.issue;
			comments = d.comments;
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not load it.', 'orange');
		}
	}
	async function comment() {
		busy = true;
		try {
			const d = await api<{ issue: Issue; comments: Comment[] }>(`/issues/${current!.id}/comments`, { method: 'POST', body: JSON.stringify({ body: reply }) });
			current = d.issue;
			comments = d.comments;
			reply = '';
			list = list!.map((x) => (x.id === d.issue.id ? d.issue : x));
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not comment.', 'orange');
		} finally {
			busy = false;
		}
	}
	async function setStatus(what: 'resolve' | 'reopen') {
		busy = true;
		try {
			if (reply.trim()) await comment();
			const is = await api<Issue>(`/issues/${current!.id}/${what}`, { method: 'POST' });
			list = list!.map((x) => (x.id === is.id ? is : x));
			detailOpen = false;
			notify(what === 'resolve' ? `Resolved.${is.userId !== session.me?.id ? ` ${is.userName} gets a notification if notifications are set up.` : ''}` : 'Reopened.', what === 'resolve' ? 'green' : 'orange');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not change it.', 'orange');
		} finally {
			busy = false;
		}
	}
	async function remove() {
		const id = current!.id;
		removeOpen = detailOpen = false;
		try {
			await api(`/issues/${id}`, { method: 'DELETE' });
			list = list!.filter((x) => x.id !== id);
			notify('Issue removed.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not remove it.', 'orange');
		}
	}
</script>

<main class="mx-auto max-w-[1600px] px-4 pb-28 pt-20 laptop:px-10 laptop:pt-24">
	<h1 class="text-2xl font-bold laptop:text-3xl">{manage ? 'Issues' : 'My issues'}</h1>
	<p class="text-sm text-gray-400">{manage ? 'Problems reported on titles in your library.' : "Problems you've reported. Report one from a title's page with Report issue."}</p>

	<div class="mt-5 flex gap-2 overflow-x-auto pb-1 text-sm">
		{#each [['open', 'Open', count('open')], ['resolved', 'Resolved', count('resolved')], ['all', 'All', list?.length ?? 0]] as [k, l, n] (k)}
			<button class="shrink-0 rounded-full border px-3 py-1 {filter === k ? 'border-primary bg-primary/15 text-white' : 'border-gray-700 text-gray-300 hover:text-white'}" onclick={() => (filter = k as typeof filter)}>{l} <span class="text-gray-500">{n}</span></button>
		{/each}
	</div>

	{#if !list}
		<p class="mt-8 text-sm text-gray-400">Loading…</p>
	{:else if !shown.length}
		<div class="card mt-6 p-8 text-center text-sm text-gray-400">{filter === 'open' ? 'No open issues.' : 'Nothing here.'}</div>
	{:else}
		<div class="mt-5 grid gap-3 laptop:grid-cols-2">
			{#each shown as i (i.id)}
				<button class="card flex gap-3 p-3 text-left transition hover:ring-1 hover:ring-primary/50" onclick={() => openIssue(i)}>
					<div class="w-14 shrink-0 overflow-hidden rounded-lg ring-1 ring-gray-700/60">{#if i.poster}<Img path={i.poster} kind="poster" sizes="56px" class="aspect-[2/3] w-full" />{/if}</div>
					<div class="min-w-0 flex-1 space-y-1">
						<div class="flex items-start justify-between gap-2">
							<div class="min-w-0"><div class="truncate font-semibold">{i.title}</div><div class="text-xs text-gray-400">{where(i) ? where(i) + ' · ' : ''}#{i.id}</div></div>
							<span class="shrink-0 rounded-md px-1.5 py-0.5 text-[11px] ring-1 {i.status === 'open' ? 'bg-yellow-500/15 text-yellow-400 ring-yellow-500/40' : 'bg-green-500/15 text-green-400 ring-green-500/40'}">{i.status === 'open' ? 'Open' : 'Resolved'}</span>
						</div>
						<div class="text-sm"><span class="font-medium {TONE[i.kind]}">{i.kind}</span>{#if i.sub}<span class="text-gray-400"> · {i.sub}</span>{/if}</div>
						{#if i.note}<p class="line-clamp-1 text-xs text-gray-400">{i.note}</p>{/if}
						<div class="flex items-center gap-3 pt-1 text-xs text-gray-500">
							<span class="flex items-center gap-1.5 text-gray-300"><Avatar user={person(i.userName, i.userAvatar)} class="size-4 text-[9px]" />{i.userName}</span>
							<span>{when(i.createdAt)}</span>
							{#if i.comments}<span>{i.comments} comment{i.comments === 1 ? '' : 's'}</span>{/if}
						</div>
					</div>
				</button>
			{/each}
		</div>
	{/if}
</main>

<Modal bind:open={detailOpen} title={current?.title ?? ''} sub={current ? `${current.kind}${current.sub ? ' · ' + current.sub : ''}${where(current) ? ' · ' + where(current) : ''} · #${current.id}` : ''}>
	{#if current}
		<div class="space-y-4 p-5">
			<a href="/{current.type}/{current.tmdbId}" class="inline-flex items-center gap-1 text-xs text-gray-400 hover:text-white">Open the {current.type === 'movie' ? 'movie' : 'series'}<Icon name="chevron" class="size-3.5" /></a>
			<div class="flex gap-3">
				<Avatar user={person(current.userName, current.userAvatar)} class="size-8 text-xs" />
				<div class="card flex-1 p-3 text-sm"><div class="mb-1 text-xs text-gray-400">{current.userName} · {when(current.createdAt)}</div>{current.note || 'No details given.'}</div>
			</div>
			{#each comments as c (c.id)}
				<div class="flex gap-3">
					<Avatar user={person(c.userName, c.userAvatar)} class="size-8 text-xs" />
					<div class="card flex-1 p-3 text-sm"><div class="mb-1 text-xs text-gray-400">{c.userName} · {when(c.createdAt)}</div><p class="whitespace-pre-line">{c.body}</p></div>
				</div>
			{/each}
			<textarea rows="2" maxlength="2000" bind:value={reply} class="w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60" placeholder="Add a comment…"></textarea>
		</div>
	{/if}
	{#snippet footer()}
		{#if manage}<button class="btn-ghost mr-auto text-red-300" disabled={busy} onclick={() => (removeOpen = true)}>Remove</button>{/if}
		<button class="btn-ghost" disabled={busy || !reply.trim()} onclick={comment}>Comment</button>
		{#if manage && current}
			<button class="btn px-5" disabled={busy} onclick={() => setStatus(current!.status === 'open' ? 'resolve' : 'reopen')}>{#if current.status === 'open'}<Icon name="check" class="size-4" />Resolve{:else}Reopen{/if}</button>
		{/if}
	{/snippet}
</Modal>

<Modal bind:open={removeOpen} title="Remove this issue?" sub={current?.title}>
	<p class="p-5 text-sm text-gray-300">The issue and its comments are deleted. Use Resolve instead if it was fixed, so the reporter hears about it.</p>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (removeOpen = false)}>Keep it</button>
		<button class="btn px-6" onclick={remove}>Remove</button>
	{/snippet}
</Modal>
