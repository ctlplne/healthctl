package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTranscriptionUsesSeparatePlatformAccount(t *testing.T) {
	c := New()
	c.secrets["openai"] = ""
	c.chatGPTToken = func(context.Context, string) (string, error) { t.Fatal("used a plan grant for audio"); return "", nil }
	calls := 0
	c.httpClients["official"] = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://api.openai.com/v1/audio/transcriptions" || r.Header.Get("Authorization") != "Bearer example-platform" {
			t.Fatal("wrong audio destination")
		}
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Fatal(err)
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(f)
		f.Close()
		if string(b) != "fictional" || r.FormValue("model") != "gpt-4o-transcribe" || r.FormValue("language") != "en" {
			t.Fatal("incorrect multipart body")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"text":"Synthetic reviewed speech"}`))}, nil
	})}
	if _, err := c.Transcribe(context.Background(), nil); err == nil || calls != 0 {
		t.Fatal("submitted audio without platform credentials")
	}
	c.secrets["openai"] = "example-platform"
	got, err := c.Transcribe(context.Background(), []byte("fictional"))
	if err != nil || got != "Synthetic reviewed speech" || calls != 1 {
		t.Fatal("transcription failed", err)
	}
}
