<script lang="ts">
	import { api, fmtBytes, fmtNum, fmtDate, prefs } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import type { CacheState } from '#lib/types.ts';
	import Toggle from '#lib/components/Toggle.svelte';
	import Modal from '#lib/components/Modal.svelte';

	const KINDS = { titles: ['Titles', 'Movies, series, seasons, collections'], people: ['People', 'Cast and crew pages'], lists: ['Lists', 'Discover rows, browse pages, genres'], searches: ['Searches', 'Search results'] };
	const PAGES = [[60, '1 hour'], [360, '6 hours'], [720, '12 hours'], [1440, '1 day'], [4320, '3 days'], [10080, '7 days']] as const;
	const LISTS = [[15, '15 minutes'], [60, '1 hour'], [180, '3 hours'], [360, '6 hours'], [720, '12 hours'], [1440, '1 day']] as const;
	const SEARCHES = [[15, '15 minutes'], [60, '1 hour'], [180, '3 hours'], [360, '6 hours'], [1440, '1 day']] as const;
	const MAX = [1000, 5000, 10000, 25000, 50000, 100000];
	const DAYS = [7, 14, 30, 60, 90];

	type TimeKey = 'pagesMinutes' | 'listsMinutes' | 'searchesMinutes' | 'maxResults';

	let st = $state<CacheState | null>(null);
	let confirmAll = $state(false);
	let confirmImages = $state(false);
	let busy = $state('');

	const total = $derived(st ? st.kinds.reduce((a, k) => ({ count: a.count + k.count, bytes: a.bytes + k.bytes }), { count: st.images.count, bytes: st.images.bytes }) : null);
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
{#snippet pick(label: string, id: string, value: number, def: number, opts: readonly (readonly [number, string])[], set: (s: CacheState, v: number) => void, about = '', disabled = false)}
	<div class="flex items-center justify-between gap-4 py-3 text-sm {disabled ? 'opacity-50' : ''}">
		<label for={'cache-' + id}>{label}{#if about}<span class="mt-0.5 block text-xs text-gray-500">{about}</span>{/if}</label>
		<select id={'cache-' + id} class="rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-1.5 text-sm disabled:cursor-not-allowed" {value} {disabled} onchange={(e) => save((s) => set(s, +(e.currentTarget as HTMLSelectElement).value))}>
			{#each opts as [v, l] (v)}<option value={v}>{l}{v === def ? ' (default)' : ''}</option>{/each}
		</select>
	</div>
{/snippet}

{#snippet keep(label: string, key: TimeKey, opts: readonly (readonly [number, string])[], about = '')}
	{@render pick(label, key, st!.settings[key], st!.defaults[key], opts, (s, v) => (s.settings[key] = v), about, !st!.settings.enabled)}
{/snippet}

{#snippet stored(label: string, about: string, count: number, bytes: number, onflush: () => void)}
	<div class="flex items-center gap-3 py-3 text-sm">
		<span class="min-w-0 flex-1"><span class="block">{label}</span><span class="block text-xs text-gray-500">{about}</span></span>
		<span class="text-right"><span class="block tabular-nums">{fmtNum(count)}</span><span class="block text-xs text-gray-500">{fmtBytes(bytes)}</span></span>
		<button class="btn-ghost px-2.5 py-1 text-xs" disabled={!count || busy !== ''} onclick={onflush}>Flush</button>
	</div>
{/snippet}

{#if st && total}
	<div>
		{@render heading('API caching')}
		<div class="card divide-y divide-gray-700/40 px-4">
			<div class="py-3">
				<Toggle checked={st.settings.enabled} label="Cache API responses" about="Answers from the Flixarr API are saved in this app's database, so repeat lookups are instant and don't go back to the API. If the API is unreachable, expired results are shown for up to a day."
					onchange={(on) => save((s) => (s.settings.enabled = on), on ? 'API responses are cached again.' : "API responses aren't cached. Every lookup goes to the Flixarr API.")} />
			</div>
			<div class="py-3 {st.settings.enabled ? '' : 'opacity-50'}">
				<Toggle checked={st.settings.auto} disabled={!st.settings.enabled} label="Automatic keep times" about="Results are kept as long as the Flixarr API says they stay fresh — hours for new and airing titles, up to a week for older ones."
					onchange={(on) => save((s) => (s.settings.auto = on), on ? 'Keep times are automatic.' : 'Keep times are set by you.')} />
			</div>
			{#if !st.settings.auto}
				{@render keep('Movie, series and people pages', 'pagesMinutes', PAGES)}
				{@render keep('Discover rows and lists', 'listsMinutes', LISTS)}
				{@render keep('Searches', 'searchesMinutes', SEARCHES)}
			{/if}
			{@render keep('Size limit', 'maxResults', MAX.map((n) => [n, fmtNum(n) + ' results'] as const), 'Oldest results are removed first.')}
		</div>
	</div>

	<div>
		{@render heading('Image caching')}
		<div class="card divide-y divide-gray-700/40 px-4">
			<div class="py-3">
				<Toggle checked={st.images.enabled} label="Cache images" about="Posters and backdrops load through your server and are saved on its disk, instead of each browser loading them from TMDB. TMDB then sees your server instead of each viewer."
					onchange={(on) => save((s) => (s.images.enabled = on), on ? 'Images are cached. New pages load them through your server.' : 'Images load from TMDB again.')} />
			</div>
			{@render pick('Remove images not viewed for', 'images', st.images.days, st.images.defaultDays, DAYS.map((d) => [d, d + ' days'] as const), (s, v) => (s.images.days = v), '', !st.images.enabled)}
		</div>
	</div>

	<div>
		{@render heading('Storage')}
		<div class="card divide-y divide-gray-700/40 px-4">
			{#each st.kinds as k (k.kind)}
				{@render stored(KINDS[k.kind][0], KINDS[k.kind][1], k.count, k.bytes, () => flush(k.kind, KINDS[k.kind][0]))}
			{/each}
			{@render stored('Saved images', 'Posters and backdrops', st.images.count, st.images.bytes, () => (confirmImages = true))}
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
					No lookups since the app started ({since}){st.settings.enabled ? '' : ' with caching on'}.
				{/if}
			</div>
		</div>
	</div>

	<Modal bind:open={confirmAll} title="Flush everything?" sub="{fmtNum(total.count)} item{total.count === 1 ? '' : 's'} · {fmtBytes(total.bytes)}">
		<p class="p-5 text-sm text-gray-300">Every saved result and image is deleted. Pages load from the Flixarr API, and images from TMDB, again until the cache fills back up.</p>
		{#snippet footer()}
			<button class="btn-ghost" onclick={() => (confirmAll = false)}>Cancel</button>
			<button class="btn px-6" disabled={busy !== ''} onclick={() => flush('all', 'Everything')}>Flush all</button>
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
