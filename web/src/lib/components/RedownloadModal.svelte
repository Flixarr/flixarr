<script lang="ts">
	// Redownload (needs the Redownload permission): deletes a bad file and gets a new copy. Radarr/Sonarr
	// blocklist the release it came from, so the same bad copy isn't grabbed again. Series: per season.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { Movie, Series } from '#lib/types.ts';
	import Modal from './Modal.svelte';

	let { open = $bindable(false), item, seasons: preset = [] }: { open?: boolean; item: Movie | Series; seasons?: number[] } = $props();

	const series = $derived(item.type === 'series' ? (item as Series) : null);
	let inPlex = $state<Record<number, number>>({});
	let picked = $state<number[]>([]);
	let sending = $state(false);
	const list = $derived(series ? series.seasons.filter((s) => s.n > 0 && (inPlex[s.n] ?? 0) > 0) : []);

	$effect(() => {
		if (!open) return;
		picked = [...preset];
		inPlex = {};
		if (item.type === 'series') api<Record<number, number>>(`/plex/seasons/${item.id}`).then((d) => (inPlex = d)).catch(() => {});
	});

	function toggle(n: number) {
		picked = picked.includes(n) ? picked.filter((x) => x !== n) : [...picked, n].sort((a, b) => a - b);
	}

	async function send() {
		sending = true;
		try {
			const d = await api<{ done: string }>('/redownload', { method: 'POST', body: JSON.stringify({ type: item.type, tmdbId: item.id, seasons: series ? picked : [] }) });
			open = false;
			notify(d.done);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not redownload it.', 'orange');
		} finally {
			sending = false;
		}
	}
	const ready = $derived(!series || picked.length > 0);
</script>

<Modal bind:open title="Redownload {item.title}">
	<div class="space-y-4 p-5 text-sm">
		<p class="text-gray-300">
			{series ? 'The files for the seasons you pick are deleted' : 'The current file is deleted'} and {series ? 'Sonarr looks for new copies' : 'Radarr looks for a new copy'}.
			The release {series ? 'they' : 'it'} came from won't be grabbed again. Until {series ? 'they arrive, those seasons are' : 'it arrives, it is'} not in Plex.
		</p>
		{#if series}
			{#if list.length}
				<div class="max-h-64 divide-y divide-gray-700/40 overflow-y-auto rounded-lg border border-gray-700/50 bg-gray-900/40">
					{#each list as s (s.n)}
						<label class="flex cursor-pointer items-center gap-3 px-3 py-2 hover:bg-white/5">
							<input type="checkbox" class="accent-[var(--color-primary)]" checked={picked.includes(s.n)} onchange={() => toggle(s.n)} />
							<span class="flex-1">{s.name}</span>
							<span class="text-xs text-gray-500">{inPlex[s.n]} episode{inPlex[s.n] === 1 ? '' : 's'} in Plex</span>
						</label>
					{/each}
				</div>
			{:else}
				<p class="text-gray-400">Loading seasons…</p>
			{/if}
		{/if}
	</div>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (open = false)}>Cancel</button>
		<button class="btn px-6" disabled={sending || !ready} onclick={send}>{sending ? 'Working…' : series ? (picked.length ? `Redownload ${picked.length} season${picked.length === 1 ? '' : 's'}` : 'Pick seasons') : 'Delete and redownload'}</button>
	{/snippet}
</Modal>
