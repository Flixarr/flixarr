package plexlib

import (
	"bufio"
	"bytes"
	"testing"
)

func TestLibraryAddedAndFrames(t *testing.T) {
	add := []byte(`{"NotificationContainer":{"type":"timeline","size":1,"TimelineEntry":[{"identifier":"com.plexapp.plugins.library","sectionID":"1","itemID":75090,"type":1,"title":"This Is 40","state":5}]}}`)
	if !libraryAdded(add) {
		t.Fatal("finished movie not seen")
	}
	if libraryAdded([]byte(`{"NotificationContainer":{"type":"timeline","TimelineEntry":[{"type":1,"state":0}]}}`)) {
		t.Fatal("still processing counted")
	}
	if !libraryAdded([]byte(`{"NotificationContainer":{"type":"activity","ActivityNotification":[{"event":"ended","Activity":{"type":"library.update.section"}}]}}`)) {
		t.Fatal("finished scan not seen")
	}
	if libraryAdded([]byte(`{"NotificationContainer":{"type":"activity","ActivityNotification":[{"event":"ended","Activity":{"type":"provider.subscriptions.process"}}]}}`)) {
		t.Fatal("other activity counted")
	}
	if libraryAdded([]byte(`{"NotificationContainer":{"type":"playing"}}`)) {
		t.Fatal("playback counted")
	}
	// A 200-byte text frame (16-bit length), unmasked as servers send it.
	p := bytes.Repeat([]byte("x"), 200)
	frame := append([]byte{0x81, 126, 0, 200}, p...)
	op, got, err := readFrame(bufio.NewReader(bytes.NewReader(frame)), nil)
	if err != nil || op != 1 || len(got) != 200 {
		t.Fatalf("frame: %d %d %v", op, len(got), err)
	}
}

func TestLibraryRemoved(t *testing.T) {
	ids, eps := libraryRemoved([]byte(`{"NotificationContainer":{"type":"timeline","size":3,"TimelineEntry":[{"type":2,"state":9,"itemID":75103},{"type":4,"state":9,"itemID":"75110"},{"type":1,"state":5,"itemID":"1"}]}}`))
	if len(ids) != 1 || ids[0] != "75103" || !eps {
		t.Fatalf("got %v %v", ids, eps)
	}
	if ids, eps := libraryRemoved([]byte(`{"NotificationContainer":{"type":"timeline","TimelineEntry":[{"type":1,"state":5,"itemID":1}]}}`)); len(ids) != 0 || eps {
		t.Fatalf("an add read as a delete: %v %v", ids, eps)
	}
	if !scanEnded([]byte(`{"NotificationContainer":{"type":"activity","ActivityNotification":[{"event":"ended","Activity":{"type":"library.update.section"}}]}}`)) {
		t.Fatal("scan end missed")
	}
}

// The exact message Plex sent when a movie was deleted (captured 2026-10-13): itemID is a string.
func TestLibraryRemovedRealMessage(t *testing.T) {
	msg := `{"NotificationContainer":{"type":"timeline","size":1,"TimelineEntry":[{"identifier":"com.plexapp.plugins.library","sectionID":"1","itemID":"75119","type":1,"title":"","state":9,"metadataState":"deleted","updatedAt":1791865425}]}}`
	if ids, _ := libraryRemoved([]byte(msg)); len(ids) != 1 || ids[0] != "75119" {
		t.Fatalf("got %v", ids)
	}
}
