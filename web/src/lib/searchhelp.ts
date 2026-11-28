// Search help (#286, #287): the search chips under the box, the example searches in it and the Browse tiles on the
// empty search page. Every word and tile comes from the Flixarr API (/v1/searchhelp/*); this file only knows how
// to put them together, so nothing here names a title, a genre or a country.
import { meta } from '#lib/api.ts';
import type { SearchHit } from '#lib/types.ts';

type Entry = { words: string[]; label: string; phrase: string; movie?: boolean; series?: boolean; decades?: string[] };
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
export const loadVocab = () => (vocabP ??= meta<Vocab>('searchhelp/vocab').catch(() => ((vocabP = null), null)));
/** The Browse tile pool (seasonal tiles only in season). */
export const loadTiles = () => meta<{ tiles: Tile[] }>('searchhelp/browse').then((d) => d.tiles).catch(() => [] as Tile[]);

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
				for (const d of (best.decades ?? []).slice(0, 3)) add(`${d} ${best.phrase}`, 'Category');
				add(`new ${best.phrase}`, 'Category');
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
		if (h.type === 'collection') add(`${h.title.replace(/ collection$/i, '').toLowerCase()} collection`, 'Collection', h.title);
		// A person only once more than their first name is typed ("tom h"), and only one: "harry" alone matches many.
		else if (h.type === 'person' && t.includes(' ') && lower(h.title).startsWith(t) && !out.some((c) => c.kind === 'Person')) add(lower(h.title), 'Person', h.title);
	}
	// Always at least three (Marc, 2026-11-27): the rest are example searches from the API, the same ones for the
	// same text so they don't change while the results load.
	let seed = [...t].reduce((n, c) => (n * 31 + c.charCodeAt(0)) >>> 0, 7);
	const pool = v.examples.filter((x) => x !== t);
	for (let i = 0; out.length < 3 && i < pool.length; i++) {
		seed = (seed * 1103515245 + 12345) >>> 0;
		const x = pool[seed % pool.length];
		add(x, LIKE.test(x) ? 'More like' : 'Category');
	}
	return out;
}
