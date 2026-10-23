<script lang="ts">
	// Seasons as headers with their episodes underneath; pick any mix across seasons. The header box picks
	// the whole season: every episode (requests), or the season as a whole, [season, 0] (issue reports).
	import { meta } from '#lib/api.ts';
	import type { Series, Season } from '#lib/types.ts';
	import Icon from './Icon.svelte';

	let { series, picked = $bindable([]), wholeSeason = false, done = {}, only = null }: {
		series: Series; picked?: [number, number][]; wholeSeason?: boolean; done?: Record<number, number>; // done: episodes in Plex per season
		only?: Record<number, number[]> | null; // only these episodes are offered (what's in Plex, for issue reports)
	} = $props();

	let open = $state<Record<number, boolean>>({});
	let eps = $state<Record<number, Season['episodes'] | 'loading'>>({});
	const seasons = $derived(series.seasons.filter((s) => s.n > 0 && (!only || only[s.n]?.length)));
	const total = (s: { n: number; episodes: number }) => (only ? (only[s.n]?.length ?? 0) : s.episodes);

	async function load(n: number) {
		if (eps[n]) return;
		eps[n] = 'loading';
		const s = await meta<Season>(`series/${series.id}/season/${n}`);
		eps[n] = only ? s.episodes.filter((e) => only[n]?.includes(e.n)) : s.episodes;
	}
	function expand(n: number) {
		open[n] = !open[n];
		if (open[n]) load(n);
	}
	const has = (s: number, e: number) => picked.some(([a, b]) => a === s && b === e);
	const countIn = (n: number) => picked.filter(([a, b]) => a === n && b > 0).length;
	const seasonState = (n: number, total: number) => (wholeSeason ? (has(n, 0) ? 'all' : countIn(n) ? 'some' : 'none') : countIn(n) && countIn(n) >= total ? 'all' : countIn(n) ? 'some' : 'none');

	async function toggleSeason(n: number, total: number) {
		const on = seasonState(n, total) !== 'all';
		const rest = picked.filter(([a]) => a !== n);
		if (wholeSeason) {
			picked = on ? [...rest, [n, 0]] : rest;
			return;
		}
		if (!on) {
			picked = rest;
			return;
		}
		await load(n);
		const list = eps[n];
		picked = [...rest, ...(Array.isArray(list) ? list.map((e) => [n, e.n] as [number, number]) : [])];
	}
	function toggleEp(n: number, e: number) {
		picked = has(n, e) ? picked.filter(([a, b]) => !(a === n && b === e)) : [...picked.filter(([a, b]) => !(a === n && b === 0)), [n, e]];
	}
</script>

<div class="max-h-[45dvh] overflow-y-auto rounded-lg border border-gray-700/50 bg-gray-900/40">
	{#each seasons as s (s.n)}
		{@const st = seasonState(s.n, total(s))}
		<div class="border-b border-gray-700/40 last:border-0">
			<div class="sticky top-0 z-10 flex items-center gap-3 bg-gray-900/95 px-3 py-2 text-sm backdrop-blur">
				<input type="checkbox" class="accent-[var(--color-primary)]" aria-label="All of {s.name}" checked={st === 'all'} indeterminate={st === 'some'} onchange={() => toggleSeason(s.n, total(s))} />
				<button class="flex flex-1 items-center gap-2 text-left" onclick={() => expand(s.n)}>
					<span class="flex-1 font-medium">{s.name}</span>
					<span class="text-xs text-gray-500">{#if only}{total(s)} in Plex{:else if done[s.n]}{done[s.n] >= s.episodes ? 'In Plex' : `${done[s.n]} of ${s.episodes} in Plex`}{:else}{s.episodes} episodes{/if}{countIn(s.n) ? ` · ${countIn(s.n)} picked` : ''}</span>
					<span class="text-gray-500 transition {open[s.n] ? 'rotate-90' : ''}"><Icon name="chevron" class="size-4" /></span>
				</button>
			</div>
			{#if open[s.n]}
				{#if eps[s.n] === 'loading' || !eps[s.n]}
					<p class="px-10 py-2 text-xs text-gray-500">Loading episodes…</p>
				{:else}
					{#each eps[s.n] as Season['episodes'] as e (e.n)}
						<label class="flex cursor-pointer items-center gap-3 py-1.5 pl-10 pr-3 text-sm hover:bg-white/5 {wholeSeason && has(s.n, 0) ? 'opacity-50' : ''}">
							<input type="checkbox" class="accent-[var(--color-primary)]" disabled={wholeSeason && has(s.n, 0)} checked={has(s.n, e.n) || (wholeSeason && has(s.n, 0))} onchange={() => toggleEp(s.n, e.n)} />
							<span class="w-8 shrink-0 text-xs text-gray-500">E{e.n}</span><span class="min-w-0 flex-1 truncate">{e.name}</span>
						</label>
					{/each}
				{/if}
			{/if}
		</div>
	{/each}
</div>
