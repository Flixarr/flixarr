// How an issue reads in a line: what's wrong, and where in a series.
import type { IssueBrief } from './types';

type Spot = Pick<IssueBrief, 'type' | 'parts' | 'season' | 'episode'>;

// Where in a series: "S3E5", "Season 2", "S1E1, S1E2 +3 more" ('' for a movie or the whole show).
export function issueWhere(i: Spot) {
	if (i.type !== 'series') return '';
	const p = i.parts?.length ? i.parts : i.season ? [[i.season, i.episode] as [number, number]] : [];
	const t = p.map(([se, e]) => (e ? `S${se}E${e}` : `Season ${se}`));
	return t.length > 4 ? `${t.slice(0, 4).join(', ')} +${t.length - 4} more` : t.join(', ');
}

// What's wrong: "Audio: too quiet", "Video".
export const issueWhat = (i: Pick<IssueBrief, 'kind' | 'sub'>) => i.kind + (i.sub ? `: ${i.sub.toLowerCase()}` : '');
