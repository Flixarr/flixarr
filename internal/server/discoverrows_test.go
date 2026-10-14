package server

import (
	"encoding/json"
	"testing"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

func TestDiscoverRows(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	t.Cleanup(func() { st.Close() })
	s := &Server{Store: st}
	rows := s.discoverRows()
	if len(rows) != len(discoverRowKeys) || !rows[0].On || rows[len(rows)-1].On {
		t.Fatalf("defaults: %+v", rows)
	}
	b, _ := json.Marshal([]discoverRow{{Key: "trending", On: false}, {Key: "custom-1", On: true, Title: " Horror ", Kind: "movie", Genre: 27}, {Key: "custom-2", Title: "Bad", Kind: "music"}, {Key: "nope", On: true}})
	st.SetSetting("discover_rows", string(b))
	rows = s.discoverRows()
	if rows[0].Key != "trending" || rows[0].On || rows[1].Key != "custom-1" || rows[1].Title != "Horror" || rows[1].Sort != "popular" {
		t.Fatalf("saved: %+v", rows[:2])
	}
	for _, r := range rows {
		if r.Key == "custom-2" || r.Key == "nope" {
			t.Fatalf("kept a bad row: %+v", r)
		}
	}
	if len(rows) != len(discoverRowKeys)+1 {
		t.Fatalf("count: %d", len(rows))
	}
}
