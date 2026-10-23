package profiles

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/Flixarr/flixarr/internal/arr"
	"github.com/Flixarr/flixarr/internal/store"
)

// Newer profile versions wait for an admin (#94): Settings shows what changes, Update applies it, Skip
// keeps the current one until the version after. Admins have a to-do while any update waits.

func skipKey(serverID int64, key string) string {
	return fmt.Sprintf("profile_skip_%d_%s", serverID, key)
}

func (m *Manager) skipped(serverID int64, key string) string {
	return m.Store.Setting(skipKey(serverID, key), "")
}

// Skip keeps a profile's current version on a server until a newer Flixarr version than today's comes.
func (m *Manager) Skip(v store.Server, key string) error {
	d := m.Doc()
	if d == nil {
		return fmt.Errorf("the profiles haven't been downloaded yet")
	}
	for _, p := range d.Profiles {
		if p.ID == key {
			if err := m.Store.SetSetting(skipKey(v.ID, key), p.Rev); err != nil {
				return err
			}
			m.UpdateTodo()
			return nil
		}
	}
	return fmt.Errorf("there's no Flixarr profile %q", key)
}

// todoKey is the admins' to-do while profile updates wait.
const todoKey = "profiles-update"

// UpdateTodo raises or clears the admins' to-do for waiting profile updates and returns how many wait.
func (m *Manager) UpdateTodo() int {
	servers, err := m.Store.Servers()
	if err != nil {
		return 0
	}
	n, names := 0, []string{}
	for _, v := range servers {
		list, _ := m.Status(v)
		for _, p := range list {
			if p.Installed && p.UpdateAvailable {
				n++
				if !slices.Contains(names, p.Label) {
					names = append(names, p.Label)
				}
			}
		}
	}
	if n == 0 {
		m.Store.ResolveTodo(todoKey)
		return 0
	}
	title := "Quality profile update"
	if n > 1 {
		title = fmt.Sprintf("%d quality profile updates", n)
	}
	m.Store.RaiseTodo(store.Alert{Key: todoKey, Audience: "admins", Title: title, Detail: joinNames(names) + " · waiting for you to apply", Link: "/settings#profiles"})
	return n
}

func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	out := names[0]
	for i, n := range names[1:] {
		if i == len(names)-2 {
			out += " and " + n
		} else {
			out += ", " + n
		}
	}
	return out
}

// Changes is what applying a newer version would change on one server, in plain terms.
type Changes struct {
	Edited           bool          `json:"edited"` // edited in Radarr/Sonarr: those edits get replaced
	QualitiesAdded   []string      `json:"qualitiesAdded"`
	QualitiesRemoved []string      `json:"qualitiesRemoved"`
	CutoffFrom       string        `json:"cutoffFrom,omitempty"`
	CutoffTo         string        `json:"cutoffTo,omitempty"`
	FormatsAdded     []string      `json:"formatsAdded"`
	FormatsRemoved   []string      `json:"formatsRemoved"`
	ScoresChanged    []ScoreChange `json:"scoresChanged"`
}

// ScoreChange is one custom format scored differently.
type ScoreChange struct {
	Name string `json:"name"`
	From int    `json:"from"`
	To   int    `json:"to"`
}

// Changes compares a profile as it is on the server with Flixarr's current version. It only reads.
func (m *Manager) Changes(ctx context.Context, v store.Server, key string) (*Changes, error) {
	d := m.Doc()
	if d == nil {
		return nil, fmt.Errorf("the profiles haven't been downloaded yet")
	}
	var p *Profile
	for i := range d.Profiles {
		if d.Profiles[i].ID == key {
			p = &d.Profiles[i]
		}
	}
	if p == nil || p.For(v.Kind) == nil {
		return nil, fmt.Errorf("there's no Flixarr profile %q for %s", key, arr.Kinds[v.Kind])
	}
	links, err := m.Store.ProfileLinks(v.ID)
	if err != nil {
		return nil, err
	}
	var link *store.ProfileLink
	for i := range links {
		if links[i].Key == key {
			link = &links[i]
		}
	}
	if link == nil || link.State == "missing" {
		return nil, fmt.Errorf("%s isn't on %s", p.Label, v.Name)
	}
	var cur arrProfile
	if err := client(v).Get(ctx, fmt.Sprintf("qualityprofile/%d", link.ArrID), &cur); err != nil {
		return nil, err
	}
	return diff(cur, p.For(v.Kind).Profile, fingerprint(cur) != link.PushedHash), nil
}

// diff compares a profile on a server with a Flixarr version: allowed qualities, the cutoff and the
// custom format scores (formats scored 0 count as not in the profile).
func diff(cur arrProfile, next Body, edited bool) *Changes {
	c := &Changes{Edited: edited, QualitiesAdded: []string{}, QualitiesRemoved: []string{}, FormatsAdded: []string{}, FormatsRemoved: []string{}, ScoresChanged: []ScoreChange{}}
	had, cutoff := map[string]bool{}, ""
	for _, it := range cur.Items {
		name := it.Name
		if it.Quality != nil && name == "" {
			name = it.Quality.Name
		}
		if it.Allowed {
			had[name] = true
		}
		id := it.ID
		if it.Quality != nil && id == 0 {
			id = it.Quality.ID
		}
		if id == cur.Cutoff {
			cutoff = name
		}
	}
	want := map[string]bool{}
	for _, it := range next.Items {
		if it.Allowed {
			want[it.Name] = true
			if !had[it.Name] {
				c.QualitiesAdded = append(c.QualitiesAdded, it.Name)
			}
		}
	}
	for name := range had {
		if !want[name] {
			c.QualitiesRemoved = append(c.QualitiesRemoved, name)
		}
	}
	if cutoff != "" && cutoff != next.Cutoff {
		c.CutoffFrom, c.CutoffTo = cutoff, next.Cutoff
	}
	old := map[string]int{}
	for _, f := range cur.FormatItems {
		if f.Score != 0 {
			old[trimFlixarr(f.Name)] = f.Score
		}
	}
	now := map[string]int{}
	for _, f := range next.FormatItems {
		if f.Score != 0 {
			now[f.Name] = f.Score
		}
	}
	for name, score := range now {
		was, ok := old[name]
		switch {
		case !ok:
			c.FormatsAdded = append(c.FormatsAdded, name)
		case was != score:
			c.ScoresChanged = append(c.ScoresChanged, ScoreChange{Name: name, From: was, To: score})
		}
	}
	for name := range old {
		if _, ok := now[name]; !ok {
			c.FormatsRemoved = append(c.FormatsRemoved, name)
		}
	}
	sort.Strings(c.QualitiesAdded)
	sort.Strings(c.QualitiesRemoved)
	sort.Strings(c.FormatsAdded)
	sort.Strings(c.FormatsRemoved)
	sort.Slice(c.ScoresChanged, func(a, b int) bool { return c.ScoresChanged[a].Name < c.ScoresChanged[b].Name })
	return c
}

// trimFlixarr drops the " (Flixarr)" Flixarr adds to its copy of a format whose name was taken.
func trimFlixarr(name string) string {
	const suffix = " (Flixarr)"
	if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
		return name[:len(name)-len(suffix)]
	}
	return name
}
