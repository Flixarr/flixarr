// Package profiles adds Flixarr's quality profiles to Radarr and Sonarr and keeps them up to date.
//
// The profiles come from the Flixarr API (/v1/profiles), which serves the profiles.json built in the
// Flixarr/flixarr-profiles repo. Each profile has a Radarr and a Sonarr version, written in their own
// shapes but keyed by name: qualities and custom formats are resolved to each server's IDs here.
package profiles

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

// Doc is the whole profiles document.
type Doc struct {
	Schema   int       `json:"schema"`
	Version  string    `json:"version"`
	Source   Source    `json:"source"`
	Profiles []Profile `json:"profiles"`
	// Every custom format the profiles use, per app ("radarr", "sonarr"), keyed by name.
	CustomFormats map[string]map[string]CustomFormat `json:"customFormats"`
}

// Source credits where the profiles came from.
type Source struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Commit  string `json:"commit"`
	License string `json:"license"`
}

// Profile is one Flixarr profile, for both apps.
type Profile struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Label       string      `json:"label"`
	Description string      `json:"description"`
	Rev         string      `json:"rev"` // changes whenever anything pushed for this profile changes
	Radarr      *AppProfile `json:"radarr,omitempty"`
	Sonarr      *AppProfile `json:"sonarr,omitempty"`
}

// For returns the profile's version for an app, or nil.
func (p Profile) For(kind string) *AppProfile {
	if kind == "sonarr" {
		return p.Sonarr
	}
	return p.Radarr
}

// AppProfile is a profile for one app.
type AppProfile struct {
	BasedOn struct {
		Name    string `json:"name"`
		TrashID string `json:"trashId"`
		URL     string `json:"url"`
	} `json:"basedOn"`
	Profile Body `json:"profile"`
}

// Body is a quality profile with qualities and formats referenced by name.
type Body struct {
	UpgradeAllowed        bool         `json:"upgradeAllowed"`
	Cutoff                string       `json:"cutoff"`
	MinFormatScore        int          `json:"minFormatScore"`
	CutoffFormatScore     int          `json:"cutoffFormatScore"`
	MinUpgradeFormatScore int          `json:"minUpgradeFormatScore"`
	Language              string       `json:"language,omitempty"` // Radarr only, e.g. "Original"
	Items                 []Item       `json:"items"`              // best first
	FormatItems           []FormatItem `json:"formatItems"`
}

// Item is a quality, or a group of qualities that count as one.
type Item struct {
	Name    string   `json:"name"`
	Allowed bool     `json:"allowed"`
	Items   []string `json:"items,omitempty"` // a group's qualities
}

// FormatItem is a custom format's score in the profile.
type FormatItem struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
}

// CustomFormat is a custom format as Radarr/Sonarr take it.
type CustomFormat struct {
	Name                            string `json:"name"`
	IncludeCustomFormatWhenRenaming bool   `json:"includeCustomFormatWhenRenaming"`
	Specifications                  []Spec `json:"specifications"`
}

// Spec is one condition of a custom format.
type Spec struct {
	Name           string  `json:"name"`
	Implementation string  `json:"implementation"`
	Negate         bool    `json:"negate"`
	Required       bool    `json:"required"`
	Fields         []Field `json:"fields"`
}

// Field is a spec setting.
type Field struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

// specHash fingerprints what a custom format matches (names of conditions don't count), so two
// formats with the same rules compare equal whatever server they came from.
func specHash(specs []Spec) string {
	type norm struct {
		Impl     string
		Negate   bool
		Required bool
		Values   map[string]string
	}
	var list []norm
	for _, s := range specs {
		n := norm{Impl: s.Implementation, Negate: s.Negate, Required: s.Required, Values: map[string]string{}}
		for _, f := range s.Fields {
			b, _ := json.Marshal(f.Value)
			n.Values[f.Name] = string(b)
		}
		list = append(list, n)
	}
	sort.Slice(list, func(i, j int) bool {
		a, _ := json.Marshal(list[i])
		b, _ := json.Marshal(list[j])
		return string(a) < string(b)
	})
	b, _ := json.Marshal(list)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}

// ArrName is the name the profile gets inside Radarr/Sonarr.
func ArrName(p Profile) string { return "Flixarr · " + p.Name }

// IsFlixarrName reports whether a Radarr/Sonarr profile name is one of Flixarr's.
func IsFlixarrName(name string) bool { return strings.HasPrefix(name, "Flixarr · ") }
