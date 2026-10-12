<script lang="ts">
	// Report an issue on a title in the library: what's wrong (and the detail), for series which season and
	// episode, and an optional note.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { Movie, Series } from '#lib/types.ts';
	import Modal from './Modal.svelte';
	import Select from './Select.svelte';

	let { open = $bindable(false), item }: { open?: boolean; item: Movie | Series } = $props();

	const KINDS: Record<string, string[]> = {
		Video: ['Bad quality', 'Wrong file', "Won't play"],
		Audio: ['Too quiet', 'Out of sync', 'Wrong language', 'No audio'],
		Subtitles: ['Missing', 'Out of sync', 'Wrong language'],
		Other: []
	};
	let kind = $state('Video');
	let sub = $state('Bad quality');
	let season = $state(0);
	let episode = $state(0);
	let note = $state('');
	let sending = $state(false);

	const series = $derived(item.type === 'series' ? (item as Series) : null);
	const seasonOpts = $derived([{ value: 0, label: 'All seasons' }, ...(series?.seasons ?? []).filter((s) => s.n > 0).map((s) => ({ value: s.n, label: s.name }))]);
	const eps = $derived(series?.seasons.find((s) => s.n === season)?.episodes ?? 0);
	const epOpts = $derived([{ value: 0, label: 'All episodes' }, ...Array.from({ length: eps }, (_, i) => ({ value: i + 1, label: `Episode ${i + 1}` }))]);

	$effect(() => {
		if (open) {
			kind = 'Video';
			sub = 'Bad quality';
			season = episode = 0;
			note = '';
		}
	});
	function pickKind(k: string) {
		kind = k;
		sub = KINDS[k][0] ?? '';
	}

	async function send() {
		sending = true;
		try {
			await api('/issues', { method: 'POST', body: JSON.stringify({ type: item.type, tmdbId: item.id, title: item.title, year: item.year, poster: item.poster ?? '', kind, sub, season, episode: season ? episode : 0, note }) });
			open = false;
			notify('Issue reported. Thanks, someone will look at it.');
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not report it.', 'orange');
		} finally {
			sending = false;
		}
	}
</script>

<Modal bind:open title="Report an issue" sub={item.title}>
	<div class="space-y-5 p-5">
		<div class="space-y-2">
			<div class="text-sm font-medium">What's wrong?</div>
			<div class="grid grid-cols-2 gap-2 tablet:grid-cols-4" role="radiogroup" aria-label="What's wrong">
				{#each Object.keys(KINDS) as k (k)}
					<button class="rounded-xl border px-3 py-2.5 text-sm {k === kind ? 'border-primary/70 bg-primary/10 text-white' : 'border-gray-700/50 bg-gray-800/40 text-gray-300 hover:text-white'}" role="radio" aria-checked={k === kind} onclick={() => pickKind(k)}>{k}</button>
				{/each}
			</div>
			{#if KINDS[kind].length}
				<div class="flex flex-wrap gap-1.5 pt-1">
					{#each KINDS[kind] as s (s)}
						<button class="rounded-full border px-3 py-1 text-xs {s === sub ? 'border-primary/70 bg-primary/10 text-white' : 'border-gray-700/60 text-gray-400 hover:text-white'}" onclick={() => (sub = s)}>{s}</button>
					{/each}
				</div>
			{/if}
		</div>
		{#if series}
			<div class="grid grid-cols-2 gap-3">
				<div>
					<label for="is-season" class="block text-sm font-medium">Season</label>
					<Select id="is-season" bind:value={season} options={seasonOpts} onchange={() => (episode = 0)} />
				</div>
				<div>
					<label for="is-ep" class="block text-sm font-medium">Episode</label>
					<Select id="is-ep" bind:value={episode} options={epOpts} disabled={!season} />
				</div>
			</div>
		{/if}
		<div>
			<label for="is-note" class="block text-sm font-medium">Details {#if kind !== 'Other'}<span class="font-normal text-gray-500">(optional)</span>{/if}</label>
			<textarea id="is-note" rows="3" maxlength="2000" bind:value={note} class="mt-1 w-full rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/60" placeholder="e.g. dialogue is too quiet starting around 20 minutes in"></textarea>
		</div>
	</div>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (open = false)}>Cancel</button>
		<button class="btn px-6" disabled={sending || (kind === 'Other' && !note.trim())} onclick={send}>{sending ? 'Sending…' : 'Submit report'}</button>
	{/snippet}
</Modal>
