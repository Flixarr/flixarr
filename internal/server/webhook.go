package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Flixarr/flixarr/internal/arr"
	"github.com/Flixarr/flixarr/internal/store"
	"github.com/Flixarr/flixarr/internal/testinstall"
)

const keyWebhookToken = "webhook_token"

// webhookToken is the secret part of the URL Radarr and Sonarr call (made on first use).
func (s *Server) webhookToken() string {
	t := s.Store.SecretSetting(keyWebhookToken, "")
	if t == "" {
		b := make([]byte, 18)
		rand.Read(b)
		t = hex.EncodeToString(b)
		s.Store.SetSecretSetting(keyWebhookToken, t)
	}
	return t
}

// webhookInfo is what Settings → Services shows: the URL to paste into Radarr and Sonarr.
func (s *Server) webhookInfo(w http.ResponseWriter, r *http.Request, _ *store.User) {
	base := s.Store.Setting("app_url", "")
	if saved := s.Store.Setting("webhook_base", ""); saved != "" {
		base = saved
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": "/api/webhook/" + s.webhookToken(), "base": base, "last": s.Store.Setting("webhook_last", "")})
}

// webhook takes Radarr/Sonarr's Webhook connection: on a grab, import or new item, the matching requests
// are followed right away instead of on the next pass. Radarr sends {"eventType", "movie": {"id"}},
// Sonarr {"eventType", "series": {"id"}}.
func (s *Server) webhook(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.PathValue("token")), []byte(s.webhookToken())) != 1 {
		writeErr(w, http.StatusNotFound, "Unknown endpoint.")
		return
	}
	var in struct {
		EventType    string `json:"eventType"`
		InstanceName string `json:"instanceName"`
		Movie        *struct {
			ID int `json:"id"`
		} `json:"movie"`
		Series *struct {
			ID int `json:"id"`
		} `json:"series"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	s.Store.SetSetting("webhook_last", time.Now().UTC().Format(time.RFC3339)+" "+in.InstanceName+" "+in.EventType)
	typ, id := "", 0
	switch {
	case in.Movie != nil:
		typ, id = "movie", in.Movie.ID
	case in.Series != nil:
		typ, id = "series", in.Series.ID
	}
	s.Log.Info("webhook", "from", in.InstanceName, "event", in.EventType, "type", typ, "id", id)
	if id == 0 && !strings.EqualFold(in.EventType, "Test") {
		go s.Arr.RefreshAll(s.BaseCtx) // a health problem (or it cleared): the cards and bell alerts now
	}
	if id != 0 && !strings.EqualFold(in.EventType, "Test") {
		go func() {
			// Now, and again shortly: the queue or the file can lag the event by a moment.
			for _, wait := range []time.Duration{0, 3 * time.Second, 15 * time.Second} {
				time.Sleep(wait)
				ctx, cancel := context.WithTimeout(s.BaseCtx, 30*time.Second)
				s.Requests.Nudge(ctx, typ, id)
				cancel()
			}
			s.Arr.RefreshAll(s.BaseCtx) // the Services cards too
		}()
	}
	w.WriteHeader(http.StatusOK)
}

// arrWebhook finds Flixarr's Webhook connection in a Radarr/Sonarr (Settings → Connect), if added.
func arrWebhook(ctx context.Context, c *arr.Client, token string) (map[string]any, error) {
	if testinstall.On {
		return nil, testinstall.ErrBlocked
	}
	var list []map[string]any
	if err := c.Get(ctx, "notification", &list); err != nil {
		return nil, err
	}
	for _, n := range list {
		if n["implementation"] != "Webhook" {
			continue
		}
		fields, _ := n["fields"].([]any)
		for _, f := range fields {
			m, _ := f.(map[string]any)
			if v, _ := m["value"].(string); m["name"] == "url" && strings.Contains(v, "/api/webhook/"+token) {
				return n, nil
			}
		}
	}
	return nil, nil
}

// webhookStatus says, per connection, whether Flixarr's webhook is added.
func (s *Server) webhookStatus(w http.ResponseWriter, r *http.Request, _ *store.User) {
	v, ok, err := s.Store.FirstServer(r.PathValue("kind"))
	if err != nil || !ok {
		writeErr(w, http.StatusNotFound, "Not connected.")
		return
	}
	n, err := arrWebhook(r.Context(), &arr.Client{Kind: v.Kind, URL: v.URL, APIKey: v.APIKey}, s.webhookToken())
	if err != nil {
		writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
		return
	}
	url := ""
	if n != nil {
		for _, f := range n["fields"].([]any) {
			if m, _ := f.(map[string]any); m["name"] == "url" {
				url, _ = m["value"].(string)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": n != nil, "url": url})
}

// webhookAdd adds (or points again) a "Flixarr" Webhook connection in Radarr/Sonarr with the events that
// matter for requests and health, then has Radarr/Sonarr test it.
func (s *Server) webhookAdd(w http.ResponseWriter, r *http.Request, _ *store.User) {
	v, ok, err := s.Store.FirstServer(r.PathValue("kind"))
	if err != nil || !ok {
		writeErr(w, http.StatusNotFound, "Not connected.")
		return
	}
	var in struct {
		Base string `json:"base"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&in); err != nil || !strings.HasPrefix(in.Base, "http") {
		writeErr(w, http.StatusBadRequest, "Enter the address Radarr and Sonarr use to reach Flixarr, like http://192.168.1.10:7575.")
		return
	}
	hook := strings.TrimRight(in.Base, "/") + "/api/webhook/" + s.webhookToken()
	c := &arr.Client{Kind: v.Kind, URL: v.URL, APIKey: v.APIKey}
	n, err := arrWebhook(r.Context(), c, s.webhookToken())
	if err != nil {
		writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
		return
	}
	if n == nil {
		var schema []map[string]any
		if err := c.Get(r.Context(), "notification/schema", &schema); err != nil {
			writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
			return
		}
		for _, sc := range schema {
			if sc["implementation"] == "Webhook" {
				n = sc
			}
		}
		if n == nil {
			writeErr(w, http.StatusBadGateway, "This version doesn't offer webhooks.")
			return
		}
	}
	n["name"] = "Flixarr"
	for _, f := range n["fields"].([]any) {
		m, _ := f.(map[string]any)
		switch m["name"] {
		case "url":
			m["value"] = hook
		case "method":
			m["value"] = 1 // POST
		}
	}
	for _, ev := range []string{"onGrab", "onDownload", "onUpgrade", "onImportComplete", "onMovieAdded", "onSeriesAdd", "onHealthIssue", "onHealthRestored", "onManualInteractionRequired"} {
		if _, has := n[ev]; has { // only the events this app has
			n[ev] = true
		}
	}
	// The Test button in Radarr/Sonarr first: it fails with a clear message when the address is wrong.
	if err := c.Post(r.Context(), "notification/test", n, nil); err != nil {
		writeErr(w, http.StatusBadGateway, arr.Kinds[v.Kind]+" couldn't reach Flixarr at "+in.Base+". Use the address "+arr.Kinds[v.Kind]+" can reach (often the local IP and port).")
		return
	}
	if id, _ := n["id"].(float64); id > 0 {
		err = c.Put(r.Context(), "notification/"+strconv.Itoa(int(id)), n, nil)
	} else {
		err = c.Post(r.Context(), "notification", n, nil)
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
		return
	}
	s.Store.SetSetting("webhook_base", in.Base)
	writeJSON(w, http.StatusOK, map[string]any{"added": true, "url": hook})
}
