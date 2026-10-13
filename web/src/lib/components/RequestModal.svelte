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

	let { open = $bindable(false), item, asked, onrequested }: { open?: boolean; item: Movie | Series; asked?: { whole: boolean; seasons: number[] }; onrequested?: (r: Request) => void } = $props();

	type Options = { connected: boolean; server: string; approval: boolean; usage: { limit: number; used: number }; defaultProfileId?: number; defaultProfileName?: string; profiles: { id: number; name: string }[];
		tvModes: { whole: boolean; seasons: boolean; episodes: boolean; default: Mode } };
	type Mode = 'whole' | 'seasons' | 'episodes';
	let opts = $state<Options | null>(null);
	let profileId = $state(0);
	let mode = $state<Mode>('whole');
	let seasons = $state<number[]>([]);
	let episodes = $state<[number, number][]>([]);
	let sending = $state(false);
	let inPlex = $state<Record<number, number>>({}); // episodes in Plex per season

	const series = $derived(item.type === 'series' ? (item as Series) : null);
	const list = $derived(series ? series.seasons.filter((s) => s.n > 0) : []);
	const complete = (n: number, eps: number) => (inPlex[n] ?? 0) >= eps && eps > 0;
	const requested = (n: number) => !!asked?.seasons.includes(n);
	const more = $derived(!!asked?.seasons.length); // some seasons already asked for: this asks for the rest
	const anyInPlex = $derived(Object.keys(inPlex).length > 0);

	$effect(() => {
		if (!open) return;
		opts = null;
		mode = 'whole';
		seasons = [];
		episodes = [];
		inPlex = {};
		if (item.type === 'series') api<Record<number, number>>(`/plex/seasons/${item.id}`).then((d) => (inPlex = d)).catch(() => {});
		api<Options>(`/requests/options?type=${item.type}`).then((o) => {
			opts = o;
			profileId = o.defaultProfileId ?? 0;
			mode = o.tvModes?.[o.tvModes.default] ? o.tvModes.default : (['whole', 'seasons', 'episodes'] as Mode[]).find((m) => o.tvModes?.[m]) ?? 'whole';
			if (more && o.tvModes?.seasons) mode = 'seasons'; // asking for more: start on the seasons left
		}).catch((e) => notify(e instanceof Error ? e.message : 'Could not load.', 'orange'));
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
					seasons: series && mode === 'seasons' ? seasons : [], episodes: series && mode === 'episodes' ? episodes : [], profileId, genres: item.genres, language: series ? series.language : '', seasonCount: list.length })
			});
			open = false;
			onrequested?.(r);
			if (r.status === 'failed') notify(`Request saved, but ${opts?.server ?? 'it'} couldn't add it: ${r.error}`, 'orange');
			else if (r.status === 'pending') notify('Requested. It waits for an admin to approve it.');
			else if (r.status === 'available') notify(`${item.title} is already in ${opts?.server}.`);
			else notify(`Requested. ${item.title} was sent to ${opts?.server}.`);
		} catch (e) {
			notify(e instanceof Error ? e.message : 'Could not request it.', 'orange');
		} finally {
			sending = false;
		}
	}

	const label = $derived(
		!series || mode === 'whole' ? 'Request'
		: mode === 'seasons' ? (seasons.length ? `Request ${seasons.length} season${seasons.length === 1 ? '' : 's'}` : 'Pick seasons')
		: episodes.length ? `Request ${episodes.length} episode${episodes.length === 1 ? '' : 's'}` : 'Pick episodes'
	);
	const ready = $derived(!series || mode === 'whole' || (mode === 'seasons' ? seasons.length > 0 : episodes.length > 0));
	const MODES: [Mode, string][] = [['whole', 'Whole show'], ['seasons', 'Seasons'], ['episodes', 'Episodes']];
</script>

<Modal bind:open title="Request {item.title}">
	<div class="space-y-5 p-5">
		<div class="flex gap-4">
			<div class="w-20 shrink-0 overflow-hidden rounded-lg ring-1 ring-gray-700/50">{#if item.poster}<Img path={item.poster} kind="poster" sizes="80px" class="aspect-[2/3] w-full" />{/if}</div>
			<div class="min-w-0 text-sm">
				<div class="font-semibold">{item.title} <span class="font-normal text-gray-400">{item.year}</span></div>
				<p class="mt-1 line-clamp-4 text-xs text-gray-400">{item.overview}</p>
			</div>
		</div>

		{#if !opts}
			<p class="text-sm text-gray-400">Loading…</p>
		{:else if !opts.connected}
			<p class="rounded-lg bg-yellow-500/10 px-3 py-2 text-sm text-yellow-300 ring-1 ring-yellow-500/30">{opts.server} isn't connected yet, so this can't be requested. An admin can connect it in Settings → Services.</p>
		{:else}
			<p class="text-xs text-gray-400">Goes to <span class="text-gray-200">{opts.server}</span>{opts.approval ? ' after an admin approves it' : ''}.{#if opts.usage?.limit}{@const left = Math.max(0, opts.usage.limit - opts.usage.used)} You have {left} {series ? 'season' : 'movie'}{left === 1 ? '' : 's'} left this week (of {opts.usage.limit}).{/if}</p>
			{#if opts.profiles.length}
				<div>
					<label for="req-quality" class="block text-sm">Quality</label>
					<Select id="req-quality" bind:value={profileId} options={opts.profiles.map((p) => ({ value: p.id, label: p.name, hint: p.id === opts!.defaultProfileId ? 'default' : '' }))} />
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
						<p class="text-xs text-gray-500">Every season, including new ones as they air.{anyInPlex || more ? ' Episodes already there or on the way are skipped.' : ''}</p>
					{:else if mode === 'seasons'}
						<div class="max-h-64 divide-y divide-gray-700/40 overflow-y-auto rounded-lg border border-gray-700/50 bg-gray-900/40">
							{#each list as s (s.n)}
								{@const done = complete(s.n, s.episodes)}
								{@const onWay = !done && requested(s.n)}
								<label class="flex items-center gap-3 px-3 py-2 text-sm {done || onWay ? 'opacity-60' : 'cursor-pointer hover:bg-white/5'}">
									<input type="checkbox" class="accent-[var(--color-primary)]" disabled={done || onWay} checked={done || onWay || seasons.includes(s.n)} onchange={() => toggle(s.n)} />
									<span class="flex-1">{s.name}</span>
									{#if done}<span class="text-xs text-green-400">In Plex</span>
									{:else if onWay}<span class="text-xs text-blue-300">Requested</span>
									{:else if inPlex[s.n]}<span class="text-xs text-green-300">{inPlex[s.n]} of {s.episodes} in Plex</span>
									{:else}<span class="text-xs text-gray-500">{s.episodes} episode{s.episodes === 1 ? '' : 's'}</span>{/if}
								</label>
							{/each}
						</div>
						<p class="mt-2 text-xs text-gray-500">Only the seasons you pick. Seasons that air later are not added.</p>
					{:else}
						<EpisodePicker {series} bind:picked={episodes} done={inPlex} />
						<p class="mt-2 text-xs text-gray-500">Open a season to pick its episodes, or tick the season for all of them.</p>
					{/if}
				</div>
			{/if}
		{/if}
	</div>
	{#snippet footer()}
		<button class="btn-ghost" onclick={() => (open = false)}>Cancel</button>
		<button class="btn px-6" disabled={!opts?.connected || sending || !ready} onclick={send}>{sending ? 'Sending…' : label}</button>
	{/snippet}
</Modal>
