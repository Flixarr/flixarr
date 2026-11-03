import { api } from './api';

// Episodes per season of a series: how many are in Plex, and how many each season has. TMDB and TheTVDB
// don't always number a show the same way, so once Plex numbers it like Sonarr and Sonarr has it, the
// counts are Sonarr's (episodes); otherwise the season's own (TMDB's) count is used.
export type PlexSeasons = { inPlex: Record<number, number>; episodes?: Record<number, number> };

export const noSeasons = (): PlexSeasons => ({ inPlex: {} });

export const loadSeasons = (id: number | string | undefined): Promise<PlexSeasons> =>
	api<PlexSeasons>(`/plex/seasons/${id}`).then((d) => ({ inPlex: d?.inPlex ?? {}, episodes: d?.episodes })).catch(noSeasons);

// episodeCount is how many episodes a season has, numbered the way Plex numbers them.
export const episodeCount = (d: PlexSeasons, s: { n: number; episodes: number }) => d.episodes?.[s.n] ?? s.episodes;
