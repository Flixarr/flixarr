package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/Flixarr/flixarr/internal/store"
)

// card is the poster-row shape the web app uses everywhere (types.ts Card).
type card struct {
	ID       int     `json:"id"`
	Type     string  `json:"type"`
	Title    string  `json:"title"`
	Poster   *string `json:"poster"`
	Backdrop *string `json:"backdrop"`
	Year     string  `json:"year"`
	Rating   float64 `json:"rating"`
	Genres   []int   `json:"genres"`
	Overview string  `json:"overview"`
}

func strOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// titleRef is a title to show as a poster.
type titleRef struct {
	Type     string
	ID       int
	Fallback card // used when the Flixarr API can't be reached
}

// cards fills poster rows from the Flixarr API's detail pages (cached after the first time), a few at a time.
func (s *Server) cards(r *http.Request, refs []titleRef) []*card {
	q := url.Values{}
	q.Set("region", s.Store.Setting("region", "US"))
	q.Set("language", s.Store.Setting("language", "en-US"))
	out := make([]*card, len(refs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4) // gentle on the API's per-install throttle
	for i, ref := range refs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if body, status, err := s.Meta.Get(r.Context(), fmt.Sprintf("%s/%d", ref.Type, ref.ID), q); err == nil && status == http.StatusOK {
				var d struct {
					card
					Genres []json.RawMessage `json:"genres"` // names on detail pages; rows don't need them
				}
				if json.Unmarshal(body, &d) == nil && d.ID != 0 {
					c := d.card
					c.Type, c.Genres = ref.Type, []int{}
					out[i] = &c
					return
				}
			}
			if ref.Fallback.ID != 0 {
				c := ref.Fallback
				out[i] = &c
			}
		}()
	}
	wg.Wait()
	list := []*card{}
	for _, c := range out {
		if c != nil {
			list = append(list, c)
		}
	}
	return list
}

// recentlyAdded is the Discover row of the newest titles in Plex.
func (s *Server) recentlyAdded(w http.ResponseWriter, r *http.Request, _ *store.User) {
	items, err := s.Store.RecentPlexItems(20)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	refs := []titleRef{}
	for _, it := range items {
		refs = append(refs, titleRef{Type: it.Type, ID: it.TmdbID})
	}
	writeJSON(w, http.StatusOK, map[string]any{"page": 1, "totalPages": 1, "results": s.cards(r, refs)})
}

// recentRequests is the Discover row of the newest requests: everyone's for people who can see them,
// otherwise your own.
func (s *Server) recentRequests(w http.ResponseWriter, r *http.Request, u *store.User) {
	owner := u.ID
	if u.Can("viewRequests") || u.Can("manageRequests") {
		owner = 0
	}
	list, err := s.Store.Requests(owner, 20)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	refs := []titleRef{}
	for _, q := range list {
		if q.Status != "declined" {
			refs = append(refs, titleRef{Type: q.Type, ID: q.TmdbID, Fallback: card{ID: q.TmdbID, Type: q.Type, Title: q.Title, Poster: strOrNil(q.Poster), Backdrop: strOrNil(q.Backdrop), Year: q.Year, Genres: []int{}}})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"page": 1, "totalPages": 1, "results": s.cards(r, refs)})
}
