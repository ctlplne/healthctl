package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDeepLinkServesApp(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>app</title>"), 0o644)
	os.WriteFile(filepath.Join(dir, "app.js"), []byte("console.log(1)"), 0o644)
	h := (&Server{Assets: dir, Name: "healthctl"}).Handler()
	get := func(path string) (int, string) {
		req := httptest.NewRequest("GET", "http://localhost"+path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		b, _ := io.ReadAll(w.Result().Body)
		return w.Code, string(b)
	}
	for _, path := range []string{"/", "/settings", "/timeline", "/encounters/0123456789abcdef", "/encounters/0123456789abcdef/history"} {
		if code, body := get(path); code != http.StatusOK || body != "<!doctype html><title>app</title>" {
			t.Fatalf("%s: a page route must serve the app, got %d", path, code)
		}
	}
	for _, path := range []string{"/missing.js", "/encounters/x/y/z/w", "/random-page"} {
		if code, _ := get(path); code != http.StatusNotFound {
			t.Fatalf("%s: only the app's own routes fall back, got %d", path, code)
		}
	}
	if code, body := get("/app.js"); code != http.StatusOK || body != "console.log(1)" {
		t.Fatal("real files are still served")
	}
}
