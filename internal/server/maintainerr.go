package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Flixarr/flixarr/internal/maintainerr"
	"github.com/Flixarr/flixarr/internal/store"
)

// Maintainerr (Settings → Services): the connection, its collections, and the leaving titles.

func (s *Server) maintainerrGet(w http.ResponseWriter, r *http.Request, _ *store.User) {
	out := map[string]any{"connected": false, "protectDays": s.Maint.ProtectDays(), "stats": s.Maint.Stats()}
	c, ok := s.Maint.Client()
	if !ok {
		writeJSON(w, http.StatusOK, out)
		return
	}
	out["connected"], out["url"], out["keySet"] = true, c.URL, c.APIKey != ""
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if st, err := c.Status(ctx); err != nil {
		out["error"] = capitalize(err.Error()) + "."
	} else {
		out["version"] = st.Version
		out["webhook"] = c.HasWebhook(ctx)
	}
	now, _ := s.Store.LeavingNow(s.Maint.Seen())
	keeps, _ := s.Store.Keeps()
	out["leaving"], out["keeps"], out["seen"] = len(now), len(keeps), s.Maint.Seen()
	writeJSON(w, http.StatusOK, out)
}

// maintainerrSet connects (or changes) Maintainerr: {url, apiKey, clearKey, protectDays}. The address is
// checked first; the API key is kept if left blank.
func (s *Server) maintainerrSet(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var in struct {
		URL         string `json:"url"`
		APIKey      string `json:"apiKey"`
		ClearKey    bool   `json:"clearKey"`
		ProtectDays *int   `json:"protectDays"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	if in.ProtectDays != nil {
		if *in.ProtectDays < 0 || *in.ProtectDays > 365 {
			writeErr(w, http.StatusBadRequest, "Pick 0 to 365 days.")
			return
		}
		s.Store.SetSetting(maintainerr.KeyProtect, strconv.Itoa(*in.ProtectDays))
	}
	if in.URL != "" {
		c := s.maintainerrClient(in.URL, in.APIKey, in.ClearKey)
		if _, err := c.Status(r.Context()); err != nil {
			writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
			return
		}
		u, key := c.URL, c.APIKey
		s.Store.SetSetting(maintainerr.KeyURL, u)
		s.Store.SetSecretSetting(maintainerr.KeyAPIKey, key)
		go s.Maint.Sync(s.BaseCtx)
	}
	s.maintainerrGet(w, r, nil)
}

// maintainerrClient is a Maintainerr at this address; a blank key keeps the saved one (unless clearKey).
func (s *Server) maintainerrClient(raw, key string, clearKey bool) *maintainerr.Client {
	u := strings.TrimRight(strings.TrimSpace(raw), "/")
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		u = "http://" + u
	}
	if key == "" && !clearKey {
		key = s.Store.SecretSetting(maintainerr.KeyAPIKey, "")
	}
	return &maintainerr.Client{URL: u, APIKey: key}
}

// maintainerrTest checks an address (and key) without saving anything: {url, apiKey} → {version}.
func (s *Server) maintainerrTest(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var in struct {
		URL    string `json:"url"`
		APIKey string `json:"apiKey"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil || strings.TrimSpace(in.URL) == "" {
		writeErr(w, http.StatusBadRequest, "Enter the address.")
		return
	}
	st, err := s.maintainerrClient(in.URL, in.APIKey, false).Status(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// maintainerrRemove disconnects Maintainerr. Titles Flixarr kept stay kept in Maintainerr.
func (s *Server) maintainerrRemove(w http.ResponseWriter, r *http.Request, _ *store.User) {
	s.Store.SetSetting(maintainerr.KeyURL, "")
	s.Store.SetSecretSetting(maintainerr.KeyAPIKey, "")
	s.Store.SyncLeaving(nil, time.Now().UTC().Format(time.RFC3339))
	s.Store.SetSetting("maintainerr_seen", "")
	w.WriteHeader(http.StatusNoContent)
}

// maintainerrFind looks for Maintainerr on its usual port: by name, on this machine, and on the Docker
// host and its local network.
func (s *Server) maintainerrFind(w http.ResponseWriter, r *http.Request, _ *store.User) {
	hosts := []string{"maintainerr", "localhost", "host.docker.internal"}
	if gw := gatewayGuess(); gw != "" {
		hosts = append(hosts, gw)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	var mu sync.Mutex
	var wg sync.WaitGroup
	found := map[string]bool{}
	for _, h := range hosts {
		wg.Add(1)
		go func(h string) {
			defer wg.Done()
			u := fmt.Sprintf("http://%s:%d", h, maintainerr.DefaultPort)
			if _, err := (&maintainerr.Client{URL: u}).Status(ctx); err == nil {
				mu.Lock()
				found[u] = true
				mu.Unlock()
			}
		}(h)
	}
	wg.Wait()
	out := []string{}
	for _, h := range hosts { // in the order tried: names first
		if u := fmt.Sprintf("http://%s:%d", h, maintainerr.DefaultPort); found[u] {
			out = append(out, u)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"found": out})
}

// gatewayGuess is this machine's default route address with the last part set to 1 (the Docker host,
// for a container on a bridge network).
func gatewayGuess() string {
	conn, err := net.Dial("udp", "192.0.2.1:9")
	if err != nil {
		return ""
	}
	defer conn.Close()
	ip := conn.LocalAddr().(*net.UDPAddr).IP.To4()
	if ip == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.1", ip[0], ip[1], ip[2])
}

// maintainerrWebhook adds Flixarr to Maintainerr's notifications: {base} is the address Maintainerr
// reaches Flixarr on (the same one Radarr and Sonarr use).
func (s *Server) maintainerrWebhook(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var in struct {
		Base string `json:"base"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&in)
	c, ok := s.Maint.Client()
	if !ok {
		writeErr(w, http.StatusBadRequest, "Connect Maintainerr first.")
		return
	}
	base := strings.TrimRight(strings.TrimSpace(in.Base), "/")
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		writeErr(w, http.StatusBadRequest, "Enter the address Maintainerr can reach Flixarr on, like http://192.168.1.10:7575.")
		return
	}
	if err := c.AddWebhook(r.Context(), base+"/api/webhook/maintainerr/"+s.webhookToken()); err != nil {
		writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
		return
	}
	s.Store.SetSetting("webhook_base", base)
	w.WriteHeader(http.StatusNoContent)
}

// maintainerrHook takes Maintainerr's webhook: something joined, left or was removed from a collection,
// so read the collections again now.
func (s *Server) maintainerrHook(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.PathValue("token")), []byte(s.webhookToken())) != 1 {
		writeErr(w, http.StatusNotFound, "Unknown endpoint.")
		return
	}
	var in struct {
		Event   string `json:"event"`
		Subject string `json:"subject"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in)
	s.Log.Info("maintainerr webhook", "event", in.Event, "subject", in.Subject)
	go func() {
		if _, err := s.Maint.Sync(s.BaseCtx); err != nil {
			s.Log.Warn("maintainerr read", "err", err)
		}
		if strings.Contains(strings.ToLower(in.Event), "handled") && !strings.Contains(strings.ToLower(in.Event), "about") {
			s.Library.Rescan(s.BaseCtx, "movie") // something was deleted: Plex notices, and so does Flixarr
			s.Library.Rescan(s.BaseCtx, "series")
		}
	}()
	w.WriteHeader(http.StatusNoContent)
}

// leavingInfo is a title's leaving state for its page.
type leavingInfo struct {
	At         string   `json:"at,omitempty"`         // when Maintainerr removes it
	Collection string   `json:"collection,omitempty"` // which rule
	Kept       string   `json:"kept,omitempty"`       // kept | fresh: Flixarr keeps it out of the rules
	KeptUntil  string   `json:"keptUntil,omitempty"`
	Asked      bool     `json:"asked,omitempty"` // you asked to keep it
	AskedBy    []string `json:"askedBy,omitempty"`
}

// leaving answers for a batch of titles (?keys=movie:603) and, without keys, lists every leaving title
// as cards for the Discover row and the Leaving soon page.
func (s *Server) leaving(w http.ResponseWriter, r *http.Request, u *store.User) {
	if _, ok := s.Maint.Client(); !ok {
		writeJSON(w, http.StatusOK, map[string]any{"connected": false, "results": []any{}})
		return
	}
	now, err := s.Store.LeavingNow(s.Maint.Seen())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	asks, _ := s.Store.KeepAsks()
	info := func(typ string, id int, l *store.Leaving) leavingInfo {
		out := leavingInfo{}
		if l != nil {
			out.At, out.Collection = l.LeaveAt, l.Collection
		}
		if k, ok := s.Store.KeepFor(typ, id); ok {
			out.Kept, out.KeptUntil = k.Reason, k.UntilAt
		}
		for _, a := range asks {
			if a.Type == typ && a.TmdbID == id {
				if a.UserID == u.ID {
					out.Asked = true
				}
				if u.Can("manageRequests") {
					out.AskedBy = append(out.AskedBy, a.UserName)
				}
			}
		}
		return out
	}
	if q := r.URL.Query().Get("keys"); q != "" {
		out := map[string]leavingInfo{}
		for _, k := range strings.Split(q, ",") {
			typ, ids, _ := strings.Cut(strings.TrimSpace(k), ":")
			id, _ := strconv.Atoi(ids)
			var l *store.Leaving
			for i := range now {
				if now[i].Type == typ && now[i].TmdbID == id {
					l = &now[i]
				}
			}
			if li := info(typ, id, l); li.At != "" || li.Kept != "" {
				out[k] = li
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"connected": true, "titles": out})
		return
	}
	refs := []titleRef{}
	for _, l := range now {
		refs = append(refs, titleRef{Type: l.Type, ID: l.TmdbID})
	}
	cards := s.cards(r, refs)
	results := []map[string]any{}
	for i, c := range cards {
		if c == nil {
			continue // the API didn't know it
		}
		results = append(results, map[string]any{"card": c, "leaving": info(now[i].Type, now[i].TmdbID, &now[i])})
	}
	// Titles people asked to keep come first for the people who decide.
	if u.Can("manageRequests") {
		slices.SortStableFunc(results, func(a, b map[string]any) int {
			return len(b["leaving"].(leavingInfo).AskedBy) - len(a["leaving"].(leavingInfo).AskedBy)
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"connected": true, "results": results})
}

// leavingAct is Keep / Let go / Give more time (manage requests) and Ask to keep (anyone who can request).
func (s *Server) leavingAct(w http.ResponseWriter, r *http.Request, u *store.User) {
	typ, act := r.PathValue("type"), r.PathValue("act")
	id, _ := strconv.Atoi(r.PathValue("id"))
	if (typ != "movie" && typ != "series") || id <= 0 {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	var err error
	switch act {
	case "ask":
		if !u.Can(map[string]string{"movie": "requestMovies", "series": "requestTv"}[typ]) {
			writeErr(w, http.StatusForbidden, "You can't ask to keep this.")
			return
		}
		err = s.Maint.Ask(typ, id, u.ID)
	case "keep", "letgo", "postpone":
		if !u.Can("manageRequests") {
			writeErr(w, http.StatusForbidden, "Only people who manage requests can do that.")
			return
		}
		switch act {
		case "keep":
			err = s.Maint.Keep(r.Context(), typ, id, u.ID)
		case "letgo":
			err = s.Maint.LetGo(r.Context(), typ, id)
		default:
			var in struct {
				Days int `json:"days"`
			}
			json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in)
			if in.Days < 1 || in.Days > 365 {
				in.Days = 30
			}
			err = s.Maint.Postpone(r.Context(), typ, id, in.Days)
		}
	default:
		writeErr(w, http.StatusNotFound, "Unknown endpoint.")
		return
	}
	if err != nil {
		writeErr(w, http.StatusConflict, capitalize(err.Error())+".")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
