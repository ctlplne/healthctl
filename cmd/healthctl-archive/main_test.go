package main

import (
	"bytes"
	"testing"
)

func TestArchive(t *testing.T) {
	pass := []byte("synthetic-archive-check")
	original := bytes.Repeat([]byte{45}, chunkSize+20)
	var enc, plain bytes.Buffer
	if err := cryptStream(bytes.NewReader(original), &enc, pass, true); err != nil {
		t.Fatal(err)
	}
	if err := cryptStream(bytes.NewReader(enc.Bytes()), &plain, pass, false); err != nil || !bytes.Equal(original, plain.Bytes()) {
		t.Fatal("roundtrip", err)
	}
	for _, bad := range [][]byte{enc.Bytes()[:enc.Len()-1], append(bytes.Clone(enc.Bytes()), 0)} {
		if cryptStream(bytes.NewReader(bad), &plain, pass, false) == nil {
			t.Fatal("accepted damaged backup")
		}
	}
	if cryptStream(bytes.NewReader(enc.Bytes()), &plain, []byte("different-password"), false) == nil {
		t.Fatal("accepted different passphrase")
	}
}
