<script lang="ts">
	// Report an issue on a title in the library: what's wrong (and the detail), for series which seasons
	// and episodes (any mix; nothing picked = the whole show), and an optional note.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { Movie, Series } from '#lib/types.ts';
	import Modal from './Modal.svelte';
	import EpisodePicker from './EpisodePicker.svelte';

	// waiting: someone requested it and it hasn't (all) arrived, so "Didn't download" can be reported.
	// inLibrary: it's in Plex, so problems with the files can be.
	let { open = $bindable(false), item, inLibrary = true, waiting = false }: { open?: boolean; item: Movie | Series; inLibrary?: boolean; waiting?: boolean } = $props();

	const DIDNT = "Didn't download";
	const ALL: Record<string, string[]> = {
		Video: ['Bad quality', 'Wrong file', "Won't play"],
		Audio: ['Too quiet', 'Out of sync', 'Wrong language', 'No audio'],
		Subtitles: ['Missing', 'Out of sync', 'Wrong language'],
		[DIDNT]: ['Never started', 'Got stuck'],
		Other: []
	};
	const KINDS = $derived(Object.fromEntries(Object.entries(ALL).filter(([k]) => (k === DIDNT ? waiting : inLibrary))));
	// Problems with files: only the seasons and episodes that are in Plex are offered.
	let inPlex = $state<Record<number, number[]> | null>(null);
	let kind = $state('Video');
	let sub = $state('Bad quality');
	let parts = $state<[number, number][]>([]);
	let note = $state('');
	let sending = $state(false);

	const series = $derived(item.type === 'series' ? (item as Series) : null);

	$effect(() => {
		if (open) {
			pickKind(inLibrary ? 'Video' : DIDNT);
			parts = [];
			note = '';
			if (item.type === 'series' && inLibrary && !inPlex) api<Record<number, number[]>>(`/plex/episodes/${item.id}`).then((d) => (inPlex = d)).catch(() => {});
		}
	});
	function pickKind(k: string) {
		kind = k;
		sub = ALL[k][0] ?? '';
	}

	async function send() {
		sending = true;
		try {
			await api('/issues', { method: 'POST', body: JSON.stringify({ type: item.type, tmdbId: item.id, title: item.title, year: item.year, poster: item.poster ?? '', kind, sub, parts, note }) });
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
			<div class="grid grid-cols-2 gap-2 {Object.keys(KINDS).length > 4 ? 'tablet:grid-cols-3' : 'tablet:grid-cols-4'}" role="radiogroup" aria-label="What's wrong">
				{#each Object.keys(KINDS) as k (k)}
					<button class="rounded-xl border px-3 py-2.5 text-sm {k === kind ? 'border-primary/70 bg-primary/10 text-white' : 'border-gray-700/50 bg-gray-800/40 text-gray-300 hover:text-white'}" role="radio" aria-checked={k === kind} onclick={() => pickKind(k)}>{k}</button>
				{/each}
			</div>
			{#if KINDS[kind]?.length}
				<div class="flex flex-wrap gap-1.5 pt-1">
					{#each KINDS[kind] ?? [] as s (s)}
						<button class="rounded-full border px-3 py-1 text-xs {s === sub ? 'border-primary/70 bg-primary/10 text-white' : 'border-gray-700/60 text-gray-400 hover:text-white'}" onclick={() => (sub = s)}>{s}</button>
					{/each}
				</div>
			{/if}
		</div>
		{#if series}
			<div>
				<div class="mb-1.5 text-sm font-medium">Where? <span class="font-normal text-gray-500">{parts.length ? `${parts.length} picked` : 'Nothing picked: the whole show'}</span></div>
				{#key kind === DIDNT}<EpisodePicker {series} bind:picked={parts} wholeSeason only={kind === DIDNT || !inPlex || !Object.keys(inPlex).length ? null : inPlex} />{/key}
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
