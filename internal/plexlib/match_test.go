package plexlib

import "testing"

func TestFileKeyIgnoresMountPoint(t *testing.T) {
	a := fileKey("/mnt/media/movies/Split (2017)/Split (2017) - [Bluray-1080p].mkv")
	b := fileKey("/data/movies/Split (2017)/split (2017) - [bluray-1080p].mkv")
	if a != b {
		t.Fatalf("same file under different mounts: %q vs %q", a, b)
	}
	if fileKey("/m/A/x.mkv") == fileKey("/m/B/x.mkv") {
		t.Fatal("different folders must not match")
	}
}
