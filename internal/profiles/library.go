package profiles

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Flixarr/flixarr/internal/store"
	"github.com/Flixarr/flixarr/internal/testinstall"
)

// Moving a library onto a profile changes which profile every title uses and nothing else. What happens
// next is the owner's choice: wait for new releases, search a few titles a day, or search them all now.

// DefaultPerDay is how many titles the slow search covers a day. Indexers often cap daily searches, so it's low.
const DefaultPerDay = 30

const keyPerDay = "library_search_per_day"

func searchKey(serverID int64) string { return fmt.Sprintf("library_search_%d", serverID) }

type libraryState struct {
	Since string `json:"since"` // titles last searched before this still need a look
}

// PerDay is how many titles the slow search covers each day.
func (m *Manager) PerDay() int {
	if n, err := strconv.Atoi(m.Store.Setting(keyPerDay, "")); err == nil && n > 0 {
		return n
	}
	return DefaultPerDay
}

// SetPerDay changes how many titles the slow search covers each day.
func (m *Manager) SetPerDay(n int) error {
	if n < 1 || n > 1000 {
		return fmt.Errorf("Choose between 1 and 1000 titles a day")
	}
	return m.Store.SetSetting(keyPerDay, strconv.Itoa(n))
}

// LibrarySearching reports whether a slow search is under way on a server.
func (m *Manager) LibrarySearching(serverID int64) bool {
	return m.Store.Setting(searchKey(serverID), "") != ""
}

// LibraryCount is how many titles a server has.
func (m *Manager) LibraryCount(ctx context.Context, v store.Server) (int, error) {
	ids, err := m.libraryIDs(ctx, v)
	return len(ids), err
}

func (m *Manager) libraryIDs(ctx context.Context, v store.Server) ([]int, error) {
	var list []struct {
		ID int `json:"id"`
	}
	if err := client(v).Get(ctx, listPath(v.Kind), &list); err != nil {
		return nil, err
	}
	ids := make([]int, len(list))
	for i, x := range list {
		ids[i] = x.ID
	}
	return ids, nil
}

func listPath(kind string) string {
	if kind == "sonarr" {
		return "series"
	}
	return "movie"
}

// MoveLibrary puts every title on a server on one profile. search is "" (nothing more), "slow" or "now".
func (m *Manager) MoveLibrary(ctx context.Context, v store.Server, profileID int, search string) (int, error) {
	if !pushAllowed() {
		return 0, testinstall.ErrBlocked
	}
	if search != "" && search != "slow" && search != "now" {
		return 0, fmt.Errorf("Unknown search option")
	}
	m.run.Lock()
	defer m.run.Unlock()
	c := client(v)
	var profiles []arrProfile
	if err := c.Get(ctx, "qualityprofile", &profiles); err != nil {
		return 0, err
	}
	found := false
	for _, p := range profiles {
		found = found || p.ID == profileID
	}
	if !found {
		return 0, fmt.Errorf("This quality profile no longer exists")
	}
	ids, err := m.libraryIDs(ctx, v)
	if err != nil {
		return 0, err
	}
	body := map[string]any{"qualityProfileId": profileID}
	if v.Kind == "sonarr" {
		body["seriesIds"] = ids
	} else {
		body["movieIds"] = ids
	}
	if err := c.Put(ctx, listPath(v.Kind)+"/editor", body, nil); err != nil {
		return 0, fmt.Errorf("Unable to move the library: %w", err)
	}
	switch search {
	case "slow":
		b, _ := json.Marshal(libraryState{Since: time.Now().UTC().Format(time.RFC3339)})
		m.Store.SetSetting(searchKey(v.ID), string(b))
	case "now":
		m.Store.SetSetting(searchKey(v.ID), "")
		if v.Kind == "sonarr" {
			for _, id := range ids {
				if err := c.Post(ctx, "command", map[string]any{"name": "SeriesSearch", "seriesId": id}, nil); err != nil {
					return len(ids), fmt.Errorf("Unable to start the search: %w", err)
				}
			}
		} else if err := c.Post(ctx, "command", map[string]any{"name": "MoviesSearch", "movieIds": ids}, nil); err != nil {
			return len(ids), fmt.Errorf("Unable to start the search: %w", err)
		}
	}
	m.Log.Info("library moved", "server", v.Name, "titles", len(ids), "profile", profileID, "search", search)
	return len(ids), nil
}

// cutoffRow is one title Radarr/Sonarr says can still upgrade.
type cutoffRow struct {
	ID             int     `json:"id"`
	SeriesID       int     `json:"seriesId"`
	SeasonNumber   int     `json:"seasonNumber"`
	LastSearchTime *string `json:"lastSearchTime"`
}

// due lists what hasn't been searched since the move, oldest pages first, up to n. A series counts by season.
func due(ctx context.Context, v store.Server, since time.Time, n int) ([]cutoffRow, error) {
	var out []cutoffRow
	seen := map[[2]int]bool{}
	const size = 500
	for page := 1; len(out) < n; page++ {
		var res struct {
			Total   int         `json:"totalRecords"`
			Records []cutoffRow `json:"records"`
		}
		if err := client(v).Get(ctx, fmt.Sprintf("wanted/cutoff?page=%d&pageSize=%d&monitored=true", page, size), &res); err != nil {
			return nil, err
		}
		for _, r := range res.Records {
			if r.LastSearchTime != nil {
				if t, err := time.Parse(time.RFC3339, *r.LastSearchTime); err == nil && !t.Before(since) {
					continue
				}
			}
			if v.Kind == "sonarr" {
				k := [2]int{r.SeriesID, r.SeasonNumber}
				if seen[k] {
					continue
				}
				seen[k] = true
			}
			out = append(out, r)
			if len(out) == n {
				break
			}
		}
		if page*size >= res.Total {
			break
		}
	}
	return out, nil
}

// RunLibrarySearch is the "Library: slow search" job: today's batch on every server that has one going.
func (m *Manager) RunLibrarySearch(ctx context.Context) (string, error) {
	servers, err := m.Store.Servers()
	if err != nil {
		return "", err
	}
	if !pushAllowed() {
		return "Skipped on a dev install", nil
	}
	searched, running := 0, 0
	for _, v := range servers {
		var st libraryState
		if json.Unmarshal([]byte(m.Store.Setting(searchKey(v.ID), "")), &st) != nil || st.Since == "" {
			continue
		}
		since, err := time.Parse(time.RFC3339, st.Since)
		if err != nil {
			continue
		}
		batch, err := due(ctx, v, since, m.PerDay())
		if err != nil {
			return "", err
		}
		if len(batch) == 0 {
			m.Store.SetSetting(searchKey(v.ID), "")
			continue
		}
		running++
		c := client(v)
		if v.Kind == "sonarr" {
			for _, r := range batch {
				if err := c.Post(ctx, "command", map[string]any{"name": "SeasonSearch", "seriesId": r.SeriesID, "seasonNumber": r.SeasonNumber}, nil); err != nil {
					return "", err
				}
			}
		} else {
			ids := make([]int, len(batch))
			for i, r := range batch {
				ids[i] = r.ID
			}
			if err := c.Post(ctx, "command", map[string]any{"name": "MoviesSearch", "movieIds": ids}, nil); err != nil {
				return "", err
			}
		}
		searched += len(batch)
	}
	if running == 0 {
		return "Nothing to search", nil
	}
	return fmt.Sprintf("Searched %d title%s", searched, map[bool]string{true: "", false: "s"}[searched == 1]), nil
}
