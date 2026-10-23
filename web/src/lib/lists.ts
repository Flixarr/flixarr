// Discover rows, each backed by a Flixarr API list. Used by the Discover page and the "See more" pages.
export const LISTS = [
	{ key: 'trending', title: 'Trending' },
	{ key: 'popular-movies', title: 'Popular Movies' },
	{ key: 'upcoming-movies', title: 'Upcoming Movies' },
	{ key: 'popular-series', title: 'Popular Series' },
	{ key: 'upcoming-series', title: 'On the Air' },
	{ key: 'now-playing', title: 'In Cinemas' },
	{ key: 'airing-today', title: 'Airing Today' },
	{ key: 'top-rated-movies', title: 'Top Rated Movies' },
	{ key: 'top-rated-series', title: 'Top Rated Series' }
] as const;

export const listHref = (key: string) => '/discover/' + key;

// Every built-in Discover row, for Settings → Discover. The first three come from Flixarr itself (Plex,
// requests, Maintainerr).
export const ROW_INFO: Record<string, { title: string; about: string }> = {
	'recently-added': { title: 'Recently Added', about: 'The newest titles in your Plex libraries.' },
	'recent-requests': { title: 'Recent Requests', about: "The newest requests, everyone's." },
	'leaving-soon': { title: 'Leaving Soon', about: 'Titles Maintainerr removes soon, soonest first (only with Maintainerr connected).' },
	trending: { title: 'Trending', about: 'Movies and series trending this week.' },
	'popular-movies': { title: 'Popular Movies', about: '' },
	'upcoming-movies': { title: 'Upcoming Movies', about: 'Coming to cinemas in your region.' },
	'popular-series': { title: 'Popular Series', about: '' },
	'upcoming-series': { title: 'On the Air', about: 'Series with episodes airing soon.' },
	'now-playing': { title: 'In Cinemas', about: 'Movies in cinemas in your region now.' },
	'airing-today': { title: 'Airing Today', about: 'Series with an episode airing today.' },
	'top-rated-movies': { title: 'Top Rated Movies', about: 'The best rated movies of all time.' },
	'top-rated-series': { title: 'Top Rated Series', about: 'The best rated series of all time.' }
};

// A row in Settings → Discover. Custom rows ("custom-…") also say what they show.
export type DiscoverRow = { key: string; on: boolean; title?: string; kind?: 'movie' | 'series'; genre?: number; genreName?: string; sort?: 'popular' | 'rating' | 'newest' };
export const isCustom = (r: DiscoverRow) => r.key.startsWith('custom-');
export const SORTS = { popular: 'Most popular', rating: 'Best rated', newest: 'Newest' } as const;

export const rowTitle = (r: DiscoverRow) => (isCustom(r) ? r.title! : (ROW_INFO[r.key]?.title ?? r.key));
export const rowAbout = (r: DiscoverRow) =>
	isCustom(r) ? [r.kind === 'series' ? 'Series' : 'Movies', r.genreName, SORTS[r.sort ?? 'popular'].toLowerCase()].filter(Boolean).join(' · ') : (ROW_INFO[r.key]?.about ?? '');

// Where a list row's titles come from: a Flixarr API list, or its discover search for custom rows.
export function rowSource(r: DiscoverRow): { path: string; params: Record<string, string | number> } {
	if (!isCustom(r)) return { path: 'lists/' + r.key, params: {} };
	const params: Record<string, string | number> = { sort: r.sort ?? 'popular' };
	if (r.genre) params.genre = r.genre;
	return { path: 'discover/' + (r.kind === 'series' ? 'series' : 'movie'), params };
}
