package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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
	region, language := s.locale(r)
	q.Set("region", region)
	q.Set("language", language)
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
	if r.URL.Query().Get("mine") == "" {
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

// myStats is the Profile page's tiles: your requests by where they are, and your open issues.
func (s *Server) myStats(w http.ResponseWriter, r *http.Request, u *store.User) {
	list, err := s.Store.Requests(u.ID, 1000)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	out := map[string]int{"requests": len(list), "available": 0, "waiting": 0, "inProgress": 0, "openIssues": 0}
	for _, q := range list {
		switch q.Status {
		case "available":
			out["available"]++
		case "pending":
			out["waiting"]++
		case "approved", "sent", "downloading":
			out["inProgress"]++
		}
	}
	if issues, err := s.Store.Issues(u.ID, "open"); err == nil {
		out["openIssues"] = len(issues)
	}
	writeJSON(w, http.StatusOK, out)
}

// library is Movies / Series → In your library: what's in Plex, newest first or A to Z, 20 at a time.
func (s *Server) library(w http.ResponseWriter, r *http.Request, _ *store.User) {
	typ := r.PathValue("type")
	if typ != "movie" && typ != "series" {
		writeErr(w, http.StatusNotFound, "Unknown type.")
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	page = max(page, 1)
	items, total, err := s.Store.PlexItemsPage(typ, r.URL.Query().Get("sort"), page, 20)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	refs := []titleRef{}
	for _, it := range items {
		refs = append(refs, titleRef{Type: it.Type, ID: it.TmdbID, Fallback: card{ID: it.TmdbID, Type: it.Type, Title: it.Title, Genres: []int{}}})
	}
	writeJSON(w, http.StatusOK, map[string]any{"page": page, "totalPages": max((total+19)/20, 1), "results": s.cards(r, refs)})
}
