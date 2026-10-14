package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Flixarr/flixarr/internal/store"
)

func (s *Server) blockList(w http.ResponseWriter, r *http.Request, _ *store.User) {
	list, err := s.Store.Blocklist()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) blockAdd(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in store.Blocked
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil || (in.Type != "movie" && in.Type != "series") || in.TmdbID <= 0 || in.Title == "" {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	if err := s.Store.Block(in, u.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) blockRemove(w http.ResponseWriter, r *http.Request, _ *store.User) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	if err := s.Store.Unblock(r.PathValue("type"), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// KeyHideBlocked: "true" = blocked titles are left out of Discover, search, browse, collections and
// recommendations for everyone who doesn't manage requests (they still find them, to unblock).
const KeyHideBlocked = "blocklist_hide"

// blockSettings reads or changes Settings → Blocklist: {hide}.
func (s *Server) blockSettings(w http.ResponseWriter, r *http.Request, _ *store.User) {
	if r.Method == http.MethodPut {
		var in struct {
			Hide bool `json:"hide"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "Bad request.")
			return
		}
		s.Store.SetSetting(KeyHideBlocked, strconv.FormatBool(in.Hide))
	}
	writeJSON(w, http.StatusOK, map[string]any{"hide": s.Store.Setting(KeyHideBlocked, "false") == "true"})
}

// hideBlocked drops blocked titles from a Flixarr API answer: any {id, type} title inside a list.
// The title's own page (not in a list) is left alone. The body is returned as it was when nothing
// is blocked or it can't be read.
func (s *Server) hideBlocked(body []byte, u *store.User) []byte {
	if u.Can("manageRequests") || s.Store.Setting(KeyHideBlocked, "false") != "true" {
		return body
	}
	list, err := s.Store.Blocklist()
	if err != nil || len(list) == 0 {
		return body
	}
	blocked := map[string]bool{}
	for _, b := range list {
		blocked[b.Type+":"+strconv.Itoa(b.TmdbID)] = true
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	var v any
	if d.Decode(&v) != nil {
		return body
	}
	out, err := json.Marshal(dropBlocked(v, blocked))
	if err != nil {
		return body
	}
	return out
}

func dropBlocked(v any, blocked map[string]bool) any {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			t[k] = dropBlocked(x, blocked)
		}
	case []any:
		keep := t[:0]
		for _, x := range t {
			if m, ok := x.(map[string]any); ok {
				typ, _ := m["type"].(string)
				if id, ok := m["id"].(json.Number); ok && blocked[typ+":"+id.String()] {
					continue
				}
			}
			keep = append(keep, dropBlocked(x, blocked))
		}
		return keep
	}
	return v
}
