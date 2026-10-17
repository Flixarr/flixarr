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
	creditId?: string; // a person's series credit: opens the show with their episodes marked
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

export interface User { id: number; plexId: number; username: string; email: string; avatar: string; role: string; perms: string[]; owner: boolean; local?: boolean; movieLimit: number; seasonLimit: number; createdAt: string }

export interface UpdateStatus {
	enabled: boolean;
	locked: boolean;
	available: boolean;
	latest: { version: string; build: string; commit: string; released: string } | null;
	checkedAt: string;
}

export interface Status {
	setupRequired: boolean; setupWizard?: boolean;
	version: { version: string; build: string; full: string; commit: string; channel: string };
	update: UpdateStatus;
	imageCache: boolean;
	signIn?: 'both' | 'plex' | 'local';
}

export interface CacheKind { kind: 'titles' | 'people' | 'lists' | 'searches'; count: number; bytes: number }

export interface CacheConfig { enabled: boolean; auto: boolean; pagesMinutes: number; listsMinutes: number; searchesMinutes: number; maxResults: number }

export interface CacheState {
	settings: CacheConfig;
	defaults: CacheConfig;
	kinds: CacheKind[];
	counters: { hits: number; misses: number; stale: number; since: string; byKind: Record<string, HitMiss> };
	images: { enabled: boolean; days: number; defaultDays: number; count: number; bytes: number } & HitMiss;
	avatars: { count: number; bytes: number } & HitMiss;
}

export interface HitMiss { hits: number; misses: number }

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

export type ServiceKind = 'radarr' | 'sonarr';

export interface ProfileStatus { key: string; name: string; label: string; description: string; arrName: string; installed: boolean; arrId?: number; state: '' | 'ok' | 'update' | 'customized' | 'missing'; updateAvailable: boolean; updatedAt?: string }
export interface FlixarrProfile { id: string; name: string; label: string; description: string; arrName: string; apps: ServiceKind[] }

export interface ServiceView {
	id: number; kind: ServiceKind; name: string; url: string; webUrl: string; rootFolder: string; qualityProfileId: number; qualityProfileName: string;
	seasonFolders: boolean; searchOnAdd: boolean; enabled: boolean; createdAt: string; updatedAt: string; apiKeySet: boolean; apiKeyEnd: string;
	status: ArrStatus | null;
	profiles: ProfileStatus[];
	warning?: string;
	nextUpdate: string | null;
	updatesPaused: boolean;
}

export interface QueueRow { title: string; quality: string; size: number; percent: number; timeLeft: string; state: 'downloading' | 'queued' | 'paused' | 'importing' | 'warning' | 'error'; message?: string }

export interface ArrStatus {
	connected: boolean; error?: string; version?: string; health: string[]; queue: QueueRow[]; folder: string; freeSpace: number; totalSpace: number;
	indexers: number; indexersDown: number; clients: string[]; clientsDown: boolean; updatedAt: string; lastConnected: string | null;
}

export interface ArrInfo {
	version: string; name: string; applicationUrl?: string;
	profiles: { id: number; name: string }[];
	folders: { id: number; path: string; freeSpace: number; accessible: boolean }[];
}

export interface Request {
	id: number; type: 'movie' | 'series'; tmdbId: number; title: string; year: string; poster: string; backdrop: string;
	userId: number; userName: string; status: 'pending' | 'approved' | 'declined' | 'sent' | 'grabbed' | 'downloading' | 'importing' | 'available' | 'failed' | 'expired' | 'removed';
	serverId: number; arrId: number; profileId: number; profileName: string; mode: 'all' | 'seasons' | 'episodes'; seasons: number[]; episodes: [number, number][]; progress: number; error: string; anime: boolean; source?: string; detail?: string; userAvatar?: string;
	createdAt: string; updatedAt: string;
}
export type RequestBrief = { id: number; status: Request['status'] | 'blocked'; progress: number; plex?: string; blocked?: boolean; asked?: { whole: boolean; seasons: number[] } };
