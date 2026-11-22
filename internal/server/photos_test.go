package server

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/ctlplne/healthctl/internal/clinical"
)

func TestObservationPhotoLifecycle(t *testing.T) {
	db, _, _ := testStore(t)
	c := caller{h: newTestServer(db, io.Discard).Handler()}
	c.signup(t)
	cfg := clinical.ProviderRef{Kind: "demo", Model: "synthetic", Structured: "auto"}
	selectProvider(t, c, cfg)
	var e clinical.Encounter
	check(t, c.call("POST", "/api/encounters", intake(cfg, clinical.DemoNailConcern)), 200, &e)
	revision := e.Revision
	path := "/api/encounters/" + e.ID
	var fixture bytes.Buffer
	if err := png.Encode(&fixture, image.NewRGBA(image.Rect(0, 0, 48, 36))); err != nil {
		t.Fatal(err)
	}
	upload := func(key string, data []byte) *httptest.ResponseRecorder {
		var body bytes.Buffer
		m := multipart.NewWriter(&body)
		for k, v := range map[string]string{"revision": strconv.Itoa(revision), "requestId": key, "photoArea": "Eye", "belongsTo": "self"} {
			if err := m.WriteField(k, v); err != nil {
				t.Fatal(err)
			}
		}
		f, err := m.CreateFormFile("file", "example-private-name.png")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write(data); err != nil {
			t.Fatal(err)
		}
		if err = m.Close(); err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("POST", "http://localhost"+path+"/documents", &body)
		r.Header.Set("Content-Type", m.FormDataContentType())
		r.Header.Set("X-CSRF-Token", c.csrf)
		r.AddCookie(c.cookie)
		w := httptest.NewRecorder()
		c.h.ServeHTTP(w, r)
		return w
	}
	check(t, upload(token(12), []byte("Not a photo")), 400, nil)
	key := token(12)
	payload := append(fixture.Bytes(), []byte("fictional metadata sentinel")...)
	check(t, upload(key, payload), 200, &e)
	check(t, upload(key, payload), 200, &e)
	if len(e.Photos) != 1 || len(e.Documents) != 0 {
		t.Fatal("Duplicate photo or misclassified as printed report")
	}
	id := e.Photos[0].ID
	url := path + "/photos/" + id
	guest := caller{h: c.h}
	check(t, guest.call("GET", url, nil), 401, nil)
	saved := c.call("GET", url, nil)
	check(t, saved, 200, nil)
	if saved.Header().Get("Content-Type") != "image/png" || bytes.Contains(saved.Body.Bytes(), []byte("metadata sentinel")) {
		t.Fatal("Unexpected image encoding or metadata leak")
	}
	check(t, c.call("GET", path+"/documents/"+id+"/original", nil), 404, nil)
	remove := object{"requestId": token(12), "revision": e.Revision}
	var removed, replay clinical.Encounter
	check(t, c.call("DELETE", url, remove), 200, &removed)
	check(t, c.call("DELETE", url, remove), 200, &replay)
	if len(removed.Photos) != 0 || len(replay.Photos) != 0 || removed.Revision != replay.Revision {
		t.Fatal("Attachment still listed")
	}
	check(t, c.call("GET", url, nil), 404, nil)
	if _, err := db.Original(context.Background(), 1, e.ID, id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("Removed image still retrievable", err)
	}
}
