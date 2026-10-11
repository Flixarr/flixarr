<script lang="ts">
	import { api, fmtBytes, fmtNum, fmtDate, prefs } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import type { CacheState } from '#lib/types.ts';
	import Toggle from '#lib/components/Toggle.svelte';
	import Modal from '#lib/components/Modal.svelte';

	const KINDS = { titles: ['Titles', 'Movies, series, seasons, collections'], people: ['People', 'Cast and crew pages'], lists: ['Lists', 'Discover rows, browse pages, genres'], searches: ['Searches', 'Search results'] };
	const PAGES = [[0, 'Automatic'], [60, '1 hour'], [360, '6 hours'], [720, '12 hours'], [1440, '1 day'], [4320, '3 days'], [10080, '7 days']] as const;
	const LISTS = [[15, '15 minutes'], [60, '1 hour'], [180, '3 hours'], [360, '6 hours'], [720, '12 hours'], [1440, '1 day']] as const;
	const SEARCHES = [[15, '15 minutes'], [60, '1 hour'], [180, '3 hours'], [360, '6 hours'], [1440, '1 day']] as const;
	const MAX = [1000, 5000, 10000, 25000, 50000, 100000];
	const DAYS = [7, 14, 30, 60, 90];

	let st = $state<CacheState | null>(null);
	let confirmAll = $state(false);
	let confirmImages = $state(false);
	let busy = $state('');

	const total = $derived(st ? st.kinds.reduce((a, k) => ({ count: a.count + k.count, bytes: a.bytes + k.bytes }), { count: 0, bytes: 0 }) : null);
	const lookups = $derived(st ? st.counters.hits + st.counters.misses : 0);

	$effect(() => {
		const load = () => api<CacheState>('/cache').then((d) => (st = d));
		load();
		const t = setInterval(load, 10000); // counts and hits stay current while the page is open
		return () => clearInterval(t);
	});
	const since = $derived(st ? fmtDate(st.counters.since, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' }) : '');

	async function save(change: (s: CacheState) => void, msg = 'Saved.') {
		if (!st) return;
		const next = $state.snapshot(st) as CacheState;
		change(next);
		try {
			st = await api<CacheState>('/cache/settings', { method: 'PUT', body: JSON.stringify({ ...next.settings, images: { enabled: next.images.enabled, days: next.images.days } }) });
			prefs.imageCache = st.images.enabled;
			if (session.status) session.status.imageCache = st.images.enabled;
			notify(msg);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not save.', 'orange');
		}
	}

	async function flush(kind: string, label: string) {
		busy = kind;
		try {
			st = await api<CacheState>('/cache/' + kind, { method: 'DELETE' });
			notify(`${label} cleared.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not clear.', 'orange');
		} finally {
			busy = '';
			confirmAll = false;
			confirmImages = false;
		}
	}
</script>

{#snippet heading(t: string)}<div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">{t}</div>{/snippet}
{#snippet pick(label: string, value: number, opts: readonly (readonly [number, string])[], set: (s: CacheState, v: number) => void)}
	<label class="flex items-center justify-between gap-4 py-3 text-sm">
		<span>{label}</span>
		<select class="rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-1.5 text-sm" {value} onchange={(e) => save((s) => set(s, +(e.currentTarget as HTMLSelectElement).value))}>
			{#each opts as [v, l] (v)}<option value={v}>{l}</option>{/each}
		</select>
	</label>
{/snippet}

{#if st && total}
	<div>
		{@render heading('Saved results')}
		<div class="card divide-y divide-gray-700/40 px-4">
			<div class="py-3">
				<Toggle checked={st.settings.enabled} label="Save results" about="Keeps answers from the Flixarr API in this app's database, so repeat lookups are instant and don't leave your server. Saved results survive restarts and updates."
					onchange={(on) => save((s) => (s.settings.enabled = on), on ? 'Results are saved again.' : "Results aren't saved. Every lookup goes to the Flixarr API.")} />
			</div>
			{#each st.kinds as k (k.kind)}
				<div class="flex items-center gap-3 py-3 text-sm">
					<span class="min-w-0 flex-1"><span class="block">{KINDS[k.kind][0]}</span><span class="block text-xs text-gray-500">{KINDS[k.kind][1]}</span></span>
					<span class="text-right"><span class="block tabular-nums">{fmtNum(k.count)}</span><span class="block text-xs text-gray-500">{fmtBytes(k.bytes)}</span></span>
					<button class="btn-ghost px-2.5 py-1 text-xs" disabled={!k.count || busy !== ''} onclick={() => flush(k.kind, KINDS[k.kind][0])}>Flush</button>
				</div>
			{/each}
			<div class="flex items-center gap-3 py-3 text-sm">
				<span class="flex-1 font-medium">Total</span>
				<span class="text-right"><span class="block font-medium tabular-nums">{fmtNum(total.count)}</span><span class="block text-xs text-gray-500">{fmtBytes(total.bytes)}</span></span>
				<button class="btn-ghost px-2.5 py-1 text-xs" disabled={!total.count || busy !== ''} onclick={() => (confirmAll = true)}>Flush all</button>
			</div>
			<div class="py-3 text-xs text-gray-400">
				{#if lookups}
					Since the app started ({since}): <span class="text-white">{fmtNum(st.counters.hits)}</span> answered from the cache, <span class="text-white">{fmtNum(st.counters.misses)}</span> fetched from the Flixarr API
					(<span class="text-green-400">{Math.round((st.counters.hits / lookups) * 100)}% from the cache</span>).
					{#if st.counters.stale}<span class="block pt-1 text-yellow-400">{fmtNum(st.counters.stale)} served from an expired copy while the Flixarr API was down.</span>{/if}
				{:else}
					No lookups since the app started ({since}){st.settings.enabled ? '' : ' with saving on'}.
				{/if}
			</div>
		</div>
	</div>

	<div>
		{@render heading('How long to keep')}
		<div class="card divide-y divide-gray-700/40 px-4">
			{@render pick('Movie, series and people pages', st.settings.pagesMinutes, PAGES, (s, v) => (s.settings.pagesMinutes = v))}
			{@render pick('Discover rows and lists', st.settings.listsMinutes, LISTS, (s, v) => (s.settings.listsMinutes = v))}
			{@render pick('Searches', st.settings.searchesMinutes, SEARCHES, (s, v) => (s.settings.searchesMinutes = v))}
			{@render pick('Size limit', st.settings.maxResults, MAX.map((n) => [n, fmtNum(n) + ' results'] as const), (s, v) => (s.settings.maxResults = v))}
		</div>
		<p class="mt-2 text-xs text-gray-500"><span class="text-gray-400">Automatic</span> keeps each page as long as it's likely to stay the same: 6 hours for airing series and new or upcoming movies, 1 day for titles still settling, 7 days for ended series and older movies, 3 days for people. When the limit is reached, the oldest results are dropped. Expired results are kept for a day so the app can still show them if the Flixarr API is down, then the "Clear expired cache" job removes them (Settings → Jobs).</p>
	</div>

	<div>
		{@render heading('Image caching')}
		<div class="card divide-y divide-gray-700/40 px-4">
			<div class="py-3">
				<Toggle checked={st.images.enabled} label="Cache images" about="Posters and backdrops load through your server and are saved on its disk, instead of each browser loading them from TMDB. TMDB then sees your server instead of each viewer."
					onchange={(on) => save((s) => (s.images.enabled = on), on ? 'Images are cached. New pages load them through your server.' : 'Images load from TMDB again.')} />
			</div>
			{@render pick('Remove images not viewed for', st.images.days, DAYS.map((d) => [d, d + ' days'] as const), (s, v) => (s.images.days = v))}
			<div class="flex items-center gap-3 py-3 text-sm">
				<span class="flex-1">Saved images</span>
				<span class="text-right"><span class="block tabular-nums">{fmtNum(st.images.count)}</span><span class="block text-xs text-gray-500">{fmtBytes(st.images.bytes)}</span></span>
				<button class="btn-ghost px-2.5 py-1 text-xs" disabled={!st.images.count || busy !== ''} onclick={() => (confirmImages = true)}>Flush</button>
			</div>
		</div>
	</div>

	<Modal bind:open={confirmAll} title="Flush all saved results?" sub="{fmtNum(total.count)} result{total.count === 1 ? '' : 's'} · {fmtBytes(total.bytes)}">
		<p class="p-5 text-sm text-gray-300">Every saved result is deleted. Pages load from the Flixarr API again until the cache fills back up. Saved images aren't touched.</p>
		{#snippet footer()}
			<button class="btn-ghost" onclick={() => (confirmAll = false)}>Cancel</button>
			<button class="btn px-6" disabled={busy !== ''} onclick={() => flush('all', 'All saved results')}>Flush all</button>
		{/snippet}
	</Modal>

	<Modal bind:open={confirmImages} title="Flush saved images?" sub="{fmtNum(st.images.count)} image{st.images.count === 1 ? '' : 's'} · {fmtBytes(st.images.bytes)}">
		<p class="p-5 text-sm text-gray-300">Every saved poster and backdrop is deleted from your server's disk. They download from TMDB again the next time someone views them{st.images.enabled ? '' : ' (if image caching is on)'}. Saved results aren't touched.</p>
		{#snippet footer()}
			<button class="btn-ghost" onclick={() => (confirmImages = false)}>Cancel</button>
			<button class="btn px-6" disabled={busy !== ''} onclick={() => flush('images', 'Saved images')}>Flush images</button>
		{/snippet}
	</Modal>
{/if}
