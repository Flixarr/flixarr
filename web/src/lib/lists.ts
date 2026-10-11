// Discover rows, each backed by a Flixarr API list. Used by the Discover page and the "See more" pages.
export const LISTS = [
	{ key: 'trending', title: 'Trending' },
	{ key: 'popular-movies', title: 'Popular Movies' },
	{ key: 'upcoming-movies', title: 'Upcoming Movies' },
	{ key: 'popular-series', title: 'Popular Series' },
	{ key: 'upcoming-series', title: 'On the Air' }
] as const;

export const listHref = (key: string) => '/discover/' + key;
