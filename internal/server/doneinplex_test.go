package server

import "testing"

func TestDoneInPlex(t *testing.T) {
	for status, want := range map[string]bool{
		"available": true, "failed": true, "declined": true, "expired": true, "removed": true,
		"pending": false, "approved": false, "sent": false, "grabbed": false, "downloading": false, "importing": false, "partial": false,
	} {
		if got := doneInPlex(status); got != want {
			t.Errorf("%s: got %v, want %v", status, got, want)
		}
	}
}
