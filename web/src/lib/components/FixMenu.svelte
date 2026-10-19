<script lang="ts">
	// One Fix button per title (#139), offering only what fits right now:
	//   a download stuck in Radarr's / Sonarr's queue → get a different release (suggested), try the import
	//     again, or remove it (#129);
	//   nothing downloaded and nothing downloading → search again (people who manage requests);
	//   a bad file already in the library → redownload, or for a series pick the episodes (Fix and redownload).
	// With a single choice that opens its own window (Redownload on a title page), the button goes straight there.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { can } from '#lib/session.svelte.ts';
	import Modal from './Modal.svelte';
	import Icon from './Icon.svelte';

	type Download = { queueId?: number; season?: number; episode?: number; problem?: string; detail?: string };
	type Live = { inArr: boolean; hasFile: boolean; queue: Download[]; server: string };
	let {
		title, series, live, search, redownload, ondone, class: cls = 'btn-ghost px-3 py-1.5 text-xs', label = 'Fix'
	}: {
		title: string; series: boolean; live?: Live; class?: string; label?: string;
		search?: () => Promise<void>; // search again (the caller's own endpoint)
		redownload?: () => void; // opens Redownload / Fix episodes
		ondone?: () => void;
	} = $props();

	const kind = $derived(series ? 'sonarr' : 'radarr');
	const app = $derived(series ? 'Sonarr' : 'Radarr');
	const stuck = $derived((live?.queue ?? []).filter((q) => q.queueId));
	const canSearch = $derived(!!search && !!live && live.inArr && !live.hasFile && !live.queue.length);
	const canRedo = $derived(!!redownload && can('redownload') && (series || !live || live.hasFile));
	const only = $derived(!stuck.length && !canSearch && canRedo);
	const shown = $derived(stuck.length > 0 || canSearch || canRedo);

	let open = $state(false);
	let busy = $state('');
	const name = (q: Download) => (q.season ? `${title} S${q.season}${q.episode ? 'E' + q.episode : ''}` : title);
	const CHOICES = $derived([
		{ action: 'different', label: 'Get a different release', about: `Removes this one from ${app} and the download client, blocks it so it isn't grabbed again, and searches for another.`, suggested: true },
		{ action: 'retry', label: 'Try the import again', about: `${app} looks at the downloaded files again. For when they weren't ready yet.` },
		{ action: 'remove', label: 'Remove it', about: `Removes it from ${app} and the download client without searching again.` }
	]);

	function start() {
		if (only) redownload!();
		else open = true;
	}
	async function queueFix(q: Download, action: string) {
		busy = `${q.queueId}:${action}`;
		try {
			const r = await api<{ done: string }>(`/queue/${kind}/${q.queueId}/fix`, { method: 'POST', body: JSON.stringify({ action }) });
			notify(`${r.done}.`, 'green');
			open = false;
			ondone?.();
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not fix it.', 'orange');
		} finally {
			busy = '';
		}
	}
	async function searchAgain() {
		busy = 'search';
		try {
			await search!();
			open = false;
		} finally {
			busy = '';
		}
	}
</script>

{#snippet section(t: string)}<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">{t}</div>{/snippet}

{#if shown}
	<button class={cls} onclick={start}><Icon name="refresh" class="size-3.5" />{label}</button>

	<Modal bind:open title="Fix" sub={title}>
		<div class="space-y-6 p-5">
			{#each stuck as q (q.queueId)}
				<div>
					{@render section(stuck.length > 1 ? `Stuck download: ${name(q).slice(title.length).trim() || title}` : 'Stuck download')}
					{#if q.detail || q.problem}<p class="mb-2 text-xs text-yellow-200">{q.detail || q.problem}</p>{/if}
					<div class="space-y-2">
						{#each CHOICES as c (c.action)}
							<button class="w-full rounded-xl px-4 py-3 text-left ring-1 transition disabled:opacity-60 {c.suggested ? 'bg-primary/10 ring-primary/50 hover:bg-primary/15' : 'ring-gray-700/60 hover:bg-white/5'}" disabled={!!busy} onclick={() => queueFix(q, c.action)}>
								<span class="flex items-center justify-between gap-2 text-sm font-medium">{c.label}{#if busy === `${q.queueId}:${c.action}`}<span class="text-[11px] font-normal text-gray-400">Working…</span>{:else if c.suggested}<span class="text-[11px] font-normal text-primary-light">Suggested</span>{/if}</span>
								<span class="mt-0.5 block text-xs text-gray-400">{c.about}</span>
							</button>
						{/each}
					</div>
				</div>
			{/each}
			{#if canSearch}
				<div>
					{@render section('Nothing downloading')}
					<button class="w-full rounded-xl px-4 py-3 text-left ring-1 ring-gray-700/60 transition hover:bg-white/5 disabled:opacity-60" disabled={!!busy} onclick={searchAgain}>
						<span class="flex items-center justify-between gap-2 text-sm font-medium">Search again{#if busy === 'search'}<span class="text-[11px] font-normal text-gray-400">Searching…</span>{/if}</span>
						<span class="mt-0.5 block text-xs text-gray-400">Asks {app} to look for a download now, and turns monitoring back on if it was off.</span>
					</button>
				</div>
			{/if}
			{#if canRedo}
				<div>
					{@render section(series ? 'Bad or missing episodes' : 'Bad file')}
					<button class="w-full rounded-xl px-4 py-3 text-left ring-1 ring-gray-700/60 transition hover:bg-white/5" onclick={() => ((open = false), redownload!())}>
						<span class="block text-sm font-medium">{series ? 'Fix episodes' : 'Redownload'}</span>
						<span class="mt-0.5 block text-xs text-gray-400">{series ? 'Pick seasons or single episodes. Missing ones are searched for again; ones with a file are deleted, their release blocked, and downloaded again.' : `Deletes the file from ${app}, blocks that release so it isn't downloaded again, and downloads a new one.`}</span>
					</button>
				</div>
			{/if}
		</div>
	</Modal>
{/if}
