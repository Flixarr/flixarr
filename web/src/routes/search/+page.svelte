<script lang="ts" module>
	// Was this page opened by Back/Forward? The browser fires popstate just before SvelteKit rebuilds the page.
	let poppedAt = -Infinity;
	if (typeof addEventListener !== 'undefined') addEventListener('popstate', () => (poppedAt = performance.now()));
	const cameBack = () => performance.now() - poppedAt < 1500;
</script>

<script lang="ts">
	import { tick } from 'svelte';
	import { restoreScroll } from '#lib/scroll.ts';
	import { replaceState, snapshot } from '$app/navigation';
	import { api, meta, href } from '#lib/api.ts';
	import type { Card, Collection, Page, SearchHit, Movie, Series } from '#lib/types.ts';
	import Poster from '#lib/components/Poster.svelte';
	import Chips from '#lib/components/Chips.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import { can } from '#lib/session.svelte.ts';
	import { statusOf } from '#lib/availability.svelte.ts';
	import Img from '#lib/components/Img.svelte';
	import { tilt, tiltEnabled } from '#lib/tilt.ts';
	import RequestButton from '#lib/components/RequestButton.svelte';
	import LoadingMore from '#lib/components/LoadingMore.svelte';
	import { infinite } from '#lib/infinite.ts';
	import { releaseKeyboard } from '#lib/keyboard.ts';

	type Kind = 'movie' | 'series' | 'person';
	type Tab = 'all' | Kind;
	const KINDS: Kind[] = ['movie', 'series', 'person'];

	// From the address bar: on Back, page.url can still describe the page we're leaving while this one is built.
	const startUrl = new URLSearchParams(location.search);
	let q = $state(startUrl.get('q') ?? '');
	let tab = $state<Tab>((KINDS as string[]).includes(startUrl.get('tab') ?? '') ? (startUrl.get('tab') as Kind) : 'all');
	let hits = $state<SearchHit[]>([]); // the mixed search: Top result and the rows
	let parsed = $state<Page<SearchHit>['parsed']>(); // a browse or like search: what it's showing (#258)
	let mine = $state<Card[]>([]); // titles in Plex matching the search, even with a typo (#251)
	let busy = $state(false);
	let trending = $state<Card[]>([]);
	let input = $state<HTMLInputElement>();
	// One kind on its own, page by page (#249): the search it was loaded for, its pages so far, and how many there are.
	let kind = $state<{ term: string; tab: Tab; list: SearchHit[]; page: number; pages: number; busy: boolean; parsed?: Page<SearchHit>['parsed'] }>({ term: '', tab: 'all', list: [], page: 0, pages: 1, busy: false });

	$effect(() => {
		// Three pages, so a wide screen gets several full rows. Kept to a multiple of 24 so every row is full
		// whether the grid has 3, 4, 6 or 8 columns.
		const pages: Card[][] = [[], [], []];
		const showTrending = () => {
			const seen = new Set<string>();
			const all = pages.flat().filter((c) => !seen.has(c.type + c.id) && seen.add(c.type + c.id));
			trending = all.length >= 24 ? all.slice(0, all.length - (all.length % 24)) : all;
		};
		pages.forEach((_, i) => {
			const show = (p: Page<Card>) => { pages[i] = p.results; showTrending(); };
			meta<Page<Card>>('lists/trending', i ? { page: i + 1 } : {}, show).then(show).catch(() => {});
		});
	});

	// Landing here puts the cursor in the search box, except when coming back with the back button
	// (that would undo the restored scroll and pop the phone keyboard). The top-bar search link and the
	// / key send flixarr:focus-search when you're already on this page.
	let focused = false;
	$effect(() => {
		if (!input || focused) return;
		focused = true;
		if (!cameBack()) input.focus();
		releaseKeyboard(); // the stand-in from the Search tap hands the keyboard over (#162)
	});
	$effect(() => {
		const f = () => { input?.focus(); input?.select(); };
		addEventListener('flixarr:focus-search', f);
		return () => removeEventListener('flixarr:focus-search', f);
	});

	// Recent searches (this device only): saved when you open a result or press Enter (#254).
	const RECENT = 'flixarr_recent_searches';
	let recent = $state<string[]>(JSON.parse(localStorage.getItem(RECENT) ?? '[]'));
	function saveRecent() {
		const term = q.trim();
		if (term.length < 2) return;
		recent = [term, ...recent.filter((r) => r.toLowerCase() !== term.toLowerCase())].slice(0, 8);
		localStorage.setItem(RECENT, JSON.stringify(recent));
	}
	function clearRecent() {
		recent = [];
		localStorage.removeItem(RECENT);
	}

	// What's in Plex: the library's own matches from the server (typos that keep the letters in order still
	// match, brkng bad = Breaking Bad), with their posters from the title pages (usually already saved).
	async function libraryCards(term: string): Promise<Card[]> {
		const found = await api<{ type: 'movie' | 'series'; id: number; title: string }[]>('/plex/search?q=' + encodeURIComponent(term)).catch(() => []);
		const cards = await Promise.all(found.slice(0, 10).map((t) =>
			meta<Movie | Series>(`${t.type}/${t.id}`).then((d) => ({ id: d.id, type: t.type, title: d.title, poster: d.poster, backdrop: d.backdrop, year: d.year, rating: d.rating, genres: [], overview: d.overview }) as Card).catch(() => null)));
		return cards.filter((c): c is Card => !!c);
	}

	// Debounced live search.
	$effect(() => {
		const term = q.trim();
		if (!term) { hits = []; mine = []; parsed = undefined; busy = false; return; }
		busy = true;
		let alive = true;
		const t = setTimeout(() => {
			const show = (p: Page<SearchHit>) => { if (alive) { hits = p.results; parsed = p.parsed; } };
			// Only the newest search may say it's done: an older one finishing late would flash "No results" (#245).
			meta<Page<SearchHit>>('search', { q: term, collections: 1 }, show).then(show).finally(() => alive && (busy = false));
			libraryCards(term).then((c) => alive && (mine = c));
		}, 250);
		return () => { clearTimeout(t); alive = false; };
	});

	// The address keeps the search and the open kind, so Back and See all land on the same view. Only touch
	// the URL when it actually changes: replacing the history entry on arrival would drop its saved snapshot.
	$effect(() => {
		const term = q.trim();
		const p = new URLSearchParams();
		if (term) p.set('q', term);
		if (term && tab !== 'all') p.set('tab', tab);
		const url = '/search' + (p.size ? '?' + p : '');
		if (url !== location.pathname + location.search) replaceState(url, {});
	});

	// A kind's next page. Starts over when the search or the kind changed.
	async function more(): Promise<boolean> {
		const term = q.trim();
		if (!term || tab === 'all') return false;
		if (kind.term !== term || kind.tab !== tab) kind = { term, tab, list: [], page: 0, pages: 1, busy: false };
		if (kind.busy || kind.page >= kind.pages) return false;
		const want = { term, tab, page: kind.page + 1 };
		kind.busy = true;
		try {
			const p = await meta<Page<SearchHit>>('search', { q: term, type: tab, page: want.page });
			if (kind.term !== want.term || kind.tab !== want.tab) return false; // the search moved on meanwhile
			const seen = new Set(kind.list.map((h) => h.type + h.id));
			kind.list = [...kind.list, ...p.results.filter((h) => !seen.has(h.type + h.id))];
			kind.page = want.page;
			kind.pages = p.totalPages;
			if (want.page === 1) kind.parsed = p.parsed;
			return kind.page < kind.pages;
		} catch {
			return false;
		} finally {
			kind.busy = false;
		}
	}
	$effect(() => {
		// A kind's first page loads as soon as it's opened (the scroll watcher fetches the rest).
		if (tab !== 'all' && q.trim() && (kind.term !== q.trim() || kind.tab !== tab)) {
			const t = setTimeout(more, 250);
			return () => clearTimeout(t);
		}
	});

	// Going back to search brings back the results, the kind you picked and where you'd scrolled to.
	snapshot({
		id: 'search',
		capture: () => ({ q, tab, hits: $state.snapshot(hits), mine: $state.snapshot(mine), parsed: $state.snapshot(parsed), kind: $state.snapshot(kind), y: scrollY }),
		restore: async (v) => {
			q = v.q;
			tab = v.tab;
			hits = v.hits;
			mine = v.mine ?? [];
			parsed = v.parsed;
			if (v.kind) kind = { ...v.kind, busy: false };
			await tick();
			restoreScroll(v.y);
		}
	});

	// A title's name the way it's compared: lower case, no accents or punctuation, & = and, no leading "the".
	const plain = (s: string) => {
		const w = s.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase().replace(/&/g, ' and ').replace(/['’]/g, '').split(/[^\p{L}\p{N}]+/u).filter(Boolean);
		if (w.length > 1 && ['the', 'a', 'an'].includes(w[0])) w.shift();
		return w.join('');
	};
	// The best match. A title in Plex wins, even with a typo (#251), unless the search found the exact title,
	// a person or collection, or read the search a special way (a nickname, a year, a season, a pasted ID).
	const top = $derived.by<SearchHit | undefined>(() => {
		const best = hits[0], own = mine[0] as SearchHit | undefined;
		if (!best || !own || parsed?.browse || parsed?.like) return best ?? own;
		const special = parsed?.id || parsed?.nick || parsed?.year || parsed?.season;
		if (special || best.type === 'person' || best.type === 'collection' || plain(best.title) === plain(q)) return best;
		return own;
	});
	const rest = $derived(hits.filter((h) => h !== top));
	// Everything else in one mixed grid, in ranked order (Marc: "I want them mixed in"). Titles only Plex
	// matched (a typo like brkng bad) go first (#251); the Top result isn't repeated.
	const topKey = $derived(top ? top.type + top.id : '');
	const mixed = $derived.by(() => {
		const seen = new Set<string>([topKey]);
		const out: SearchHit[] = [];
		// A browse or like search isn't a title, so the library's name matches don't belong in it.
		const own = parsed?.browse || parsed?.like ? [] : (mine as SearchHit[]);
		for (const h of [...own, ...rest]) {
			const k = h.type + h.id;
			if (!seen.has(k)) { seen.add(k); out.push(h); }
		}
		return out;
	});
	const ROW_NAME = { movie: 'Movies', series: 'Series', person: 'People' };
	const kindList = $derived(kind.term === q.trim() && kind.tab === tab ? kind.list.filter((h) => h.type + h.id !== topKey) : []);
	const kindDone = $derived(kind.term === q.trim() && kind.tab === tab && kind.page >= kind.pages);

	// The Top result's title page, for its Request or Play button (#248).
	let topItem = $state<Movie | Series | null>(null);
	$effect(() => {
		const h = top;
		topItem = null;
		if (!h || h.type === 'person' || h.type === 'collection') return;
		let alive = true;
		meta<Movie | Series>(`${h.type}/${h.id}`).then((d) => alive && (topItem = d)).catch(() => {});
		return () => { alive = false; };
	});

	// A collection as the Top result: its movies, to say how many are missing (same rule as the collection page).
	let topParts = $state<Card[] | null>(null);
	$effect(() => {
		const h = top;
		topParts = null;
		if (h?.type !== 'collection') return;
		let alive = true;
		meta<Collection>(`collection/${h.id}`).then((d) => alive && (topParts = d.parts)).catch(() => {});
		return () => { alive = false; };
	});
	const partsMissing = $derived.by(() => {
		if (!topParts) return undefined;
		const st = topParts.map((p) => statusOf(`movie:${p.id}`));
		if (st.some((x) => x === undefined)) return undefined; // still loading
		return st.filter((x) => !x || ['failed', 'declined', 'expired', 'removed'].includes(x.status)).length;
	});

	// Coming 2027 for titles not out yet, New for this year's (#252).
	const thisYear = new Date().getFullYear();
	const label = (h: { type: string; year?: string }) => {
		if (h.type === 'person' || h.type === 'collection') return '';
		const y = Number(h.year) || 0;
		return !y ? '' : y > thisYear ? `Coming ${y}` : y === thisYear ? 'New' : '';
	};
	// Collections and the Top result's poster get the same mouse-following hover as every other poster (#263).
	const tilting = tiltEnabled();
	const KIND_NAME = { movie: 'Movie', series: 'Series', person: 'Person', collection: 'Collection' };
	// A person as the Top result: the titles the API put right after them are what they're known for (#258).
	const knownFor = $derived.by(() => {
		if (top?.type !== 'person' || hits[0] !== top) return [];
		const out: SearchHit[] = [];
		for (const h of hits.slice(1)) {
			if (h.type !== 'movie' && h.type !== 'series') break;
			out.push(h);
			if (out.length === 3) break;
		}
		return out;
	});
	// What a browse or like search is showing, for the line above the results (#258).
	const reading = $derived.by(() => {
		const p = tab === 'all' ? parsed : kind.term === q.trim() && kind.tab === tab ? kind.parsed : undefined;
		if (p?.browse) return p.browse.label;
		if (p?.like) return `More like ${p.like.title}${p.like.year ? ` (${p.like.year})` : ''}`;
		return '';
	});
</script>

{#snippet person(h: SearchHit, cls: string)}
	<a href="/person/{h.id}" class="group block {cls}">
		<div class="aspect-[2/3] w-full overflow-hidden rounded-xl border border-gray-700/40 bg-gray-800/40 shadow-xl">
			{#if h.poster}<Img path={h.poster} kind="profile" sizes="(min-width: 640px) 144px, 112px" class="h-full w-full" />{:else}<div class="grid h-full place-items-center text-3xl text-gray-600">{h.title.charAt(0)}</div>{/if}
		</div>
		<div class="mt-2 truncate text-[13px] font-medium">{h.title}</div>
		<div class="truncate text-xs text-gray-400">{h.dept ?? ''}</div>
	</a>
{/snippet}

{#snippet collection(h: SearchHit, cls: string)}
	<a href="/collection/{h.id}" class="group relative block {cls}">
		<div use:tilt class="relative aspect-[2/3] w-full overflow-hidden rounded-xl border border-gray-700/40 bg-gray-800/40 shadow-xl">
			{#if h.poster}<Img path={h.poster} kind="poster" sizes="(min-width: 640px) 144px, 112px" class="h-full w-full {tilting ? '' : 'transition duration-300 group-hover:scale-105'}" />{:else}<div class="grid h-full place-items-center text-3xl text-gray-600">{h.title.charAt(0)}</div>{/if}
			{#if tilting}<div data-glare class="pointer-events-none absolute inset-0 opacity-0 mix-blend-soft-light"></div>{/if}
		</div>
		<div class="relative">
			<div class="mt-2 truncate text-[13px] font-medium">{h.title}</div>
			<div class="truncate text-xs text-gray-400">Collection</div>
		</div>
	</a>
{/snippet}

{#snippet hit(h: SearchHit, cls: string)}
	{#if h.type === 'person'}{@render person(h, cls)}{:else if h.type === 'collection'}{@render collection(h, cls)}{:else}<Poster item={h as Card} class={cls} sub={label(h)} />{/if}
{/snippet}

<main class="mx-auto max-w-[1600px] space-y-6 pt-20 laptop:px-10 laptop:pt-24">
	<div class="px-4 laptop:px-0">
		<label class="card flex items-center gap-3 px-4 py-3.5 text-gray-400 focus-within:ring-2 focus-within:ring-primary/60">
			<Icon name="search" class="size-5 shrink-0" />
			<input bind:this={input} bind:value={q} autocomplete="off" enterkeyhint="search" onkeydown={(e) => { if (e.key === 'Enter') { saveRecent(); input?.blur(); } }} class="w-full bg-transparent text-base text-white placeholder-gray-500 outline-none" placeholder="Search movies, series, people" />
			{#if q}<button class="text-xs text-gray-400 hover:text-white" onclick={() => { q = ''; tab = 'all'; input?.focus(); }}>Clear</button>
			{:else}<kbd class="hidden rounded border border-gray-600 px-1.5 text-[11px] text-gray-500 laptop:inline">/</kbd>{/if}
		</label>
	</div>
	{#if q.trim()}
		<Chips bind:value={tab} options={[{ key: 'all', label: 'All' }, { key: 'movie', label: 'Movies' }, { key: 'series', label: 'Series' }, { key: 'person', label: 'People' }]} />
		<!-- Opening a result saves the search to Recent (clicks bubble up from the links). -->
		<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
		<div onclick={(e) => (e.target as Element).closest('a') && saveRecent()}>
			<div class="space-y-6">
				{#if top}
					<!-- The best match, big, with its button (#248). -->
					<div class="px-4 laptop:px-0">
						<div class="relative overflow-hidden rounded-2xl ring-1 ring-gray-700/50">
							{#if top.backdrop}<Img path={top.backdrop} kind="backdrop" sizes="100vw" class="absolute inset-0 h-full w-full opacity-30" />{/if}
							<div class="absolute inset-0 bg-gradient-to-r from-gray-950 via-gray-950/80 to-transparent"></div>
							<div class="relative flex gap-4 p-4 tablet:gap-5 tablet:p-5 laptop:p-7">
								<a href={href(top)} class="group w-24 shrink-0 self-start tablet:w-32 laptop:w-40">
									<div use:tilt class="relative aspect-[2/3] rounded-xl overflow-hidden bg-gray-800 shadow-2xl ring-1 ring-white/10">
										{#if top.poster}<Img path={top.poster} kind={top.type === 'person' ? 'profile' : 'poster'} sizes="160px" class="h-full w-full {tilting ? '' : 'transition duration-300 group-hover:scale-105'}" />{:else}<div class="grid h-full place-items-center text-3xl text-gray-600">{top.title.charAt(0)}</div>{/if}
										{#if tilting}<div data-glare class="pointer-events-none absolute inset-0 opacity-0 mix-blend-soft-light"></div>{/if}
									</div>
								</a>
								<div class="min-w-0 space-y-2 self-center">
									<div class="text-[11px] font-bold uppercase tracking-wider text-primary">Top result</div>
									<a href={href(top)} class="block text-xl font-bold hover:text-primary-light tablet:text-2xl laptop:text-4xl">{top.title}</a>
									<div class="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-gray-300">
										{#if top.type === 'person'}{top.dept ?? 'Person'}{:else if top.type === 'collection'}Collection{:else}{top.year || 'TBA'} · {KIND_NAME[top.type]}{#if top.rating} · ★ {top.rating}{/if}{/if}
										{#if label(top)}<span class="rounded bg-sky-500/15 px-1.5 py-0.5 text-[11px] font-semibold text-sky-300 ring-1 ring-sky-500/40">{label(top)}</span>{/if}
									</div>
									{#if top.overview}<p class="line-clamp-2 max-w-2xl text-sm text-gray-400">{top.overview}</p>{/if}
									{#if knownFor.length}<p class="line-clamp-2 max-w-2xl text-sm text-gray-400">Known for {#each knownFor as k, i (k.type + k.id)}{i ? (i === knownFor.length - 1 ? ' and ' : ', ') : ''}<a href={href(k)} class="text-gray-200 hover:text-primary-light">{k.title}</a>{/each}</p>{/if}
									<div class="flex flex-wrap gap-2 pt-1">
										{#if topItem && topItem.id === top.id}<RequestButton item={topItem} />{/if}
										{#if top.type === 'collection' && partsMissing && can('requestMovies')}<a href="/collection/{top.id}?request=1" class="btn act"><Icon name="plus" class="size-4" />Request {partsMissing} missing</a>
										{:else if top.type === 'collection' && partsMissing === 0}<span class="act inline-flex items-center gap-2 rounded-lg bg-green-500/15 font-medium text-green-300 ring-1 ring-green-500/40"><Icon name="check" class="size-4" />All in your library</span>{/if}
										<a href={href(top)} class="btn-ghost act">{top.type === 'person' ? 'View person' : top.type === 'collection' ? 'View collection' : 'Details'}</a>
									</div>
								</div>
							</div>
						</div>
					</div>
				{/if}
				{#if reading}<h2 class="px-4 text-base font-semibold laptop:px-0">{reading}</h2>{/if}
				{#if tab === 'all'}
					{#if !hits.length && !mine.length && !busy}
						<p class="px-4 py-20 text-center text-sm text-gray-400">No results for “{q.trim()}”.</p>
					{:else}
						<div class="grid grid-cols-3 gap-3 px-4 tablet:grid-cols-4 laptop:grid-cols-6 laptop:px-0 desktop:grid-cols-8">
							{#each mixed as h (h.type + h.id)}{@render hit(h, 'w-full')}{/each}
						</div>
					{/if}
				{:else if !kindList.length && kindDone}
					<p class="px-4 py-20 text-center text-sm text-gray-400">No {ROW_NAME[tab].toLowerCase()} for “{q.trim()}”.</p>
				{:else}
					<div class="grid grid-cols-3 gap-3 px-4 tablet:grid-cols-4 laptop:grid-cols-6 laptop:px-0 desktop:grid-cols-8">
						{#each kindList as h (h.type + h.id)}{@render hit(h, 'w-full')}{/each}
					</div>
					<div use:infinite={more}><LoadingMore busy={kind.busy || !kindList.length} done={kindDone} /></div>
				{/if}
			</div>
		</div>
	{:else}
		{#if recent.length}
			<section class="space-y-3 px-4 laptop:px-0">
				<div class="flex items-center justify-between"><h2 class="text-base font-semibold">Recent searches</h2><button class="text-xs text-gray-400 hover:text-white" onclick={clearRecent}>Clear</button></div>
				<div class="flex flex-wrap gap-2">
					{#each recent as r (r)}<button class="rounded-full border border-gray-700/60 bg-gray-800/40 px-3 py-1 text-sm text-gray-300 hover:text-white" onclick={() => (q = r)}>{r}</button>{/each}
				</div>
			</section>
		{/if}
		<section class="space-y-3">
			<h2 class="px-4 text-base font-semibold laptop:px-0">Trending now</h2>
			<div class="grid grid-cols-3 gap-3 px-4 tablet:grid-cols-4 laptop:grid-cols-6 laptop:px-0 desktop:grid-cols-8">
				{#each trending as c (c.type + c.id)}<Poster item={c} class="w-full" />{/each}
			</div>
		</section>
	{/if}
</main>
