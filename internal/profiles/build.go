package profiles

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// arrQuality keeps Radarr/Sonarr's quality object exactly as they sent it, plus the two fields used here.
type arrQuality struct {
	ID   int
	Name string
	Raw  json.RawMessage
}

func (q *arrQuality) UnmarshalJSON(b []byte) error {
	var v struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	q.ID, q.Name, q.Raw = v.ID, v.Name, append(json.RawMessage(nil), b...)
	return nil
}

func (q arrQuality) MarshalJSON() ([]byte, error) { return q.Raw, nil }

// arrItem is a quality or a group in a Radarr/Sonarr profile.
type arrItem struct {
	ID      int         `json:"id,omitempty"`   // groups only
	Name    string      `json:"name,omitempty"` // groups only
	Quality *arrQuality `json:"quality,omitempty"`
	Items   []arrItem   `json:"items"`
	Allowed bool        `json:"allowed"`
}

type arrFormatItem struct {
	Format int    `json:"format"`
	Name   string `json:"name,omitempty"`
	Score  int    `json:"score"`
}

type arrLanguage struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// arrProfile is a Radarr/Sonarr quality profile.
type arrProfile struct {
	ID                    int             `json:"id,omitempty"`
	Name                  string          `json:"name"`
	UpgradeAllowed        bool            `json:"upgradeAllowed"`
	Cutoff                int             `json:"cutoff"`
	MinFormatScore        int             `json:"minFormatScore"`
	CutoffFormatScore     int             `json:"cutoffFormatScore"`
	MinUpgradeFormatScore int             `json:"minUpgradeFormatScore"`
	Items                 []arrItem       `json:"items"`
	FormatItems           []arrFormatItem `json:"formatItems"`
	Language              *arrLanguage    `json:"language,omitempty"`
}

// arrFormat is a custom format as Radarr/Sonarr return it.
type arrFormat struct {
	ID                              int    `json:"id,omitempty"`
	Name                            string `json:"name"`
	IncludeCustomFormatWhenRenaming bool   `json:"includeCustomFormatWhenRenaming"`
	Specifications                  []Spec `json:"specifications"`
}

// qualitiesOf lists every quality in a profile (or the schema), by name.
func qualitiesOf(items []arrItem) map[string]*arrQuality {
	out := map[string]*arrQuality{}
	var walk func([]arrItem)
	walk = func(list []arrItem) {
		for _, it := range list {
			if it.Quality != nil {
				out[it.Quality.Name] = it.Quality
			}
			walk(it.Items)
		}
	}
	walk(items)
	return out
}

// buildProfile turns a Flixarr profile body into a Radarr/Sonarr profile for one server.
// schema is the server's /qualityprofile/schema; formatIDs maps Flixarr format names to that server's
// custom format IDs; allFormats is every custom format on the server (each needs a score, 0 if unused);
// languages is the server's language list (Radarr).
func buildProfile(name string, b Body, schema arrProfile, formatIDs map[string]int, allFormats []arrFormat, languages []arrLanguage) (arrProfile, error) {
	quals := qualitiesOf(schema.Items)
	used := map[string]bool{}
	single := func(n string, allowed bool) (arrItem, error) {
		q, ok := quals[n]
		if !ok {
			return arrItem{}, fmt.Errorf("this server has no quality named %q", n)
		}
		used[n] = true
		return arrItem{Quality: q, Items: []arrItem{}, Allowed: allowed}, nil
	}
	var best []arrItem // best first, as Flixarr writes them
	cutoff, groupID := -1, 1000
	for _, it := range b.Items {
		if len(it.Items) == 0 {
			// A plain quality, unless this server has it under a group of the same name (Sonarr's "WEB 1080p").
			if _, ok := quals[it.Name]; !ok {
				return arrProfile{}, fmt.Errorf("this server has no quality named %q", it.Name)
			}
			x, _ := single(it.Name, it.Allowed)
			best = append(best, x)
			if it.Name == b.Cutoff {
				cutoff = x.Quality.ID
			}
			continue
		}
		g := arrItem{ID: groupID, Name: it.Name, Allowed: it.Allowed, Items: []arrItem{}}
		groupID++
		for _, n := range it.Items {
			x, err := single(n, it.Allowed)
			if err != nil {
				return arrProfile{}, err
			}
			g.Items = append(g.Items, x)
		}
		best = append(best, g)
		if it.Name == b.Cutoff {
			cutoff = g.ID
		}
	}
	if cutoff < 0 {
		return arrProfile{}, fmt.Errorf("cutoff %q isn't one of the profile's qualities", b.Cutoff)
	}
	// Qualities the profile doesn't mention are kept, not allowed, at the bottom.
	var rest []string
	for n := range quals {
		if !used[n] {
			rest = append(rest, n)
		}
	}
	sort.Strings(rest)
	for _, n := range rest {
		x, _ := single(n, false)
		best = append(best, x)
	}
	// Radarr and Sonarr list qualities worst first.
	items := make([]arrItem, len(best))
	for i, it := range best {
		items[len(best)-1-i] = it
	}

	scores := map[int]int{}
	for _, f := range b.FormatItems {
		id, ok := formatIDs[f.Name]
		if !ok {
			return arrProfile{}, fmt.Errorf("custom format %q wasn't added", f.Name)
		}
		scores[id] = f.Score
	}
	fi := make([]arrFormatItem, 0, len(allFormats))
	for _, f := range allFormats {
		fi = append(fi, arrFormatItem{Format: f.ID, Name: f.Name, Score: scores[f.ID]})
	}

	p := arrProfile{Name: name, UpgradeAllowed: b.UpgradeAllowed, Cutoff: cutoff, MinFormatScore: b.MinFormatScore,
		CutoffFormatScore: b.CutoffFormatScore, MinUpgradeFormatScore: b.MinUpgradeFormatScore, Items: items, FormatItems: fi}
	if b.Language != "" && schema.Language != nil {
		p.Language = schema.Language
		for _, l := range languages {
			if l.Name == b.Language {
				l := l
				p.Language = &l
			}
		}
	}
	return p, nil
}

// fingerprint captures what a profile does (qualities, order, cutoff, scores, language), ignoring
// its ID, name and formats scored 0 (new custom formats get added to every profile at 0). If the
// fingerprint changed since Flixarr pushed it, someone edited it in Radarr/Sonarr.
func fingerprint(p arrProfile) string {
	type q struct {
		G       string
		IDs     []int
		Allowed bool
	}
	var items []q
	for _, it := range p.Items {
		x := q{G: it.Name, Allowed: it.Allowed}
		if it.Quality != nil {
			x.IDs = []int{it.Quality.ID}
		}
		for _, s := range it.Items {
			if s.Quality != nil {
				x.IDs = append(x.IDs, s.Quality.ID)
			}
		}
		items = append(items, x)
	}
	var scores [][2]int
	for _, f := range p.FormatItems {
		if f.Score != 0 {
			scores = append(scores, [2]int{f.Format, f.Score})
		}
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i][0] < scores[j][0] })
	cutoff := p.Cutoff
	// Groups get new IDs when saved, so the cutoff is compared by position.
	for i, it := range p.Items {
		if (it.ID != 0 && it.ID == p.Cutoff) || (it.Quality != nil && it.ID == 0 && it.Quality.ID == p.Cutoff && len(it.Items) == 0) {
			cutoff = -1 - i
		}
	}
	lang := 0
	if p.Language != nil {
		lang = p.Language.ID
	}
	b, _ := json.Marshal([]any{p.UpgradeAllowed, cutoff, p.MinFormatScore, p.CutoffFormatScore, p.MinUpgradeFormatScore, lang, items, scores})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}
