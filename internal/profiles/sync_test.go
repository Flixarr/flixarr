package profiles

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

// fakeArr is a tiny Radarr: a quality schema, custom formats and quality profiles it stores as sent.
type fakeArr struct {
	mu       sync.Mutex
	next     int
	formats  map[int]json.RawMessage
	profiles map[int]json.RawMessage
	posts    int
}

const schemaJSON = `{"name":"","upgradeAllowed":false,"cutoff":0,"minFormatScore":0,"cutoffFormatScore":0,"minUpgradeFormatScore":1,"language":{"id":1,"name":"English"},"formatItems":[],
"items":[{"quality":{"id":0,"name":"Unknown","source":"unknown","resolution":0,"modifier":"none"},"items":[],"allowed":false},
{"quality":{"id":4,"name":"HDTV-720p","source":"tv","resolution":720,"modifier":"none"},"items":[],"allowed":false},
{"quality":{"id":5,"name":"WEBDL-720p","source":"webdl","resolution":720,"modifier":"none"},"items":[],"allowed":false},
{"quality":{"id":14,"name":"WEBRip-720p","source":"webrip","resolution":720,"modifier":"none"},"items":[],"allowed":false},
{"quality":{"id":6,"name":"Bluray-720p","source":"bluray","resolution":720,"modifier":"none"},"items":[],"allowed":false},
{"quality":{"id":3,"name":"WEBDL-1080p","source":"webdl","resolution":1080,"modifier":"none"},"items":[],"allowed":false},
{"quality":{"id":15,"name":"WEBRip-1080p","source":"webrip","resolution":1080,"modifier":"none"},"items":[],"allowed":false},
{"quality":{"id":7,"name":"Bluray-1080p","source":"bluray","resolution":1080,"modifier":"none"},"items":[],"allowed":false}]}`

func newFake() *fakeArr {
	f := &fakeArr{next: 100, formats: map[int]json.RawMessage{}, profiles: map[int]json.RawMessage{}}
	// Already on the server: an identical BR-DISK, and someone's own x265 (HD) with different rules.
	f.formats[1] = json.RawMessage(`{"id":1,"name":"BR-DISK","includeCustomFormatWhenRenaming":false,"specifications":[{"name":"BR-DISK","implementation":"QualityModifierSpecification","negate":false,"required":true,"fields":[{"name":"value","label":"Quality Modifier","value":5}]}]}`)
	f.formats[2] = json.RawMessage(`{"id":2,"name":"x265 (HD)","includeCustomFormatWhenRenaming":false,"specifications":[{"name":"x265","implementation":"ReleaseTitleSpecification","negate":false,"required":true,"fields":[{"name":"value","value":"x265"}]}]}`)
	f.profiles[1] = json.RawMessage(`{"id":1,"name":"Default Quality","items":[],"formatItems":[]}`)
	return f
}

func (f *fakeArr) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/api/v3/")
	parts := strings.Split(path, "/")
	list := func(m map[int]json.RawMessage) {
		out := []json.RawMessage{}
		for i := 0; i < 1000; i++ {
			if v, ok := m[i]; ok {
				out = append(out, v)
			}
		}
		json.NewEncoder(w).Encode(out)
	}
	store := func(m map[int]json.RawMessage, id int) {
		var v map[string]any
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &v)
		v["id"] = id
		raw, _ := json.Marshal(v)
		m[id] = raw
		w.Write(raw)
	}
	switch {
	case path == "qualityprofile/schema":
		w.Write([]byte(schemaJSON))
	case path == "language":
		w.Write([]byte(`[{"id":-2,"name":"Original"},{"id":1,"name":"English"}]`))
	case parts[0] == "customformat" || parts[0] == "qualityprofile":
		m := f.formats
		if parts[0] == "qualityprofile" {
			m = f.profiles
		}
		switch {
		case r.Method == http.MethodGet && len(parts) == 1:
			list(m)
		case r.Method == http.MethodGet:
			id, _ := strconv.Atoi(parts[1])
			if v, ok := m[id]; ok {
				w.Write(v)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case r.Method == http.MethodPost:
			f.posts++
			f.next++
			store(m, f.next)
		case r.Method == http.MethodPut:
			id, _ := strconv.Atoi(parts[1])
			store(m, id)
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func testDoc(rev string, brScore int) *Doc {
	return &Doc{Version: "v" + rev, Profiles: []Profile{{ID: "standard", Name: "Standard", Label: "Standard (1080p)", Rev: rev,
		Radarr: &AppProfile{Profile: Body{UpgradeAllowed: true, Cutoff: "Bluray-1080p", CutoffFormatScore: 10000, MinUpgradeFormatScore: 1, Language: "Original",
			Items:       []Item{{Name: "Bluray-1080p", Allowed: true}, {Name: "WEB 1080p", Allowed: true, Items: []string{"WEBRip-1080p", "WEBDL-1080p"}}, {Name: "Bluray-720p", Allowed: true}, {Name: "HDTV-720p", Allowed: false}},
			FormatItems: []FormatItem{{Name: "BR-DISK", Score: brScore}, {Name: "x265 (HD)", Score: -10000}, {Name: "WEB Tier 01", Score: 1700}}}}}},
		CustomFormats: map[string]map[string]CustomFormat{"radarr": {
			"BR-DISK":     {Name: "BR-DISK", Specifications: []Spec{{Name: "BR-DISK", Implementation: "QualityModifierSpecification", Required: true, Fields: []Field{{Name: "value", Value: 5}}}}},
			"x265 (HD)":   {Name: "x265 (HD)", Specifications: []Spec{{Name: "x265", Implementation: "ReleaseTitleSpecification", Required: true, Fields: []Field{{Name: "value", Value: "[xh][ .]?265|\\bHEVC(\\b|\\d)"}}}}},
			"WEB Tier 01": {Name: "WEB Tier 01", Specifications: []Spec{{Name: "BLURAY", Implementation: "SourceSpecification", Negate: true, Fields: []Field{{Name: "value", Value: 9}}}}},
		}}}
}

func TestAddSyncCustomize(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, err := store.Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	fake := newFake()
	srv := httptest.NewServer(fake)
	defer srv.Close()
	v, _ := st.SaveServer(store.Server{Kind: "radarr", Name: "Radarr", URL: srv.URL, APIKey: "k", RootFolder: "/m", QualityProfileID: 1, Enabled: true})
	m := &Manager{Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	m.doc = testDoc("r1", -10000)
	ctx := context.Background()

	if err := m.Add(ctx, v, nil); err != nil {
		t.Fatal(err)
	}
	// BR-DISK was identical: reused, untouched. x265 (HD) differed: a "(Flixarr)" copy was added beside it.
	var names []string
	for _, raw := range fake.formats {
		var f arrFormat
		json.Unmarshal(raw, &f)
		names = append(names, f.Name)
	}
	joined := strings.Join(names, "|")
	if !strings.Contains(joined, "x265 (HD) (Flixarr)") || strings.Count(joined, "BR-DISK") != 1 || !strings.Contains(joined, "WEB Tier 01") {
		t.Fatalf("formats on server: %v", names)
	}
	if string(fake.formats[2]) != `{"id":2,"name":"x265 (HD)","includeCustomFormatWhenRenaming":false,"specifications":[{"name":"x265","implementation":"ReleaseTitleSpecification","negate":false,"required":true,"fields":[{"name":"value","value":"x265"}]}]}` {
		t.Fatal("someone's own custom format was changed")
	}
	links, _ := st.ProfileLinks(v.ID)
	if len(links) != 1 || links[0].State != "ok" {
		t.Fatalf("links: %+v", links)
	}
	var p arrProfile
	json.Unmarshal(fake.profiles[links[0].ArrID], &p)
	if p.Name != "Flixarr · Standard" || p.Language == nil || p.Language.Name != "Original" {
		t.Fatalf("profile: %+v", p)
	}
	// Worst first: the last item is the best quality; the WEB group got an ID and the cutoff points at Bluray-1080p.
	if last := p.Items[len(p.Items)-1]; last.Quality == nil || last.Quality.Name != "Bluray-1080p" || p.Cutoff != 7 {
		t.Fatalf("items/cutoff: %+v cutoff %d", last, p.Cutoff)
	}
	scores := map[string]int{}
	for _, f := range p.FormatItems {
		scores[f.Name] = f.Score
	}
	if scores["BR-DISK"] != -10000 || scores["x265 (HD) (Flixarr)"] != -10000 || scores["x265 (HD)"] != 0 || scores["WEB Tier 01"] != 1700 {
		t.Fatalf("scores: %v", scores)
	}

	// Same version: a sync changes nothing.
	posts := fake.posts
	if n, err := m.Sync(ctx, v); err != nil || n != 0 || fake.posts != posts {
		t.Fatalf("idle sync: %d %v", n, err)
	}
	// A new version is pushed to an untouched profile.
	m.doc = testDoc("r2", -9999)
	if n, err := m.Sync(ctx, v); err != nil || n != 1 {
		t.Fatalf("update sync: %d %v", n, err)
	}
	json.Unmarshal(fake.profiles[links[0].ArrID], &p)
	for _, f := range p.FormatItems {
		if f.Name == "BR-DISK" && f.Score != -9999 {
			t.Fatal("update wasn't pushed")
		}
	}
	// Someone edits it in Radarr: the next version is held back and the profile is marked customized.
	var edit map[string]any
	json.Unmarshal(fake.profiles[links[0].ArrID], &edit)
	edit["cutoffFormatScore"] = 500
	fake.profiles[links[0].ArrID], _ = json.Marshal(edit)
	m.doc = testDoc("r3", -10000)
	if n, _ := m.Sync(ctx, v); n != 0 {
		t.Fatal("pushed over someone's edit")
	}
	stat, _ := m.Status(v)
	if stat[0].State != "customized" || !stat[0].UpdateAvailable {
		t.Fatalf("status after edit: %+v", stat[0])
	}
	// "Sync" takes Flixarr's version anyway.
	if err := m.UseFlixarrVersion(ctx, v, "standard"); err != nil {
		t.Fatal(err)
	}
	stat, _ = m.Status(v)
	if stat[0].State != "ok" || stat[0].UpdateAvailable {
		t.Fatalf("status after sync: %+v", stat[0])
	}
	// Deleted in Radarr: marked missing, not put back on its own.
	delete(fake.profiles, links[0].ArrID)
	posts = fake.posts
	m.Sync(ctx, v)
	stat, _ = m.Status(v)
	if stat[0].State != "missing" || stat[0].Installed || fake.posts != posts {
		t.Fatalf("status after delete: %+v posts %d→%d", stat[0], posts, fake.posts)
	}
	// Adding it again works.
	if err := m.Add(ctx, v, []string{"standard"}); err != nil {
		t.Fatal(err)
	}
	if stat, _ = m.Status(v); stat[0].State != "ok" {
		t.Fatalf("re-add: %+v", stat[0])
	}
}
