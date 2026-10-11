// Shapes returned by the Flixarr API (and the data server behind /api/meta).

export type MediaType = 'movie' | 'series';

export interface Card {
	id: number;
	type: MediaType;
	title: string;
	poster: string | null;
	backdrop: string | null;
	year: string;
	rating: number;
	genres: number[];
	overview: string;
	character?: string;
}

export interface Page<T> {
	page: number;
	totalPages: number;
	results: T[];
}

export interface CastMember { id: number; name: string; character: string; profile: string | null }

export interface Movie {
	id: number; type: 'movie'; title: string; tagline: string; overview: string;
	poster: string | null; backdrop: string | null; logo: string | null;
	year: string; release: string | null; runtime: number; cert: string; genres: string[];
	rating: number; votes: number; status: string; budget: number; revenue: number; language: string;
	studios: string[]; countries: string[]; director: string[]; writers: string[]; producers: string[];
	cast: CastMember[]; trailer: string | null; keywords: string[];
	recommendations: Card[]; similar: Card[];
	collection: { id: number; name: string; backdrop: string | null } | null;
}

export interface SeasonSummary { n: number; name: string; episodes: number; air: string | null; poster: string | null; overview: string }
export interface Episode { n: number; name: string; air: string | null; still: string | null; runtime: number | null; overview: string }
export interface Season { n: number; name: string; overview: string; air: string | null; poster: string | null; episodes: Episode[] }

export interface Series {
	id: number; type: 'series'; title: string; tagline: string; overview: string;
	poster: string | null; backdrop: string | null; logo: string | null;
	year: string; first: string | null; last: string | null; cert: string; genres: string[];
	rating: number; votes: number; status: string; networks: string[]; creators: string[];
	runtime: number; language: string; studios: string[]; seasons: SeasonSummary[]; next: string | null;
	cast: CastMember[]; trailer: string | null; keywords: string[];
	recommendations: Card[]; similar: Card[];
}

export interface Person {
	id: number; name: string; bio: string; profile: string | null; born: string | null; died: string | null;
	place: string | null; dept: string | null; credits: Card[];
}

export interface Collection { id: number; name: string; overview: string; poster: string | null; backdrop: string | null; parts: Card[] }

export interface SearchHit extends Omit<Partial<Card>, 'type'> { id: number; type: MediaType | 'person'; title: string; poster: string | null; dept?: string | null }

export interface User { id: number; plexId: number; username: string; email: string; avatar: string; role: string; createdAt: string }

export interface UpdateStatus {
	enabled: boolean;
	locked: boolean;
	available: boolean;
	latest: { version: string; build: string; commit: string; released: string } | null;
	checkedAt: string;
}

export interface Status {
	setupRequired: boolean;
	version: { version: string; build: string; full: string; commit: string; channel: string };
	update: UpdateStatus;
	imageCache: boolean;
}

export interface CacheKind { kind: 'titles' | 'people' | 'lists' | 'searches'; count: number; bytes: number }

export interface CacheState {
	settings: { enabled: boolean; pagesMinutes: number; listsMinutes: number; searchesMinutes: number; maxResults: number };
	kinds: CacheKind[];
	counters: { hits: number; misses: number; stale: number; since: string };
	images: { enabled: boolean; days: number; count: number; bytes: number };
}

export interface Job {
	id: string;
	name: string;
	about: string;
	schedule: string;
	default: string;
	enabled: boolean;
	running: boolean;
	next: string;
	last: { at: string; seconds: number; result: string; error?: string } | null;
}
