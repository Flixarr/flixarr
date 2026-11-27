<script lang="ts">
	// Settings → Services → Downloads: one list across Radarr and Sonarr, stuck downloads first, each with Fix.
	// Fix all gets a different copy of every stuck one (after a confirm). Show all opens the whole queue in a
	// window with a tab per app, opening on the app with stuck downloads.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { on } from '#lib/live.svelte.ts';
	import type { QueueRow, ServiceKind } from '#lib/types.ts';
	import Alert from '#lib/components/Alert.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import QueueItem from './QueueItem.svelte';
	import DownloadFix from './DownloadFix.svelte';
	import SettingsSection from './SettingsSection.svelte';
	import { KINDS, isStuck, needsManualImport, type Services } from './services.ts';

	let { saved }: { saved: Services } = $props();

	const SHOWN = 6;
	type Item = { kind: ServiceKind; q: QueueRow };
	// Fixed downloads leave the list right away; the next read of the queue confirms it.
	let fixed = $state<Record<string, true>>({});
	let searching = $state(0);
	const keyOf = (i: Item) => `${i.kind}:${i.q.id}`;
	const apps = $derived(KINDS.filter((k) => saved[k.kind]?.status?.connected));
	const all = $derived(
		apps.flatMap((k) => saved[k.kind]!.status!.queue.map((q) => ({ kind: k.kind, q })))
			.filter((i) => !fixed[keyOf(i)])
			.sort((a, b) => Number(isStuck(b.q)) - Number(isStuck(a.q)))
	);
	const stuck = $derived(all.filter((i) => isStuck(i.q)));
	const fixable = $derived(stuck.filter((i) => !needsManualImport(i.q)));
	const kindOf = (k: ServiceKind) => KINDS.find((x) => x.kind === k)!;
	const webOf = (k: ServiceKind) => saved[k]!.webUrl || saved[k]!.url;

	function onfixed(kind: ServiceKind, id: number, action: 'different' | 'remove') {
		fixed = { ...fixed, [`${kind}:${id}`]: true };
		if (action === 'different') searching++;
	}

	// Fix all runs on the server in the background (#233): the window closes at once, a progress bar shows
	// here (also after leaving and coming back), and each download leaves the list as it's done.
	type Progress = { running: boolean; total: number; done: number; failed: number; fixed: string[] };
	let confirmAll = $state(false);
	let starting = $state(false);
	let progress = $state<Progress | null>(null);
	let finished = $state(false); // the last run ended while this page was open: show how it went
	function track(p: Progress) {
		if (progress?.running && !p.running) finished = true;
		progress = p;
		if (p.fixed.length) fixed = { ...fixed, ...Object.fromEntries(p.fixed.map((k) => [k, true as const])) };
	}
	$effect(() => {
		api<Progress>('/queue/fix-all').then((p) => p.running && track(p)).catch(() => {});
		return on('queuefix', (p: Progress) => track(p));
	});
	async function fixAll() {
		starting = true;
		try {
			finished = false;
			track(await api<Progress>('/queue/fix-all', { method: 'POST', body: JSON.stringify({ items: fixable.map((i) => ({ kind: i.kind, id: i.q.id })) }) }));
			confirmAll = false;
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Unable to start Fix all', 'orange');
		} finally {
			starting = false;
		}
	}
	const pct = $derived(progress?.total ? Math.round((progress.done / progress.total) * 100) : 0);

	// Show all: a tab per app.
	let showAll = $state(false);
	let tab = $state<ServiceKind>('radarr');
	function openAll() {
		tab = apps.find((k) => stuck.some((i) => i.kind === k.kind))?.kind ?? apps[0]?.kind ?? 'radarr';
		showAll = true;
	}
	const inTab = $derived(all.filter((i) => i.kind === tab));
	const tabStuck = $derived(inTab.filter((i) => isStuck(i.q)).length);
</script>

{#snippet fixFor(i: Item)}
	<DownloadFix q={i.q} kind={i.kind} name={kindOf(i.kind).name} web={webOf(i.kind)} onfixed={(id, a) => onfixed(i.kind, id, a)} />
{/snippet}
{#snippet item(i: Item, logo: boolean)}
	{#if isStuck(i.q)}
		<QueueItem q={i.q} kind={i.kind} color={kindOf(i.kind).color} {logo}>{#snippet action()}{@render fixFor(i)}{/snippet}</QueueItem>
	{:else}
		<QueueItem q={i.q} kind={i.kind} color={kindOf(i.kind).color} {logo} />
	{/if}
{/snippet}

<SettingsSection title="Downloads" id="svc-downloads">
	{#snippet right()}
		{#if all.length}
			{all.length} in the queue{all.length === 1 ? '' : 's'}{#if stuck.length}{' · '}<span class="text-yellow-400">{stuck.length} {stuck.length === 1 ? 'needs' : 'need'} attention</span>{/if}
			{#if fixable.length > 1 && !progress?.running}<button class="ml-2 inline-flex items-center rounded-md bg-yellow-500/15 px-2 py-0.5 text-[11px] font-medium text-yellow-200 ring-1 ring-yellow-500/40 hover:bg-yellow-500/25" onclick={() => (confirmAll = true)}>Fix all {fixable.length}</button>{/if}
		{/if}
	{/snippet}
	{#if progress?.running}
		<div class="mb-2 rounded-lg bg-gray-800/60 px-3 py-2.5 ring-1 ring-gray-700/50">
			<div class="flex items-baseline justify-between gap-3 text-xs"><span class="text-gray-200">Getting a different copy of {progress.total} download{progress.total === 1 ? '' : 's'}…</span><span class="shrink-0 tabular-nums text-gray-400">{progress.done} of {progress.total}</span></div>
			<div class="mt-2 h-1.5 overflow-hidden rounded-full bg-gray-700/60"><div class="h-full rounded-full bg-primary transition-all duration-500" style="width:{Math.max(pct, 2)}%"></div></div>
			<div class="mt-1.5 text-[11px] text-gray-500">Keeps going if you leave this page.{progress.failed ? ` ${progress.failed} couldn't be fixed so far.` : ''}</div>
		</div>
	{:else if finished && progress}
		<div class="mb-2 rounded-lg px-3 py-2 text-xs ring-1 {progress.failed ? 'bg-yellow-500/10 text-yellow-300 ring-yellow-500/30' : 'bg-green-500/10 text-green-300 ring-green-500/30'}">
			Fixed {progress.total - progress.failed} of {progress.total}.{progress.failed ? ` ${progress.failed} couldn't be fixed and ${progress.failed === 1 ? 'is' : 'are'} still on the list.` : ''} New copies are being searched for; they come back here once one is found.
		</div>
	{:else if searching}
		<Alert tone="blue" icon="refresh" class="mb-2">Looking for a different copy of {searching} download{searching === 1 ? '' : 's'}. They come back here once a new copy is found.</Alert>
	{/if}
	<div class="card px-4">
		{#if all.length}
			<div class="divide-y divide-gray-700/30">
				{#each all.slice(0, SHOWN) as i (keyOf(i))}{@render item(i, true)}{/each}
			</div>
			{#if all.length > SHOWN}<div class="py-2.5 text-xs text-gray-500">{all.length - SHOWN} more · <button class="text-primary-light hover:underline" onclick={openAll}>Show all</button></div>{/if}
		{:else}
			<div class="py-3 text-sm text-gray-500">Nothing downloading.</div>
		{/if}
	</div>
</SettingsSection>

<Modal bind:open={showAll} title="Downloads" wide>
	{#snippet top()}
		<div class="border-b border-gray-700/40 px-5 pt-4">
			<div class="pr-8 font-semibold">Downloads</div>
			<div class="text-xs text-gray-400">{inTab.length} in the {kindOf(tab).name} queue{#if tabStuck}{' · '}<span class="text-yellow-400">{tabStuck} {tabStuck === 1 ? 'needs' : 'need'} attention</span>{/if}</div>
			{#if apps.length > 1}
				<div class="-mb-px mt-3 flex gap-5" role="tablist">
					{#each apps as k (k.kind)}
						{@const n = all.filter((i) => i.kind === k.kind)}
						{@const s = n.filter((i) => isStuck(i.q)).length}
						<button class="flex items-center gap-2 border-b-2 pb-2.5 text-sm {tab === k.kind ? 'border-primary text-white' : 'border-transparent text-gray-400 hover:text-gray-200'}" role="tab" aria-selected={tab === k.kind} onclick={() => (tab = k.kind)}>
							<img src="/logos/{k.kind}.svg" alt="" class="size-4" />{k.name}<span class="rounded-full px-1.5 text-[11px] {s ? 'bg-yellow-500/15 text-yellow-300' : 'bg-gray-700/60 text-gray-300'}">{n.length}</span>
						</button>
					{/each}
				</div>
			{:else}<div class="h-4"></div>{/if}
		</div>
	{/snippet}
	<div class="px-5 pb-2">
		{#if inTab.length}
			<div class="divide-y divide-gray-700/30">{#each inTab as i (keyOf(i))}{@render item(i, false)}{/each}</div>
		{:else}
			<div class="py-6 text-center text-sm text-gray-500">Nothing downloading in {kindOf(tab).name}.</div>
		{/if}
	</div>
</Modal>

<Modal bind:open={confirmAll} title="Fix all {fixable.length} stuck downloads?">
	<p class="px-5 py-4 text-sm text-gray-300">Each one is removed, that copy is blocked, and Radarr or Sonarr search for a different one. Anything that needs a manual import stays on the list. It runs in the background; you can leave this page.</p>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (confirmAll = false)}>Cancel</button>
		<button class="btn px-6" disabled={starting} onclick={fixAll}>{starting ? 'Starting…' : `Fix all ${fixable.length}`}</button>
	{/snippet}
</Modal>
