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
	import type { Card, Collection, Page, SearchHit, Movie, Series, LikeSource } from '#lib/types.ts';
	import Poster from '#lib/components/Poster.svelte';
	import Chips from '#lib/components/Chips.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import { can } from '#lib/session.svelte.ts';
	import { statusOf } from '#lib/availability.svelte.ts';
	import Img from '#lib/components/Img.svelte';
	import TiltFrame from '#lib/components/TiltFrame.svelte';
	import RequestButton from '#lib/components/RequestButton.svelte';
	import LoadingMore from '#lib/components/LoadingMore.svelte';
	import BrowseTile from '#lib/components/BrowseTile.svelte';
	import MoreLikeBlock from '#lib/components/MoreLikeBlock.svelte';
	import Alert from '#lib/components/Alert.svelte';
	import { infinite } from '#lib/infinite.ts';
	import { releaseKeyboard } from '#lib/keyboard.ts';
	import { loadVocab, loadTiles, pickTiles, buildChips, type Vocab, type Tile, type Chip } from '#lib/searchhelp.ts';

	type Kind = 'movie' | 'series' | 'person';
	type Tab = 'all' | Kind;
	type Owned = { type: 'movie' | 'series'; id: number; title: string; score: number };
	const KINDS: Kind[] = ['movie', 'series', 'person'];

	// From the address bar: on Back, page.url can still describe the page we're leaving while this one is built.
	const startUrl = new URLSearchParams(location.search);
	let q = $state(startUrl.get('q') ?? '');
	let tab = $state<Tab>((KINDS as string[]).includes(startUrl.get('tab') ?? '') ? (startUrl.get('tab') as Kind) : 'all');
	let hits = $state<SearchHit[]>([]); // the mixed search: Top result and the rows
	let parsed = $state<Page<SearchHit>['parsed']>(); // a category or like search: what it's showing (#258)
	let mine = $state<Card[]>([]); // titles in Plex matching the search, even with a typo (#251)
	let busy = $state(false);
	let trending = $state<Card[]>([]);
	let input = $state<HTMLInputElement>();
	// One kind on its own, page by page (#249): the search it was loaded for, its pages so far, and how many there are.
	let kind = $state<{ term: string; tab: Tab; list: SearchHit[]; page: number; pages: number; busy: boolean; parsed?: Page<SearchHit>['parsed'] }>({ term: '', tab: 'all', list: [], page: 0, pages: 1, busy: false });
	// "Search for titles named X instead" (#290): the search it was asked for, and what the category was called.
	let plainFor = $state('');
	let plainFrom = $state('');
	// A like search's title, picked with "Not this one?" or found in Plex for a misspelled one (#291, #293).
	let likePick = $state<{ term: string; src: LikeSource; list: SearchHit[]; pages: number } | null>(null);
	// Everything past the first page on All (#292).
	let extra = $state<{ key: string; list: SearchHit[]; page: Record<string, number>; done: Record<string, boolean>; turn: number; busy: boolean }>({ key: '', list: [], page: {}, done: {}, turn: 0, busy: false });
	// Search help from the API: the words search understands and the Browse tiles (#286, #287).
	let vocab = $state<Vocab | null>(null);
	let tiles = $state<Tile[]>([]);
	loadVocab().then((v) => (vocab = v));
	loadTiles().then((t) => !tiles.length && (tiles = pickTiles(t)));

	const term = $derived(q.trim());
	const plain = $derived(!!term && plainFor === term);

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

	// The empty box cycles through example searches from the API, so people find out what search can do (#286).
	let example = $state(-1);
	$effect(() => {
		const ex = vocab?.examples ?? [];
		if (!ex.length || q) return;
		example = Math.floor(Math.random() * ex.length);
		const t = setInterval(() => (example = (example + 1) % ex.length), 2600);
		return () => clearInterval(t);
	});
	const placeholder = $derived(vocab?.examples?.length && example >= 0 ? `Try “${vocab.examples[example]}”` : 'Search movies, series, people');

	// Recent searches (this device only): saved when you open a result or press Enter (#254).
	const RECENT = 'flixarr_recent_searches';
	let recent = $state<string[]>(JSON.parse(localStorage.getItem(RECENT) ?? '[]'));
	function saveRecent() {
		if (term.length < 2) return;
		recent = [term, ...recent.filter((r) => r.toLowerCase() !== term.toLowerCase())].slice(0, 8);
		localStorage.setItem(RECENT, JSON.stringify(recent));
	}
	function clearRecent() {
		recent = [];
		localStorage.removeItem(RECENT);
	}

	// What's in Plex for some text: the server's matches with how close each is (#293).
	const owned = (text: string) => api<Owned[]>('/plex/search?q=' + encodeURIComponent(text)).catch(() => [] as Owned[]);
	// What's in Plex: the library's own matches (typos too), with their posters from the title pages.
	async function libraryCards(text: string): Promise<Card[]> {
		const found = await owned(text);
		const cards = await Promise.all(found.slice(0, 10).map((t) =>
			meta<Movie | Series>(`${t.type}/${t.id}`).then((d) => ({ id: d.id, type: t.type, title: d.title, poster: d.poster, backdrop: d.backdrop, year: d.year, rating: d.rating, genres: [], overview: d.overview }) as Card).catch(() => null)));
		return cards.filter((c): c is Card => !!c);
	}

	// Live search (#288). Waits a quarter second after typing (Enter skips the wait). The results on screen stay until
	// the new ones are ready, and the API's answer and what's in Plex are shown together, so the Top result doesn't
	// switch when one arrives after the other. Plex gets 0.4 s more; anything slower fills in after.
	let now = $state(''); // a search asked for with Enter: no wait
	$effect(() => {
		const t0 = term, asPlain = plain;
		if (!t0) { hits = []; mine = []; parsed = undefined; busy = false; return; }
		busy = true;
		let alive = true;
		const run = () => {
			const params = { q: t0, collections: 1, plain: asPlain ? 1 : undefined };
			const lib = libraryCards(t0);
			meta<Page<SearchHit>>('search', params, (p) => { if (alive) { hits = p.results; parsed = p.parsed; } })
				.then(async (p) => {
					const own = await Promise.race([lib, new Promise<null>((r) => setTimeout(() => r(null), 400))]);
					if (!alive) return;
					hits = p.results;
					parsed = p.parsed;
					mine = own ?? [];
					if (!own) lib.then((c) => alive && (mine = c));
				})
				.catch(() => {})
				// Only the newest search may say it's done: an older one finishing late would flash "No results" (#245).
				.finally(() => alive && (busy = false));
		};
		const t = setTimeout(run, now === t0 ? 0 : 250);
		return () => { clearTimeout(t); alive = false; };
	});

	// The address keeps the search and the open kind, so Back and See all land on the same view. Only touch
	// the URL when it actually changes: replacing the history entry on arrival would drop its saved snapshot.
	$effect(() => {
		const p = new URLSearchParams();
		if (term) p.set('q', term);
		if (term && tab !== 'all') p.set('tab', tab);
		const url = '/search' + (p.size ? '?' + p : '');
		if (url !== location.pathname + location.search) replaceState(url, {});
	});

	// A kind's next page. Starts over when the search or the kind changed.
	async function more(): Promise<boolean> {
		if (!term || tab === 'all') return false;
		if (kind.term !== term || kind.tab !== tab) kind = { term, tab, list: [], page: 0, pages: 1, busy: false };
		if (kind.busy || kind.page >= kind.pages) return false;
		const want = { term, tab, page: kind.page + 1 };
		kind.busy = true;
		try {
			const p = await meta<Page<SearchHit>>('search', { q: term, type: tab, page: want.page, plain: plain ? 1 : undefined });
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
		if (tab !== 'all' && term && (kind.term !== term || kind.tab !== tab)) {
			const t = setTimeout(more, 250);
			return () => clearTimeout(t);
		}
	});

	// A title's name the way it's compared: lower case, no accents or punctuation, & = and, no leading "the".
	const plainName = (s: string) => {
		const w = s.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase().replace(/&/g, ' and ').replace(/['’]/g, '').split(/[^\p{L}\p{N}]+/u).filter(Boolean);
		if (w.length > 1 && ['the', 'a', 'an'].includes(w[0])) w.shift();
		return w.join('');
	};

	// ---------- like searches (#291, #293) ----------
	// Which kind a like search asked for: "movies like" and "shows like" stay one kind, plain "like" gives both.
	const likeKind = $derived(/^(movies?|films?)\s+(like|similar)/i.test(term) ? 'movie' : /^(shows?|series|tv)\b.*\b(like|similar)/i.test(term) ? 'series' : '');
	const likeSrc = $derived<LikeSource | null>(tab !== 'all' || plain ? null : likePick?.term === term ? likePick.src : (parsed?.like ?? null));
	const likeType = (s: { type: string }) => (s.type === 'series' || s.type === 'tv' ? 'series' : 'movie');
	// Titles like a title, page by page (the API ranks them).
	const likePage = (s: { type: string; id: number }, page: number) =>
		meta<Page<SearchHit>>(`searchhelp/like/${likeType(s)}/${s.id}`, { page, type: likeKind || undefined });
	async function pickLike(h: { type: string; id: number }) {
		const forTerm = term;
		const [d, p] = await Promise.all([meta<Movie | Series>(`${likeType(h)}/${h.id}`), likePage(h, 1)]).catch(() => [null, null] as const);
		if (!d || !p || forTerm !== term) return;
		likePick = { term: forTerm, src: { id: d.id, type: likeType(h), title: d.title, year: d.year, poster: d.poster, backdrop: d.backdrop, overview: d.overview, rating: d.rating }, list: p.results, pages: p.totalPages };
	}
	// A like search whose title the API couldn't find: one in Plex that's a real typo match (not a loose one) is
	// taken instead; otherwise the page says so (#293).
	let missChecked = $state('');
	$effect(() => {
		const miss = parsed?.likeMiss, forTerm = term;
		if (!miss || tab !== 'all' || plain || likePick?.term === forTerm) return;
		missChecked = '';
		let alive = true;
		owned(miss).then((found) => {
			if (!alive) return;
			const best = found.find((f) => f.score >= 40);
			if (best) pickLike(best);
			else missChecked = forTerm;
		});
		return () => { alive = false; };
	});
	const likeMissShown = $derived(!!parsed?.likeMiss && !likeSrc && missChecked === term);
	// "Not this one?": other titles with the same name as the one the like search found.
	let alts = $state<SearchHit[]>([]);
	$effect(() => {
		const base = parsed?.like, cur = likeSrc;
		alts = [];
		if (!base || !cur) return;
		let alive = true;
		meta<Page<SearchHit>>('search', { q: base.title, type: likeType(base), plain: 1 }).then((p) => {
			if (!alive) return;
			alts = p.results.filter((h) => plainName(h.title).startsWith(plainName(base.title)) && !(h.id === cur.id && likeType(h) === likeType(cur))).slice(0, 5);
			const orig = { ...base, type: likeType(base) } as unknown as SearchHit;
			if (!(cur.id === base.id) && !alts.some((a) => a.id === base.id)) alts = [orig, ...alts].slice(0, 5);
		}).catch(() => {});
		return () => { alive = false; };
	});

	// ---------- the Top result ----------
	// The best match. A title in Plex wins, even with a typo (#251), unless the search found the exact title,
	// a person or collection, or read the search a special way (a nickname, a year, a season, a pasted ID).
	const top = $derived.by<SearchHit | undefined>(() => {
		if (likeSrc || parsed?.likeMiss) return undefined; // a like search shows its MORE LIKE block instead
		const best = hits[0], own = mine[0] as SearchHit | undefined;
		if (!best || !own || parsed?.browse) return best ?? own;
		const special = parsed?.id || parsed?.nick || parsed?.year || parsed?.season;
		if (special || best.type === 'person' || best.type === 'collection' || plainName(best.title) === plainName(q)) return best;
		return own;
	});
	const topKey = $derived(top ? top.type + top.id : '');
	// Everything else in one mixed grid, in ranked order (Marc: "I want them mixed in"). Titles only Plex
	// matched (a typo like brkng bad) go first (#251); the Top result isn't repeated.
	const mixed = $derived.by(() => {
		if (likeSrc) {
			const list = likePick?.term === term ? likePick.list : hits;
			return list.filter((h) => !(h.id === likeSrc.id && likeType(h) === likeType(likeSrc)));
		}
		if (parsed?.likeMiss) return [];
		const seen = new Set<string>([topKey]);
		const out: SearchHit[] = [];
		// A category search isn't a title, so the library's name matches don't belong in it.
		const plainSearch = !parsed?.browse;
		const own = plainSearch ? (mine as SearchHit[]) : [];
		// The Top result's franchise first: the collection, then its movies in release order (#273), so a search
		// like fast furious lists the films in order instead of mixed in with every title that shares the words.
		const franchise: SearchHit[] = [];
		if (plainSearch && topColl && topParts && topParts.length > 1) {
			const card = hits.find((h) => h.type === 'collection' && h.id === topColl.id);
			if (card) franchise.push(card);
			franchise.push(...([...topParts].sort((a, b) => (a.year || '9999').localeCompare(b.year || '9999')) as SearchHit[]));
		}
		for (const h of [...franchise, ...own, ...hits]) {
			const k = h.type + h.id;
			if (!seen.has(k)) { seen.add(k); out.push(h); }
		}
		return out;
	});
	const ROW_NAME = { movie: 'Movies', series: 'Series', person: 'People' };
	const kindList = $derived(kind.term === term && kind.tab === tab ? kind.list.filter((h) => h.type + h.id !== topKey) : []);
	const kindDone = $derived(kind.term === term && kind.tab === tab && kind.page >= kind.pages);

	// The title page of the Top result or the like search's title, for its Request or Play button (#248).
	let topItem = $state<Movie | Series | null>(null);
	const subject = $derived(likeSrc ? { type: likeType(likeSrc), id: likeSrc.id } : top && (top.type === 'movie' || top.type === 'series') ? { type: top.type, id: top.id } : null);
	$effect(() => {
		const h = subject;
		topItem = null;
		if (!h) return;
		let alive = true;
		meta<Movie | Series>(`${h.type}/${h.id}`).then((d) => alive && (topItem = d)).catch(() => {});
		return () => { alive = false; };
	});

	// The Top result's collection: the collection itself, or the one a movie Top result belongs to (#267). Its
	// movies say how many are missing (same rule as the collection page).
	const topColl = $derived.by<{ id: number; name: string } | null>(() => {
		if (top?.type === 'collection') return { id: top.id, name: top.title };
		const c = top?.type === 'movie' && topItem?.id === top.id && 'collection' in topItem ? topItem.collection : null;
		return c ? { id: c.id, name: c.name } : null;
	});
	// "Part of the Toy Story Collection", but "Part of The Avengers Collection".
	const theName = (name: string) => (name.toLowerCase().startsWith('the ') ? '' : 'the ');
	let topParts = $state<Card[] | null>(null);
	$effect(() => {
		const id = topColl?.id;
		topParts = null;
		if (!id) return;
		let alive = true;
		meta<Collection>(`collection/${id}`).then((d) => alive && (topParts = d.parts)).catch(() => {});
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
	const KIND_NAME = { movie: 'Movie', series: 'Series', person: 'Person', collection: 'Collection' };
	// A person as the Top result: what they're best known for (#258), the same while their name is typed.
	// The person page's pick (it counts lead TV roles like The Office). The search's own list only if that fails, so
	// the line never changes under the same person.
	type Known = { id: number; type: 'movie' | 'series'; title: string };
	let pageKnown = $state<{ id: number; known: Known[] } | null>(null);
	$effect(() => {
		const id = top?.type === 'person' ? top.id : 0;
		if (!id || pageKnown?.id === id) return;
		let alive = true;
		const own = top?.known ?? [];
		meta<{ known?: Known[] }>(`person/${id}`).then((d) => alive && (pageKnown = { id, known: d.known?.length ? d.known : own })).catch(() => alive && (pageKnown = { id, known: own }));
		return () => { alive = false; };
	});
	const knownFor = $derived<Known[]>(top?.type === 'person' && pageKnown?.id === top.id ? pageKnown.known : []);

	// ---------- the category reading (#290) ----------
	const kindParsed = $derived(kind.term === term && kind.tab === tab ? kind.parsed : undefined);
	const browseNow = $derived(plain ? undefined : tab === 'all' ? parsed?.browse : kindParsed?.browse);
	// A like reading on the Movies or Series tab keeps its plain heading (the MORE LIKE block is on All).
	const likeHeading = $derived(!plain && tab !== 'all' && kindParsed?.like ? `More like ${kindParsed.like.title}${kindParsed.like.year ? ` (${kindParsed.like.year})` : ''}` : '');
	const boxTag = $derived(plain ? '' : browseNow ? 'Category' : likeSrc ? 'More like' : '');
	function titlesInstead() {
		plainFrom = browseNow?.label ?? (likeSrc ? `titles like ${likeSrc.title}` : '');
		plainFor = term;
	}
	const fmtNum = (n: number) => n.toLocaleString('en-US');

	// ---------- search chips (#287) ----------
	// A half-typed like search ("movies like incep"): titles in Plex for the name so far, real typo matches only.
	let ownedForChips = $state<{ title: string }[]>([]);
	$effect(() => {
		const m = term.toLowerCase().match(/^(?:movies like|shows like|something like|similar to|like)\s+(.{2,})$/);
		ownedForChips = [];
		if (!m) return;
		let alive = true;
		const t = setTimeout(() => owned(m[1]).then((f) => alive && (ownedForChips = f.filter((x) => x.score >= 40))), 150);
		return () => { alive = false; clearTimeout(t); };
	});
	const chips = $derived<Chip[]>(buildChips(q, vocab, { top: top ?? (likeSrc ? ({ title: likeSrc.title, type: 'movie', id: likeSrc.id, poster: null } as SearchHit) : undefined), hits, owned: ownedForChips, reading: browseNow?.label ?? '' }));
	const CHIP_ICON: Record<Chip['kind'], string> = { Category: 'browse', 'More like': 'film', Collection: 'layers', Nickname: 'tag', Person: 'profile' };
	function search(text: string) {
		q = text;
		tab = 'all';
		now = text.trim(); // a picked search runs right away
		input?.blur();
	}

	// ---------- endless results on All (#292) ----------
	const moreKey = $derived(`${term}|${plain}|${likeSrc ? likeType(likeSrc) + likeSrc.id : ''}|${parsed?.browse?.label ?? ''}`);
	$effect(() => {
		if (extra.key !== moreKey) extra = { key: moreKey, list: [], page: {}, done: {}, turn: 0, busy: false };
	});
	const canMore = $derived(tab === 'all' && !!term && !parsed?.id && !parsed?.likeMiss && mixed.length > 0);
	async function moreAll(): Promise<boolean> {
		if (!canMore || extra.busy || extra.key !== moreKey) return false;
		const key = moreKey, src = likeSrc, browse = parsed?.browse;
		// A like search goes on through its ranked list; a category in its own kind; a title search takes turns
		// between more movies and more series.
		const kinds = src ? ['like'] : browse ? [browse.type === 'series' ? 'series' : 'movie'] : ['movie', 'series'];
		const open = kinds.filter((k) => !extra.done[k]);
		if (!open.length) return false;
		const k = open[extra.turn % open.length];
		const first = src ? (likePick?.term === term ? 2 : 3) : browse ? 3 : 2;
		const page = extra.page[k] ?? first;
		extra.busy = true;
		try {
			const p = src ? await likePage(src, page) : await meta<Page<SearchHit>>('search', { q: term, type: k, page, plain: plain ? 1 : undefined });
			if (extra.key !== key) return false;
			const seen = new Set([...mixed, ...extra.list].map((h) => h.type + h.id));
			if (top) seen.add(topKey);
			extra.list = [...extra.list, ...p.results.filter((h) => !seen.has(h.type + h.id) && (seen.add(h.type + h.id), true))];
			extra.page = { ...extra.page, [k]: page + 1 };
			if (page >= p.totalPages || !p.results.length) extra.done = { ...extra.done, [k]: true };
			extra.turn++;
			return kinds.some((x) => !extra.done[x]);
		} catch {
			extra.done = { ...extra.done, [k]: true };
			return false;
		} finally {
			extra.busy = false;
		}
	}
	const allDone = $derived(!canMore || (extra.key === moreKey && Object.keys(extra.done).length > 0 && (likeSrc ? !!extra.done.like : parsed?.browse ? !!extra.done[parsed.browse.type === 'series' ? 'series' : 'movie'] : !!extra.done.movie && !!extra.done.series)));
	const grid = $derived(extra.key === moreKey ? [...mixed, ...extra.list] : mixed);

	// ---------- keyboard (#289) ----------
	// Down from the box goes to the chips, then the Top result, then the results card by card; left and right move
	// one card, up and down a row (by where cards are on screen). Up from the top row goes back.
	let page = $state<HTMLElement>();
	function nav(e: KeyboardEvent) {
		if (!['ArrowDown', 'ArrowUp', 'ArrowLeft', 'ArrowRight'].includes(e.key) || !page) return;
		const a = document.activeElement as HTMLElement | null;
		if (!a || !page.contains(a)) return;
		const cs = [...page.querySelectorAll<HTMLElement>('[data-chip]')];
		const ks = [...page.querySelectorAll<HTMLElement>('[data-top-link], [data-results] > a, [data-results] > button')];
		const go = (el?: HTMLElement | null) => { if (!el) return; e.preventDefault(); el.focus(); el.scrollIntoView({ block: 'nearest' }); };
		const row = (list: HTMLElement[], cur: HTMLElement, dir: number) => {
			const top = (el: HTMLElement) => Math.round(el.getBoundingClientRect().top);
			const rows = [...new Set(list.map(top))].sort((x, y) => x - y);
			const want = rows[rows.indexOf(top(cur)) + dir];
			if (want === undefined) return null;
			const left = cur.getBoundingClientRect().left;
			return list.filter((el) => top(el) === want).sort((x, y) => Math.abs(x.getBoundingClientRect().left - left) - Math.abs(y.getBoundingClientRect().left - left))[0];
		};
		if (a === input) {
			if (e.key === 'ArrowDown') go(cs[0] ?? ks[0]);
		} else if (a && cs.includes(a)) {
			const n = cs.indexOf(a);
			if (e.key === 'ArrowRight') go(cs[n + 1]);
			else if (e.key === 'ArrowLeft') go(cs[n - 1] ?? input);
			else if (e.key === 'ArrowUp') go(input);
			else go(ks[0]);
		} else if (a && ks.includes(a)) {
			const n = ks.indexOf(a);
			if (e.key === 'ArrowRight') go(ks[n + 1]);
			else if (e.key === 'ArrowLeft') go(ks[n - 1]);
			else if (e.key === 'ArrowDown') go(row(ks, a, 1));
			else go(row(ks, a, -1) ?? cs[0] ?? input);
		}
	}

	// Going back to search brings back the results, the kind you picked, the Browse tiles, the like search's title,
	// the extra pages and where you'd scrolled to (Rule 35).
	snapshot({
		id: 'search',
		capture: () => ({ q, tab, hits: $state.snapshot(hits), mine: $state.snapshot(mine), parsed: $state.snapshot(parsed), kind: $state.snapshot(kind), tiles: $state.snapshot(tiles), plainFor, plainFrom, likePick: $state.snapshot(likePick), extra: $state.snapshot(extra), y: scrollY }),
		restore: async (v) => {
			q = v.q;
			tab = v.tab;
			hits = v.hits;
			mine = v.mine ?? [];
			parsed = v.parsed;
			if (v.kind) kind = { ...v.kind, busy: false };
			if (v.tiles?.length) tiles = v.tiles;
			plainFor = v.plainFor ?? '';
			plainFrom = v.plainFrom ?? '';
			likePick = v.likePick ?? null;
			if (v.extra) extra = { ...v.extra, busy: false };
			await tick();
			restoreScroll(v.y);
		}
	});
</script>

{#snippet person(h: SearchHit, cls: string)}
	<a href="/person/{h.id}" class="group relative block {cls}">
		<TiltFrame class="aspect-[2/3] w-full rounded-xl border border-gray-700/40 bg-gray-800/40 shadow-xl">
			{#if h.poster}<Img path={h.poster} kind="profile" sizes="(min-width: 640px) 144px, 112px" class="h-full w-full" />{:else}<div class="grid h-full place-items-center text-3xl text-gray-600">{h.title.charAt(0)}</div>{/if}
		</TiltFrame>
		<div class="relative">
			<div class="mt-2 truncate text-[13px] font-medium">{h.title}</div>
			<div class="truncate text-xs text-gray-400">{h.dept ?? ''}</div>
		</div>
	</a>
{/snippet}

{#snippet collection(h: SearchHit, cls: string)}
	<a href="/collection/{h.id}" class="group relative block {cls}">
		<TiltFrame class="aspect-[2/3] w-full rounded-xl border border-gray-700/40 bg-gray-800/40 shadow-xl">
			{#if h.poster}<Img path={h.poster} kind="poster" sizes="(min-width: 640px) 144px, 112px" class="h-full w-full" />{:else}<div class="grid h-full place-items-center text-3xl text-gray-600">{h.title.charAt(0)}</div>{/if}
			<div class="absolute left-1.5 top-1.5 z-10"><span class="rounded-md bg-violet-600/85 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide">Collection</span></div>
		</TiltFrame>
		<div class="relative">
			<div class="mt-2 truncate text-[13px] font-medium">{h.title}</div>
			<div class="truncate text-xs text-gray-400">Collection</div>
		</div>
	</a>
{/snippet}

{#snippet hit(h: SearchHit, cls: string)}
	{#if h.type === 'person'}{@render person(h, cls)}{:else if h.type === 'collection'}{@render collection(h, cls)}{:else}<Poster item={h as Card} class={cls} tag={label(h)} />{/if}
{/snippet}

<svelte:window onkeydown={nav} />
<main bind:this={page} class="mx-auto max-w-[1600px] space-y-6 pt-20 laptop:px-10 laptop:pt-24">
	<div class="px-4 laptop:px-0">
		<label class="card flex items-center gap-3 px-4 py-3.5 text-gray-400 focus-within:ring-2 focus-within:ring-primary/60">
			<Icon name="search" class="size-5 shrink-0" />
			<input bind:this={input} bind:value={q} autocomplete="off" enterkeyhint="search" onkeydown={(e) => { if (e.key === 'Enter') { now = term; saveRecent(); input?.blur(); } }} class="min-w-0 flex-1 bg-transparent text-base text-white placeholder-gray-500 outline-none" {placeholder} />
			{#if boxTag}
				<!-- How the search is being read; the x searches for titles instead (#290). -->
				<span class="inline-flex shrink-0 items-center gap-1 rounded-md bg-primary/15 py-0.5 pl-2 pr-1 text-xs font-semibold text-primary-light ring-1 ring-primary/40">{boxTag}<button aria-label="Search titles instead" class="rounded p-0.5 hover:bg-primary/30" onclick={(e) => { e.preventDefault(); titlesInstead(); }}><Icon name="close" class="size-3" /></button></span>
			{/if}
			{#if q}<button class="shrink-0 text-xs text-gray-400 hover:text-white" onclick={() => { q = ''; tab = 'all'; plainFor = ''; likePick = null; input?.focus(); }}>Clear</button>
			{:else}<kbd class="hidden rounded border border-gray-600 px-1.5 text-[11px] text-gray-500 laptop:inline">/</kbd>{/if}
		</label>
		{#if term && chips.length}
			<!-- Search chips: the smart searches that finish what's typed (#287). -->
			<div class="no-scrollbar mt-3 flex gap-2 overflow-x-auto laptop:flex-wrap">
				{#each chips as c (c.q)}
					<button data-chip onclick={() => search(c.q)} class="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-primary/30 bg-primary/10 py-1 pl-2.5 pr-3 text-sm text-white outline-none transition hover:border-primary/60 focus-visible:border-primary focus-visible:bg-primary/25">
						<Icon name={CHIP_ICON[c.kind]} class="size-3.5 text-primary-light" />{c.label}
					</button>
				{/each}
			</div>
		{/if}
	</div>
	{#if term}
		<Chips bind:value={tab} options={[{ key: 'all', label: 'All' }, { key: 'movie', label: 'Movies' }, { key: 'series', label: 'Series' }, { key: 'person', label: 'People' }]} />
		<!-- Opening a result saves the search to Recent (clicks bubble up from the links). -->
		<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
		<div onclick={(e) => (e.target as Element).closest('a') && saveRecent()}>
			<div class="space-y-6">
				{#if plain && plainFrom}
					<p class="px-4 text-sm text-gray-400 laptop:px-0">Titles named “{term}”. <button class="text-primary-light hover:text-white" onclick={() => (plainFor = '')}>Show {plainFrom.charAt(0).toLowerCase() + plainFrom.slice(1)} instead</button></p>
				{/if}
				{#if likeSrc && tab === 'all'}
					<MoreLikeBlock src={likeSrc} item={topItem} {alts} onpick={pickLike} />
				{:else if parsed?.likeMiss && tab === 'all' && likeMissShown}
					<div class="px-4 laptop:px-0"><Alert tone="yellow">Couldn’t find a title called “{parsed.likeMiss}”. Check the spelling, or search for the title on its own.</Alert></div>
				{:else if top}
					<!-- The best match, big, with its button (#248). -->
					<div class="px-4 laptop:px-0">
						<div class="relative overflow-hidden rounded-2xl ring-1 ring-gray-700/50">
							{#if top.backdrop}<Img path={top.backdrop} kind="backdrop" sizes="100vw" class="absolute inset-0 h-full w-full opacity-30" />{/if}
							<div class="absolute inset-0 bg-gradient-to-r from-gray-950 via-gray-950/80 to-transparent"></div>
							<div class="relative flex gap-4 p-4 tablet:gap-5 tablet:p-5 laptop:p-7">
								<a href={href(top)} data-top-link class="group w-24 shrink-0 self-start rounded-xl outline-none focus-visible:ring-2 focus-visible:ring-primary tablet:w-32 laptop:w-40">
									<TiltFrame class="aspect-[2/3] rounded-xl bg-gray-800 shadow-2xl ring-1 ring-white/10">
										{#if top.poster}<Img path={top.poster} kind={top.type === 'person' ? 'profile' : 'poster'} sizes="160px" class="h-full w-full" />{:else}<div class="grid h-full place-items-center text-3xl text-gray-600">{top.title.charAt(0)}</div>{/if}
									</TiltFrame>
								</a>
								<div class="min-w-0 space-y-2 self-center">
									<div class="text-[11px] font-bold uppercase tracking-wider text-primary">Top result</div>
									<a href={href(top)} class="block text-xl font-bold hover:text-primary-light tablet:text-2xl laptop:text-4xl">{top.title}</a>
									<div class="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-gray-300">
										{#if top.type === 'person'}{top.dept ?? 'Person'}{:else if top.type === 'collection'}Collection{:else}{top.year || 'TBA'} · {KIND_NAME[top.type]}{#if top.rating} · ★ {top.rating}{/if}{/if}
										{#if label(top)}<span class="rounded px-1.5 py-0.5 text-[11px] font-bold uppercase tracking-wide text-gray-950 {label(top) === 'New' ? 'bg-green-400' : 'bg-amber-400'}">{label(top)}</span>{/if}
									</div>
									{#if top.overview}<p class="line-clamp-2 max-w-2xl text-sm text-gray-400">{top.overview}</p>{/if}
									{#if top.type === 'movie' && topColl && topParts && topParts.length > 1 && partsMissing !== undefined}
										<p class="text-sm text-gray-400">Part of {theName(topColl.name)}<a href="/collection/{topColl.id}" class="text-gray-200 hover:text-primary-light">{topColl.name}</a>
											{#if partsMissing && can('requestMovies')} · <a href="/collection/{topColl.id}?request=1" class="font-medium text-primary-light hover:text-white">Request {partsMissing} missing</a>
											{:else if partsMissing === 0} · <span class="text-green-300">All in your library</span>{/if}</p>
									{/if}
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
				{#if browseNow}
					<!-- A category search: what it's showing, and a way to search for titles with that name instead (#290). -->
					<div class="flex flex-col gap-1 px-4 laptop:flex-row laptop:items-end laptop:justify-between laptop:gap-6 laptop:px-0">
						<div>
							<div class="text-[11px] font-bold uppercase tracking-wider text-primary">Category</div>
							<h2 class="text-xl font-bold text-white laptop:text-2xl">{browseNow.label}{#if browseNow.total}<span class="ml-2 text-sm font-normal text-gray-400">{fmtNum(browseNow.total)} titles</span>{/if}</h2>
						</div>
						<button class="text-left text-sm text-gray-400 hover:text-white" onclick={titlesInstead}>Search for titles named “{term}” instead <Icon name="arrow" class="inline size-3.5 align-[-2px]" /></button>
					</div>
				{:else if likeHeading}
					<h2 class="px-4 text-base font-semibold laptop:px-0">{likeHeading}</h2>
				{/if}
				{#if tab === 'all'}
					{#if likeMissShown}
						<!-- nothing to list: the yellow box above says why -->
					{:else if parsed?.likeMiss && !likeSrc}
						<LoadingMore busy={true} done={false} />
					{:else if !grid.length && !mine.length && !busy}
						<p class="px-4 py-20 text-center text-sm text-gray-400">No results for “{term}”.</p>
					{:else}
						<div data-results class="grid grid-cols-3 gap-3 px-4 tablet:grid-cols-4 laptop:grid-cols-6 laptop:px-0 desktop:grid-cols-8">
							{#each grid as h (h.type + h.id)}{@render hit(h, 'w-full')}{/each}
						</div>
						{#if canMore}<div use:infinite={moreAll}><LoadingMore busy={extra.busy} done={allDone} /></div>{/if}
					{/if}
				{:else if !kindList.length && kindDone}
					<p class="px-4 py-20 text-center text-sm text-gray-400">No {ROW_NAME[tab].toLowerCase()} for “{term}”.</p>
				{:else}
					<div data-results class="grid grid-cols-3 gap-3 px-4 tablet:grid-cols-4 laptop:grid-cols-6 laptop:px-0 desktop:grid-cols-8">
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
		{#if tiles.length}
			<!-- Browse: smart searches to start from, a fresh 10 each visit (#286). -->
			<section class="space-y-3 px-4 laptop:px-0">
				<h2 class="text-base font-semibold">Browse</h2>
				<div data-results class="grid grid-cols-2 gap-3 tablet:grid-cols-3 laptop:grid-cols-5">
					{#each tiles as t (t.q)}<BrowseTile tile={t} onpick={search} />{/each}
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

<style>
	/* Arrow keys scroll a picked chip or card into view below the top bar, not under it (#289). */
	:global([data-results] > *),
	:global([data-chip]),
	:global([data-top-link]) {
		scroll-margin-top: 6rem;
		scroll-margin-bottom: 5rem;
	}
	/* A result picked with the arrow keys (#289). */
	:global([data-results] > a:focus-visible) {
		outline: 2px solid var(--color-primary);
		outline-offset: 4px;
		border-radius: 0.75rem;
	}
</style>
