<script lang="ts">
	// The request window on movie and series pages: where it goes, the quality (for people allowed to choose), and for series
	// the whole show, chosen seasons or chosen episodes (whichever Settings → Requests allows).
	import { api } from '#lib/api.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { Movie, Series, Request } from '#lib/types.ts';
	import Modal from './Modal.svelte';
	import Select from './Select.svelte';
	import Img from './Img.svelte';
	import EpisodePicker from './EpisodePicker.svelte';
	import Alert from './Alert.svelte';
	import LimitRing from './LimitRing.svelte';
	import Icon from './Icon.svelte';
	import { loadSeasons, noSeasons, episodeCount } from '#lib/seasons.ts';

	let { open = $bindable(false), item, asked, onrequested }: { open?: boolean; item: Movie | Series; asked?: { whole: boolean; seasons: number[] }; onrequested?: (r: Request) => void } = $props();

	type Options = { connected: boolean; server: string; approval: boolean; usage: { limit: number; used: number }; defaultProfileId?: number; defaultProfileName?: string; profiles: { id: number; name: string; resolution?: string; mbPerMin?: number }[];
		tvModes: { whole: boolean; seasons: boolean; episodes: boolean; default: Mode } };
	type Mode = 'whole' | 'seasons' | 'episodes';
	let opts = $state<Options | null>(null);
	let profileId = $state(0);
	let mode = $state<Mode>('whole');
	let seasons = $state<number[]>([]);
	let episodes = $state<[number, number][]>([]);
	let sending = $state(false);
	let done = $state(false); // the Requested moment (#281): shown a moment before the window closes
	let plex = $state(noSeasons()); // episodes in Plex per season, and each season's count
	const inPlex = $derived(plex.inPlex);
	const eps = (s: { n: number; episodes: number }) => episodeCount(plex, s);

	const series = $derived(item.type === 'series' ? (item as Series) : null);
	const list = $derived(series ? series.seasons.filter((s) => s.n > 0) : []);
	const complete = (n: number, eps: number) => (inPlex[n] ?? 0) >= eps && eps > 0;
	const requested = (n: number) => !!asked?.seasons.includes(n);
	const more = $derived(!!asked?.seasons.length); // some seasons already asked for: this asks for the rest
	const anyInPlex = $derived(Object.keys(inPlex).length > 0);
	// Every season that's out is ticked, already asked for or in Plex: that's the whole show, so it's sent
	// as one and also follows seasons that air later.
	const today = new Date().toISOString().slice(0, 10);
	const allLeft = $derived(
		mode === 'seasons' && seasons.length > 0 &&
		list.filter((s) => eps(s) > 0 && (!s.air || s.air <= today)).every((s) => seasons.includes(s.n) || requested(s.n) || complete(s.n, eps(s)))
	);

	$effect(() => {
		if (!open) return;
		opts = null;
		done = false;
		mode = 'whole';
		seasons = [];
		episodes = [];
		plex = noSeasons();
		if (item.type === 'series') loadSeasons(item.id).then((d) => (plex = d));
		api<Options>(`/requests/options?type=${item.type}`).then((o) => {
			opts = o;
			profileId = o.defaultProfileId ?? 0;
			mode = o.tvModes?.[o.tvModes.default] ? o.tvModes.default : (['whole', 'seasons', 'episodes'] as Mode[]).find((m) => o.tvModes?.[m]) ?? 'whole';
			if (more && o.tvModes?.seasons) mode = 'seasons'; // asking for more: start on the seasons left
		}).catch((e) => notify(e instanceof Error ? e.message : 'Unable to load this', 'orange'));
	});

	function toggle(n: number) {
		seasons = seasons.includes(n) ? seasons.filter((x) => x !== n) : [...seasons, n].sort((a, b) => a - b);
	}

	async function send() {
		sending = true;
		try {
			const r = await api<Request>('/requests', {
				method: 'POST',
				body: JSON.stringify({ type: item.type, tmdbId: item.id, title: item.title, year: item.year, poster: item.poster ?? '', backdrop: item.backdrop ?? '',
					seasons: series && mode === 'seasons' && !allLeft ? seasons : [], episodes: series && mode === 'episodes' ? episodes : [], profileId, genres: item.genres, language: series ? series.language : '', seasonCount: list.length })
			});
			onrequested?.(r);
			if (r.status === 'failed' || r.status === 'available') {
				open = false;
				if (r.status === 'failed') notify(r.detail || `Unable to add ${item.title}`, 'orange');
				else notify(`${item.title} is already in your library.`);
			} else {
				done = true;
				setTimeout(() => (open = false), 1600);
			}
		} catch (e) {
			notify(e instanceof Error ? e.message : `Unable to request ${item.title}`, 'orange');
		} finally {
			sending = false;
		}
	}

	const label = $derived(
		!series || mode === 'whole' ? 'Request'
		: mode === 'seasons' ? (allLeft ? (more ? 'Request the rest' : 'Request the whole show') : seasons.length ? `Request ${seasons.length} season${seasons.length === 1 ? '' : 's'}` : 'Pick seasons')
		: episodes.length ? `Request ${episodes.length} episode${episodes.length === 1 ? '' : 's'}` : 'Pick episodes'
	);
	// How many requests this uses against a weekly limit, the way the server counts it: 1 movie, or seasons.
	const wants = $derived(
		!series ? 1
		: mode === 'whole' ? Math.max(list.length - (asked?.seasons.length ?? 0), 1)
		: mode === 'seasons' ? seasons.length
		: new Set(episodes.map(([sn]) => sn)).size
	);
	const left = $derived(opts?.usage?.limit ? Math.max(0, opts.usage.limit - opts.usage.used) : Infinity);
	const over = $derived(wants > left);
	// Not out yet (#279): the release date is still ahead.
	const release = $derived(item.type === 'movie' ? (item as Movie).release : null);
	const notOut = $derived(!!release && release > today);
	const releaseDay = $derived(release ? new Date(release + 'T00:00:00').toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) : '');
	// What a profile gives, in the Quality menu (#280): "1080p, about 8 GB", series per episode.
	const minutes = $derived(item.runtime || (series ? 45 : 110));
	function gives(p: { resolution?: string; mbPerMin?: number }) {
		const mb = (p.mbPerMin ?? 0) * minutes;
		const size = !mb ? '' : mb >= 1000 ? `about ${Math.round(mb / 100) / 10} GB` : `about ${Math.round(mb / 50) * 50 || 50} MB`;
		return [p.resolution, size && (series ? `${size} an episode` : size)].filter(Boolean).join(', ');
	}
	const ready = $derived(!series || mode === 'whole' || (mode === 'seasons' ? seasons.length > 0 : episodes.length > 0));
	const MODES: [Mode, string][] = [['whole', 'Whole show'], ['seasons', 'Seasons'], ['episodes', 'Episodes']];
</script>

<Modal bind:open title="Request {item.title}">
	{#snippet top()}
		<div class="relative overflow-hidden rounded-t-xl">
			{#if item.backdrop}<div class="absolute inset-0"><Img path={item.backdrop} kind="backdrop" sizes="(min-width: 640px) 768px, 100vw" eager class="h-full w-full" imgClass="object-[center_25%]" /></div>{/if}
			<div class="absolute inset-0 bg-linear-to-t from-gray-900 via-gray-900/75 to-gray-900/45"></div>
			<div class="relative flex items-center gap-4 p-5 pr-12">
				<div class="w-16 shrink-0 overflow-hidden rounded-lg shadow-lg ring-1 ring-white/10">{#if item.poster}<Img path={item.poster} kind="poster" sizes="64px" eager class="aspect-[2/3] w-full" />{/if}</div>
				<div class="min-w-0">
					<div class="text-lg leading-tight font-bold text-white">{item.title}</div>
					<div class="mt-0.5 text-sm text-gray-400">{[item.year, series ? `Series · ${list.length} season${list.length === 1 ? '' : 's'}` : 'Movie'].filter(Boolean).join(' · ')}</div>
				</div>
			</div>
		</div>
	{/snippet}
	<div class="space-y-5 px-5 pt-1 pb-5">
		{#if !opts}
			<p class="text-sm text-gray-400">Loading…</p>
		{:else if !opts.connected}
			<Alert tone="yellow" icon="warning">{item.type === 'series' ? 'Series' : 'Movie'} requests aren't set up yet (Settings, Services).</Alert>
		{:else}
			{#if notOut}<Alert tone="gray" icon="calendar">Not out yet. Your download will start when it comes out on {releaseDay}.</Alert>{/if}
			{#if opts.approval}<Alert tone="yellow">Your download will start once an admin approves your request</Alert>{/if}
			{#if opts.profiles.length}
				<div>
					<label for="req-quality" class="block text-sm">Quality</label>
					<Select id="req-quality" bind:value={profileId} options={opts.profiles.map((p) => ({ value: p.id, label: p.name, hint: p.id === opts!.defaultProfileId ? 'default' : '', sub: gives(p) }))} />
				</div>
			{/if}
			{#if series}
				<div>
					{#if MODES.filter(([m]) => opts!.tvModes?.[m]).length > 1}
						<div class="mb-2 flex gap-1 rounded-lg border border-gray-700/60 bg-gray-800/70 p-0.5 text-sm" role="radiogroup" aria-label="What to request">
							{#each MODES.filter(([m]) => opts!.tvModes?.[m]) as [m, l] (m)}
								<button class="flex-1 rounded-md px-3 py-1.5 {mode === m ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'}" role="radio" aria-checked={mode === m} onclick={() => (mode = m)}>{l}</button>
							{/each}
						</div>
					{/if}
					{#if mode === 'whole'}
						<p class="text-xs text-gray-400">Every season, including new ones as they air.{anyInPlex || more ? ' Episodes already there or on the way are skipped.' : ''}</p>
					{:else if mode === 'seasons'}
						<div class="max-h-64 divide-y divide-gray-700/40 overflow-y-auto rounded-lg border border-gray-700/50 bg-gray-900/40">
							{#each list as s (s.n)}
								{@const done = complete(s.n, eps(s))}
								{@const onWay = !done && requested(s.n)}
								<label class="flex items-center gap-3 px-3 py-2 text-sm {done || onWay ? 'opacity-60' : 'cursor-pointer hover:bg-white/5'}">
									<input type="checkbox" class="accent-[var(--color-primary)]" disabled={done || onWay} checked={done || onWay || seasons.includes(s.n)} onchange={() => toggle(s.n)} />
									<span class="flex-1">{s.name}</span>
									{#if done}<span class="text-xs text-green-400">In Plex</span>
									{:else if onWay}<span class="text-xs text-blue-300">Requested</span>
									{:else if inPlex[s.n]}<span class="text-xs text-green-300">{inPlex[s.n]} of {eps(s)} in Plex</span>
									{:else}<span class="text-xs text-gray-400">{eps(s)} episode{eps(s) === 1 ? '' : 's'}</span>{/if}
								</label>
							{/each}
						</div>
						<p class="mt-2 text-xs {allLeft ? 'text-green-300' : 'text-gray-400'}">{allLeft ? "That's every season, so new ones will come in as they air." : 'Only the seasons you pick. Seasons that air later are not added.'}</p>
					{:else}
						<EpisodePicker {series} bind:picked={episodes} done={inPlex} />
						<p class="mt-2 text-xs text-gray-400">Open a season to pick its episodes, or tick the season for all of them.</p>
					{/if}
				</div>
			{/if}
			{#if over && left > 0}<p class="-mt-3 text-xs text-red-300">That's {wants} {series ? 'seasons' : 'movies'} and you have {left} left this week.</p>{/if}
		{/if}
	</div>
	{#snippet footer()}
		{#if done}
			<a href="/requests" class="mr-auto inline-flex items-center gap-1 self-center text-sm text-primary-light hover:underline" onclick={() => (open = false)}>See it on Requests <Icon name="arrow" class="size-4" /></a>
			<span class="inline-flex items-center gap-2 rounded-lg bg-green-600 px-5 py-2 font-medium text-white"><Icon name="check" class="size-4" />Requested</span>
		{:else}
			{#if opts?.usage?.limit}<LimitRing class="mr-auto" limit={opts.usage.limit} used={opts.usage.used} unit={series ? 'season' : 'movie'} />{/if}
			<button class="btn-ghost" onclick={() => (open = false)}>Cancel</button>
			<button class="btn px-6 disabled:cursor-not-allowed disabled:opacity-40 disabled:saturate-0" disabled={!opts?.connected || sending || !ready || over} onclick={send}>{sending ? 'Sending…' : label}</button>
		{/if}
	{/snippet}
</Modal>
