package profiles

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/Flixarr/flixarr/internal/store"
	"github.com/Flixarr/flixarr/internal/testinstall"
)

// Radarr and Sonarr's Quality Definitions (their size limits) apply to the whole server. Flixarr sets them
// with the profiles so a good file isn't refused for its size. The values it found are kept for Restore,
// and if someone changes them by hand Flixarr leaves that server's limits alone from then on.

// Size states on a server.
const (
	SizesApplied  = "applied"
	SizesRestored = "restored"
	SizesEdited   = "edited"
)

type sizeState struct {
	State  string           `json:"state"`
	Hash   string           `json:"hash"`
	Backup []map[string]any `json:"backup,omitempty"`
}

func sizeKey(serverID int64) string { return fmt.Sprintf("size_limits_%d", serverID) }

func (m *Manager) sizeState(serverID int64) sizeState {
	var st sizeState
	json.Unmarshal([]byte(m.Store.Setting(sizeKey(serverID), "")), &st)
	return st
}

func (m *Manager) saveSizeState(serverID int64, st sizeState) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return m.Store.SetSetting(sizeKey(serverID), string(b))
}

// SizeLimits says how Flixarr's size limits stand on a server: "", applied, restored or edited.
func (m *Manager) SizeLimits(serverID int64) string { return m.sizeState(serverID).State }

// sizesHash fingerprints the limits, ignoring everything else in a definition (titles, weights).
func sizesHash(defs []map[string]any) string {
	type row struct {
		ID             string
		Min, Pref, Max any
	}
	rows := make([]row, 0, len(defs))
	for _, d := range defs {
		id, _ := json.Marshal(d["id"])
		rows = append(rows, row{string(id), rounded(d["minSize"]), rounded(d["preferredSize"]), rounded(d["maxSize"])})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	b, _ := json.Marshal(rows)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}

// rounded keeps a limit comparable however the app stored it. A limit that's not set stays nil.
func rounded(v any) any {
	if f, ok := v.(float64); ok {
		return math.Round(f*10) / 10
	}
	return v
}

// withLimits returns the definitions with Flixarr's limits filled in.
func withLimits(defs []map[string]any, want map[string]SizeLimit) []map[string]any {
	out := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		c := make(map[string]any, len(d))
		for k, v := range d {
			c[k] = v
		}
		q, _ := d["quality"].(map[string]any)
		name, _ := q["name"].(string)
		if l, ok := want[name]; ok {
			c["minSize"] = l.Min
			if l.Preferred != nil {
				c["preferredSize"] = *l.Preferred
			}
			if l.Max != nil {
				c["maxSize"] = *l.Max
			}
		}
		out = append(out, c)
	}
	return out
}

func pushAllowed() bool { return !testinstall.On || testinstall.PushProfiles }

// applySizes sets Flixarr's limits on a server unless they were restored or edited (force puts them back).
func (m *Manager) applySizes(ctx context.Context, v store.Server, d *Doc, force bool) error {
	want := d.Sizes[v.Kind]
	if len(want) == 0 || !pushAllowed() {
		return nil
	}
	st := m.sizeState(v.ID)
	if !force && (st.State == SizesRestored || st.State == SizesEdited) {
		return nil
	}
	c := client(v)
	var defs []map[string]any
	if err := c.Get(ctx, "qualitydefinition", &defs); err != nil {
		return err
	}
	if !force && st.State == SizesApplied && sizesHash(defs) != st.Hash {
		st.State = SizesEdited
		return m.saveSizeState(v.ID, st)
	}
	if st.Backup == nil {
		st.Backup = defs
	}
	next := withLimits(defs, want)
	if st.State != SizesApplied || sizesHash(next) != st.Hash {
		if err := c.Put(ctx, "qualitydefinition/update", next, nil); err != nil {
			return fmt.Errorf("Unable to set the size limits: %w", err)
		}
	}
	st.State, st.Hash = SizesApplied, sizesHash(next)
	return m.saveSizeState(v.ID, st)
}

// ApplySizes puts Flixarr's size limits on a server, replacing whatever is there (Use Flixarr's limits).
func (m *Manager) ApplySizes(ctx context.Context, v store.Server) error {
	d, err := m.docOrFetch(ctx)
	if err != nil {
		return err
	}
	return m.applySizes(ctx, v, d, true)
}

// RestoreSizes puts back the limits the server had before Flixarr changed them.
func (m *Manager) RestoreSizes(ctx context.Context, v store.Server) error {
	if !pushAllowed() {
		return testinstall.ErrBlocked
	}
	st := m.sizeState(v.ID)
	if st.Backup == nil {
		return fmt.Errorf("There are no saved size limits to restore")
	}
	if err := client(v).Put(ctx, "qualitydefinition/update", st.Backup, nil); err != nil {
		return fmt.Errorf("Unable to restore the size limits: %w", err)
	}
	st.State = SizesRestored
	return m.saveSizeState(v.ID, st)
}
