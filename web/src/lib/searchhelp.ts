// Search help (#286, #287): the search chips under the box, the example searches in it and the Browse tiles on the
// empty search page. Every word and tile comes from the Flixarr API (/v1/searchhelp/*); this file only knows how
// to put them together, so nothing here names a title, a genre or a country.
import { meta } from '#lib/api.ts';
import type { SearchHit } from '#lib/types.ts';

type Entry = { words: string[]; label: string; phrase: string; movie?: boolean; series?: boolean; decades?: string[]; with?: string[] };
export interface Vocab {
	genres: Entry[];
	themes: Entry[];
	places: Entry[];
	nicknames: { nick: string; title: string }[];
	kinds: { movie: string[]; series: string[] };
	like: string[];
	examples: string[];
}
export interface Tile { q: string; label: string; kind: 'Category' | 'More like' | 'Collection'; backdrop: string; posters: string[]; season?: string }
export type ChipKind = 'Category' | 'More like' | 'Collection' | 'Nickname' | 'Person';
export interface Chip { q: string; kind: ChipKind; label: string }

let vocabP: Promise<Vocab | null> | null = null;
/** The words search understands (once per visit; the app's cache keeps it like a list). */
// The number after vocab/ and browse/ only skips copies kept from before (the API ignores it): raise it when what
// the app reads from them changes.
export const loadVocab = () => (vocabP ??= meta<Vocab>('searchhelp/vocab/2').catch(() => ((vocabP = null), null)));
/** The Browse tile pool (seasonal tiles only in season). */
export const loadTiles = () => meta<{ tiles: Tile[] }>('searchhelp/browse/2').then((d) => d.tiles).catch(() => [] as Tile[]);

/** A fresh 10 for the empty search page: 6 categories, 2 like searches and 2 collections, mixed up. */
export function pickTiles(all: Tile[]): Tile[] {
	const shuffle = <T>(l: T[]) => l.map((x) => [Math.random(), x] as const).sort((a, b) => a[0] - b[0]).map(([, x]) => x);
	const of = (k: Tile['kind'], n: number) => shuffle(all.filter((t) => t.kind === k)).slice(0, n);
	// Something for the season, when there is one, is always among the categories.
	const season = shuffle(all.filter((t) => t.season)).slice(0, 1);
	const cats = [...season, ...of('Category', 6).filter((t) => !season.includes(t))].slice(0, 6);
	return shuffle([...cats, ...of('More like', 2), ...of('Collection', 2)]);
}

const lower = (s: string) => s.toLowerCase().replace(/\s+/g, ' ').trim();
const DECADE = /^((?:19|20)?\d0)s?(?:\s+(.*))?$/;
const LIKE = /^(movies like|shows like|something like|similar to|like)(?:\s+(.*))?$/;

/** The categories in the order people reach for them: the API's example searches lead (comedies, horror...). */
function categories(v: Vocab): Entry[] {
	const all = [...v.genres.filter((g) => g.movie), ...v.themes];
	const rank = (e: Entry) => {
		const i = v.examples.findIndex((x) => e.words.some((w) => ` ${x} `.includes(` ${w} `)));
		return i < 0 ? 999 : i;
	};
	return all.map((e, i) => ({ e, i })).sort((a, b) => rank(a.e) - rank(b.e) || a.i - b.i).map(({ e }) => e);
}

/** Is every word a word search reads as part of a category (a decade, "new", a genre, theme or place, a kind)? */
function known(v: Vocab, word: string): boolean {
	if (DECADE.test(word) || ['new', 'latest', 'recent'].includes(word)) return true;
	const all = [...v.genres, ...v.themes, ...v.places];
	return all.some((e) => e.words.includes(word)) || v.kinds.movie.includes(word) || v.kinds.series.includes(word);
}

/** "60s" → 1960, "00s" → 2000, "2010s" → 2010. */
const decadeYear = (d: string) => { const n = Number(d.replace(/s$/, '')); return n >= 100 ? n : n >= 30 ? 1900 + n : 2000 + n; };

/**
 * Chips for a category (Marc, 2026-11-27: not just decades, and more decades only where they fit). `base` is the
 * search as typed or completed. In order: its best decades (three for a category that was biggest before 1980,
 * two before 2000, one after: westerns get 60s, 50s, 70s; horror 80s, 70s), the genres it most goes with
 * (western comedies, horror thrillers), the same as shows when it has them, and a new one. All from the API.
 */
function categoryChips(e: Entry, base: string, t: string, add: (q: string, kind: ChipKind, label?: string) => void) {
	const plainBase = base.replace(/^(new|latest|recent) /, '');
	const decades = e.decades ?? [];
	const era = decades.length ? decadeYear(decades[0]) : 2020;
	const n = era < 1980 ? 3 : era < 2000 ? 2 : 1;
	// "korean horror" keeps its "korean"; a plain "westerns" goes back to the word "western".
	const core = plainBase.replace(/ (movies?|films?|shows?|series|tv)$/, '');
	const alone = core === e.label.toLowerCase() || e.words.includes(core);
	const lead = alone ? e.label.toLowerCase() : core;
	const shows = /\b(shows?|series|tv)\b/.test(t), kinded = shows || /\b(movies?|films?)\b/.test(t);
	// The decades are worked out from the category's movies on their own, so they're only offered for exactly that
	// (not "50s history shows" or "70s korean horror", which can come back nearly empty).
	if (alone && !shows && !DECADE.test(t.split(' ')[0])) for (const d of decades.slice(0, n)) add(`${d} ${plainBase}`, 'Category');
	// The genres it goes with are movie phrases ("comedies"), so a search for shows doesn't get them.
	if (!shows) for (const w of (e.with ?? []).slice(0, 2)) add(`${lead} ${w}`, 'Category');
	if (!kinded && e.series) add(`${lead} shows`, 'Category');
	// Shows: the movies of it instead (TMDB has few shows per decade or for "new", so those come back nearly empty).
	if (shows && e.movie !== false) add(`${lead} movies`, 'Category');
	else if (plainBase === base) add(`new ${base}`, 'Category');
}

/**
 * The search chips for what's typed. ctx: the Top result and the results on screen (collections and people that
 * aren't the Top result get a chip), titles in Plex matching a half-typed like search, and the current reading,
 * so a chip never repeats the Top result or the category already shown.
 */
export function buildChips(raw: string, v: Vocab | null, ctx: { top?: SearchHit; hits: SearchHit[]; owned: { title: string }[]; reading: string }): Chip[] {
	const t = lower(raw);
	if (!t || !v) return [];
	const out: Chip[] = [];
	const add = (q: string, kind: ChipKind, label = q) => {
		if (out.length >= 6 || out.some((c) => c.q === q || c.label.toLowerCase() === label.toLowerCase())) return;
		if (q === t && kind !== 'Nickname' && kind !== 'Collection') return;
		if (label.toLowerCase() === (ctx.top?.title ?? '').toLowerCase() || label.toLowerCase() === ctx.reading.toLowerCase()) return;
		out.push({ q, kind, label });
	};
	const cats = categories(v);

	// A nickname: what it stands for.
	const nick = v.nicknames.find((n) => n.nick === t);
	if (nick) add(t, 'Nickname', nick.title);

	// A like search: a title in Plex for the half-typed name, then examples that start the same way.
	const like = t.match(LIKE);
	if (like) {
		const rest = like[2] ?? '';
		if (rest.length >= 2) for (const o of ctx.owned.slice(0, 3)) add(`${like[1]} ${o.title.toLowerCase()}`, 'More like', `${like[1]} ${o.title}`);
		for (const x of v.examples) if (x.startsWith(t) && x !== t) add(x, 'More like');
	}

	// A decade: that decade with the most asked-for categories, or the ones that start with what follows it.
	const dec = !like && t.match(DECADE);
	if (dec) {
		const d = dec[1] + 's', rest = dec[2] ?? '';
		for (const e of cats) if (!rest || e.phrase.startsWith(rest) || e.words.some((w) => w.startsWith(rest))) add(`${d} ${e.phrase}`, 'Category');
	}

	// Words that are part of a category, with the last one still being typed ("korean hor" → korean horror movies).
	if (!like && !dec) {
		const words = t.split(' ');
		const last = words.pop() ?? '';
		const before = words.join(' ');
		if (last.length >= 2 && words.every((w) => known(v, w))) {
			const found = [...cats, ...v.places].filter((e) => e.words.some((w) => w.startsWith(last)));
			// The best match gets its strongest decades from the API ("westerns": 60s, 50s, 70s westerns) and a
			// "new" one; other matches just their own chip.
			const best = found[0];
			if (best && !before) {
				add(best.phrase, 'Category');
				categoryChips(best, best.words.includes(last) ? t : best.phrase, t, add);
			}
			for (const e of found) add(before ? `${before} ${e.phrase}` : e.phrase, 'Category');
		}
	}

	// From the results: titles like the Top result, a collection or a person that isn't the Top result.
	const top = ctx.top;
	// A collection as the Top result: titles like its first movie.
	const first = top?.type === 'collection' ? ctx.hits.find((h) => h.type === 'movie') : undefined;
	if (!like && first) add(`movies like ${first.title.toLowerCase()}`, 'More like', `Movies like ${first.title}`);
	if (!like && !ctx.reading && top && (top.type === 'movie' || top.type === 'series')) add(`${top.type === 'movie' ? 'movies' : 'shows'} like ${top.title.toLowerCase()}`, 'More like', `${top.type === 'movie' ? 'Movies' : 'Shows'} like ${top.title}`);
	for (const h of ctx.hits.slice(0, 12)) {
		if (h === top) continue;
		// A collection whose name has a note in brackets ("(Animated)") can't be found again by name: no chip.
		if (h.type === 'collection' && !/[()]/.test(h.title)) add(`${h.title.replace(/ collection$/i, '').toLowerCase()} collection`, 'Collection', h.title);
		// A person only once more than their first name is typed ("tom h"), and only one: "harry" alone matches many.
		else if (h.type === 'person' && t.includes(' ') && lower(h.title).startsWith(t) && !out.some((c) => c.kind === 'Person')) add(lower(h.title), 'Person', h.title);
	}
	// Topped up to three with example searches from the API, but only ones like this search (Marc, 2026-11-27:
	// "shows like the office" never gets "90s christmas movies"): more like searches of the same kind, or categories
	// sharing a word. A search with nothing at all gets plain examples. The same picks for the same text, so they
	// don't change while the results load.
	let seed = [...t].reduce((n, c) => (n * 31 + c.charCodeAt(0)) >>> 0, 7);
	const pick = (pool: string[], n: number) => {
		for (let i = 0; out.length < n && pool.length && i < pool.length * 2; i++) {
			seed = (seed * 1103515245 + 12345) >>> 0;
			const x = pool[seed % pool.length];
			add(x, LIKE.test(x) ? 'More like' : 'Category');
		}
	};
	const words = t.split(' ').filter((w) => w.length > 2 && !['movies', 'shows', 'like', 'the'].includes(w));
	if (like) {
		// The closest: the same kind of search for the titles it found ("shows like the office" → shows like Parks
		// and Recreation, shows like Superstore).
		const cap = like[1].charAt(0).toUpperCase() + like[1].slice(1);
		for (const h of ctx.hits) {
			if (out.length >= 3) break;
			if ((h.type === 'movie' || h.type === 'series') && h !== top) add(`${like[1]} ${h.title.toLowerCase()}`, 'More like', `${cap} ${h.title}`);
		}
		pick(v.examples.filter((x) => LIKE.test(x) && x.startsWith(like[1].split(' ')[0])), 3);
	} else if (dec || ctx.reading) {
		// A category already shown ("history shows", "westerns"): the same idea from the category's own data.
		if (ctx.reading && !dec && out.length < 3) {
			const e = [...cats, ...v.places].find((x) => x.words.some((w) => ` ${t} `.includes(` ${w} `)));
			if (e) categoryChips(e, t, t, add);
		}
		pick(v.examples.filter((x) => !LIKE.test(x) && words.some((w) => ` ${x} `.includes(` ${w}`))), 3);
	}
	// A category with nothing else to offer ("anime"): the same thing as movies and as shows.
	if (!out.length && ctx.reading) {
		const bare = t.replace(/ (movies?|films?|shows?|series|tv)$/, '');
		if (!/(movies?|films?)$/.test(t)) add(`${bare} movies`, 'Category');
		if (!/(shows?|series|tv)$/.test(t)) add(`${bare} shows`, 'Category');
	}
	// Plain examples only when the search found nothing at all.
	if (!out.length && !ctx.top && !ctx.reading && !ctx.hits.length) pick(v.examples, 3);
	return out;
}
