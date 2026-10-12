// Discover rows, each backed by a Flixarr API list. Used by the Discover page and the "See more" pages.
export const LISTS = [
	{ key: 'trending', title: 'Trending' },
	{ key: 'popular-movies', title: 'Popular Movies' },
	{ key: 'upcoming-movies', title: 'Upcoming Movies' },
	{ key: 'popular-series', title: 'Popular Series' },
	{ key: 'upcoming-series', title: 'On the Air' }
] as const;

export const listHref = (key: string) => '/discover/' + key;

// Every Discover row, for Settings → Discover. The first two come from Flixarr itself (Plex and requests).
export const ROW_INFO: Record<string, { title: string; about: string }> = {
	'recently-added': { title: 'Recently Added', about: 'The newest titles in your Plex libraries.' },
	'recent-requests': { title: 'Recent Requests', about: "The newest requests (everyone's, for people who can see them)." },
	trending: { title: 'Trending', about: 'Movies and series trending this week.' },
	'popular-movies': { title: 'Popular Movies', about: '' },
	'upcoming-movies': { title: 'Upcoming Movies', about: 'Coming to cinemas in your region.' },
	'popular-series': { title: 'Popular Series', about: '' },
	'upcoming-series': { title: 'On the Air', about: 'Series with episodes airing soon.' }
};
export type DiscoverRow = { key: string; on: boolean };
