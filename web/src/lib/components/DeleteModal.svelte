<script lang="ts">
	// Delete (admin panel): tick what goes. The title and its files are only ever deleted through Radarr /
	// Sonarr (Plex is asked to scan and drops it); Flixarr's data is its requests and issues. Only what the
	// title is actually in is offered, and everything starts ticked.
	import { api, fmtBytes } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { can } from '#lib/session.svelte.ts';
	import { remember } from '#lib/availability.svelte.ts';
	import type { Movie, Series } from '#lib/types.ts';
	import Modal from './Modal.svelte';

	type Title = { inArr: boolean; server: string; size: number; downloading: boolean; plex: boolean; kept: boolean; requests: number; issues: number };
	let { open = $bindable(false), item, t, ondone }: { open?: boolean; item: Movie | Series; t: Title; ondone?: () => void } = $props();

	const what = $derived(item.type === 'series' ? 'show' : 'movie');
	const arrName = $derived(item.type === 'series' ? 'Sonarr' : 'Radarr');
	const canFiles = $derived(t.inArr && can('redownload'));
	const hasData = $derived(t.requests + t.issues > 0);
	let files = $state(true);
	let data = $state(true);
	let busy = $state(false);
	$effect(() => {
		if (open) (files = canFiles), (data = hasData);
	});

	const count = (n: number, one: string) => `${n} ${one}${n === 1 ? '' : 's'}`;
	const dataText = $derived([t.requests ? count(t.requests, 'request') : '', t.issues ? count(t.issues, 'issue') : ''].filter(Boolean).join(' and '));
	const filesText = $derived(
		(t.size ? `${t.server} deletes it with its files (${fmtBytes(t.size)})` : t.downloading ? `${t.server} deletes it and stops the download` : `${t.server} deletes it (no files yet)`) +
			(t.plex ? '; Plex drops it after Flixarr asks it to scan.' : '.')
	);
	const label = $derived(!files && !data ? 'Nothing ticked' : files ? `Delete ${what}${data ? ' and clear data' : ''}` : 'Clear data');

	async function go() {
		busy = true;
		try {
			const d = await api<{ done: string }>(`/titles/${item.type}/${item.id}/delete`, { method: 'POST', body: JSON.stringify({ files, data }) });
			remember(`${item.type}:${item.id}`, null);
			notify(d.done);
			open = false;
			ondone?.();
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not delete it.', 'orange');
			ondone?.();
		} finally {
			busy = false;
		}
	}
</script>

{#snippet option(checked: boolean, set: (v: boolean) => void, title: string, sub: string)}
	<label class="flex cursor-pointer gap-3 rounded-lg border border-gray-700/50 bg-gray-900/40 p-3 hover:border-gray-600">
		<input type="checkbox" class="mt-0.5 size-4 shrink-0 accent-[var(--color-primary)]" {checked} onchange={(e) => set((e.currentTarget as HTMLInputElement).checked)} />
		<span><span class="block font-medium text-gray-100">{title}</span><span class="block text-xs text-gray-400">{sub}</span></span>
	</label>
{/snippet}

<Modal bind:open title="Delete {item.title}?" sub="Tick what should go.">
	<div class="space-y-3 p-5 text-sm">
		{#if !t.inArr && t.plex}<p class="rounded-lg bg-gray-800/60 px-3 py-2 text-xs text-gray-400">It isn't in {arrName}, so Flixarr can't delete its files. Delete it in Plex if you want it gone.</p>{/if}
		{#if canFiles}{@render option(files, (v) => (files = v), `Delete the ${what} and its files`, filesText)}{/if}
		{#if hasData}{@render option(data, (v) => (data = v), 'Clear its data in Flixarr', `${dataText}, with ${t.requests + t.issues > 1 ? 'their' : 'its'} history and comments.`)}{/if}
		{#if !canFiles && !hasData}<p class="text-gray-400">There's nothing Flixarr can delete for it.</p>{/if}
		{#if t.kept && files && canFiles}<p class="text-xs text-gray-400">Maintainerr keeps this {what}; that keep is let go too.</p>{/if}
		{#if !files && data && canFiles}<p class="rounded-lg bg-yellow-500/10 px-3 py-2 text-xs text-yellow-300 ring-1 ring-yellow-500/30">The {what} and its files stay in {t.server}{t.plex ? ' and Plex' : ''}; only Flixarr forgets its requests and issues.</p>{/if}
		<p class="text-xs text-gray-500">This can't be undone.</p>
	</div>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (open = false)}>Cancel</button>
		<button class="inline-flex items-center rounded-lg bg-red-600/90 px-4 py-2 font-medium text-white transition hover:bg-red-600 disabled:opacity-40" disabled={busy || (!files && !data)} onclick={go}>{busy ? 'Deleting…' : label}</button>
	{/snippet}
</Modal>
