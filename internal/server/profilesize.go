package server

// What a quality profile gives, for the request window's Quality menu (#280): the resolution of the quality it
// aims for (its cutoff) and that quality's preferred size in MB per minute from the server's size limits.
// The window multiplies it by the runtime ("1080p, about 8 GB"; series per episode).

type arrQuality struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Resolution int    `json:"resolution"`
}

type arrProfileItem struct {
	ID      int              `json:"id"` // groups only
	Quality *arrQuality      `json:"quality"`
	Items   []arrProfileItem `json:"items"`
	Allowed bool             `json:"allowed"`
}

type arrProfile struct {
	ID     int              `json:"id"`
	Name   string           `json:"name"`
	Cutoff int              `json:"cutoff"`
	Items  []arrProfileItem `json:"items"`
}

type arrQualityDef struct {
	Quality       arrQuality `json:"quality"`
	MinSize       float64    `json:"minSize"`
	MaxSize       *float64   `json:"maxSize"`
	PreferredSize *float64   `json:"preferredSize"`
}

// profileTarget is the quality a profile aims for: its cutoff, or the best quality in the cutoff's group.
func profileTarget(p arrProfile) *arrQuality {
	var best *arrQuality
	for _, it := range p.Items {
		if it.Quality != nil && it.Quality.ID == p.Cutoff {
			return it.Quality
		}
		if len(it.Items) > 0 && it.ID == p.Cutoff {
			for _, sub := range it.Items {
				if sub.Quality != nil && (best == nil || sub.Quality.Resolution > best.Resolution) {
					best = sub.Quality
				}
			}
		}
	}
	return best
}

// resolutionLabel turns a quality's resolution into the words people know.
func resolutionLabel(px int) string {
	switch {
	case px >= 2160:
		return "4K"
	case px >= 1080:
		return "1080p"
	case px >= 720:
		return "720p"
	case px > 0:
		return "SD"
	}
	return ""
}

// unlimited: Radarr and Sonarr's "no limit" sits at the top of their sliders (1999 and 995 MB per minute), which
// says nothing about a file's real size, so sizes at or above this aren't shown.
const unlimited = 900

// mbPerMinute is a quality's usual size: its preferred size, or halfway between its limits when there's none;
// 0 when the server's limits are set to unlimited.
func mbPerMinute(d arrQualityDef) float64 {
	if d.PreferredSize != nil && *d.PreferredSize > 0 && *d.PreferredSize < unlimited {
		return *d.PreferredSize
	}
	if d.MaxSize != nil && *d.MaxSize > 0 && *d.MaxSize < unlimited {
		return (d.MinSize + *d.MaxSize) / 2
	}
	return 0
}

// profileGives is a profile's resolution label and MB per minute ("" and 0 when unknown).
func profileGives(p arrProfile, defs []arrQualityDef) (string, float64) {
	q := profileTarget(p)
	if q == nil {
		return "", 0
	}
	for _, d := range defs {
		if d.Quality.ID == q.ID {
			return resolutionLabel(q.Resolution), mbPerMinute(d)
		}
	}
	return resolutionLabel(q.Resolution), 0
}
