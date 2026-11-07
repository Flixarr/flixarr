<script lang="ts" module>
	// The columns, shared with the page's header row. People who manage requests get a tick box and Quality.
	export const rowCols = (manager: boolean) =>
		manager ? 'laptop:grid-cols-[1rem_2.75rem_minmax(0,1fr)_12rem_9rem_11rem]' : 'laptop:grid-cols-[2.75rem_minmax(0,1fr)_12rem_11rem]';
</script>

<script lang="ts">
	// One request on the Requests page: the title (with the year, what was asked, and a line for a failure,
	// a decline, a release that isn't out yet, a stuck download or open issues), who asked and when, the
	// quality for people who manage requests, and where it is. Tapping it opens the request window.
	import { ago } from '#lib/api.ts';
	import type { IssueBrief, Request } from '#lib/types.ts';
	import { askedFor, NOT_RELEASED, waitingRelease } from '#lib/requeststatus.ts';
	import { issueWhat, issueWhere } from '#lib/issuetext.ts';
	import Img from './Img.svelte';
	import Icon from './Icon.svelte';
	import Check from './Check.svelte';
	import Person from './Person.svelte';
	import StatusPill from './StatusPill.svelte';

	let { r, issues = [], manager = false, picked = false, busy = false, onpick, onopen, onapprove, ondecline }: {
		r: Request; issues?: IssueBrief[]; manager?: boolean; picked?: boolean; busy?: boolean;
		onpick?: () => void; onopen: () => void; onapprove?: () => void; ondecline?: () => void;
	} = $props();

	const who = $derived((r.source === 'redownload' ? 'Redownload by ' : '') + (r.userName || 'Someone'));
	const asked = $derived(r.type === 'movie' ? 'Movie' : askedFor(r));
	const replies = $derived(issues.reduce((n, i) => n + i.comments, 0));
	const first = $derived(issues[0]);
	const moving = $derived((r.status === 'downloading' || r.status === 'partial') && r.progress > 0);
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div class="grid cursor-pointer items-center gap-3 border-b border-gray-800/80 px-3 py-2.5 transition last:border-0 hover:bg-gray-800/50 laptop:gap-4 laptop:px-4 {manager ? 'grid-cols-[1rem_2.75rem_minmax(0,1fr)_auto]' : 'grid-cols-[2.75rem_minmax(0,1fr)_auto]'} {rowCols(manager)} {picked ? 'bg-primary/10' : ''}" onclick={onopen}>
	{#if manager}<Check on={picked} label="Pick {r.title}" onchange={() => onpick?.()} />{/if}
	<div class="overflow-hidden rounded ring-1 ring-gray-700/60">{#if r.poster}<Img path={r.poster} kind="poster" sizes="48px" class="block aspect-[2/3] w-full" />{:else}<div class="aspect-[2/3] bg-gray-800"></div>{/if}</div>

	<div class="min-w-0">
		<div class="flex min-w-0 items-baseline gap-1.5"><span class="truncate text-sm font-semibold">{r.title}</span>{#if r.year}<span class="shrink-0 text-xs text-gray-500">{r.year}</span>{/if}</div>
		<div class="mt-0.5 truncate text-xs text-gray-400">{asked}<span class="laptop:hidden"> · {who} · {ago(r.createdAt)}</span></div>
		{#if r.status === 'failed' && r.error}<div class="mt-1 truncate text-xs text-red-300">{r.error}</div>{/if}
		{#if r.status === 'declined' && r.error}<div class="mt-1 truncate text-xs text-gray-400">Declined: {r.error}</div>{/if}
		{#if waitingRelease(r)}<div class="mt-1 truncate text-xs text-yellow-200">{NOT_RELEASED}</div>{/if}
		{#if r.trouble && r.status !== 'failed'}<div class="mt-1 truncate text-xs text-yellow-200">{manager ? 'The download is stuck. Open it to fix it.' : 'The download hit a problem. Someone who manages requests can sort it out.'}</div>{/if}
		{#if first}
			<div class="mt-1.5 flex min-w-0 items-center gap-1.5 text-xs text-amber-300">
				<Icon name="issues" class="size-3.5 shrink-0" />
				<span class="shrink-0 font-medium">{issues.length === 1 ? 'Open issue' : `${issues.length} open issues`}</span>
				<span class="truncate text-amber-200/70">{[issueWhat(first), issueWhere(first), first.note.trim() ? `"${first.note.trim()}"` : ''].filter(Boolean).join(' · ')}</span>
				{#if replies}<span class="flex shrink-0 items-center gap-0.5 text-gray-400"><Icon name="chat" class="size-3" />{replies}</span>{/if}
			</div>
		{/if}
	</div>

	<Person name={who} avatar={r.userAvatar} sub={ago(r.createdAt)} class="hidden laptop:flex" />
	{#if manager}<div class="hidden truncate text-xs text-gray-400 laptop:block">{r.profileName || 'Default'}</div>{/if}

	<div class="flex flex-col items-end gap-1.5 laptop:items-start">
		<StatusPill status={r.status} progress={r.progress} series={r.type === 'series'} />
		{#if moving}
			<div class="flex w-24 items-center gap-1.5 laptop:w-full"><div class="h-1 flex-1 overflow-hidden rounded-full bg-gray-700/70"><div class="h-full rounded-full bg-blue-400" style="width:{r.progress}%"></div></div>{#if r.type !== 'series'}<span class="text-[10px] tabular-nums text-blue-300">{r.progress}%</span>{/if}</div>
		{/if}
		{#if manager && r.status === 'pending'}
			<div class="flex gap-1.5">
				<button class="btn px-2.5 py-0.5 text-[11px]" disabled={busy} onclick={(e) => (e.stopPropagation(), onapprove?.())}>{busy ? 'Approving…' : 'Approve'}</button>
				<button class="btn-ghost px-2.5 py-0.5 text-[11px]" disabled={busy} onclick={(e) => (e.stopPropagation(), ondecline?.())}>Decline</button>
			</div>
		{/if}
	</div>
</div>
