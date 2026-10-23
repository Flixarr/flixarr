package backup

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

// The example from AWS's Signature Version 4 documentation for S3 (GET with a Range header).
func TestSignMatchesAWSExample(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	now, _ := time.Parse("20060102T150405Z", "20130524T000000Z")
	sign(req, nil, "us-east-1", "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", now)
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestOffsiteCopies(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	t.Cleanup(func() { st.Close() })
	m := &Manager{Store: st, DataDir: dir}
	st.SetSetting(KeyKeep, "2")
	var mu sync.Mutex
	objects := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=key/") {
			w.WriteHeader(403)
			return
		}
		switch r.Method {
		case "PUT":
			b, _ := io.ReadAll(r.Body)
			objects[strings.TrimPrefix(r.URL.Path, "/bucket/")] = len(b)
		case "DELETE":
			delete(objects, strings.TrimPrefix(r.URL.Path, "/bucket/"))
		case "GET":
			fmt.Fprint(w, "<ListBucketResult>")
			for k := range objects {
				fmt.Fprintf(w, "<Contents><Key>%s</Key></Contents>", k)
			}
			fmt.Fprint(w, "</ListBucketResult>")
		}
	}))
	defer srv.Close()
	folder := filepath.Join(t.TempDir(), "offsite")
	if err := m.SaveOffsite(Offsite{Folder: dir}); err == nil {
		t.Fatal("the data folder was accepted as the offsite folder")
	}
	if err := m.SaveOffsite(Offsite{Folder: folder, S3: S3{Endpoint: srv.URL, Bucket: "bucket", Prefix: "/flixarr/", AccessKey: "key", SecretKey: "s"}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		b, err := m.Create("manual")
		if err != nil {
			t.Fatal(err)
		}
		if err := m.CopyOffsite(context.Background(), b.Name); err != nil {
			t.Fatalf("copy: %v", err)
		}
		time.Sleep(1100 * time.Millisecond) // backup names have one-second stamps
	}
	files, _ := os.ReadDir(folder)
	if len(files) != 2 {
		t.Fatalf("folder keeps 2, has %d", len(files))
	}
	if len(objects) != 2 {
		t.Fatalf("bucket keeps 2, has %v", objects)
	}
	for k := range objects {
		if !strings.HasPrefix(k, "flixarr/flixarr-") {
			t.Fatalf("key without the prefix: %s", k)
		}
	}
	if st := m.LastOffsite(); st.At == "" || st.Error != "" {
		t.Fatalf("status: %+v", st)
	}
}
