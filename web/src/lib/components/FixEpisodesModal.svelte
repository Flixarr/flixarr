<script lang="ts">
	// Fix episodes: pick seasons or single episodes. Missing ones are searched for again; ones with a file
	// are redownloaded (the file deleted and the release it came from blocked). For people with the Fix and
	// redownload permission.
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { Series } from '#lib/types.ts';
	import Modal from './Modal.svelte';
	import Icon from './Icon.svelte';

	type Ep = { season: number; episode: number; title: string; aired: boolean; hasFile: boolean; inPlex: boolean };
	let { open = $bindable(false), item, ondone }: { open?: boolean; item: Series; ondone?: () => void } = $props();
	let eps = $state<Ep[] | null>(null);
	let error = $state('');
	let picked = $state<Set<string>>(new Set());
	let shown = $state<Record<number, boolean>>({});
	let busy = $state(false);

	$effect(() => {
		if (!open) return;
		eps = null;
		error = '';
		picked = new Set();
		shown = {};
		api<{ episodes: Ep[] }>(`/titles/series/${item.id}/episodes`)
			.then((d) => {
				eps = d.episodes;
				// Seasons with something missing start open, so the gaps are in view.
				for (const s of seasons) if (s.missing) shown[s.n] = true;
			})
			.catch((e) => (error = e instanceof Error ? e.message : 'Could not load the episodes.'));
	});

	const k = (e: Ep) => `${e.season}:${e.episode}`;
	// Missing: aired, and no file. Not aired yet can't be fixed.
	const missing = (e: Ep) => e.aired && !e.hasFile;
	const fixable = (e: Ep) => e.aired || e.hasFile;
	const seasons = $derived.by(() => {
		const by = new Map<number, Ep[]>();
		for (const e of eps ?? []) by.set(e.season, [...(by.get(e.season) ?? []), e]);
		return [...by].map(([n, list]) => ({ n, list, have: list.filter((e) => e.hasFile).length, aired: list.filter((e) => e.aired).length, missing: list.filter(missing).length }));
	});
	const allMissing = $derived((eps ?? []).filter(missing));
	const picks = $derived((eps ?? []).filter((e) => picked.has(k(e))));
	const redo = $derived(picks.filter((e) => e.hasFile).length);
	const search = $derived(picks.length - redo);

	function toggle(e: Ep) {
		const next = new Set(picked);
		next.has(k(e)) ? next.delete(k(e)) : next.add(k(e));
		picked = next;
	}
	function toggleSeason(list: Ep[]) {
		const can = list.filter(fixable);
		const all = can.every((e) => picked.has(k(e)));
		const next = new Set(picked);
		for (const e of can) all ? next.delete(k(e)) : next.add(k(e));
		picked = next;
	}
	const seasonState = (list: Ep[]) => {
		const can = list.filter(fixable);
		const n = can.filter((e) => picked.has(k(e))).length;
		return n === 0 ? 'none' : n === can.length ? 'all' : 'some';
	};
	function pickMissing() {
		picked = new Set(allMissing.map(k));
		for (const s of seasons) if (s.missing) shown[s.n] = true;
	}

	async function go() {
		busy = true;
		try {
			const d = await api<{ done: string }>(`/titles/series/${item.id}/episodes/fix`, {
				method: 'POST',
				body: JSON.stringify({ episodes: picks.map((e) => [e.season, e.episode]), title: item.title, year: item.year, poster: item.poster ?? '' })
			});
			notify(d.done);
			open = false;
			ondone?.();
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not fix them.', 'orange');
		} finally {
			busy = false;
		}
	}
	const label = $derived(!picks.length ? 'Pick episodes' : [search ? `Search for ${search}` : '', redo ? `redownload ${redo}` : ''].filter(Boolean).join(', ').replace(/^r/, 'R'));
</script>

<Modal bind:open title="Fix episodes" sub={item.title} wide>
	<div class="space-y-4 p-5 text-sm">
		<p class="text-gray-400">Pick what's missing or bad. Missing episodes are searched for again. Episodes with a file are redownloaded: the file is deleted and the release it came from is blocked, so it isn't downloaded again.</p>
		{#if error}
			<p class="rounded-lg bg-red-500/10 px-3 py-2 text-red-200 ring-1 ring-red-500/30">{error}</p>
		{:else if !eps}
			<p class="text-gray-400">Loading episodes…</p>
		{:else}
			{#if allMissing.length}
				<button class="btn-ghost w-full justify-center py-2 text-sm" onclick={pickMissing}>Pick the {allMissing.length} missing episode{allMissing.length === 1 ? '' : 's'}</button>
			{/if}
			<div class="max-h-[50dvh] overflow-y-auto rounded-xl border border-gray-700/50 bg-gray-900/40">
				{#each seasons as s (s.n)}
					{@const st = seasonState(s.list)}
					<div class="border-b border-gray-700/40 last:border-0">
						<div class="sticky top-0 z-10 flex items-center gap-3 bg-gray-900/95 px-3 py-2.5 backdrop-blur">
							<input type="checkbox" class="size-4 shrink-0 accent-[var(--color-primary)]" aria-label="All of season {s.n}" checked={st === 'all'} indeterminate={st === 'some'} onchange={() => toggleSeason(s.list)} />
							<button class="flex min-w-0 flex-1 items-center gap-2 text-left" onclick={() => (shown[s.n] = !shown[s.n])}>
								<span class="flex-1 font-medium">Season {s.n}</span>
								<span class="shrink-0 text-xs {s.missing ? 'text-yellow-300' : 'text-gray-500'}">{s.missing ? `${s.missing} missing` : `${s.have} of ${s.aired}`}</span>
								<Icon name="chevron" class="size-4 shrink-0 text-gray-500 transition {shown[s.n] ? 'rotate-90' : ''}" />
							</button>
						</div>
						{#if shown[s.n]}
							{#each s.list as e (e.episode)}
								<label class="flex items-center gap-3 py-2 pl-10 pr-3 {fixable(e) ? 'cursor-pointer hover:bg-white/5' : 'opacity-50'}">
									<input type="checkbox" class="size-4 shrink-0 accent-[var(--color-primary)]" disabled={!fixable(e)} checked={picked.has(k(e))} onchange={() => toggle(e)} />
									<span class="w-9 shrink-0 text-xs tabular-nums text-gray-500">E{e.episode}</span>
									<span class="min-w-0 flex-1 truncate">{e.title || `Episode ${e.episode}`}</span>
									<span class="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium {!e.aired ? 'bg-gray-800 text-gray-400' : e.hasFile ? (e.inPlex ? 'bg-green-500/15 text-green-300' : 'bg-blue-500/15 text-blue-300') : 'bg-yellow-500/15 text-yellow-300'}">{!e.aired ? 'Not out' : e.hasFile ? (e.inPlex ? 'In Plex' : 'Downloaded') : 'Missing'}</span>
								</label>
							{/each}
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	</div>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (open = false)}>Cancel</button>
		<button class="btn px-5" disabled={busy || !picks.length} onclick={go}>{busy ? 'Working…' : label}</button>
	{/snippet}
</Modal>
