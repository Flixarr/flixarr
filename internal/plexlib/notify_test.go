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
