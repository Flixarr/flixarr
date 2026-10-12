package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

func TestBackupPruneRestore(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, err := store.Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	st.SetSetting("marker", "before")
	m := &Manager{Store: st, DataDir: dir}
	st.SetSetting(KeyKeep, "2")
	b1, err := m.Create("")
	if err != nil || b1.Size == 0 {
		t.Fatalf("create: %+v %v", b1, err)
	}
	for i := 0; i < 2; i++ {
		time.Sleep(1100 * time.Millisecond) // names are per second
		if _, err := m.Create("manual"); err != nil {
			t.Fatal(err)
		}
	}
	if l := m.List(); len(l) != 2 || l[0].Kind != "manual" {
		t.Fatalf("kept: %+v", l)
	}
	if _, err := m.Path("../flixarr.db"); err == nil {
		t.Fatal("path escape allowed")
	}
	// Restore the newest backup after changing the marker.
	target := m.List()[0].Name
	st.SetSetting("marker", "after")
	time.Sleep(1100 * time.Millisecond)
	if err := m.PrepareRestore(target); err != nil {
		t.Fatal(err)
	}
	st.Close()
	if ok, err := ApplyRestore(dir); !ok || err != nil {
		t.Fatalf("apply: %v %v", ok, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "restore.db")); err == nil {
		t.Fatal("staged file left behind")
	}
	st2, err := store.Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	if v := st2.Setting("marker", ""); v != "before" {
		t.Fatalf("marker after restore: %q", v)
	}
	kinds := map[string]bool{}
	for _, b := range (&Manager{Store: st2, DataDir: dir}).List() {
		kinds[b.Kind] = true
	}
	if !kinds["before-restore"] {
		t.Fatalf("no before-restore backup: %v", kinds)
	}
}
