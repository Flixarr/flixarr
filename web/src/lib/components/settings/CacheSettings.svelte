<script lang="ts">
	import { api, fmtBytes, fmtNum, fmtDate, prefs } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import type { CacheState } from '#lib/types.ts';
	import Toggle from '#lib/components/Toggle.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import Tip from '#lib/components/Tip.svelte';

	const KINDS = { titles: ['Titles', 'Movies, series, seasons, collections'], people: ['People', 'Cast and crew pages'], lists: ['Lists', 'Discover rows, browse pages, genres'], searches: ['Searches', 'Search results'] };
	const PAGES = [[360, '6 hours'], [720, '12 hours'], [1440, '1 day'], [4320, '3 days'], [10080, '7 days']] as const;
	const LISTS = [[360, '6 hours'], [720, '12 hours'], [1440, '1 day'], [4320, '3 days']] as const;
	const SEARCHES = [[360, '6 hours'], [720, '12 hours'], [1440, '1 day'], [4320, '3 days'], [10080, '7 days']] as const;
	const MAX = [5000, 10000, 25000, 50000, 100000];
	const DAYS = [7, 14, 30, 60, 90];
	const API_OFF = 'Cache API responses is off.';

	type TimeKey = 'pagesMinutes' | 'listsMinutes' | 'searchesMinutes' | 'maxResults';

	let st = $state<CacheState | null>(null);
	let confirmAll = $state(false);
	let confirmImages = $state(false);
	let busy = $state('');

	const total = $derived(st ? st.kinds.reduce((a, k) => ({ count: a.count + k.count, bytes: a.bytes + k.bytes }), { count: st.images.count, bytes: st.images.bytes }) : null);
	// Average size of one saved result, measured on whatever server this runs on once 50 are saved; a typical figure before that.
	const results = $derived(st ? st.kinds.reduce((a, k) => ({ count: a.count + k.count, bytes: a.bytes + k.bytes }), { count: 0, bytes: 0 }) : { count: 0, bytes: 0 });
	const measured = $derived(results.count >= 50);
	const perResult = $derived(measured ? results.bytes / results.count : 12 * 1024);
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

{#snippet small(label: string, key: TimeKey, opts: readonly (readonly [number, string])[])}
	<div class="flex items-center justify-between gap-4 py-1.5 text-xs">
		<label for={'cache-' + key} class="text-gray-400">{label}</label>
		<select id={'cache-' + key} class="rounded-lg border border-gray-700/60 bg-gray-800/70 px-2.5 py-1 text-xs disabled:cursor-not-allowed" value={st!.settings[key]} disabled={!st!.settings.enabled}
			onchange={(e) => save((s) => (s.settings[key] = +(e.currentTarget as HTMLSelectElement).value))}>
			{#each opts as [v, l] (v)}<option value={v}>{l}{v === st!.defaults[key] ? ' (default)' : ''}</option>{/each}
		</select>
	</div>
{/snippet}

{#snippet stored(label: string, about: string, count: number, bytes: number, onflush: () => void)}
	<div class="flex items-center gap-3 py-3 text-sm">
		<span class="min-w-0 flex-1"><span class="block">{label}</span><span class="block text-xs text-gray-500">{about}</span></span>
		<span class="text-right"><span class="block tabular-nums">{fmtNum(count)}</span><span class="block text-xs text-gray-500">{fmtBytes(bytes)}</span></span>
		<button class="btn-ghost px-2.5 py-1 text-xs" disabled={!count || busy !== ''} onclick={onflush}>Flush</button>
	</div>
{/snippet}

{#snippet stat(label: string, value: string, sub: string)}
	<div class="card px-4 py-3">
		<div class="text-xs font-semibold uppercase tracking-wide text-gray-500">{label}</div>
		<div class="mt-1 text-2xl font-semibold tabular-nums">{value}</div>
		<div class="text-xs text-gray-500">{sub}</div>
	</div>
{/snippet}

{#if st && total}
	<div class="grid gap-3 sm:grid-cols-3">
		{@render stat('Hit rate', lookups ? Math.round((st.counters.hits / lookups) * 100) + '%' : 'No lookups yet', lookups ? `${fmtNum(st.counters.hits)} of ${fmtNum(lookups)} since ${since}` : `Since ${since}`)}
		<div class="card px-4 py-3">
			<div class="text-xs font-semibold uppercase tracking-wide text-gray-500">API calls saved</div>
			<div class="mt-1 text-2xl font-semibold tabular-nums">{fmtNum(st.counters.hits)}</div>
			<div class="text-xs text-gray-500">{fmtNum(st.counters.misses)} fetched from the Flixarr API</div>
			{#if st.counters.stale}<div class="text-xs text-yellow-400">{fmtNum(st.counters.stale)} served while the API was down</div>{/if}
		</div>
		{@render stat('Cache size', fmtBytes(total.bytes), `${fmtNum(total.count - st.images.count)} results · ${fmtNum(st.images.count)} images`)}
	</div>

	<div>
		{@render heading('Caching')}
		<div class="card divide-y divide-gray-700/40 px-4">
			<div class="py-3">
				<Toggle checked={st.settings.enabled} label="Cache API responses" about="Answers from the Flixarr API are saved in this app's database, so repeat lookups are instant and don't go back to the API. If the API is unreachable, expired results are shown for up to a day."
					onchange={(on) => save((s) => (s.settings.enabled = on), on ? 'API responses are cached again.' : "API responses aren't cached. Every lookup goes to the Flixarr API.")} />
			</div>
			<div class="py-3">
				<Toggle checked={st.images.enabled} label="Cache images" about="Posters and backdrops load through your server and are saved on its disk, instead of each browser loading them from TMDB. TMDB then sees your server instead of each viewer."
					onchange={(on) => save((s) => (s.images.enabled = on), on ? 'Images are cached. New pages load them through your server.' : 'Images load from TMDB again.')} />
			</div>
		</div>
	</div>

	<div>
		{@render heading('How long to keep')}
		<div class="card divide-y divide-gray-700/40 px-4">
			<div class="py-3 {st.settings.enabled ? '' : 'opacity-50'}">
				<div class="flex items-center justify-between gap-4">
					<span class="text-sm"><span class="block">Keep times</span><span class="block text-xs text-gray-500">{!st.settings.enabled ? API_OFF : st.settings.auto ? 'The Flixarr API decides how long each result is kept, based on how often that title still changes.' : 'You pick how long each kind of result is kept.'}</span></span>
					<span class="inline-flex shrink-0 rounded-lg border border-gray-700/60 bg-gray-800/70 p-0.5 text-sm" role="radiogroup" aria-label="Keep times">
						{#each [[true, 'Automatic'], [false, 'Custom']] as const as [auto, name] (name)}
							<button class="rounded-md px-3 py-1 transition disabled:cursor-not-allowed {st.settings.auto === auto ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'}" role="radio" aria-checked={st.settings.auto === auto} disabled={!st.settings.enabled}
								onclick={() => st!.settings.auto !== auto && save((s) => (s.settings.auto = auto), auto ? 'Keep times are automatic.' : 'Keep times are set by you.')}>{name}</button>
						{/each}
					</span>
				</div>
				{#if !st.settings.auto}
					<div class="mt-3 rounded-lg border border-gray-700/50 bg-gray-900/60 px-4 py-1.5">
						{@render small('Movie, series and people pages', 'pagesMinutes', PAGES)}
						{@render small('Discover rows and lists', 'listsMinutes', LISTS)}
						{@render small('Searches', 'searchesMinutes', SEARCHES)}
					</div>
				{/if}
			</div>
			<div class="flex items-center justify-between gap-4 py-3 text-sm {st.settings.enabled ? '' : 'opacity-50'}">
				<span>
					<span class="flex items-center gap-1.5"><label for="cache-maxResults">Size limit</label>
						<Tip label="How much space each size limit takes">
							<span class="block pb-1 font-medium text-gray-200">Space used when full</span>
							{#each MAX as n (n)}<span class="flex justify-between tabular-nums"><span>{fmtNum(n)} results</span><span>about {fmtBytes(n * perResult)}</span></span>{/each}
							<span class="block pt-1.5 text-gray-500">{measured ? `Based on the average size of your saved results (${fmtBytes(perResult)} each).` : 'Based on a typical result size of about 12 KB.'}</span>
						</Tip>
					</span>
					<span class="mt-0.5 block text-xs text-gray-500">{st.settings.enabled ? 'Oldest results are removed first.' : API_OFF}</span>
				</span>
				<select id="cache-maxResults" class="rounded-lg border border-gray-700/60 bg-gray-800/70 px-3 py-1.5 text-sm disabled:cursor-not-allowed" value={st.settings.maxResults} disabled={!st.settings.enabled}
					onchange={(e) => save((s) => (s.settings.maxResults = +(e.currentTarget as HTMLSelectElement).value))}>
					{#each MAX as n (n)}<option value={n}>{fmtNum(n)} results{n === st.defaults.maxResults ? ' (default)' : ''}</option>{/each}
				</select>
			</div>
			{@render pick('Remove images not viewed for', 'images', st.images.days, st.images.defaultDays, DAYS.map((d) => [d, d + ' days'] as const), (s, v) => (s.images.days = v), st.images.enabled ? "Images nobody has looked at in this long are deleted from your server's disk. They download again the next time someone views them." : 'Cache images is off.', !st.images.enabled)}
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
